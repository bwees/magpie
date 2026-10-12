package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/platform/settings"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	jobsTable               = "transcode_jobs"
	attemptsTable           = "transcode_job_attempts"
	eligibleFilesTable      = "transcode_eligible_files"
	exclusionsTable         = "transcode_exclusions"
	skipsTable              = "transcode_skips"
	passedCapabilitiesTable = "transcode_passed_capabilities"

	newestJobsFirst = "queued_at DESC, id DESC"
	offerOrder      = "priority DESC, queued_at, id"
)

// ExclusionScope is what a transcode exclusion names.
type ExclusionScope string

const (
	ExcludeTitle      ExclusionScope = "TITLE"
	ExcludeRootFolder ExclusionScope = "ROOT_FOLDER"
)

// agentHeld are the states in which an agent holds the job and must heartbeat it.
var agentHeld = []api.JobState{api.StateLeased, api.StateFetching, api.StateEncoding, api.StateUploading}

// slotStates are the states that take a device slot: offered or held by an agent.
var slotStates = append([]api.JobState{api.StateOffered}, agentHeld...)

// openStates are the states of a job that hasn't finished.
var openStates = []api.JobState{
	api.StateQueued, api.StateOffered, api.StateLeased, api.StateFetching, api.StateEncoding,
	api.StateUploading, api.StateVerifying, api.StateVerified,
}

func activeStates() []api.JobState {
	return []api.JobState{api.StateOffered, api.StateLeased, api.StateFetching, api.StateEncoding, api.StateUploading, api.StateVerifying}
}

type jobRow struct {
	ID             domain.JobID `gorm:"primaryKey"`
	Kind           api.JobKind
	MediaFileID    domain.MediaFileID
	TitleID        domain.TitleID
	Origin         domain.Origin
	State          api.JobState
	Priority       int
	Attempt        int
	NotBefore      *time.Time
	TargetCodec    api.TargetCodec
	MaxRateKbps    int
	HDR10          bool            `gorm:"column:hdr10"`
	PolicyHash     string          // TRANSCODE jobs: the policy they were queued under
	Language       domain.Language // WHISPER jobs only, like WhisperModel
	WhisperModel   string
	AgentID        *domain.AgentID
	DeviceID       *string
	OfferedAt      *time.Time
	LeaseExpiresAt *time.Time
	Prefetch       bool
	LastAgentID    *domain.AgentID
	AvoidAgentID   *domain.AgentID
	AvoidUntil     *time.Time
	ExcludedAgents string
	SourceETag     string `gorm:"column:source_etag"`
	TransferETag   string `gorm:"column:transfer_etag"`
	SourceSize     int64
	StagingPath    string
	Spec           *string
	Result         *string
	OutputSHA256   string `gorm:"column:output_sha256"`
	OutputSize     int64
	Progress       float64
	Error          string
	QueuedAt       time.Time
	UpdatedAt      time.Time `gorm:"autoUpdateTime:false"`
	FinishedAt     *time.Time
}

func (jobRow) TableName() string { return jobsTable }

func (r jobRow) job() api.Job {
	return api.Job{
		ID:         r.ID,
		Kind:       r.Kind,
		MediaFile:  r.MediaFileID,
		State:      r.State,
		Priority:   r.Priority,
		Agent:      r.AgentID,
		Attempt:    r.Attempt,
		Progress:   r.Progress,
		Error:      r.Error,
		QueuedAt:   r.QueuedAt,
		FinishedAt: r.FinishedAt,
	}
}

func (r jobRow) excluded() ([]domain.AgentID, error) {
	var agents []domain.AgentID
	if err := json.Unmarshal([]byte(r.ExcludedAgents), &agents); err != nil {
		return nil, fmt.Errorf("decoding excluded agents of job %s: %w", r.ID, err)
	}
	return agents, nil
}

func (r jobRow) spec() (agentproto.JobSpec, error) {
	var spec agentproto.JobSpec
	if r.Spec == nil {
		return spec, fmt.Errorf("job %s has no spec", r.ID)
	}
	return spec, json.Unmarshal([]byte(*r.Spec), &spec)
}

func (r jobRow) result() (agentproto.JobResult, error) {
	var result agentproto.JobResult
	if r.Result == nil {
		return result, fmt.Errorf("job %s has no result", r.ID)
	}
	return result, json.Unmarshal([]byte(*r.Result), &result)
}

func (r jobRow) plan() jobPlan {
	return jobPlan{TargetCodec: r.TargetCodec, MaxRateKbps: r.MaxRateKbps, HDR10: r.HDR10, PolicyHash: r.PolicyHash}
}

// backlog is a transcode of a file a library scan or a migration found; it runs on spare slots only.
func (r jobRow) backlog() bool {
	return r.Kind == api.JobKindTranscode && r.Origin != domain.OriginDownload
}

func encodeJSON(v any) (*string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := string(raw)
	return &s, nil
}

type attemptRow struct {
	JobID     domain.JobID `gorm:"primaryKey"`
	Attempt   int          `gorm:"primaryKey"`
	AgentID   domain.AgentID
	Outcome   string
	Error     string
	StartedAt time.Time
	EndedAt   *time.Time
}

func (attemptRow) TableName() string { return attemptsTable }

type eligibleFileRow struct {
	MediaFileID domain.MediaFileID `gorm:"primaryKey"`
	TitleID     domain.TitleID
	Origin      domain.Origin
	CutoffMetAt time.Time
}

func (eligibleFileRow) TableName() string { return eligibleFilesTable }

type exclusionRow struct {
	Scope ExclusionScope `gorm:"primaryKey"`
	Ref   string         `gorm:"primaryKey"`
}

func (exclusionRow) TableName() string { return exclusionsTable }

type skipRow struct {
	MediaFileID domain.MediaFileID `gorm:"primaryKey"`
	Reason      api.SkipReason
	Detail      string
	PolicyHash  string
	SkippedAt   time.Time
}

func (skipRow) TableName() string { return skipsTable }

var ErrJobNotFound = errors.New("transcode: job not found")

// JobRepository stores jobs, their attempts, eligible files and skips. It is Transcode's api.Reader.
type JobRepository struct {
	db     *db.DB
	agents *AgentRepository
	store  *settings.Store
}

var _ api.Reader = (*JobRepository)(nil)

func NewJobRepository(d *db.DB, agents *AgentRepository, store *settings.Store) *JobRepository {
	return &JobRepository{db: d, agents: agents, store: store}
}

func (r *JobRepository) Job(ctx context.Context, id domain.JobID) (api.Job, error) {
	row, err := jobIn(r.db.Read(ctx), id)
	if errors.Is(err, ErrJobNotFound) {
		return api.Job{}, api.ErrNotFound
	}
	return row.job(), err
}

func (r *JobRepository) JobsForMediaFile(ctx context.Context, id domain.MediaFileID) ([]api.Job, error) {
	var rows []jobRow
	if err := r.db.Read(ctx).Where("media_file_id = ?", id).Order(newestJobsFirst).Find(&rows).Error; err != nil {
		return nil, err
	}
	return publicJobs(rows), nil
}

func (r *JobRepository) ActiveJobs(ctx context.Context) ([]api.Job, error) {
	var rows []jobRow
	if err := r.db.Read(ctx).Where("state IN ?", activeStates()).Order(offerOrder).Find(&rows).Error; err != nil {
		return nil, err
	}
	return publicJobs(rows), nil
}

// expectedSavingPct is the share of a queued file a transcode usually saves, for the queue's estimate.
const expectedSavingPct = 40

func (r *JobRepository) QueueSummary(ctx context.Context) (api.QueueSummary, error) {
	counts, err := r.counts(ctx)
	if err != nil {
		return api.QueueSummary{}, err
	}
	var queuedBytes int64
	err = r.db.Read(ctx).Model(&jobRow{}).Select("COALESCE(SUM(source_size), 0)").
		Where("state = ? AND kind = ?", api.StateQueued, api.JobKindTranscode).Scan(&queuedBytes).Error
	return api.QueueSummary{Counts: counts, EstimatedSavedBytes: queuedBytes * expectedSavingPct / percent}, err
}

const percent = 100

func (r *JobRepository) counts(ctx context.Context) (map[api.JobState]int, error) {
	var rows []struct {
		State api.JobState
		N     int
	}
	if err := r.db.Read(ctx).Model(&jobRow{}).Select("state, COUNT(*) AS n").Group("state").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[api.JobState]int{}
	for _, row := range rows {
		counts[row.State] = row.N
	}
	return counts, nil
}

// Agents lists every agent, ordered by hostname, with the jobs it holds.
func (r *JobRepository) Agents(ctx context.Context) ([]api.Agent, error) {
	conn := r.db.Read(ctx)
	agents, err := r.agents.agents(conn, nil)
	if err != nil {
		return nil, err
	}
	var held []jobRow
	if err := conn.Where("agent_id IS NOT NULL AND state IN ?", slotStates).Order(offerOrder).Find(&held).Error; err != nil {
		return nil, err
	}
	byAgent := map[domain.AgentID][]domain.JobID{}
	for _, job := range held {
		byAgent[*job.AgentID] = append(byAgent[*job.AgentID], job.ID)
	}
	for i := range agents {
		agents[i].Jobs = byAgent[agents[i].ID]
		if agents[i].Jobs == nil {
			agents[i].Jobs = []domain.JobID{}
		}
	}
	return agents, nil
}

func (r *JobRepository) IsExcluded(ctx context.Context, title domain.TitleID) (bool, error) {
	var n int64
	err := r.db.Read(ctx).Model(&exclusionRow{}).Where("scope = ? AND ref = ?", ExcludeTitle, title).Count(&n).Error
	return n > 0, err
}

func (r *JobRepository) Target(ctx context.Context) (api.Target, error) {
	policy, err := policyKey.Get(ctx, r.store)
	return policy.target(), err
}

// JobFilter narrows List. Empty fields match everything.
type JobFilter struct {
	States []api.JobState
	Kind   api.JobKind
	Limit  int
}

// list returns jobs, those an agent holds first, then queued ones in offer order, then the rest newest first.
func (r *JobRepository) list(ctx context.Context, f JobFilter) ([]jobRow, error) {
	q := r.db.Read(ctx).Model(&jobRow{})
	if len(f.States) > 0 {
		q = q.Where("state IN ?", f.States)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	waiting := append(activeStates(), api.StateQueued)
	order := clause.OrderBy{Expression: clause.Expr{
		SQL: "CASE WHEN state IN (?) THEN 0 WHEN state = ? THEN 1 ELSE 2 END, " +
			"CASE WHEN state IN (?) THEN priority END DESC, CASE WHEN state IN (?) THEN queued_at END, updated_at DESC, id",
		Vars:               []any{activeStates(), api.StateQueued, waiting, waiting},
		WithoutParentheses: true,
	}}
	var rows []jobRow
	err := q.Order(order).Limit(f.Limit).Find(&rows).Error
	return rows, err
}

func publicJobs(rows []jobRow) []api.Job {
	jobs := make([]api.Job, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, row.job())
	}
	return jobs
}

func jobIn(conn *gorm.DB, id domain.JobID) (jobRow, error) {
	var row jobRow
	err := conn.Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, ErrJobNotFound
	}
	return row, err
}

// moveJob applies changes to a job that is in one of from, and reports whether it was.
func moveJob(tx *gorm.DB, id domain.JobID, from []api.JobState, changes map[string]any) (bool, error) {
	result := tx.Model(&jobRow{}).Where("id = ? AND state IN ?", id, from).Updates(changes)
	return result.RowsAffected > 0, result.Error
}

// openJobFor returns the unfinished job of a kind for a file, if there is one.
func openJobFor(tx *gorm.DB, file domain.MediaFileID, kind api.JobKind) (jobRow, bool, error) {
	var rows []jobRow
	err := tx.Where("media_file_id = ? AND kind = ? AND state IN ?", file, kind, openStates).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return jobRow{}, false, err
	}
	return rows[0], true, nil
}

// openWhisperJob reports whether the file already has an unfinished Whisper job for the language.
func openWhisperJob(tx *gorm.DB, file domain.MediaFileID, language domain.Language) (bool, error) {
	var n int64
	err := tx.Model(&jobRow{}).Where("media_file_id = ? AND kind = ? AND language = ? AND state IN ?", file, api.JobKindWhisper, language, openStates).
		Count(&n).Error
	return n > 0, err
}

func openJobsWhere(tx *gorm.DB, where string, args ...any) ([]jobRow, error) {
	var rows []jobRow
	err := tx.Where(where, args...).Where("state IN ?", openStates).Find(&rows).Error
	return rows, err
}

// failuresOn counts the failed attempts an agent made at a job.
func failuresOn(tx *gorm.DB, job domain.JobID, agent domain.AgentID) (int64, error) {
	var n int64
	err := tx.Model(&attemptRow{}).Where("job_id = ? AND agent_id = ? AND outcome IN ?", job, agent, failedOutcomes).Count(&n).Error
	return n, err
}

func endAttempt(tx *gorm.DB, job jobRow, outcome, reason string, at time.Time) error {
	return tx.Model(&attemptRow{}).Where("job_id = ? AND attempt = ? AND ended_at IS NULL", job.ID, job.Attempt).
		Updates(map[string]any{"outcome": outcome, "error": reason, "ended_at": at}).Error
}

func skipFor(tx *gorm.DB, file domain.MediaFileID) (skipRow, bool, error) {
	var rows []skipRow
	err := tx.Where("media_file_id = ?", file).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return skipRow{}, false, err
	}
	return rows[0], true, nil
}

func saveSkip(tx *gorm.DB, row skipRow) error {
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func deleteSkip(tx *gorm.DB, file domain.MediaFileID) error {
	return tx.Where("media_file_id = ?", file).Delete(&skipRow{}).Error
}

// doneUnderPolicy reports whether Magpie transcoded the file, or found it not worth it, under the policy.
func doneUnderPolicy(tx *gorm.DB, file domain.MediaFileID, policyHash string) (bool, error) {
	var skips int64
	err := tx.Model(&skipRow{}).Where("media_file_id = ? AND reason = ? AND policy_hash = ?", file, api.SkipNotWorthIt, policyHash).
		Count(&skips).Error
	if err != nil || skips > 0 {
		return skips > 0, err
	}
	var swapped int64
	err = tx.Model(&jobRow{}).Where("media_file_id = ? AND kind = ? AND state = ? AND policy_hash = ?",
		file, api.JobKindTranscode, api.StateSwapped, policyHash).Count(&swapped).Error
	return swapped > 0, err
}

// excludedFile reports whether the title or the root folder holding the file is excluded.
func excludedFile(tx *gorm.DB, title domain.TitleID, rootFolder string) (bool, error) {
	var n int64
	err := tx.Model(&exclusionRow{}).
		Where("(scope = ? AND ref = ?) OR (scope = ? AND ref = ?)", ExcludeTitle, title, ExcludeRootFolder, cleanFolder(rootFolder)).
		Count(&n).Error
	return n > 0, err
}

// skipCounts counts the skipped files that are still eligible, per reason.
func skipCounts(conn *gorm.DB) (map[api.SkipReason]int, error) {
	var rows []struct {
		Reason api.SkipReason
		N      int
	}
	err := conn.Model(&skipRow{}).Select("reason, COUNT(*) AS n").
		Where("media_file_id IN (?)", conn.Model(&eligibleFileRow{}).Select("media_file_id")).
		Group("reason").Scan(&rows).Error
	counts := map[api.SkipReason]int{}
	for _, row := range rows {
		counts[row.Reason] = row.N
	}
	return counts, err
}

type passedCapabilityRow struct {
	Capability agentproto.Capability `gorm:"primaryKey"`
	AgentID    domain.AgentID
	PassedAt   time.Time
}

func (passedCapabilityRow) TableName() string { return passedCapabilitiesTable }

// everPassed reports whether any agent, online or not, has passed the capability.
func everPassed(tx *gorm.DB, c agentproto.Capability) (bool, error) {
	var n int64
	err := tx.Model(&passedCapabilityRow{}).Where("capability = ?", c).Count(&n).Error
	return n > 0, err
}
