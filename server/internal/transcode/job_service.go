package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/settings"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var (
	ErrJobFinished   = errors.New("transcode: the job has already finished")
	ErrJobNotFailed  = errors.New("transcode: only a failed or not-worth-it job can be retried")
	ErrNotLeased     = errors.New("transcode: the job isn't leased to this agent")
	errUnknownDevice = errors.New("transcode: the agent has no such device")
)

type JobDeps struct {
	fx.In

	DB       *db.DB
	Bus      *bus.Bus
	Jobs     *JobRepository
	Agents   *AgentRepository
	Hub      *AgentHub
	Store    *settings.Store
	Library  libraryapi.Reader
	Catalog  catalogapi.Reader
	Files    FileStat
	Prober   MediaProber
	Queue    TaskQueue
	Gate     *TransferGate
	Lifetime *Lifetime
	Wake     *Wake
	Timings  Timings
}

// JobService runs jobs through their states: queueing on cutoff, offering to agents, leases,
// results, server-side verification, the swap outcome, cancellation and retries.
type JobService struct {
	deps JobDeps
	now  func() time.Time
}

func NewJobService(deps JobDeps) *JobService {
	return &JobService{deps: deps, now: func() time.Time { return time.Now().UTC() }}
}

// clearAssignment frees a job from its agent and device.
func clearAssignment(changes map[string]any) map[string]any {
	for column, value := range map[string]any{
		"agent_id": nil, "device_id": nil, "offered_at": nil, "lease_expires_at": nil, "prefetch": false,
	} {
		changes[column] = value
	}
	return changes
}

// afterCommit runs fn in the background once tx commits, so a handler never waits on an agent or the disk.
func (s *JobService) afterCommit(tx *gorm.DB, fn func(ctx context.Context)) error {
	return db.AfterCommit(tx, func() { s.deps.Lifetime.Go(fn) })
}

func (s *JobService) kickAfterCommit(tx *gorm.DB) error {
	return db.AfterCommit(tx, s.deps.Wake.Kick)
}

const emptyAgents = "[]"

// QueueWhisper queues a Whisper job for the file and language unless one is already open. It runs
// ahead of every transcode.
func (s *JobService) QueueWhisper(ctx context.Context, tx *gorm.DB, run api.RunAgentJob) error {
	if run.Kind != api.JobKindWhisper {
		slog.WarnContext(ctx, "transcode: ignoring an agent job of an unknown kind", "kind", run.Kind, "file", run.MediaFile)
		return nil
	}
	if open, err := openWhisperJob(tx, run.MediaFile, run.Language); err != nil || open {
		return err
	}
	file, err := s.deps.Library.File(ctx, run.MediaFile)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	now := s.now()
	row := jobRow{
		ID:             domain.NewID[domain.JobID](),
		Kind:           api.JobKindWhisper,
		MediaFileID:    run.MediaFile,
		TitleID:        file.Title,
		Origin:         file.Origin,
		State:          api.StateQueued,
		Priority:       priorityWhisper,
		Attempt:        1,
		Language:       run.Language,
		WhisperModel:   run.Model,
		ExcludedAgents: emptyAgents,
		SourceSize:     file.Size,
		QueuedAt:       now,
		UpdatedAt:      now,
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	queued := api.JobQueued{Job: row.ID, Kind: row.Kind, MediaFile: row.MediaFileID, Title: row.TitleID, Origin: row.Origin}
	if err := s.deps.Bus.Publish(tx, bus.EventOf(queued)); err != nil {
		return err
	}
	return s.kickAfterCommit(tx)
}

// Accept leases an offered job to the agent that accepted it. An agent accepting a job that isn't
// offered to it any more is told to drop it.
func (s *JobService) Accept(ctx context.Context, agent domain.AgentID, accept agentproto.JobAccept) error {
	var leased bool
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := jobIn(tx, accept.Job)
		if errors.Is(err, ErrJobNotFound) || (err == nil && !offeredTo(row, agent)) {
			return nil
		}
		if err != nil {
			return err
		}
		now := s.now()
		leased, err = moveJob(tx, row.ID, []api.JobState{api.StateOffered}, map[string]any{
			"state": api.StateLeased, "lease_expires_at": now.Add(s.deps.Timings.LeaseTTL), "offered_at": nil,
			"progress": 0, "updated_at": now,
		})
		if err != nil || !leased {
			return err
		}
		attempt := attemptRow{JobID: row.ID, Attempt: row.Attempt, AgentID: agent, StartedAt: now}
		if err := tx.Save(&attempt).Error; err != nil {
			return err
		}
		started := api.JobStarted{Job: row.ID, Kind: row.Kind, MediaFile: row.MediaFileID, Agent: agent, Attempt: row.Attempt}
		return s.deps.Bus.Publish(tx, bus.EventOf(started))
	})
	if err != nil || leased {
		return err
	}
	s.sendCancel(ctx, agent, accept.Job, "the offer is no longer open")
	return nil
}

func offeredTo(row jobRow, agent domain.AgentID) bool {
	return row.State == api.StateOffered && row.AgentID != nil && *row.AgentID == agent
}

func heldBy(row jobRow, agent domain.AgentID) bool {
	return slices.Contains(agentHeld, row.State) && row.AgentID != nil && *row.AgentID == agent
}

// Reject returns an offered job to the queue without counting an attempt. A busy device takes no offers
// until its agent's next heartbeat; for any other reason the agent isn't offered the job for a while.
func (s *JobService) Reject(ctx context.Context, agent domain.AgentID, reject agentproto.JobReject) error {
	slog.InfoContext(ctx, "transcode: agent rejected a job", "agent", agent, "job", reject.Job, "reason", reject.Reason)
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := jobIn(tx, reject.Job)
		if errors.Is(err, ErrJobNotFound) || (err == nil && !offeredTo(row, agent)) {
			return nil
		}
		if err != nil {
			return err
		}
		if reject.Reason == agentproto.RejectDeviceBusy {
			s.deps.Hub.markBusy(slotOf(row))
			return s.requeueOffer(tx, row, nil)
		}
		return s.requeueOffer(tx, row, &agent)
	})
}

// requeueOffer puts an offered job back in the queue. avoid, when set, isn't offered the job for a while.
func (s *JobService) requeueOffer(tx *gorm.DB, row jobRow, avoid *domain.AgentID) error {
	now := s.now()
	changes := clearAssignment(map[string]any{"state": api.StateQueued, "updated_at": now})
	if avoid != nil {
		changes["avoid_agent_id"] = *avoid
		changes["avoid_until"] = now.Add(s.deps.Timings.AvoidAgent)
	}
	if _, err := moveJob(tx, row.ID, []api.JobState{api.StateOffered}, changes); err != nil {
		return err
	}
	return s.kickAfterCommit(tx)
}

// Heartbeat extends the lease of every job the agent reports and records its stage and progress. A job
// the agent holds without a lease is cancelled on the agent. Its busy devices take offers again.
func (s *JobService) Heartbeat(ctx context.Context, agent domain.AgentID, heartbeat agentproto.Heartbeat) error {
	var progressed []api.JobProgressed
	var strays []domain.JobID
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		progressed, strays = nil, nil
		now := s.now()
		for _, held := range heartbeat.Jobs {
			row, err := jobIn(tx, held.Job)
			if errors.Is(err, ErrJobNotFound) {
				strays = append(strays, held.Job)
				continue
			}
			if err != nil {
				return err
			}
			state, known := stageStates[held.Stage]
			if !known || !heldBy(row, agent) {
				if !finishing(row, agent) {
					strays = append(strays, held.Job)
				}
				continue
			}
			_, err = moveJob(tx, row.ID, agentHeld, map[string]any{
				"state": state, "progress": held.Progress, "lease_expires_at": now.Add(s.deps.Timings.LeaseTTL), "updated_at": now,
			})
			if err != nil {
				return err
			}
			progressed = append(progressed, api.JobProgressed{
				Job: row.ID, MediaFile: row.MediaFileID, Stage: state, Progress: held.Progress, Speed: held.Speed, ETA: held.ETA,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.deps.Hub.clearBusy(agent)
	for _, p := range progressed {
		if p.Stage == api.StateUploading {
			s.deps.Gate.Release(TransferHolder{Job: p.Job, Direction: DirectionSource})
		}
		s.deps.Bus.Emit(bus.EventOf(p))
	}
	for _, job := range strays {
		s.sendCancel(ctx, agent, job, "the job isn't leased to this agent")
	}
	return nil
}

// finishing is a job the agent handed back that the server is still checking; its last heartbeat may
// cross the result on the wire.
func finishing(row jobRow, agent domain.AgentID) bool {
	return (row.State == api.StateVerifying || row.State == api.StateVerified) && row.AgentID != nil && *row.AgentID == agent
}

// Result ends the agent's part of a job. An uploaded output is queued for the server's checks; a
// failed attempt is retried or ends the job.
func (s *JobService) Result(ctx context.Context, agent domain.AgentID, result agentproto.JobResult) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := jobIn(tx, result.Job)
		if errors.Is(err, ErrJobNotFound) || (err == nil && !heldBy(row, agent)) {
			slog.WarnContext(ctx, "transcode: ignoring a result for a job the agent doesn't hold", "agent", agent, "job", result.Job)
			return nil
		}
		if err != nil {
			return err
		}
		switch result.Outcome {
		case agentproto.OutcomeUploaded:
			return s.uploaded(tx, row, result)
		case agentproto.OutcomeNotWorthIt:
			return s.notWorthIt(tx, row, result)
		default:
			return s.failAttempt(tx, row, agentFailure(result))
		}
	})
}

func (s *JobService) uploaded(tx *gorm.DB, row jobRow, result agentproto.JobResult) error {
	if row.Kind == api.JobKindTranscode {
		if f, ok := judge(result.Verify); !ok {
			return s.failAttempt(tx, row, f)
		}
	}
	if row.OutputSHA256 == "" {
		return s.failAttempt(tx, row, retryable(ReasonUploadMissing))
	}
	encoded, err := encodeJSON(result)
	if err != nil {
		return err
	}
	now := s.now()
	moved, err := moveJob(tx, row.ID, agentHeld, map[string]any{
		"state": api.StateVerifying, "result": encoded, "progress": 0, "lease_expires_at": nil, "prefetch": false, "updated_at": now,
	})
	if err != nil || !moved {
		return err
	}
	if err := endAttempt(tx, row, outcomeUploaded, "", now); err != nil {
		return err
	}
	if err := s.deps.Queue.EnqueueTx(tx, verifyTask{Job: row.ID}); err != nil {
		return err
	}
	return s.releaseAfterCommit(tx, row.ID)
}

func (s *JobService) notWorthIt(tx *gorm.DB, row jobRow, result agentproto.JobResult) error {
	encoded, err := encodeJSON(result)
	if err != nil {
		return err
	}
	now := s.now()
	moved, err := moveJob(tx, row.ID, agentHeld, map[string]any{
		"state": api.StateNotWorthIt, "result": encoded, "finished_at": now, "updated_at": now, "lease_expires_at": nil,
		"prefetch": false, "last_agent_id": row.AgentID,
	})
	if err != nil || !moved {
		return err
	}
	if err := endAttempt(tx, row, outcomeNotWorthIt, result.Error, now); err != nil {
		return err
	}
	policy, err := policyKey.GetTx(tx)
	if err != nil {
		return err
	}
	detail := result.Error
	if detail == "" {
		detail = "The output wasn't enough smaller than the source"
	}
	if err := s.skip(tx, row.MediaFileID, api.SkipNotWorthIt, detail, policy); err != nil {
		return err
	}
	return s.cleanUpAfterCommit(tx, row, nil)
}

// failAttempt ends the current attempt. A retryable failure queues the next attempt after a backoff,
// or away from the agent after a lost lease; the last attempt or a final failure fails the job.
func (s *JobService) failAttempt(tx *gorm.DB, row jobRow, f failure) error {
	now := s.now()
	if err := endAttempt(tx, row, f.outcome, f.reason, now); err != nil {
		return err
	}
	last := row.Attempt >= MaxAttempts || !f.retryable
	changes := clearAssignment(map[string]any{
		"error": f.reason, "progress": 0, "updated_at": now, "result": nil, "output_sha256": "", "output_size": 0,
	})
	if row.AgentID != nil {
		changes["last_agent_id"] = *row.AgentID
	}
	switch {
	case last:
		changes["state"] = api.StateFailed
		changes["finished_at"] = now
	case f.leaseLost:
		changes["state"] = api.StateQueued
		changes["attempt"] = row.Attempt + 1
		changes["avoid_agent_id"] = row.AgentID
		changes["avoid_until"] = now.Add(s.deps.Timings.AvoidAgent)
	default:
		changes["state"] = api.StateQueued
		changes["attempt"] = row.Attempt + 1
		changes["not_before"] = now.Add(backoffAfter(row.Attempt))
	}
	if row.AgentID != nil && !last {
		excluded, err := s.excludeRepeatOffender(tx, row)
		if err != nil {
			return err
		}
		changes["excluded_agents"] = excluded
	}
	moved, err := moveJob(tx, row.ID, []api.JobState{row.State}, changes)
	if err != nil || !moved {
		return err
	}
	failed := api.JobFailed{Job: row.ID, Kind: row.Kind, MediaFile: row.MediaFileID, Reason: f.reason, Final: last, Language: row.Language}
	if err := s.deps.Bus.Publish(tx, bus.EventOf(failed)); err != nil {
		return err
	}
	return s.cleanUpAfterCommit(tx, row, nil)
}

// excludeRepeatOffender adds the job's agent to its exclusions once it has failed the job twice.
func (s *JobService) excludeRepeatOffender(tx *gorm.DB, row jobRow) (string, error) {
	excluded, err := row.excluded()
	if err != nil {
		return "", err
	}
	failures, err := failuresOn(tx, row.ID, *row.AgentID)
	if err != nil {
		return "", err
	}
	if failures >= failuresBeforeExclusion && !slices.Contains(excluded, *row.AgentID) {
		excluded = append(excluded, *row.AgentID)
	}
	encoded, err := json.Marshal(excluded)
	return string(encoded), err
}

// cleanUpAfterCommit frees what a job that left an agent held: its transfer slots, its staged output and,
// when cancel is set, the agent's copy of the job.
func (s *JobService) cleanUpAfterCommit(tx *gorm.DB, row jobRow, cancel *agentproto.Cancel) error {
	if err := s.kickAfterCommit(tx); err != nil {
		return err
	}
	return s.afterCommit(tx, func(ctx context.Context) {
		s.deps.Gate.ReleaseJob(row.ID)
		if cancel != nil && row.AgentID != nil {
			s.sendCancel(ctx, *row.AgentID, cancel.Job, cancel.Reason)
		}
		removeStaged(ctx, row.StagingPath)
	})
}

func (s *JobService) releaseAfterCommit(tx *gorm.DB, job domain.JobID) error {
	if err := s.kickAfterCommit(tx); err != nil {
		return err
	}
	return db.AfterCommit(tx, func() { s.deps.Gate.ReleaseJob(job) })
}

func removeStaged(ctx context.Context, path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "transcode: removing a staged output", "path", path, "error", err)
	}
}

func (s *JobService) sendCancel(ctx context.Context, agent domain.AgentID, job domain.JobID, reason string) {
	err := s.deps.Hub.Send(ctx, agent, agentproto.TypeCancel, agentproto.Cancel{Job: job, Reason: reason})
	if err != nil && !errors.Is(err, ErrAgentNotConnected) {
		slog.WarnContext(ctx, "transcode: sending cancel", "agent", agent, "job", job, "error", err)
	}
}

// Cancel stops an unfinished job: the agent drops it and its staged output is removed. Subtitles waits
// for a reply to every Whisper job it asked for, so a cancelled one ends as a final JobFailed.
func (s *JobService) Cancel(ctx context.Context, id domain.JobID, reason string) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := jobIn(tx, id)
		if err != nil {
			return err
		}
		if !slices.Contains(openStates, row.State) {
			return ErrJobFinished
		}
		if err := s.cancelTx(tx, row, reason); err != nil || row.Kind != api.JobKindWhisper {
			return err
		}
		failed := api.JobFailed{Job: row.ID, Kind: row.Kind, MediaFile: row.MediaFileID, Reason: reason, Final: true, Language: row.Language}
		return s.deps.Bus.Publish(tx, bus.EventOf(failed))
	})
}

func (s *JobService) cancelTx(tx *gorm.DB, row jobRow, reason string) error {
	now := s.now()
	moved, err := moveJob(tx, row.ID, openStates, clearAssignment(map[string]any{
		"state": api.StateCancelled, "error": reason, "finished_at": now, "updated_at": now,
	}))
	if err != nil || !moved {
		return err
	}
	if slices.Contains(agentHeld, row.State) {
		if err := endAttempt(tx, row, outcomeCancelled, reason, now); err != nil {
			return err
		}
	}
	if row.Kind == api.JobKindTranscode {
		cancelled := api.JobCancelled{Job: row.ID, MediaFile: row.MediaFileID, Reason: reason}
		if err := s.deps.Bus.Publish(tx, bus.EventOf(cancelled)); err != nil {
			return err
		}
	}
	var cancel *agentproto.Cancel
	if slices.Contains(slotStates, row.State) {
		cancel = &agentproto.Cancel{Job: row.ID, Reason: reason}
	}
	return s.cleanUpAfterCommit(tx, row, cancel)
}

// CancelForFile cancels every unfinished job of a file that left the library.
func (s *JobService) CancelForFile(tx *gorm.DB, file domain.MediaFileID, reason string) error {
	if err := tx.Where("media_file_id = ?", file).Delete(&eligibleFileRow{}).Error; err != nil {
		return err
	}
	if err := deleteSkip(tx, file); err != nil {
		return err
	}
	rows, err := openJobsWhere(tx, "media_file_id = ?", file)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.cancelTx(tx, row, reason); err != nil {
			return err
		}
	}
	return nil
}

// CancelForTitle cancels the jobs of every file of a removed title and drops its exclusion.
func (s *JobService) CancelForTitle(tx *gorm.DB, title domain.TitleID) error {
	files := tx.Model(&eligibleFileRow{}).Select("media_file_id").Where("title_id = ?", title)
	if err := tx.Where("media_file_id IN (?)", files).Delete(&skipRow{}).Error; err != nil {
		return err
	}
	if err := tx.Where("title_id = ?", title).Delete(&eligibleFileRow{}).Error; err != nil {
		return err
	}
	if err := tx.Where("scope = ? AND ref = ?", ExcludeTitle, title).Delete(&exclusionRow{}).Error; err != nil {
		return err
	}
	rows, err := openJobsWhere(tx, "title_id = ?", title)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.cancelTx(tx, row, ReasonTitleRemoved); err != nil {
			return err
		}
	}
	return nil
}

// Retry queues a failed or not-worth-it job again from its first attempt and forgets its skip.
func (s *JobService) Retry(ctx context.Context, id domain.JobID) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := jobIn(tx, id)
		if err != nil {
			return err
		}
		retryable := []api.JobState{api.StateFailed, api.StateNotWorthIt}
		if !slices.Contains(retryable, row.State) {
			return ErrJobNotFailed
		}
		if err := tx.Where("job_id = ?", id).Delete(&attemptRow{}).Error; err != nil {
			return err
		}
		if row.Kind == api.JobKindTranscode {
			if err := tx.Where("media_file_id = ?", row.MediaFileID).Delete(&skipRow{}).Error; err != nil {
				return err
			}
		}
		_, err = moveJob(tx, id, retryable, clearAssignment(map[string]any{
			"state": api.StateQueued, "attempt": 1, "excluded_agents": emptyAgents, "not_before": nil,
			"avoid_agent_id": nil, "avoid_until": nil, "last_agent_id": nil, "error": "", "finished_at": nil,
			"progress": 0, "result": nil, "output_sha256": "", "output_size": 0, "updated_at": s.now(),
		}))
		if err != nil {
			return err
		}
		return s.kickAfterCommit(tx)
	})
}

// Swapped marks the file's verified job SWAPPED once Library swapped the output in.
func (s *JobService) Swapped(tx *gorm.DB, transcoded libraryapi.FileTranscoded) error {
	if err := tx.Where("media_file_id = ?", transcoded.MediaFile).Delete(&eligibleFileRow{}).Error; err != nil {
		return err
	}
	var rows []jobRow
	err := tx.Where("media_file_id = ? AND kind = ? AND state = ?", transcoded.MediaFile, api.JobKindTranscode, api.StateVerified).Find(&rows).Error
	if err != nil {
		return err
	}
	now := s.now()
	for _, row := range rows {
		if _, err := moveJob(tx, row.ID, []api.JobState{api.StateVerified}, map[string]any{
			"state": api.StateSwapped, "progress": 1, "finished_at": now, "updated_at": now,
		}); err != nil {
			return err
		}
		if err := endAttempt(tx, row, outcomeSwapped, "", now); err != nil {
			return err
		}
		if err := s.afterCommit(tx, func(ctx context.Context) { removeStaged(ctx, row.StagingPath) }); err != nil {
			return err
		}
	}
	return nil
}

// swapRetryable are the rejections a new attempt can fix.
var swapRetryable = []libraryapi.SwapRejectReason{libraryapi.SwapHashMismatch, libraryapi.SwapProbeFailed}

// SwapRejected fails the attempt Library refused to swap in. A changed or missing source ends the job;
// a new CutoffMet queues the file again.
func (s *JobService) SwapRejected(tx *gorm.DB, rejected libraryapi.TranscodeSwapRejected) error {
	row, err := jobIn(tx, rejected.Job)
	if errors.Is(err, ErrJobNotFound) || (err == nil && row.State != api.StateVerified) {
		return nil
	}
	if err != nil {
		return err
	}
	reason := "Library refused the swap: " + string(rejected.Reason)
	f := final(reason)
	if slices.Contains(swapRetryable, rejected.Reason) {
		f = retryable(reason)
	}
	return s.failAttempt(tx, row, f)
}

// LeasedJob returns a job leased to the agent, for its byte endpoints. A job still offered to it counts:
// the agent starts fetching as it sends its accept, which may not have been handled yet.
func (s *JobService) LeasedJob(ctx context.Context, agent domain.AgentID, id domain.JobID) (jobRow, error) {
	row, err := jobIn(s.deps.DB.Read(ctx), id)
	if errors.Is(err, ErrJobNotFound) || (err == nil && !heldBy(row, agent) && !offeredTo(row, agent)) {
		return jobRow{}, ErrNotLeased
	}
	return row, err
}

// RecordUpload stores the digest of a completed upload, which the agent's result must name.
func (s *JobService) RecordUpload(ctx context.Context, agent domain.AgentID, id domain.JobID, sha256 string, size int64) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		result := tx.Model(&jobRow{}).Where("id = ? AND agent_id = ? AND state IN ?", id, agent, agentHeld).
			Updates(map[string]any{"output_sha256": sha256, "output_size": size, "updated_at": s.now()})
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrNotLeased
		}
		return result.Error
	})
}

// throughputWeight is how much one transfer moves an agent's average link speed.
const throughputWeight = 0.3

// ObserveTransfer folds one transfer's speed into the agent's average.
func (s *JobService) ObserveTransfer(ctx context.Context, agent domain.AgentID, bytes int64, took time.Duration) error {
	if bytes <= 0 || took <= 0 {
		return nil
	}
	sample := float64(bytes) / took.Seconds()
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := s.deps.Agents.row(tx, agent)
		if err != nil {
			return err
		}
		next := sample
		if row.ThroughputBps > 0 {
			next = throughputWeight*sample + (1-throughputWeight)*float64(row.ThroughputBps)
		}
		return tx.Model(&agentRow{}).Where("id = ?", agent).Update("throughput_bps", int64(next)).Error
	})
}

// SavedBytes is what each agent's swapped transcodes saved.
func (s *JobService) SavedBytes(ctx context.Context) (map[domain.AgentID]int64, error) {
	var rows []struct {
		AgentID domain.AgentID
		Saved   int64
	}
	err := s.deps.DB.Read(ctx).Model(&jobRow{}).Select("agent_id, SUM(source_size - output_size) AS saved").
		Where("state = ? AND agent_id IS NOT NULL", api.StateSwapped).Group("agent_id").Scan(&rows).Error
	saved := map[domain.AgentID]int64{}
	for _, row := range rows {
		saved[row.AgentID] = row.Saved
	}
	return saved, err
}

// TransferMode is the agent's effective transfer mode.
func (s *JobService) TransferMode(ctx context.Context, agent domain.AgentID) (api.TransferMode, error) {
	row, err := s.deps.Agents.row(s.deps.DB.Read(ctx), agent)
	if err != nil {
		return "", err
	}
	if row.ModeOverride != nil {
		return api.TransferMode(*row.ModeOverride), nil
	}
	return api.TransferMode(row.ReportedMode), nil
}

// TransferCap is the server-wide limit on concurrent transfers.
func (s *JobService) TransferCap(ctx context.Context) (int, error) {
	current, err := agentSettingsKey.Get(ctx, s.deps.Store)
	return current.TransferCap, err
}

// SourceOf is where a job's source and staged output live.
func (s *JobService) SourceOf(ctx context.Context, row jobRow) (libraryapi.ProcessingSource, error) {
	return s.deps.Library.SourceForProcessing(ctx, row.MediaFileID)
}

// stagedExtensions is the file type each kind of job uploads.
var stagedExtensions = map[api.JobKind]string{
	api.JobKindTranscode: ".mkv",
	api.JobKindWhisper:   ".srt",
}

func stagedName(job domain.JobID, kind api.JobKind) string {
	return fmt.Sprintf("%s%s", job, stagedExtensions[kind])
}

func stagedPath(stagingDir string, job domain.JobID, kind api.JobKind) string {
	return filepath.Join(stagingDir, stagedName(job, kind))
}
