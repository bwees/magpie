package api

import "magpie/internal/domain"

const (
	TypeRequestCreated       = "requests.RequestCreated"
	TypeRequestApproved      = "requests.RequestApproved"
	TypeRequestDeclined      = "requests.RequestDeclined"
	TypeRequestCancelled     = "requests.RequestCancelled"
	TypeRequestAvailable     = "requests.RequestAvailable"
	TypeIssueReported        = "requests.IssueReported"
	TypeIssueCommented       = "requests.IssueCommented"
	TypeIssueActionRequested = "requests.IssueActionRequested"
	TypeIssueResolved        = "requests.IssueResolved"
)

// IssueActionRequested.Action values.
const (
	IssueActionRegrab         = "REGRAB"
	IssueActionRerunSubtitles = "RERUN_SUBTITLES"
)

// IssueResolvedByAction is IssueResolved.By when a completed action closed the issue.
const IssueResolvedByAction = "ACTION"

// RequestCreated is durable: a request is waiting for approval. Not published for auto-approved requests.
type RequestCreated struct {
	Request domain.RequestID `json:"request"`
	User    domain.UserID    `json:"user"`
	Kind    domain.MediaKind `json:"kind"`
	Tmdb    domain.TmdbID    `json:"tmdb"`
	Scope   string           `json:"scope"`
	Seasons []int            `json:"seasons"`
}

func (RequestCreated) Type() string          { return TypeRequestCreated }
func (e RequestCreated) AggregateID() string { return string(e.Request) }
func (RequestCreated) Durable() bool         { return true }

// RequestApproved is durable: Catalog must make the scope tracked and monitored.
type RequestApproved struct {
	Request domain.RequestID `json:"request"`
	User    domain.UserID    `json:"user"`
	Kind    domain.MediaKind `json:"kind"`
	Tmdb    domain.TmdbID    `json:"tmdb"`
	Scope   string           `json:"scope"`   // ScopeMovie | ScopeAllSeasons | ScopeSeasons
	Seasons []int            `json:"seasons"` // set for ScopeSeasons
	Profile domain.ProfileID `json:"profile"`
	// Auto is true when approved by the user's auto-approve toggle or admin role.
	Auto       bool          `json:"auto"`
	ApprovedBy domain.UserID `json:"approvedBy"`
}

func (RequestApproved) Type() string          { return TypeRequestApproved }
func (e RequestApproved) AggregateID() string { return string(e.Request) }
func (RequestApproved) Durable() bool         { return true }

// RequestDeclined is durable.
type RequestDeclined struct {
	Request domain.RequestID `json:"request"`
	User    domain.UserID    `json:"user"`
	Reason  string           `json:"reason"`
}

func (RequestDeclined) Type() string          { return TypeRequestDeclined }
func (e RequestDeclined) AggregateID() string { return string(e.Request) }
func (RequestDeclined) Durable() bool         { return true }

// RequestCancelled is durable: the requester withdrew their pending request, and its row is gone.
type RequestCancelled struct {
	Request domain.RequestID `json:"request"`
	User    domain.UserID    `json:"user"`
}

func (RequestCancelled) Type() string          { return TypeRequestCancelled }
func (e RequestCancelled) AggregateID() string { return string(e.Request) }
func (RequestCancelled) Durable() bool         { return true }

// RequestAvailable is durable: every item in scope is confirmed in Jellyfin. Published once per request.
type RequestAvailable struct {
	Request domain.RequestID `json:"request"`
	User    domain.UserID    `json:"user"`
	Title   domain.TitleID   `json:"title"`
}

func (RequestAvailable) Type() string          { return TypeRequestAvailable }
func (e RequestAvailable) AggregateID() string { return string(e.Request) }
func (RequestAvailable) Durable() bool         { return true }

// IssueReported is durable.
type IssueReported struct {
	Issue   domain.IssueID    `json:"issue"`
	Title   domain.TitleID    `json:"title"`
	Episode *domain.EpisodeID `json:"episode"`
	Kind    string            `json:"kind"` // IssueKind*
	User    domain.UserID     `json:"user"`
}

func (IssueReported) Type() string          { return TypeIssueReported }
func (e IssueReported) AggregateID() string { return string(e.Issue) }
func (IssueReported) Durable() bool         { return true }

// IssueCommented is durable: someone replied on an Issue.
type IssueCommented struct {
	Issue    domain.IssueID `json:"issue"`
	Author   domain.UserID  `json:"author"`
	Reporter domain.UserID  `json:"reporter"`
}

func (IssueCommented) Type() string          { return TypeIssueCommented }
func (e IssueCommented) AggregateID() string { return string(e.Issue) }
func (IssueCommented) Durable() bool         { return true }

// IssueActionRequested is durable: an admin chose a fix. It carries Title + Episode, not a MediaFile:
// Requests can't read Library. Acquisition and Subtitles resolve the file through library.Reader.
type IssueActionRequested struct {
	Issue   domain.IssueID    `json:"issue"`
	Title   domain.TitleID    `json:"title"`
	Episode *domain.EpisodeID `json:"episode"`
	Action  string            `json:"action"` // IssueActionRegrab | IssueActionRerunSubtitles
}

func (IssueActionRequested) Type() string          { return TypeIssueActionRequested }
func (e IssueActionRequested) AggregateID() string { return string(e.Issue) }
func (IssueActionRequested) Durable() bool         { return true }

// IssueResolved is durable: the Issue was closed, by a completed action or by a person.
type IssueResolved struct {
	Issue    domain.IssueID `json:"issue"`
	Reporter domain.UserID  `json:"reporter"`
	By       string         `json:"by"` // IssueResolvedByAction | user ID
}

func (IssueResolved) Type() string          { return TypeIssueResolved }
func (e IssueResolved) AggregateID() string { return string(e.Issue) }
func (IssueResolved) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{
		RequestCreated{},
		RequestApproved{},
		RequestDeclined{},
		RequestCancelled{},
		RequestAvailable{},
		IssueReported{},
		IssueCommented{},
		IssueActionRequested{},
		IssueResolved{},
	}
}
