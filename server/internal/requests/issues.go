package requests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/requests/api"

	"gorm.io/gorm"
)

var (
	ErrCannotReport   = errors.New("requests: reporting issues is turned off for this user")
	ErrNotReporter    = errors.New("requests: only the reporter or an admin may do this")
	ErrInvalidIssue   = errors.New("requests: invalid issue")
	ErrIssueStatus    = errors.New("requests: not allowed in this issue status")
	ErrUnknownAction  = errors.New("requests: unknown issue action")
	ErrHiddenReporter = errors.New("requests: other users' issues are hidden")
)

// ImportWaitingError rejects a REGRAB while a download for the issue's items waits for the user to map
// its import. The admin either maps it or asks again to remove it first.
type ImportWaitingError struct {
	Download    domain.DownloadID
	ReleaseName string
}

func (e ImportWaitingError) Error() string {
	return fmt.Sprintf("requests: %s is waiting for its files to be mapped", e.ReleaseName)
}

var issueKinds = []string{api.IssueKindVideo, api.IssueKindAudio, api.IssueKindSubtitle, api.IssueKindOther}

var issueActions = []string{api.IssueActionRegrab, api.IssueActionRerunSubtitles}

// AllIssueStatuses lists every issue status in the order the UI shows them.
var AllIssueStatuses = []string{api.IssueStatusOpen, api.IssueStatusActionPending, api.IssueStatusResolved}

// NewIssue is what a user reports.
type NewIssue struct {
	Title   domain.TitleID
	Episode *domain.EpisodeID
	Kind    string
	Text    string
}

// IssueView is an issue as the HTTP layer shows it.
type IssueView struct {
	api.Issue
	ReporterName  string
	TitleName     string
	EpisodeInfo   *catalogapi.Episode
	PendingAction string
	ResolvedAt    *time.Time
	ResolvedBy    string
}

// CommentView is one reply in an issue's thread.
type CommentView struct {
	ID         CommentID
	Author     domain.UserID
	AuthorName string
	Body       string
	CreatedAt  time.Time
}

// IssueQuery fields that are nil or empty don't filter.
type IssueQuery struct {
	Statuses  []string
	User      *domain.UserID
	Title     *domain.TitleID
	CountOnly bool
}

type IssueList struct {
	Items []IssueView
	Total int
}

// Report stores an issue on a Title or one of its episodes.
func (s *Service) Report(ctx context.Context, viewer domain.Principal, in NewIssue) (IssueView, error) {
	if !viewer.Admin && !viewer.Permissions.ReportIssues {
		return IssueView{}, ErrCannotReport
	}
	in, err := s.checkNewIssue(ctx, in)
	if err != nil {
		return IssueView{}, err
	}
	row := issueRow{
		ID:        domain.NewID[domain.IssueID](),
		UserID:    viewer.User,
		TitleID:   in.Title,
		EpisodeID: in.Episode,
		Kind:      in.Kind,
		Text:      in.Text,
		Status:    api.IssueStatusOpen,
		CreatedAt: s.now().UTC(),
	}
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := s.repo.saveIssue(tx, row); err != nil {
			return err
		}
		return s.publish(tx, api.IssueReported{Issue: row.ID, Title: row.TitleID, Episode: row.EpisodeID, Kind: row.Kind, User: row.UserID})
	})
	if err != nil {
		return IssueView{}, err
	}
	return s.issueView(ctx, row.ID)
}

func (s *Service) checkNewIssue(ctx context.Context, in NewIssue) (NewIssue, error) {
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		return in, fmt.Errorf("%w: describe the problem", ErrInvalidIssue)
	}
	if !slices.Contains(issueKinds, in.Kind) {
		return in, fmt.Errorf("%w: kind must be one of %s", ErrInvalidIssue, strings.Join(issueKinds, ", "))
	}
	if _, err := s.catalog.Title(ctx, in.Title); errors.Is(err, catalogapi.ErrNotFound) {
		return in, fmt.Errorf("%w: unknown title %s", ErrInvalidIssue, in.Title)
	} else if err != nil {
		return in, err
	}
	if in.Episode == nil {
		return in, nil
	}
	episode, err := s.catalog.Episode(ctx, *in.Episode)
	if errors.Is(err, catalogapi.ErrNotFound) || (err == nil && episode.Title != in.Title) {
		return in, fmt.Errorf("%w: episode %s isn't in this title", ErrInvalidIssue, *in.Episode)
	}
	return in, err
}

// Issues lists the issues the viewer may see, newest first. Without ViewOthersRequests that is their own.
func (s *Service) Issues(ctx context.Context, viewer domain.Principal, q IssueQuery) (IssueList, error) {
	if !seesEveryone(viewer) {
		if q.User != nil && *q.User != viewer.User {
			return IssueList{}, ErrHiddenReporter
		}
		q.User = &viewer.User
	}
	conn := s.db.Read(ctx)
	f := issueFilter{Statuses: q.Statuses, User: q.User, Title: q.Title}
	total, err := s.repo.countIssues(conn, f)
	if err != nil || q.CountOnly {
		return IssueList{Items: []IssueView{}, Total: total}, err
	}
	rows, err := s.repo.issues(conn, f)
	if err != nil {
		return IssueList{}, err
	}
	items, err := s.issueViews(ctx, rows)
	return IssueList{Items: items, Total: total}, err
}

// IssueThread returns an issue the viewer may see with its comments, oldest first.
func (s *Service) IssueThread(ctx context.Context, viewer domain.Principal, id domain.IssueID) (IssueView, []CommentView, error) {
	view, err := s.visibleIssue(ctx, viewer, id)
	if err != nil {
		return IssueView{}, nil, err
	}
	rows, err := s.repo.comments(s.db.Read(ctx), id)
	if err != nil {
		return IssueView{}, nil, err
	}
	authors := make([]domain.UserID, 0, len(rows))
	for _, row := range rows {
		authors = append(authors, row.UserID)
	}
	names, err := s.names(ctx, authors)
	if err != nil {
		return IssueView{}, nil, err
	}
	comments := make([]CommentView, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, CommentView{ID: row.ID, Author: row.UserID, AuthorName: names[row.UserID], Body: row.Body, CreatedAt: row.CreatedAt.UTC()})
	}
	return view, comments, nil
}

func (s *Service) visibleIssue(ctx context.Context, viewer domain.Principal, id domain.IssueID) (IssueView, error) {
	view, err := s.issueView(ctx, id)
	if err != nil {
		return IssueView{}, err
	}
	if !seesEveryone(viewer) && view.User != viewer.User {
		return IssueView{}, api.ErrNotFound
	}
	return view, nil
}

// Comment adds a reply by the reporter or an admin.
func (s *Service) Comment(ctx context.Context, viewer domain.Principal, id domain.IssueID, body string) (CommentView, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return CommentView{}, fmt.Errorf("%w: the comment is empty", ErrInvalidIssue)
	}
	row := commentRow{ID: domain.NewID[CommentID](), IssueID: id, UserID: viewer.User, Body: body, CreatedAt: s.now().UTC()}
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		issue, err := s.ownedIssue(tx, viewer, id)
		if err != nil {
			return err
		}
		if err := s.repo.addComment(tx, row); err != nil {
			return err
		}
		return s.publish(tx, api.IssueCommented{Issue: id, Author: viewer.User, Reporter: issue.UserID})
	})
	if err != nil {
		return CommentView{}, err
	}
	names, err := s.names(ctx, []domain.UserID{viewer.User})
	return CommentView{ID: row.ID, Author: row.UserID, AuthorName: names[row.UserID], Body: row.Body, CreatedAt: row.CreatedAt}, err
}

// ownedIssue loads an issue the viewer reported, or any issue for an admin. Others' issues they can't
// see are ErrNotFound; ones they can see but didn't report are ErrNotReporter.
func (s *Service) ownedIssue(tx *gorm.DB, viewer domain.Principal, id domain.IssueID) (issueRow, error) {
	issue, err := s.repo.issue(tx, id)
	switch {
	case err != nil:
		return issueRow{}, err
	case viewer.Admin || issue.UserID == viewer.User:
		return issue, nil
	case seesEveryone(viewer):
		return issueRow{}, ErrNotReporter
	default:
		return issueRow{}, api.ErrNotFound
	}
}

// RequestAction asks the owning context to fix an open issue. A REGRAB while a download for the issue's
// items waits for its import to be mapped is an ImportWaitingError, unless waitingRemoved says the admin
// removed it (Remove & blocklist) first; Acquisition may not have dropped it yet, so the event names it.
func (s *Service) RequestAction(ctx context.Context, id domain.IssueID, action string, waitingRemoved bool) (IssueView, error) {
	if !slices.Contains(issueActions, action) {
		return IssueView{}, fmt.Errorf("%w: %s", ErrUnknownAction, action)
	}
	issue, err := s.repo.issue(s.db.Read(ctx), id)
	if err != nil {
		return IssueView{}, err
	}
	requested := api.IssueActionRequested{Issue: id, Title: issue.TitleID, Episode: issue.EpisodeID, Action: action, RemovedDownloads: []domain.DownloadID{}}
	if action == api.IssueActionRegrab {
		waiting, err := s.waitingDownloads(ctx, issue)
		if err != nil {
			return IssueView{}, err
		}
		if len(waiting) > 0 && !waitingRemoved {
			return IssueView{}, ImportWaitingError{Download: waiting[0].ID, ReleaseName: waiting[0].ReleaseName}
		}
		for _, d := range waiting {
			requested.RemovedDownloads = append(requested.RemovedDownloads, d.ID)
		}
	}
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.issue(tx, id)
		if err != nil {
			return err
		}
		if row.Status != api.IssueStatusOpen {
			return fmt.Errorf("%w: issue is %s", ErrIssueStatus, row.Status)
		}
		row.Status, row.PendingAction = api.IssueStatusActionPending, &action
		if err := s.repo.saveIssue(tx, row); err != nil {
			return err
		}
		return s.publish(tx, requested)
	})
	if err != nil {
		return IssueView{}, err
	}
	return s.issueView(ctx, id)
}

// waitingDownloads are the title's downloads waiting for the user to map their import that hold the
// issue's items: any of them for a movie or a whole-title issue, else those covering its episode.
func (s *Service) waitingDownloads(ctx context.Context, issue issueRow) ([]acquisitionapi.Download, error) {
	downloads, err := s.acquisition.DownloadsForTitle(ctx, issue.TitleID)
	if err != nil {
		return nil, err
	}
	waiting := []acquisitionapi.Download{}
	for _, d := range downloads {
		if d.State != acquisitionapi.StateImportPendingUser || d.RemovedAt != nil {
			continue
		}
		if issue.EpisodeID == nil || len(d.Episodes) == 0 || slices.Contains(d.Episodes, *issue.EpisodeID) {
			waiting = append(waiting, d)
		}
	}
	return waiting, nil
}

// Resolve closes an issue for its reporter or an admin.
func (s *Service) Resolve(ctx context.Context, viewer domain.Principal, id domain.IssueID) (IssueView, error) {
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		issue, err := s.ownedIssue(tx, viewer, id)
		if err != nil {
			return err
		}
		if issue.Status == api.IssueStatusResolved {
			return fmt.Errorf("%w: issue is already resolved", ErrIssueStatus)
		}
		return s.resolve(tx, issue, string(viewer.User))
	})
	if err != nil {
		return IssueView{}, err
	}
	return s.issueView(ctx, id)
}

func (s *Service) resolve(tx *gorm.DB, issue issueRow, by string) error {
	now := s.now().UTC()
	issue.Status, issue.ResolvedAt, issue.ResolvedBy = api.IssueStatusResolved, &now, &by
	if err := s.repo.saveIssue(tx, issue); err != nil {
		return err
	}
	return s.publish(tx, api.IssueResolved{Issue: issue.ID, Reporter: issue.UserID, By: by})
}

// ResolveAction resolves the issues on a Title waiting for action whose episode is among episodes. An issue
// on the whole Title, or any issue when episodes is empty (a movie), matches.
func (s *Service) ResolveAction(tx *gorm.DB, action string, title domain.TitleID, episodes []domain.EpisodeID) error {
	issues, err := s.repo.issues(tx, issueFilter{Statuses: []string{api.IssueStatusActionPending}, Title: &title})
	if err != nil {
		return err
	}
	for _, issue := range issues {
		matches := issue.EpisodeID == nil || len(episodes) == 0 || slices.Contains(episodes, *issue.EpisodeID)
		if !issue.pending(action) || !matches {
			continue
		}
		if err := s.resolve(tx, issue, api.IssueResolvedByAction); err != nil {
			return err
		}
	}
	return nil
}

// CloseForRemovedTitle resolves the open issues of a Title Catalog removed.
func (s *Service) CloseForRemovedTitle(tx *gorm.DB, title domain.TitleID) error {
	issues, err := s.repo.issues(tx, issueFilter{Statuses: openIssueStatuses, Title: &title})
	if err != nil {
		return err
	}
	for _, issue := range issues {
		if err := s.resolve(tx, issue, api.IssueResolvedByTitleRemoved); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) issueView(ctx context.Context, id domain.IssueID) (IssueView, error) {
	row, err := s.repo.issue(s.db.Read(ctx), id)
	if err != nil {
		return IssueView{}, err
	}
	views, err := s.issueViews(ctx, []issueRow{row})
	if err != nil {
		return IssueView{}, err
	}
	return views[0], nil
}

// issueViews adds the reporters' names and each issue's title name and episode numbering. A title or
// episode Catalog no longer knows shows without them.
func (s *Service) issueViews(ctx context.Context, rows []issueRow) ([]IssueView, error) {
	reporters := make([]domain.UserID, 0, len(rows))
	for _, row := range rows {
		reporters = append(reporters, row.UserID)
	}
	names, err := s.names(ctx, reporters)
	if err != nil {
		return nil, err
	}
	titles := map[domain.TitleID]string{}
	views := make([]IssueView, 0, len(rows))
	for _, row := range rows {
		view := IssueView{Issue: row.toAPI(), ReporterName: names[row.UserID], ResolvedAt: row.ResolvedAt, ResolvedBy: deref(row.ResolvedBy), PendingAction: deref(row.PendingAction)}
		name, known := titles[row.TitleID]
		if !known {
			if name, err = s.titleName(ctx, row.TitleID); err != nil {
				return nil, err
			}
			titles[row.TitleID] = name
		}
		view.TitleName = name
		if row.EpisodeID != nil {
			episode, err := s.catalog.Episode(ctx, *row.EpisodeID)
			switch {
			case err == nil:
				view.EpisodeInfo = &episode
			case !errors.Is(err, catalogapi.ErrNotFound):
				return nil, err
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) titleName(ctx context.Context, id domain.TitleID) (string, error) {
	title, err := s.catalog.Title(ctx, id)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return "", nil
	}
	return title.Name, err
}

// names maps users to their display names. A user Identity no longer knows has none.
func (s *Service) names(ctx context.Context, users []domain.UserID) (map[domain.UserID]string, error) {
	ids := make([]domain.UserID, 0, len(users))
	for _, id := range users {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	found, err := s.identity.Users(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := map[domain.UserID]string{}
	for _, u := range found {
		names[u.ID] = u.DisplayName
	}
	return names, nil
}
