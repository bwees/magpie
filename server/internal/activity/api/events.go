package api

import "magpie/internal/domain"

const (
	TypeNotificationCreated = "activity.NotificationCreated"
	TypeAttentionChanged    = "activity.AttentionChanged"
	TypeProjectionUpdated   = "activity.ProjectionUpdated"
)

// ProjectionUpdated.Projection values.
const (
	ProjectionTimeline = "TIMELINE"
	ProjectionHistory  = "HISTORY"
	ProjectionPipeline = "PIPELINE"
)

// historyAggregate orders ProjectionUpdated events that name no title.
const historyAggregate = "HISTORY"

// Notification kinds: Notification.Kind and NotificationCreated.Kind values.
const (
	KindRequestPending      = "REQUEST_PENDING"
	KindRequestApproved     = "REQUEST_APPROVED"
	KindRequestDeclined     = "REQUEST_DECLINED"
	KindRequestAvailable    = "REQUEST_AVAILABLE"
	KindIssueReported       = "ISSUE_REPORTED"
	KindIssueReply          = "ISSUE_REPLY"
	KindIssueResolved       = "ISSUE_RESOLVED"
	KindImportFailed        = "IMPORT_FAILED"
	KindDownloadFailed      = "DOWNLOAD_FAILED"
	KindTranscodeFailed     = "TRANSCODE_FAILED"
	KindServiceUnreachable  = "SERVICE_UNREACHABLE"
	KindAgentOffline        = "AGENT_OFFLINE"
	KindGrabbed             = "GRABBED"
	KindImported            = "IMPORTED"
	KindTranscoded          = "TRANSCODED"
	KindBlockedFileRejected = "BLOCKED_FILE_REJECTED"
)

// NotificationCreated is durable. One per recipient per notification, so the live gateway can push it
// to that user only.
type NotificationCreated struct {
	Notification string        `json:"notification"` // ID
	User         domain.UserID `json:"user"`
	Kind         string        `json:"kind"`
	Title        string        `json:"title"`
	Link         string        `json:"link"`
}

// AttentionChanged is durable. An attention item opened or cleared. Admin-only on the live gateway.
type AttentionChanged struct {
	Item  string `json:"item"` // attention item ID
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	Open  bool   `json:"open"`  // false = cleared
	Count int    `json:"count"` // open items after this change (Home badge)
}

// ProjectionUpdated is ephemeral. A handler committed a change to the timeline, history or pipeline,
// so views showing it refetch. Views can't refetch on the source event: the projection may not have it yet.
type ProjectionUpdated struct {
	Projection string          `json:"projection"` // ProjectionTimeline | ProjectionHistory | ProjectionPipeline
	Title      *domain.TitleID `json:"title"`      // nil for history entries about no title
}

func (ProjectionUpdated) Type() string { return TypeProjectionUpdated }
func (e ProjectionUpdated) AggregateID() string {
	if e.Title == nil {
		return historyAggregate
	}
	return string(*e.Title)
}
func (ProjectionUpdated) Durable() bool { return false }

func (NotificationCreated) Type() string          { return TypeNotificationCreated }
func (e NotificationCreated) AggregateID() string { return string(e.User) }
func (NotificationCreated) Durable() bool         { return true }

func (AttentionChanged) Type() string          { return TypeAttentionChanged }
func (e AttentionChanged) AggregateID() string { return e.Item }
func (AttentionChanged) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{NotificationCreated{}, AttentionChanged{}, ProjectionUpdated{}}
}
