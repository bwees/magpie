package flows_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity"
	activityapi "magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/live"
	"magpie/internal/requests"
	requestsapi "magpie/internal/requests/api"
	subtitlesapi "magpie/internal/subtitles/api"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	availableTmdb    = domain.TmdbID(1001)
	availableName    = "Available Flow"
	availableRelease = "Available.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	availableItem    = "jf-available-flow"

	// failingHandlerName fails every FileImported it gets, next to the real handlers of that event.
	failingHandlerName = "flows.always-fails"
)

var errFlowHandler = errors.New("flow test handler always fails")

// flowOneEvents are flow 1's durable events after the approval, in the order they happen.
var flowOneEvents = []string{
	requestsapi.TypeRequestApproved,
	catalogapi.TypeTitleAdded,
	acquisitionapi.TypeSearchStarted,
	acquisitionapi.TypeReleaseGrabbed,
	acquisitionapi.TypeDownloadCompleted,
	libraryapi.TypeFileImported,
	libraryapi.TypeItemAvailable,
	requestsapi.TypeRequestAvailable,
}

func alwaysFailingHandler() fx.Option {
	return fx.Provide(bus.AsHandler(func() bus.Handler {
		return bus.Handler{
			Name:       failingHandlerName,
			EventTypes: []string{libraryapi.TypeFileImported},
			Handle:     func(context.Context, *gorm.DB, bus.Event) error { return errFlowHandler },
		}
	}))
}

// englishSubbedMovie carries English subtitles inside the file, so Subtitles settles without a provider.
func englishSubbedMovie() libraryapi.MediaInfo {
	info := h264Movie()
	info.Streams = append(info.Streams, libraryapi.Stream{Index: len(info.Streams), Kind: libraryapi.StreamSubtitle, Codec: "subrip", Language: english})
	return info
}

// Flow 1: alice's approved request is added, searched, grabbed, downloaded and imported; Jellyfin confirms
// the file, the request becomes AVAILABLE and alice hears about it. The admin's live socket sees every step
// in order, across a dropped connection, and a handler that fails on the import holds back no one else.
func TestRequestedMovieBecomesAvailable(t *testing.T) {
	f := newRequestFlow(t, alwaysFailingHandler())
	f.movieOnTMDB(availableTmdb, availableName, movieYear)
	item := release(availableRelease, releaseSeeders)
	f.answer(item)
	socket := f.openLive(f.admin)

	pending := f.request(f.alice, availableTmdb)
	if rec := f.as(f.admin, http.MethodPost, requestsPath+"/"+string(pending.ID)+"/approve", requests.ApproveRequestRequest{}); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body)
	}
	title := f.titleAdded(availableTmdb)
	grab := f.grabbed(item)
	socket.awaitType(acquisitionapi.TypeReleaseGrabbed)
	socket.drop()

	f.startDownload(item, videoOf(item))
	f.completeDownload(item, grab, englishSubbedMovie(), videoOf(item))
	imported := f.imported(grab.Download)
	socket.resume()
	f.jellyfinAdds(availableItem, imported.Path)

	available := func() []string { return f.availableProblems(pending.ID, title, imported) }
	settles(t, "the available request", available)
	f.assertLiveInOrder(socket)
	f.assertFailingHandlerHeldAlone()

	f.redeliverAll()
	settles(t, "the available request after redelivery", available)
}

func (f *requestFlow) availableProblems(request domain.RequestID, title domain.TitleID, imported libraryapi.FileImported) []string {
	f.t.Helper()
	var problems []string
	f.session = f.alice
	if got := getJSON[requests.RequestResponse](f.harness, requestsPath+"/"+string(request)); got.Status != requestsapi.StatusAvailable {
		problems = append(problems, fmt.Sprintf("request = %+v, want AVAILABLE", got))
	}
	list := getJSON[activity.NotificationListResponse](f.harness, notificationsPath)
	available := slices.DeleteFunc(list.Notifications, func(n activity.NotificationResponse) bool { return n.Kind != activityapi.KindRequestAvailable })
	if len(available) != 1 {
		problems = append(problems, fmt.Sprintf("alice's notifications = %+v, want one REQUEST_AVAILABLE", list.Notifications))
	}

	f.session = f.admin
	rows := getJSON[[]activity.PipelineRowResponse](f.harness, pipelinePath+titleQuery+string(title))
	if len(rows) != 1 || rows[0].Stage != string(activityapi.StageAvailable) || rows[0].Subs == nil || rows[0].Subs.State != activityapi.SubsSettled {
		problems = append(problems, fmt.Sprintf("pipeline = %+v, want one AVAILABLE row with subtitles settled", rows))
	}
	if !exists(imported.Path) || imported.Origin != domain.OriginDownload {
		problems = append(problems, fmt.Sprintf("imported %+v, on disk %v", imported, exists(imported.Path)))
	}
	if !slices.Contains(f.jellyfin.Refreshed(), filepath.Dir(imported.Path)) {
		problems = append(problems, fmt.Sprintf("Jellyfin refreshed %v, want the title's folder", f.jellyfin.Refreshed()))
	}
	for _, eventType := range flowOneEvents {
		if got := f.outbox(eventType); len(got) != 1 {
			problems = append(problems, fmt.Sprintf("%s published %d times, want once", eventType, len(got)))
		}
	}
	settled := published(f.harness, subtitlesapi.TypeCoverageSettled, func(e subtitlesapi.CoverageSettled) bool { return e.MediaFile == imported.MediaFile })
	cutoff := published(f.harness, acquisitionapi.TypeCutoffMet, func(e acquisitionapi.CutoffMet) bool {
		return e.File == imported.MediaFile && e.Origin == domain.OriginDownload
	})
	if len(settled) != 1 || !slices.Equal(settled[0].Satisfied, []domain.Language{english}) || len(cutoff) != 1 {
		problems = append(problems, fmt.Sprintf("coverage settled %+v, cutoff met %+v; want each once", settled, cutoff))
	}
	return problems
}

// assertLiveInOrder checks the socket got each flow event once, with the payload in the outbox, in flow
// order and with rising sequence numbers, including those sent while it was disconnected.
func (f *requestFlow) assertLiveInOrder(socket *liveClient) {
	f.t.Helper()
	t := f.t
	if resets := socket.resetsSeen(); len(resets) != 1 {
		t.Errorf("resets = %+v, want only the first connect's", resets)
	}
	received := socket.messages()
	for i := 1; i < len(received); i++ {
		if received[i].Seq <= received[i-1].Seq {
			t.Errorf("live seq %d came after %d", received[i].Seq, received[i-1].Seq)
		}
	}
	positions := make([]int, 0, len(flowOneEvents))
	for _, eventType := range flowOneEvents {
		onSocket := socket.ofType(eventType)
		stored := f.outbox(eventType)
		if len(onSocket) != len(stored) || len(onSocket) == 0 || !sameJSON(t, string(onSocket[0].Data), stored[0]) {
			t.Errorf("%s: live %d, stored %d, or the payload differs", eventType, len(onSocket), len(stored))
			continue
		}
		positions = append(positions, slices.IndexFunc(received, func(m live.LiveMessage) bool { return m.Type == eventType }))
	}
	if !slices.IsSorted(positions) {
		t.Errorf("flow events arrived at positions %v, want flow order", positions)
	}
}

func sameJSON(t *testing.T, a, b string) bool {
	t.Helper()
	var left, right any
	if err := json.Unmarshal([]byte(a), &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(b), &right); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(left, right)
}

// assertFailingHandlerHeldAlone checks the failing handler still retries the import while every other
// handler of it went on.
func (f *requestFlow) assertFailingHandlerHeldAlone() {
	f.t.Helper()
	var held []struct {
		Handler  string
		Attempts int
	}
	err := f.DB.Read(context.Background()).Table(handlerQueueTable).Select("handler, attempts").Find(&held).Error
	if err != nil {
		f.t.Fatal(err)
	}
	if len(held) != 1 || held[0].Handler != failingHandlerName || held[0].Attempts < 1 {
		f.t.Errorf("handler queue = %+v, want only the failing handler retrying", held)
	}
}
