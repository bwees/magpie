package transcode

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/transcode/api"

	"gorm.io/gorm"
)

// QueueOnCutoff makes a file eligible and runs the policy on it.
func (s *JobService) QueueOnCutoff(ctx context.Context, tx *gorm.DB, met acquisitionapi.CutoffMet) error {
	eligible := eligibleFileRow{MediaFileID: met.File, TitleID: met.Title, Origin: met.Origin, CutoffMetAt: s.now()}
	if err := tx.Save(&eligible).Error; err != nil {
		return err
	}
	policy, err := policyKey.GetTx(tx)
	if err != nil {
		return err
	}
	return s.evaluate(ctx, tx, eligible, policy)
}

// evaluate runs the policy on one eligible file. It queues a transcode, re-plans or cancels one still
// waiting in the queue, or records a skip. A skip already recorded for the same reason and policy
// publishes nothing again. Jobs an agent already holds finish under the policy they started with.
func (s *JobService) evaluate(ctx context.Context, tx *gorm.DB, eligible eligibleFileRow, policy Policy) error {
	if !policy.Enabled {
		return nil
	}
	open, hasOpen, err := openJobFor(tx, eligible.MediaFileID, api.JobKindTranscode)
	if err != nil || (hasOpen && open.State != api.StateQueued) {
		return err
	}
	file, err := s.deps.Library.File(ctx, eligible.MediaFileID)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	facts, err := s.factsOf(tx, eligible, file, policy)
	if err != nil {
		return err
	}
	d := policy.decide(facts)
	switch d.Result {
	case resultSkip:
		if hasOpen {
			if err := s.cancelTx(tx, open, ReasonPolicySkips+d.Detail); err != nil {
				return err
			}
		}
		return s.skipOnce(tx, eligible.MediaFileID, d, policy)
	case resultQueue:
		if err := deleteSkip(tx, eligible.MediaFileID); err != nil {
			return err
		}
		if hasOpen {
			return replan(tx, open, d.Plan, s.now())
		}
		return s.queueTranscode(tx, eligible, file, d.Plan)
	}
	return nil
}

func (s *JobService) factsOf(tx *gorm.DB, eligible eligibleFileRow, file libraryapi.MediaFile, policy Policy) (fileFacts, error) {
	facts := fileFacts{Info: file.MediaInfo}
	var err error
	if facts.Excluded, err = excludedFile(tx, eligible.TitleID, file.RootFolder); err != nil {
		return facts, err
	}
	if facts.DoneUnderPolicy, err = doneUnderPolicy(tx, eligible.MediaFileID, policy.Hash()); err != nil {
		return facts, err
	}
	facts.CapableAgentSeen, err = everPassed(tx, targetVideos[policy.TargetCodec].needs)
	return facts, err
}

func (s *JobService) skipOnce(tx *gorm.DB, file domain.MediaFileID, d decision, policy Policy) error {
	existing, ok, err := skipFor(tx, file)
	if err != nil || (ok && existing.Reason == d.Skip && existing.PolicyHash == policy.Hash()) {
		return err
	}
	return s.skip(tx, file, d.Skip, d.Detail, policy)
}

func (s *JobService) skip(tx *gorm.DB, file domain.MediaFileID, reason api.SkipReason, detail string, policy Policy) error {
	row := skipRow{MediaFileID: file, Reason: reason, Detail: detail, PolicyHash: policy.Hash(), SkippedAt: s.now()}
	if err := saveSkip(tx, row); err != nil {
		return err
	}
	return s.deps.Bus.Publish(tx, bus.EventOf(api.TranscodeSkipped{MediaFile: file, Reason: reason, Detail: detail}))
}

func (s *JobService) queueTranscode(tx *gorm.DB, eligible eligibleFileRow, file libraryapi.MediaFile, plan jobPlan) error {
	now := s.now()
	row := jobRow{
		ID:             domain.NewID[domain.JobID](),
		Kind:           api.JobKindTranscode,
		MediaFileID:    eligible.MediaFileID,
		TitleID:        eligible.TitleID,
		Origin:         eligible.Origin,
		State:          api.StateQueued,
		Priority:       transcodePriority(eligible.Origin),
		Attempt:        1,
		TargetCodec:    plan.TargetCodec,
		MaxRateKbps:    plan.MaxRateKbps,
		HDR10:          plan.HDR10,
		PolicyHash:     plan.PolicyHash,
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

// replan moves a queued job onto the current policy's target and cap.
func replan(tx *gorm.DB, row jobRow, plan jobPlan, now time.Time) error {
	if row.plan() == plan {
		return nil
	}
	_, err := moveJob(tx, row.ID, []api.JobState{api.StateQueued}, map[string]any{
		"target_codec": plan.TargetCodec, "max_rate_kbps": plan.MaxRateKbps, "hdr10": plan.HDR10,
		"policy_hash": plan.PolicyHash, "updated_at": now,
	})
	return err
}

// cleanFolder is how a root folder exclusion is stored and compared.
func cleanFolder(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}
