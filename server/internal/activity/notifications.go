package activity

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

// MaxNotificationURLs is how many URLs one user may add.
const MaxNotificationURLs = 10

const (
	maskedSuffix   = "…"
	schemeSuffix   = "://"
	maxLabelLength = 60
)

var (
	ErrNotificationNotFound = errors.New("activity: notification not found")
	ErrURLNotFound          = errors.New("activity: notification url not found")
	ErrTooManyURLs          = errors.New("activity: too many notification urls")
	ErrKindNotAllowed       = errors.New("activity: notification kind not allowed for this user")
)

// testMessage is what Send test delivers.
var testMessage = Message{
	Title: "Magpie test notification",
	Body:  "This URL works. It gets the events you ticked for it.",
}

// NotificationURL is one of a user's external URLs as its owner sees it: the URL itself is masked.
type NotificationURL struct {
	ID         string
	Label      string
	Masked     string
	Kinds      []string
	CreatedAt  time.Time
	LastSentAt *time.Time
	LastError  string
}

type NewNotificationURL struct {
	Label string
	URL   string
	Kinds []string
}

// NotificationURLChange edits a URL. Nil fields stay as they are.
type NotificationURLChange struct {
	Label *string
	Kinds []string
}

// Notify writes the in-app notifications an event causes, announces each one to its user, and queues
// one send per subscribed external URL.
func (s *Service) Notify(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
	if err := s.rememberRefs(tx, ev); err != nil {
		return err
	}
	build, ok := noticeBuilders[ev.Type]
	if !ok {
		return nil
	}
	notices, err := build(ctx, lookup{s: s, tx: tx}, ev)
	if err != nil {
		return err
	}
	for _, n := range notices {
		spec, ok := kindSpecOf(n.Kind)
		if !ok {
			continue
		}
		if !spec.OptIn {
			if err := s.notifyInApp(ctx, tx, ev, n); err != nil {
				return err
			}
		}
		if err := s.queueSends(ctx, tx, n, spec); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recipients(ctx context.Context, n notice) ([]identityapi.User, error) {
	if n.To == toAdmins {
		return s.identity.Admins(ctx)
	}
	user, err := s.identity.User(ctx, n.User)
	switch {
	case errors.Is(err, identityapi.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	case user.Disabled:
		return nil, nil
	}
	return []identityapi.User{user}, nil
}

type notificationID string

func (s *Service) notifyInApp(ctx context.Context, tx *gorm.DB, ev bus.Event, n notice) error {
	recipients, err := s.recipients(ctx, n)
	if err != nil {
		return err
	}
	for _, user := range recipients {
		row := notificationRow{
			ID: string(domain.NewID[notificationID]()), UserID: string(user.ID), EventID: ev.ID, Kind: n.Kind,
			Title: n.Title, Body: n.Body, Link: n.Link, CreatedAt: ev.OccurredAt.UTC(),
		}
		added, err := s.repo.addNotification(tx, &row)
		if err != nil {
			return err
		}
		if !added {
			continue
		}
		err = s.bus.Publish(tx, bus.EventOf(api.NotificationCreated{
			Notification: row.ID, User: user.ID, Kind: n.Kind, Title: n.Title, Link: n.Link,
		}))
		if err != nil {
			return err
		}
	}
	return nil
}

// queueSends queues one send per URL subscribed to the notice's kind whose owner may get it: admins get
// every kind, requesters only their own requester-scoped notices.
func (s *Service) queueSends(ctx context.Context, tx *gorm.DB, n notice, spec KindSpec) error {
	rows, err := s.repo.urlsSubscribedTo(tx, n.Kind)
	if err != nil || len(rows) == 0 {
		return err
	}
	owners, err := s.owners(ctx, rows)
	if err != nil {
		return err
	}
	message := s.external(n)
	for _, row := range rows {
		owner, ok := owners[domain.UserID(row.UserID)]
		if !ok || owner.Disabled {
			continue
		}
		own := n.To == toUser && n.User == owner.ID
		if !owner.IsAdmin() && (spec.Scope != ScopeRequester || !own) {
			continue
		}
		if err := s.jobs.EnqueueTx(tx, newSendTask(row.ID, n.Kind, message)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) owners(ctx context.Context, rows []notificationURLRow) (map[domain.UserID]identityapi.User, error) {
	var ids []domain.UserID
	for _, row := range rows {
		if id := domain.UserID(row.UserID); !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	users, err := s.identity.Users(ctx, ids)
	if err != nil {
		return nil, err
	}
	owners := make(map[domain.UserID]identityapi.User, len(users))
	for _, user := range users {
		owners[user.ID] = user
	}
	return owners, nil
}

// external is the notice as an external message, linking to MAGPIE_BASE_URL when it is set.
func (s *Service) external(n notice) Message {
	m := Message{Title: n.Title, Body: n.Body}
	if s.baseURL != nil && n.Link != "" {
		m.Link = s.baseURL.JoinPath(n.Link).String()
	}
	return m
}

// Deliver runs one send. A service that is unavailable is retried after the task's next wait; when no
// waits are left, or the service refuses the message, the error is stored on the URL.
func (s *Service) Deliver(ctx context.Context, task sendTask, later delayedEnqueue) error {
	row, err := s.repo.notificationURL(ctx, task.URL)
	if errors.Is(err, ErrURLNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	allowed, err := s.mayStillSend(ctx, row, task.Kind)
	if err != nil || !allowed {
		return err
	}
	err = s.sender.Send(ctx, row.URL, task.Message)
	switch {
	case err == nil:
		return s.repo.recordSent(ctx, row.ID, s.now())
	case errors.Is(err, ErrUnavailable) && len(task.Retries) > 0:
		return later(ctx, task.retried(), task.Retries[0])
	}
	return s.repo.recordFailure(ctx, row.ID, err.Error())
}

// mayStillSend checks the URL still subscribes to kind and its owner may still hold it: a requester
// demoted or a kind removed since the send was queued drops the send.
func (s *Service) mayStillSend(ctx context.Context, row notificationURLRow, kind string) (bool, error) {
	kinds, err := row.kinds()
	if err != nil || !slices.Contains(kinds, kind) {
		return false, err
	}
	owner, err := s.identity.User(ctx, domain.UserID(row.UserID))
	if errors.Is(err, identityapi.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !owner.Disabled && mayHold(owner.IsAdmin(), kind), nil
}

// Notifications returns the user's in-app notifications and how many are unread.
func (s *Service) Notifications(ctx context.Context, user domain.UserID, unreadOnly bool, p api.Page) ([]api.Notification, int, error) {
	notifications, err := s.repo.Notifications(ctx, user, unreadOnly, p)
	if err != nil {
		return nil, 0, err
	}
	unread, err := s.repo.UnreadCount(ctx, user)
	return notifications, unread, err
}

func (s *Service) MarkRead(ctx context.Context, user domain.UserID, id string) error {
	return s.repo.markRead(ctx, user, id, s.now())
}

func (s *Service) MarkAllRead(ctx context.Context, user domain.UserID) error {
	return s.repo.markAllRead(ctx, user, s.now())
}

// PruneNotifications deletes in-app notifications older than the retention setting.
func (s *Service) PruneNotifications(ctx context.Context) error {
	days, err := notificationRetentionDays.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	return s.repo.pruneNotifications(ctx, s.now().AddDate(0, 0, -days))
}

func (s *Service) NotificationURLs(ctx context.Context, viewer domain.Principal) ([]NotificationURL, error) {
	rows, err := s.repo.notificationURLs(ctx, viewer.User)
	if err != nil {
		return nil, err
	}
	return convert(rows, notificationURLOf)
}

func (s *Service) AddNotificationURL(ctx context.Context, viewer domain.Principal, in NewNotificationURL) (NotificationURL, error) {
	raw := strings.TrimSpace(in.URL)
	if err := s.sender.Validate(raw); err != nil {
		return NotificationURL{}, err
	}
	kinds, err := allowedKinds(viewer, in.Kinds)
	if err != nil {
		return NotificationURL{}, err
	}
	row := notificationURLRow{
		ID: string(domain.NewID[notificationURLID]()), UserID: string(viewer.User), URL: raw,
		Label: labelOr(in.Label, raw), CreatedAt: s.now().UTC(),
	}
	if err := row.setKinds(kinds); err != nil {
		return NotificationURL{}, err
	}
	if err := s.repo.addNotificationURL(ctx, &row, MaxNotificationURLs); err != nil {
		return NotificationURL{}, err
	}
	return notificationURLOf(row)
}

type notificationURLID string

func (s *Service) UpdateNotificationURL(ctx context.Context, viewer domain.Principal, id string, change NotificationURLChange) (NotificationURL, error) {
	row, err := s.ownURL(ctx, viewer, id)
	if err != nil {
		return NotificationURL{}, err
	}
	if change.Label != nil {
		row.Label = labelOr(*change.Label, row.URL)
	}
	if change.Kinds != nil {
		kinds, err := allowedKinds(viewer, change.Kinds)
		if err != nil {
			return NotificationURL{}, err
		}
		if err := row.setKinds(kinds); err != nil {
			return NotificationURL{}, err
		}
	}
	if err := s.repo.saveNotificationURL(ctx, &row); err != nil {
		return NotificationURL{}, err
	}
	return notificationURLOf(row)
}

func (s *Service) DeleteNotificationURL(ctx context.Context, viewer domain.Principal, id string) error {
	if _, err := s.ownURL(ctx, viewer, id); err != nil {
		return err
	}
	return s.repo.deleteNotificationURL(ctx, id)
}

// TestNotificationURL sends a test message now and records the outcome on the URL.
func (s *Service) TestNotificationURL(ctx context.Context, viewer domain.Principal, id string) error {
	row, err := s.ownURL(ctx, viewer, id)
	if err != nil {
		return err
	}
	sendErr := s.sender.Send(ctx, row.URL, testMessage)
	if sendErr == nil {
		return s.repo.recordSent(ctx, row.ID, s.now())
	}
	if err := s.repo.recordFailure(ctx, row.ID, sendErr.Error()); err != nil {
		return err
	}
	return sendErr
}

// ownURL returns the viewer's URL. Someone else's URL is ErrURLNotFound, as if it didn't exist.
func (s *Service) ownURL(ctx context.Context, viewer domain.Principal, id string) (notificationURLRow, error) {
	row, err := s.repo.notificationURL(ctx, id)
	if err == nil && row.UserID != string(viewer.User) {
		return notificationURLRow{}, ErrURLNotFound
	}
	return row, err
}

// allowedKinds dedupes kinds into catalogue order, or fails with ErrKindNotAllowed if the viewer's role
// may not subscribe to one of them.
func allowedKinds(viewer domain.Principal, kinds []string) ([]string, error) {
	for _, kind := range kinds {
		if !mayHold(viewer.Admin, kind) {
			return nil, ErrKindNotAllowed
		}
	}
	allowed := []string{}
	for _, spec := range kindSpecs {
		if slices.Contains(kinds, spec.Kind) {
			allowed = append(allowed, spec.Kind)
		}
	}
	return allowed, nil
}

// labelOr is the trimmed label, or the URL's service name when it is blank.
func labelOr(label, raw string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		scheme, _, _ := strings.Cut(raw, schemeSuffix)
		label = capitalize(scheme)
	}
	if runes := []rune(label); len(runes) > maxLabelLength {
		label = string(runes[:maxLabelLength])
	}
	return label
}

// mask shows only the URL's scheme and host, which is enough to tell URLs apart without their tokens.
func mask(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return maskedSuffix
	}
	return parsed.Scheme + schemeSuffix + parsed.Hostname() + maskedSuffix
}

func notificationURLOf(row notificationURLRow) (NotificationURL, error) {
	kinds, err := row.kinds()
	return NotificationURL{
		ID: row.ID, Label: row.Label, Masked: mask(row.URL), Kinds: kinds,
		CreatedAt: row.CreatedAt, LastSentAt: row.LastSentAt, LastError: row.LastError,
	}, err
}
