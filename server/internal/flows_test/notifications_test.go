package flows_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity"
	activityapi "magpie/internal/activity/api"
	activityfakes "magpie/internal/activity/fakes"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	notificationsPath    = "/api/activity/notifications"
	notificationURLsPath = "/api/activity/notification-urls"
	flowNtfyURL          = "ntfy://ntfy.sh/magpie-flow"
)

// An admin subscribed to import failures hears about one in-app and on their URL, through the send job.
func TestImportFailureReachesTheBellAndTheAdminsURL(t *testing.T) {
	sender := activityfakes.NewNotificationSender()
	h := boot(t, fx.Decorate(func(activity.NotificationSender) activity.NotificationSender { return sender }))
	h.signInAsAdmin()

	rec := h.do(http.MethodPost, notificationURLsPath, activity.NotificationURLRequest{URL: flowNtfyURL, Kinds: []string{activityapi.KindImportFailed}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add URL = %d %s", rec.Code, rec.Body)
	}

	download := domain.NewID[domain.DownloadID]()
	err := h.DB.Write(context.Background(), func(tx *gorm.DB) error {
		if err := h.Bus.Publish(tx, bus.EventOf(acquisitionapi.ReleaseGrabbed{Download: download, Title: domain.NewID[domain.TitleID]()})); err != nil {
			return err
		}
		return h.Bus.Publish(tx, bus.EventOf(libraryapi.ImportFailed{Download: download, Reason: "no video file"}))
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(waitForHandler)
	for time.Now().Before(deadline) {
		var list activity.NotificationListResponse
		rec := h.get(notificationsPath)
		if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
			t.Fatal(err)
		}
		if list.UnreadCount == 1 && len(sender.Sent()) == 1 {
			if list.Notifications[0].Kind != activityapi.KindImportFailed || sender.Sent()[0].URL != flowNtfyURL {
				t.Errorf("notification %+v, sent %+v", list.Notifications[0], sender.Sent())
			}
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatalf("the import failure never reached the bell and the URL; sent %+v", sender.Sent())
}
