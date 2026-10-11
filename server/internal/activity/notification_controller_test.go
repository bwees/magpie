package activity

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"magpie/internal/activity/api"
)

const urlsPath = "/api/activity/notification-urls"

func call[T any](t *testing.T, handler http.Handler, method, path string, body any, wantStatus int) T {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var out T
	if rec.Code != wantStatus {
		t.Fatalf("%s %s = %d %s, want %d", method, path, rec.Code, rec.Body, wantStatus)
	}
	if rec.Code < http.StatusMultipleChoices && rec.Code != http.StatusNoContent {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// Seed 12.
func TestRequesterCannotSubscribeToAdminKinds(t *testing.T) {
	h := newHarness(t)
	user := h.requester("alice")
	me := h.serve(&user)

	call[any](t, me, http.MethodPost, urlsPath, NotificationURLRequest{URL: ntfyURL, Kinds: []string{api.KindImportFailed}}, http.StatusUnprocessableEntity)

	added := call[NotificationURLResponse](t, me, http.MethodPost, urlsPath, NotificationURLRequest{URL: ntfyURL, Kinds: []string{api.KindRequestAvailable}}, http.StatusCreated)
	call[any](t, me, http.MethodPatch, urlsPath+"/"+added.ID, NotificationURLChangeRequest{Kinds: []string{api.KindGrabbed}}, http.StatusUnprocessableEntity)
}

// Seed 15.
func TestInvalidURLIsRejectedAndNotStored(t *testing.T) {
	h := newHarness(t)
	user := h.requester("alice")
	me := h.serve(&user)

	call[any](t, me, http.MethodPost, urlsPath, NotificationURLRequest{URL: "gopher://nowhere"}, http.StatusUnprocessableEntity)

	if urls := get[[]NotificationURLResponse](t, me, urlsPath, http.StatusOK); len(urls) != 0 {
		t.Errorf("urls = %+v, want none stored", urls)
	}
}

// Seed 16.
func TestURLsAreMaskedAndOnlyTheirOwnerSeesThem(t *testing.T) {
	h := newHarness(t)
	owner, other := h.requester("alice"), h.admin("admin")
	me, them := h.serve(&owner), h.serve(&other)
	added := call[NotificationURLResponse](t, me, http.MethodPost, urlsPath, NotificationURLRequest{Label: "Phone", URL: ntfyURL}, http.StatusCreated)

	urls := get[[]NotificationURLResponse](t, me, urlsPath, http.StatusOK)
	if len(urls) != 1 || urls[0].MaskedURL != "ntfy://ntfy.sh…" || urls[0].Label != "Phone" {
		t.Fatalf("urls = %+v, want the masked URL", urls)
	}
	if strings.Contains(urls[0].MaskedURL, "secret") || strings.Contains(added.MaskedURL, "secret") {
		t.Errorf("the token leaked: %+v", urls[0])
	}

	if theirs := get[[]NotificationURLResponse](t, them, urlsPath, http.StatusOK); len(theirs) != 0 {
		t.Errorf("another user listed %+v", theirs)
	}
	path := urlsPath + "/" + added.ID
	call[any](t, them, http.MethodPatch, path, NotificationURLChangeRequest{Kinds: []string{}}, http.StatusNotFound)
	call[any](t, them, http.MethodPost, path+"/test", nil, http.StatusNotFound)
	call[any](t, them, http.MethodDelete, path, nil, http.StatusNotFound)

	call[any](t, me, http.MethodDelete, path, nil, http.StatusNoContent)
	if urls := get[[]NotificationURLResponse](t, me, urlsPath, http.StatusOK); len(urls) != 0 {
		t.Errorf("urls = %+v after delete", urls)
	}
}

func TestEditingAURLKeepsWhatIsLeftOut(t *testing.T) {
	h := newHarness(t)
	admin := h.admin("admin")
	me := h.serve(&admin)
	added := call[NotificationURLResponse](t, me, http.MethodPost, urlsPath,
		NotificationURLRequest{URL: ntfyURL, Kinds: []string{api.KindGrabbed, api.KindImportFailed, api.KindGrabbed}}, http.StatusCreated)
	if !slices.Equal(added.Kinds, []string{api.KindImportFailed, api.KindGrabbed}) || added.Label != "Ntfy" {
		t.Fatalf("added = %+v, want deduped kinds in catalogue order and the service as label", added)
	}

	label := "Desk"
	renamed := call[NotificationURLResponse](t, me, http.MethodPatch, urlsPath+"/"+added.ID, NotificationURLChangeRequest{Label: &label}, http.StatusOK)
	if renamed.Label != label || !slices.Equal(renamed.Kinds, added.Kinds) {
		t.Errorf("renamed = %+v, want the new label and the same kinds", renamed)
	}
	cleared := call[NotificationURLResponse](t, me, http.MethodPatch, urlsPath+"/"+added.ID, NotificationURLChangeRequest{Kinds: []string{}}, http.StatusOK)
	if len(cleared.Kinds) != 0 || cleared.Label != label {
		t.Errorf("cleared = %+v, want no kinds", cleared)
	}
}

func TestSendTestReportsTheOutcome(t *testing.T) {
	h := newHarness(t)
	user := h.requester("alice")
	me := h.serve(&user)
	added := call[NotificationURLResponse](t, me, http.MethodPost, urlsPath, NotificationURLRequest{URL: ntfyURL}, http.StatusCreated)
	path := urlsPath + "/" + added.ID + "/test"

	call[any](t, me, http.MethodPost, path, nil, http.StatusNoContent)
	if sent := h.sender.messages(); len(sent) != 1 || sent[0] != testMessage {
		t.Errorf("sent = %+v, want the test message", sent)
	}

	h.sender.failNext(ErrRejected)
	call[any](t, me, http.MethodPost, path, nil, http.StatusUnprocessableEntity)
	h.sender.failNext(ErrUnavailable)
	call[any](t, me, http.MethodPost, path, nil, http.StatusBadGateway)
	if urls := get[[]NotificationURLResponse](t, me, urlsPath, http.StatusOK); urls[0].LastError == "" {
		t.Errorf("url = %+v, want the failure stored", urls[0])
	}
}

func TestNotificationKindsFollowTheRole(t *testing.T) {
	h := newHarness(t)
	admin, user := h.admin("admin"), h.requester("alice")

	requesterKinds := get[[]NotificationKindResponse](t, h.serve(&user), "/api/activity/notification-kinds", http.StatusOK)
	for _, kind := range requesterKinds {
		if kind.Scope != string(ScopeRequester) || kind.OptIn {
			t.Errorf("requester may subscribe to %+v", kind)
		}
	}
	adminKinds := get[[]NotificationKindResponse](t, h.serve(&admin), "/api/activity/notification-kinds", http.StatusOK)
	if len(adminKinds) != len(kindSpecs) || len(requesterKinds) == 0 || len(requesterKinds) >= len(adminKinds) {
		t.Errorf("admin kinds %d, requester kinds %d", len(adminKinds), len(requesterKinds))
	}
}

func TestNotificationsEndpoints(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.requester("alice"), h.requester("bob")
	h.deliver(h.approvedRequest(alice, h.movie()))
	h.deliver(h.approvedRequest(alice, h.movie()))
	me := h.serve(&alice)

	list := get[NotificationListResponse](t, me, "/api/activity/notifications", http.StatusOK)
	if len(list.Notifications) != 2 || list.UnreadCount != 2 || !list.Notifications[0].CreatedAt.After(list.Notifications[1].CreatedAt) && list.Notifications[0].CreatedAt != list.Notifications[1].CreatedAt {
		t.Fatalf("list = %+v, want 2 unread, newest first", list)
	}
	first := list.Notifications[0].ID
	call[any](t, h.serve(&bob), http.MethodPost, "/api/activity/notifications/"+first+"/read", nil, http.StatusNotFound)
	call[any](t, me, http.MethodPost, "/api/activity/notifications/"+first+"/read", nil, http.StatusNoContent)

	unread := get[NotificationListResponse](t, me, "/api/activity/notifications?unread=true", http.StatusOK)
	if len(unread.Notifications) != 1 || unread.UnreadCount != 1 {
		t.Errorf("unread = %+v, want 1", unread)
	}
	call[any](t, me, http.MethodPost, "/api/activity/notifications/read-all", nil, http.StatusNoContent)
	if after := get[NotificationListResponse](t, me, "/api/activity/notifications", http.StatusOK); after.UnreadCount != 0 {
		t.Errorf("unread count = %d after read-all", after.UnreadCount)
	}
	get[any](t, h.serve(nil), "/api/activity/notifications", http.StatusUnauthorized)
}
