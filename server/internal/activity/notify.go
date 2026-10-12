package activity

import (
	"context"
	"errors"
	"fmt"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"
	transcodeapi "magpie/internal/transcode/api"

	"gorm.io/gorm"
)

// audience is who gets a notice in-app.
type audience string

const (
	toAdmins audience = "ADMINS"
	toUser   audience = "USER"
)

// In-app links, relative to the web app.
const (
	linkRequests    = "/requests"
	linkTitles      = "/titles/"
	linkConnections = "/settings/connections"
	linkAgents      = "/activity/agents"
	linkActivity    = "/activity"
	// linkIssueQuery names the issue whose thread the Requests page opens.
	linkIssueQuery = "issue"
)

const (
	someone          = "Someone"
	someTitle        = "A title"
	someRequest      = "Your request"
	someIssueSubject = "a title"
)

// notice is one notification an event causes: its kind, who gets it in-app, and its text.
type notice struct {
	Kind  string
	To    audience
	User  domain.UserID // set when To is toUser
	Title string
	Body  string
	Link  string
}

// lookup finds names and titles for notice text, inside the handler's transaction.
type lookup struct {
	s  *Service
	tx *gorm.DB
}

var noticeBuilders = map[string]func(context.Context, lookup, bus.Event) ([]notice, error){}

// onNotice registers which notices events of one payload type cause.
func onNotice[T domain.Message](fn func(context.Context, lookup, T) ([]notice, error)) {
	var zero T
	noticeBuilders[zero.Type()] = func(ctx context.Context, l lookup, ev bus.Event) ([]notice, error) {
		payload, err := bus.Decode[T](ev)
		if err != nil {
			return nil, err
		}
		return fn(ctx, l, payload)
	}
}

func titleLink(title domain.TitleID) string {
	return linkTitles + string(title)
}

// titleName is the title's name, or a fallback when the catalog doesn't know it.
func (l lookup) titleName(ctx context.Context, title domain.TitleID) (string, error) {
	t, err := l.s.catalog.Title(ctx, title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return someTitle, nil
	}
	return t.Name, err
}

// requestSubject names what a request asked for and the title it became, if the catalog has it yet.
// The event's kind and TMDB ID are used when the request itself can't be read.
func (l lookup) requestSubject(ctx context.Context, request domain.RequestID, kind domain.MediaKind, tmdb domain.TmdbID) (string, *domain.TitleID, error) {
	r, err := l.s.requests.Request(ctx, request)
	switch {
	case errors.Is(err, requestsapi.ErrNotFound):
	case err != nil:
		return "", nil, err
	default:
		kind, tmdb = r.Kind, r.Tmdb
		if r.Title != nil {
			name, err := l.titleName(ctx, *r.Title)
			return name, r.Title, err
		}
	}
	if kind == "" {
		return someRequest, nil, nil
	}
	t, err := l.s.catalog.TitleByTmdb(ctx, kind, tmdb)
	switch {
	case errors.Is(err, catalogapi.ErrNotFound):
		return "Your " + humanize(kind) + " request", nil, nil
	case err != nil:
		return "", nil, err
	}
	return t.Name, &t.ID, nil
}

func requestLink(title *domain.TitleID) string {
	if title == nil {
		return linkRequests
	}
	return titleLink(*title)
}

// issueLink opens the issue's thread on the Requests page.
func issueLink(issue domain.IssueID) string {
	return linkRequests + "?" + linkIssueQuery + "=" + string(issue)
}

func (l lookup) issueTitle(ctx context.Context, issue domain.IssueID) (string, error) {
	i, err := l.s.requests.Issue(ctx, issue)
	if errors.Is(err, requestsapi.ErrNotFound) {
		return someIssueSubject, nil
	}
	if err != nil {
		return "", err
	}
	return l.titleName(ctx, i.Title)
}

// refTitle finds the title a download or media file belongs to.
func (l lookup) refTitle(ctx context.Context, kind refKind, ref string) (string, *domain.TitleID, error) {
	title, _, found, err := l.s.repo.lookup(l.tx, kind, ref)
	if err != nil || !found {
		return "", nil, err
	}
	name, err := l.titleName(ctx, title)
	return name, &title, err
}

func (l lookup) userName(ctx context.Context, user domain.UserID) (string, error) {
	u, err := l.s.identity.User(ctx, user)
	switch {
	case errors.Is(err, identityapi.ErrNotFound):
		return someone, nil
	case err != nil:
		return "", err
	case u.DisplayName != "":
		return u.DisplayName, nil
	}
	return u.Username, nil
}

// failure is an admin notice about a title found through a ref, named "<what>: <title>" when known.
func (l lookup) failure(ctx context.Context, kind string, what string, ref refKind, id string, reason string) ([]notice, error) {
	name, title, err := l.refTitle(ctx, ref, id)
	if err != nil {
		return nil, err
	}
	n := notice{Kind: kind, To: toAdmins, Title: what, Body: reason, Link: linkActivity}
	if title != nil {
		n.Title, n.Link = what+": "+name, titleLink(*title)
	}
	return []notice{n}, nil
}

func init() {
	onNotice(func(ctx context.Context, l lookup, e requestsapi.RequestCreated) ([]notice, error) {
		name, _, err := l.requestSubject(ctx, e.Request, e.Kind, e.Tmdb)
		if err != nil {
			return nil, err
		}
		user, err := l.userName(ctx, e.User)
		return []notice{{
			Kind: api.KindRequestPending, To: toAdmins, Title: user + " requested " + name,
			Body: "Waiting for your approval.", Link: linkRequests,
		}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e requestsapi.RequestApproved) ([]notice, error) {
		name, title, err := l.requestSubject(ctx, e.Request, e.Kind, e.Tmdb)
		return []notice{{
			Kind: api.KindRequestApproved, To: toUser, User: e.User, Title: name + " was approved",
			Body: "Magpie is looking for it now.", Link: requestLink(title),
		}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e requestsapi.RequestDeclined) ([]notice, error) {
		name, _, err := l.requestSubject(ctx, e.Request, "", 0)
		return []notice{{
			Kind: api.KindRequestDeclined, To: toUser, User: e.User, Title: name + " was declined",
			Body: e.Reason, Link: linkRequests,
		}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e requestsapi.RequestAvailable) ([]notice, error) {
		name, err := l.titleName(ctx, e.Title)
		return []notice{{
			Kind: api.KindRequestAvailable, To: toUser, User: e.User, Title: name + " is available",
			Body: "Ready to watch in Jellyfin.", Link: titleLink(e.Title),
		}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e requestsapi.IssueReported) ([]notice, error) {
		name, err := l.titleName(ctx, e.Title)
		if err != nil {
			return nil, err
		}
		user, err := l.userName(ctx, e.User)
		return []notice{{
			Kind: api.KindIssueReported, To: toAdmins, Title: "Issue reported on " + name,
			Body: fmt.Sprintf("%s reported a %s issue.", user, humanize(e.Kind)), Link: issueLink(e.Issue),
		}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e requestsapi.IssueCommented) ([]notice, error) {
		name, err := l.issueTitle(ctx, e.Issue)
		if err != nil {
			return nil, err
		}
		n := notice{Kind: api.KindIssueReply, To: toUser, User: e.Reporter, Title: "New reply on your issue about " + name, Link: issueLink(e.Issue)}
		if e.Author == e.Reporter {
			n.To, n.User, n.Title = toAdmins, "", "New reply on the issue about "+name
		}
		return []notice{n}, nil
	})
	onNotice(func(ctx context.Context, l lookup, e requestsapi.IssueResolved) ([]notice, error) {
		name, err := l.issueTitle(ctx, e.Issue)
		return []notice{{
			Kind: api.KindIssueResolved, To: toUser, User: e.Reporter,
			Title: "Your issue about " + name + " was resolved", Link: issueLink(e.Issue),
		}}, err
	})

	onNotice(func(ctx context.Context, l lookup, e libraryapi.ImportFailed) ([]notice, error) {
		return l.failure(ctx, api.KindImportFailed, "Import failed", refDownload, string(e.Download), e.Reason)
	})
	onNotice(func(ctx context.Context, l lookup, e acquisitionapi.DownloadFailed) ([]notice, error) {
		name, err := l.titleName(ctx, e.Title)
		return []notice{{
			Kind: api.KindDownloadFailed, To: toAdmins, Title: "Download failed: " + name, Body: e.Reason, Link: titleLink(e.Title),
		}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e transcodeapi.JobFailed) ([]notice, error) {
		if e.Kind != transcodeapi.JobKindTranscode || !e.Final {
			return nil, nil
		}
		return l.failure(ctx, api.KindTranscodeFailed, "Transcode failed", refMediaFile, string(e.MediaFile), e.Reason)
	})
	onNotice(func(_ context.Context, _ lookup, e platformapi.ServiceHealthChanged) ([]notice, error) {
		if e.Healthy {
			return nil, nil
		}
		return []notice{{
			Kind: api.KindServiceUnreachable, To: toAdmins, Title: serviceName(e.Service) + " unreachable",
			Body: e.Error, Link: linkConnections,
		}}, nil
	})
	onNotice(func(_ context.Context, _ lookup, _ transcodeapi.AgentOffline) ([]notice, error) {
		return []notice{{
			Kind: api.KindAgentOffline, To: toAdmins, Title: "Agent offline",
			Body: "An agent stopped answering. Its jobs go to other agents.", Link: linkAgents,
		}}, nil
	})

	onNotice(func(ctx context.Context, l lookup, e acquisitionapi.ReleaseGrabbed) ([]notice, error) {
		name, err := l.titleName(ctx, e.Title)
		return []notice{{Kind: api.KindGrabbed, Title: "Grabbed " + name, Body: e.Release, Link: titleLink(e.Title)}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e libraryapi.FileImported) ([]notice, error) {
		name, err := l.titleName(ctx, e.Title)
		return []notice{{Kind: api.KindImported, Title: "Imported " + name, Body: e.ReleaseName, Link: titleLink(e.Title)}}, err
	})
	onNotice(func(ctx context.Context, l lookup, e libraryapi.FileTranscoded) ([]notice, error) {
		name, title, err := l.refTitle(ctx, refMediaFile, string(e.MediaFile))
		if err != nil || title == nil {
			return nil, err
		}
		n := notice{Kind: api.KindTranscoded, Title: "Transcoded " + name, Link: titleLink(*title)}
		if saved := savedPercentOf(e.OldSize, e.NewSize); saved != nil {
			n.Body = fmt.Sprintf("Saved %d%%.", *saved)
		}
		return []notice{n}, nil
	})
	onNotice(func(ctx context.Context, l lookup, e acquisitionapi.ReleaseRejected) ([]notice, error) {
		if e.BlockedFile == nil {
			return nil, nil
		}
		name, err := l.titleName(ctx, e.Title)
		return []notice{{
			Kind: api.KindBlockedFileRejected, Title: "Rejected a release for " + name,
			Body: e.Release + ": blocked file " + *e.BlockedFile, Link: titleLink(e.Title),
		}}, err
	})
}
