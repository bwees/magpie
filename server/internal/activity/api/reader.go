// Package api is Activity's public contract: the timeline/pipeline/notification Reader, the events
// Activity publishes and the migration Importer. Spec: docs/spec/contexts/activity.md.
package api

import (
	"context"
	"time"

	"magpie/internal/domain"
)

// Stage is a main pipeline stage. Subtitles and transcode are separate chips beside the bar.
type Stage string

const (
	StageRequested   Stage = "REQUESTED"
	StageSearching   Stage = "SEARCHING"
	StageDownloading Stage = "DOWNLOADING"
	StageImporting   Stage = "IMPORTING"
	StageAvailable   Stage = "AVAILABLE"
)

// Stages lists the main stages from least to most advanced.
func Stages() []Stage {
	return []Stage{StageRequested, StageSearching, StageDownloading, StageImporting, StageAvailable}
}

// Severity values, from least to most severe.
const (
	SeverityInfo  = "INFO"
	SeverityWarn  = "WARN"
	SeverityError = "ERROR"
)

// SubsChip.State values.
const (
	SubsPending = "PENDING"
	SubsSettled = "SETTLED"
)

// TranscodeChip.State values.
const (
	TranscodeQueued  = "QUEUED"
	TranscodeRunning = "RUNNING"
	TranscodeDone    = "DONE"
	TranscodeSkipped = "SKIPPED"
	TranscodeFailed  = "FAILED"
)

// AttentionItem.Kind and AttentionChanged.Kind values.
const (
	AttentionRequestPending  = "REQUEST_PENDING"
	AttentionIssueOpen       = "ISSUE_OPEN"
	AttentionPipelineFailed  = "PIPELINE_FAILED"
	AttentionAgentOffline    = "AGENT_OFFLINE"
	AttentionServiceDown     = "SERVICE_DOWN"
	AttentionTranscodeFailed = "TRANSCODE_FAILED"
)

type PipelineRow struct {
	Title     domain.TitleID
	Episode   *domain.EpisodeID // nil for movies
	Stage     Stage
	Failed    bool
	Detail    string         // human text, e.g. "Waiting for Jellyfin", "No accepted release"
	Subs      *SubsChip      // nil until a file exists
	Transcode *TranscodeChip // nil until the file is eligible (CutoffMet) or skipped
	Updated   time.Time
}

type SubsChip struct {
	State     string            // SubsPending | SubsSettled
	Satisfied []domain.Language // e.g. [en] → "Subs ✓ en"
	Missing   []domain.Language
}

type TranscodeChip struct {
	State  string        // TranscodeQueued | TranscodeRunning | TranscodeDone | TranscodeSkipped | TranscodeFailed
	Saved  *int          // percent saved, when DONE
	Reason string        // skip/failure reason
	Job    *domain.JobID // RUNNING: the UI overlays live % from ephemeral transcode.JobProgressed
}

type AttentionItem struct {
	ID, Kind, Ref, Text, Action string
	Title                       *domain.TitleID
	OpenedAt                    time.Time
}

type TimelineEntry struct {
	EventID  string
	Type     string
	Severity string
	Episodes []domain.EpisodeID
	Text     string
	Detail   string
	At       time.Time
}

type Notification struct {
	ID, Kind, Title, Body, Link string
	CreatedAt                   time.Time
	Read                        bool
}

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

// Page selects entries strictly older than Before (nil = from the newest), at most Limit of them.
type Page struct {
	Before *time.Time
	Limit  int // ≤ 0 means DefaultPageLimit; capped at MaxPageLimit
}

// EffectiveLimit is the number of entries a Reader returns at most for p.
func (p Page) EffectiveLimit() int {
	if p.Limit <= 0 {
		return DefaultPageLimit
	}
	return min(p.Limit, MaxPageLimit)
}

// Reader is read-only and backed by Activity's repository. No other context depends on it in v1.
type Reader interface {
	// Timeline returns a title's entries, newest first. Unknown title → empty slice, no error.
	Timeline(ctx context.Context, title domain.TitleID, p Page) ([]TimelineEntry, error)
	// History returns the global feed (90-day retention), newest first. minSeverity keeps entries at
	// that severity or above (INFO < WARN < ERROR); "" keeps all.
	History(ctx context.Context, minSeverity string, p Page) ([]TimelineEntry, error)
	// Pipeline returns one row per movie or per episode of the title, episodes in season/number order.
	// A title with no events yet returns rows at REQUESTED once catalog.TitleAdded is processed; unknown → empty.
	Pipeline(ctx context.Context, title domain.TitleID) ([]PipelineRow, error)
	// TitleStages returns the summary stage per title: the least-advanced row (first in season/number
	// order on ties), for library grids. Unknown titles are absent from the map.
	TitleStages(ctx context.Context, titles []domain.TitleID) (map[domain.TitleID]PipelineRow, error)
	// Notifications returns a user's in-app notifications, newest first.
	Notifications(ctx context.Context, user domain.UserID, unreadOnly bool, p Page) ([]Notification, error)
	// UnreadCount returns how many of the user's in-app notifications are unread.
	UnreadCount(ctx context.Context, user domain.UserID) (int, error)
	// Attention returns open attention items, oldest first. Admin view; requesters get none.
	Attention(ctx context.Context) ([]AttentionItem, error)
}
