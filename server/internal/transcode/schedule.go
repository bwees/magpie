package transcode

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"

	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"gorm.io/gorm"
)

// Schedule is one scheduler pass: expired offers and leases go back to the queue, prefetched jobs whose
// device freed up start, and queued jobs are offered to free slots.
func (s *JobService) Schedule(ctx context.Context) error {
	if err := s.expireOffers(ctx); err != nil {
		return fmt.Errorf("expiring offers: %w", err)
	}
	if err := s.expireLeases(ctx); err != nil {
		return fmt.Errorf("expiring leases: %w", err)
	}
	fleet, prefetched, err := s.fleet(ctx)
	if err != nil {
		return err
	}
	s.startPrefetched(ctx, &fleet, prefetched)

	policy, err := policyKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	queued, rows, err := s.queued(ctx, policy.Enabled)
	if err != nil {
		return err
	}
	agents := map[domain.AgentID]api.Agent{}
	for _, agent := range fleet.Agents {
		agents[agent.ID] = agent
	}
	for _, offer := range Plan(queued, fleet) {
		if err := s.offer(ctx, rows[offer.Job], offer, agents[offer.Slot.Agent], policy); err != nil {
			slog.ErrorContext(ctx, "transcode: offering a job", "job", offer.Job, "agent", offer.Slot.Agent, "error", err)
		}
	}
	return nil
}

func (s *JobService) expireOffers(ctx context.Context) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		var rows []jobRow
		cutoff := s.now().Add(-s.deps.Timings.OfferTimeout)
		if err := tx.Where("state = ? AND offered_at < ?", api.StateOffered, cutoff).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := s.requeueOffer(tx, row, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *JobService) expireLeases(ctx context.Context) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		var rows []jobRow
		if err := tx.Where("state IN ? AND lease_expires_at < ?", agentHeld, s.now()).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			slog.WarnContext(ctx, "transcode: lease expired", "job", row.ID, "agent", agentOf(row), "state", row.State)
			if err := s.failAttempt(tx, row, leaseExpired); err != nil {
				return err
			}
		}
		return nil
	})
}

// fleet reads the agents and what their devices run. prefetched are the jobs agents hold for later.
func (s *JobService) fleet(ctx context.Context) (Fleet, []jobRow, error) {
	agents, err := s.deps.Jobs.Agents(ctx)
	if err != nil {
		return Fleet{}, nil, err
	}
	current, err := agentSettingsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return Fleet{}, nil, err
	}
	var held []jobRow
	if err := s.deps.DB.Read(ctx).Where("state IN ? AND agent_id IS NOT NULL", slotStates).Find(&held).Error; err != nil {
		return Fleet{}, nil, err
	}
	fleet := Fleet{
		Agents: agents, Running: map[SlotKey]int{}, Prefetching: map[domain.AgentID]bool{}, Busy: s.deps.Hub.Busy(),
		DefaultSlots: current.DefaultSlotsPerGPU,
	}
	var prefetched []jobRow
	for _, row := range held {
		if row.Prefetch {
			fleet.Prefetching[*row.AgentID] = true
			prefetched = append(prefetched, row)
			continue
		}
		fleet.Running[slotOf(row)]++
	}
	return fleet, prefetched, nil
}

func slotOf(row jobRow) SlotKey {
	key := SlotKey{Agent: *row.AgentID}
	if row.DeviceID != nil {
		key.Device = *row.DeviceID
	}
	return key
}

// startPrefetched tells an agent to encode the job it prefetched once that job's device has a free slot.
func (s *JobService) startPrefetched(ctx context.Context, fleet *Fleet, prefetched []jobRow) {
	for _, row := range prefetched {
		if !slices.Contains(agentHeld, row.State) {
			continue
		}
		key := slotOf(row)
		if fleet.freeSlots()[key] == 0 {
			continue
		}
		err := s.deps.Hub.Send(ctx, key.Agent, agentproto.TypeStart, agentproto.Start{Job: row.ID})
		if err != nil {
			slog.WarnContext(ctx, "transcode: starting a prefetched job", "job", row.ID, "agent", key.Agent, "error", err)
			continue
		}
		err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
			_, err := moveJob(tx, row.ID, agentHeld, map[string]any{"prefetch": false, "updated_at": s.now()})
			return err
		})
		if err != nil {
			slog.ErrorContext(ctx, "transcode: recording a started prefetch", "job", row.ID, "error", err)
			continue
		}
		fleet.Running[key]++
		delete(fleet.Prefetching, key.Agent)
		s.deps.Gate.ReleaseJob(row.ID)
	}
}

// queued lists the jobs that may be offered now, in offer order. Transcodes wait while transcoding is
// turned off.
func (s *JobService) queued(ctx context.Context, transcoding bool) ([]QueuedJob, map[domain.JobID]jobRow, error) {
	now := s.now()
	var rows []jobRow
	q := s.deps.DB.Read(ctx).Where("state = ? AND (not_before IS NULL OR not_before <= ?)", api.StateQueued, now)
	if !transcoding {
		q = q.Where("kind <> ?", api.JobKindTranscode)
	}
	err := q.Order(offerOrder).Find(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	resumable := s.deps.Hub.Resumable()
	queued := make([]QueuedJob, 0, len(rows))
	byID := make(map[domain.JobID]jobRow, len(rows))
	for _, row := range rows {
		excluded, err := row.excluded()
		if err != nil {
			return nil, nil, err
		}
		job := QueuedJob{
			ID:             row.ID,
			Needs:          row.needs(),
			ExcludedAgents: excluded,
			SourceSize:     row.SourceSize,
			LastAgent:      row.LastAgentID,
			Backlog:        row.backlog(),
		}
		if row.AvoidAgentID != nil && row.AvoidUntil != nil && row.AvoidUntil.After(now) {
			job.Avoid = row.AvoidAgentID
		}
		if held, ok := resumable[row.ID]; ok && held.etag == row.TransferETag {
			job.ResumableOn = &held.agent
		}
		queued = append(queued, job)
		byID[row.ID] = row
	}
	return queued, byID, nil
}

// offer builds the job's spec for the chosen device, records the offer and sends it.
func (s *JobService) offer(ctx context.Context, row jobRow, offer Offer, agent api.Agent, policy Policy) error {
	source, err := s.deps.Library.SourceForProcessing(ctx, row.MediaFileID)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return s.failQueued(ctx, row, final(ReasonSourceGone))
	}
	if err != nil {
		return err
	}
	info, err := s.deps.Files.Stat(source.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return s.failQueued(ctx, row, final(ReasonSourceMissing))
	}
	if err != nil {
		return err
	}
	device, ok := deviceOf(agent, offer.Slot.Device)
	if !ok {
		return errUnknownDevice
	}
	spec := agentproto.JobSpec{
		Job:      row.ID,
		DeviceID: device.ID,
		Source: agentproto.SourceRef{
			URL:       agentproto.SourcePath(row.ID),
			Size:      info.Size,
			ETag:      info.ETag(),
			Container: source.MediaInfo.Container,
		},
		OutputURL:  agentproto.OutputPath(row.ID),
		OutputName: stagedName(row.ID, row.Kind),
	}
	if row.Kind == api.JobKindWhisper {
		params, ok := whisperParams(row, source.MediaInfo)
		if !ok {
			return s.failQueued(ctx, row, final(ReasonNoAudio))
		}
		spec.Kind, spec.Whisper = agentproto.KindWhisper, &params
	} else {
		keep, err := languagesToKeep(ctx, s.deps.Catalog, row.TitleID)
		if err != nil {
			return err
		}
		params, err := transcodeParams(policy, row.plan(), agentproto.Vendor(device.Vendor), source.MediaInfo, keep)
		if err != nil {
			return err
		}
		spec.Kind, spec.Transcode = agentproto.KindTranscode, &params
	}
	encoded, err := encodeJSON(spec)
	if err != nil {
		return err
	}
	now := s.now()
	lease := now.Add(s.deps.Timings.LeaseTTL)
	var offered bool
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		offered, err = moveJob(tx, row.ID, []api.JobState{api.StateQueued}, map[string]any{
			"state": api.StateOffered, "agent_id": offer.Slot.Agent, "device_id": device.ID, "offered_at": now,
			"lease_expires_at": lease, "prefetch": offer.Prefetch, "spec": encoded, "source_etag": source.ETag,
			"transfer_etag": spec.Source.ETag, "source_size": info.Size, "staging_path": stagedPath(source.StagingDir, row.ID, row.Kind),
			"updated_at": now,
		})
		return err
	})
	if err != nil || !offered {
		return err
	}
	jobOffer := agentproto.JobOffer{Spec: spec, LeaseExpires: lease, Prefetch: offer.Prefetch}
	if err := s.deps.Hub.Send(ctx, offer.Slot.Agent, agentproto.TypeJobOffer, jobOffer); err != nil {
		return errors.Join(err, s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
			row.State = api.StateOffered
			return s.requeueOffer(tx, row, nil)
		}))
	}
	return nil
}

// failQueued ends a queued job that can't run, such as one whose file is gone.
func (s *JobService) failQueued(ctx context.Context, row jobRow, f failure) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row.AgentID = nil
		return s.failAttempt(tx, row, f)
	})
}

func deviceOf(agent api.Agent, id string) (api.Device, bool) {
	i := slices.IndexFunc(agent.Devices, func(d api.Device) bool { return d.ID == id })
	if i < 0 {
		return api.Device{}, false
	}
	return agent.Devices[i], true
}
