package activity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"
	transcodeapi "magpie/internal/transcode/api"

	"github.com/mikestefanello/backlite"
)

const (
	ntfyURL      = "ntfy://ntfy.sh/secret-topic-token"
	importReason = "no video file in the download"
)

func (h *harness) user(name string, role identityapi.Role) domain.Principal {
	id := domain.NewID[domain.UserID]()
	h.identity.Put(identityapi.User{ID: id, Username: name, Role: role})
	return domain.Principal{User: id, Admin: role == identityapi.RoleAdmin}
}

func (h *harness) admin(name string) domain.Principal {
	return h.user(name, identityapi.RoleAdmin)
}

func (h *harness) requester(name string) domain.Principal {
	return h.user(name, identityapi.RoleRequester)
}

func (h *harness) notifications(user domain.Principal) []api.Notification {
	h.t.Helper()
	notifications, _, err := h.service.Notifications(context.Background(), user.User, false, api.Page{})
	if err != nil {
		h.t.Fatal(err)
	}
	return notifications
}

func (h *harness) notificationsCreated() []api.NotificationCreated {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	var created []api.NotificationCreated
	for _, ev := range h.published {
		if ev.Type != api.TypeNotificationCreated {
			continue
		}
		n, err := bus.Decode[api.NotificationCreated](ev)
		if err != nil {
			h.t.Fatal(err)
		}
		created = append(created, n)
	}
	return created
}

// subscribe adds a URL for user subscribed to kinds.
func (h *harness) subscribe(user domain.Principal, kinds ...string) NotificationURL {
	h.t.Helper()
	added, err := h.service.AddNotificationURL(context.Background(), user, NewNotificationURL{URL: ntfyURL, Kinds: kinds})
	if err != nil {
		h.t.Fatal(err)
	}
	return added
}

// approvedRequest stores an approved request by user for a movie and returns the event that approved it.
func (h *harness) approvedRequest(user domain.Principal, title domain.TitleID) requestsapi.RequestApproved {
	request := domain.NewID[domain.RequestID]()
	h.requests.PutRequest(requestsapi.Request{
		ID: request, User: user.User, Kind: domain.MediaKindMovie, Title: &title, Status: requestsapi.StatusApproved,
	})
	return requestsapi.RequestApproved{Request: request, User: user.User, Kind: domain.MediaKindMovie}
}

func kindsOf(notifications []api.Notification) []string {
	kinds := make([]string, 0, len(notifications))
	for _, n := range notifications {
		kinds = append(kinds, n.Kind)
	}
	return kinds
}

// Seed 10.
func TestRequestApprovedNotifiesOnlyTheRequester(t *testing.T) {
	h := newHarness(t)
	admin, user := h.admin("admin"), h.requester("alice")
	title := h.movie()

	h.deliver(h.approvedRequest(user, title))

	got := h.notifications(user)
	if len(got) != 1 || got[0].Kind != api.KindRequestApproved || got[0].Title != movieName+" was approved" {
		t.Fatalf("requester notifications = %+v, want one REQUEST_APPROVED for %s", got, movieName)
	}
	if got[0].Link != titleLink(title) || got[0].Read {
		t.Errorf("notification = %+v, want an unread link to the title", got[0])
	}
	created := h.notificationsCreated()
	if len(created) != 1 || created[0].User != user.User || created[0].Notification != got[0].ID {
		t.Errorf("NotificationCreated = %+v, want one for the requester", created)
	}
	if n := h.notifications(admin); len(n) != 0 {
		t.Errorf("admin got %+v, want nothing", n)
	}
}

func TestRedeliveredEventNotifiesOnce(t *testing.T) {
	h := newHarness(t)
	user := h.requester("alice")
	ev := bus.EventOf(h.approvedRequest(user, h.movie()))

	h.deliverEvent(ev)
	h.deliverEvent(ev)

	if got := h.notifications(user); len(got) != 1 {
		t.Errorf("got %d notifications, want 1", len(got))
	}
	if created := h.notificationsCreated(); len(created) != 1 {
		t.Errorf("got %d NotificationCreated, want 1", len(created))
	}
}

// importFailed walks a download to a failed import for title.
func importFailed(h *harness, title domain.TitleID) {
	download := domain.NewID[domain.DownloadID]()
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: download, Title: title, Release: releaseName})
	h.deliver(libraryapi.ImportFailed{Download: download, Reason: importReason})
}

// Seed 11.
func TestImportFailedNotifiesEnabledAdminsAndTheirSubscribedURLs(t *testing.T) {
	h := newHarness(t)
	first, second, user := h.admin("first"), h.admin("second"), h.requester("alice")
	disabled := h.admin("disabled")
	h.identity.Put(identityapi.User{ID: disabled.User, Username: "disabled", Role: identityapi.RoleAdmin, Disabled: true})
	subscribed := h.subscribe(first, api.KindImportFailed)
	h.subscribe(second, api.KindDownloadFailed)
	title := h.movie()
	h.jobs.sends()

	importFailed(h, title)

	for _, admin := range []domain.Principal{first, second} {
		got := h.notifications(admin)
		if !slices.Equal(kindsOf(got), []string{api.KindImportFailed}) || got[0].Body != importReason {
			t.Errorf("admin notifications = %+v, want one IMPORT_FAILED with the reason", got)
		}
	}
	for _, nobody := range []domain.Principal{disabled, user} {
		if got := h.notifications(nobody); len(got) != 0 {
			t.Errorf("%s got %+v, want nothing", nobody.User, got)
		}
	}
	sends := h.jobs.sends()
	if len(sends) != 1 || sends[0].URL != subscribed.ID || sends[0].Kind != api.KindImportFailed {
		t.Fatalf("sends = %+v, want one to the subscribed URL", sends)
	}
	if sends[0].Message.Title != "Import failed: "+movieName {
		t.Errorf("message = %+v", sends[0].Message)
	}
}

// Seed 13.
func TestOptInKindsNeverGoInApp(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	title := h.movie()

	h.deliver(acquisitionapi.ReleaseGrabbed{Download: domain.NewID[domain.DownloadID](), Title: title, Release: releaseName})
	if got, sends := h.notifications(admin), h.jobs.sends(); len(got) != 0 || len(sends) != 0 {
		t.Fatalf("without a subscriber: notifications %+v, sends %+v; want none", got, sends)
	}

	h.subscribe(admin, api.KindGrabbed)
	h.deliver(acquisitionapi.ReleaseGrabbed{Download: domain.NewID[domain.DownloadID](), Title: title, Release: releaseName})
	if got := h.notifications(admin); len(got) != 0 {
		t.Errorf("notifications = %+v, want none in-app for an opt-in kind", got)
	}
	if sends := h.jobs.sends(); len(sends) != 1 || sends[0].Kind != api.KindGrabbed {
		t.Errorf("sends = %+v, want one GRABBED", sends)
	}
}

func TestBlockedFileRejectionIsOptIn(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	h.subscribe(admin, api.KindBlockedFileRejected)
	title := h.movie()
	blocked := "Dune/setup.exe"

	h.deliver(acquisitionapi.ReleaseRejected{Title: title, Release: releaseName, Reason: acquisitionapi.RejectionReason("STALLED")})
	h.deliver(acquisitionapi.ReleaseRejected{Title: title, Release: releaseName, Reason: acquisitionapi.RejectionReason("BLOCKED_FILE"), BlockedFile: &blocked})

	if sends := h.jobs.sends(); len(sends) != 1 || !strings.Contains(sends[0].Message.Body, blocked) {
		t.Errorf("sends = %+v, want one naming the blocked file", sends)
	}
}

// Seed 20.
func TestOnlyFinalTranscodeFailuresNotify(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	title := h.movie()
	file := availableMovie(h, title)

	h.deliver(transcodeapi.JobFailed{Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindTranscode, MediaFile: file, Reason: "encoder crashed"})
	if got := h.notifications(admin); len(got) != 0 {
		t.Fatalf("non-final failure notified: %+v", got)
	}
	if items := h.attention(); len(items) != 0 {
		t.Fatalf("non-final failure opened %+v", items)
	}

	h.deliver(transcodeapi.JobFailed{Job: domain.NewID[domain.JobID](), Kind: transcodeapi.JobKindTranscode, MediaFile: file, Reason: "encoder crashed", Final: true})
	got := h.notifications(admin)
	if !slices.Equal(kindsOf(got), []string{api.KindTranscodeFailed}) || got[0].Title != "Transcode failed: "+movieName {
		t.Errorf("notifications = %+v, want one TRANSCODE_FAILED naming the title", got)
	}
	if items := h.attention(); len(items) != 1 || items[0].Kind != api.AttentionTranscodeFailed {
		t.Errorf("attention = %+v, want one TRANSCODE_FAILED item", items)
	}
}

func TestRequesterURLsGetOnlyTheirOwnNotices(t *testing.T) {
	h := newHarness(t)
	admin, alice, bob := h.admin("admin"), h.requester("alice"), h.requester("bob")
	aliceURL := h.subscribe(alice, api.KindRequestApproved)
	h.subscribe(bob, api.KindRequestApproved)
	adminURL := h.subscribe(admin, api.KindRequestApproved)

	h.deliver(h.approvedRequest(alice, h.movie()))

	var urls []string
	for _, send := range h.jobs.sends() {
		urls = append(urls, send.URL)
	}
	slices.Sort(urls)
	want := []string{aliceURL.ID, adminURL.ID}
	slices.Sort(want)
	if !slices.Equal(urls, want) {
		t.Errorf("sends went to %v, want alice's URL and the admin's", urls)
	}
}

func TestIssueRepliesByTheReporterGoToAdmins(t *testing.T) {
	h := newHarness(t)
	admin, reporter := h.admin("admin"), h.requester("alice")
	issue := domain.NewID[domain.IssueID]()
	h.requests.PutIssue(requestsapi.Issue{ID: issue, User: reporter.User, Title: h.movie()})

	h.deliver(requestsapi.IssueCommented{Issue: issue, Author: reporter.User, Reporter: reporter.User})
	if got := h.notifications(admin); !slices.Equal(kindsOf(got), []string{api.KindIssueReply}) {
		t.Errorf("admin notifications = %+v, want the reporter's reply", got)
	}
	if got := h.notifications(reporter); len(got) != 0 {
		t.Errorf("reporter was told about their own reply: %+v", got)
	}

	h.deliver(requestsapi.IssueCommented{Issue: issue, Author: admin.User, Reporter: reporter.User})
	if got := h.notifications(reporter); !slices.Equal(kindsOf(got), []string{api.KindIssueReply}) {
		t.Errorf("reporter notifications = %+v, want the admin's reply", got)
	}
}

// deliverSends runs every queued send, and every retry it schedules, until none are left.
func (h *harness) deliverSends() {
	h.t.Helper()
	pending := h.jobs.sends()
	for len(pending) > 0 {
		task := pending[0]
		pending = pending[1:]
		later := func(_ context.Context, next backlite.Task, _ time.Duration) error {
			pending = append(pending, next.(sendTask))
			return nil
		}
		if err := h.service.Deliver(context.Background(), task, later); err != nil {
			h.t.Fatal(err)
		}
	}
}

func (h *harness) url(user domain.Principal, id string) NotificationURL {
	h.t.Helper()
	urls, err := h.service.NotificationURLs(context.Background(), user)
	if err != nil {
		h.t.Fatal(err)
	}
	i := slices.IndexFunc(urls, func(u NotificationURL) bool { return u.ID == id })
	if i < 0 {
		h.t.Fatalf("url %s not found", id)
	}
	return urls[i]
}

// Seed 14.
func TestUnavailableServiceIsRetriedThenItsErrorIsStored(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	subscribed := h.subscribe(admin, api.KindImportFailed)
	h.sender.failNext(ErrUnavailable, ErrUnavailable, ErrUnavailable)

	importFailed(h, h.movie())
	h.deliverSends()

	if sent := h.sender.messages(); len(sent) != 0 {
		t.Errorf("sent %+v after three failures, want nothing", sent)
	}
	got := h.url(admin, subscribed.ID)
	if !strings.Contains(got.LastError, ErrUnavailable.Error()) || got.LastSentAt != nil {
		t.Errorf("url = %+v, want the last error stored", got)
	}
	if n := h.notifications(admin); len(n) != 1 {
		t.Errorf("in-app notifications = %+v, want it kept", n)
	}
}

func TestDeliveryClearsTheLastError(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	subscribed := h.subscribe(admin, api.KindImportFailed)
	h.sender.failNext(ErrRejected)
	importFailed(h, h.movie())
	h.deliverSends()
	if got := h.url(admin, subscribed.ID); !strings.Contains(got.LastError, ErrRejected.Error()) {
		t.Fatalf("url = %+v, want a refused message stored at once", got)
	}

	importFailed(h, h.movie())
	h.deliverSends()

	got := h.url(admin, subscribed.ID)
	if got.LastError != "" || got.LastSentAt == nil || len(h.sender.messages()) != 1 {
		t.Errorf("url = %+v, want the error cleared after a delivery", got)
	}
}

func TestSendsToADemotedAdminAreSkipped(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	h.subscribe(admin, api.KindImportFailed)
	importFailed(h, h.movie())
	h.identity.Put(identityapi.User{ID: admin.User, Username: "admin", Role: identityapi.RoleRequester})

	h.deliverSends()

	if sent := h.sender.messages(); len(sent) != 0 {
		t.Errorf("sent %+v to a requester, want an admin-only kind skipped", sent)
	}
}

func TestNotificationURLRules(t *testing.T) {
	h := newHarness(t)
	admin, user := h.admin("admin"), h.requester("alice")

	if _, err := h.service.AddNotificationURL(context.Background(), user, NewNotificationURL{URL: ntfyURL, Kinds: []string{api.KindImportFailed}}); !errors.Is(err, ErrKindNotAllowed) {
		t.Errorf("requester subscribing to IMPORT_FAILED: err = %v", err)
	}
	if _, err := h.service.AddNotificationURL(context.Background(), user, NewNotificationURL{URL: "gopher://nowhere"}); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("invalid URL: err = %v", err)
	}
	for range MaxNotificationURLs {
		h.subscribe(admin)
	}
	if _, err := h.service.AddNotificationURL(context.Background(), admin, NewNotificationURL{URL: ntfyURL}); !errors.Is(err, ErrTooManyURLs) {
		t.Errorf("URL past the limit: err = %v", err)
	}
}

func TestNotificationsOlderThanTheRetentionArePruned(t *testing.T) {
	h := newHarness(t)
	user := h.requester("alice")
	old := bus.EventOf(h.approvedRequest(user, h.movie()))
	old.OccurredAt = ago(staleDays * day)
	h.deliverEvent(old)
	fresh := bus.EventOf(h.approvedRequest(user, h.movie()))
	fresh.OccurredAt = ago(freshDays * day)
	h.deliverEvent(fresh)

	if err := h.service.PruneNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := h.notifications(user); len(got) != 1 || !got[0].CreatedAt.Equal(fresh.OccurredAt) {
		t.Errorf("notifications = %+v, want only the fresh one", got)
	}
}

func TestMarkRead(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.requester("alice"), h.requester("bob")
	h.deliver(h.approvedRequest(alice, h.movie()))
	h.deliver(h.approvedRequest(alice, h.movie()))
	first := h.notifications(alice)[0]

	if err := h.service.MarkRead(context.Background(), bob.User, first.ID); !errors.Is(err, ErrNotificationNotFound) {
		t.Errorf("marking someone else's notification: err = %v", err)
	}
	if err := h.service.MarkRead(context.Background(), alice.User, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ := h.service.Notifications(context.Background(), alice.User, false, api.Page{}); unread != 1 {
		t.Errorf("unread = %d, want 1", unread)
	}
	if err := h.service.MarkAllRead(context.Background(), alice.User); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ := h.service.Notifications(context.Background(), alice.User, false, api.Page{}); unread != 0 {
		t.Errorf("unread = %d after read-all, want 0", unread)
	}
}
