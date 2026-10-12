package flows_test

import (
	"context"
	"testing"
	"time"

	activityapi "magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

func TestActivityProjectsPublishedEvents(t *testing.T) {
	var reader activityapi.Reader
	h := boot(t, fx.Populate(&reader))
	title := domain.NewID[domain.TitleID]()
	updates := make(chan activityapi.ProjectionUpdated, 1)
	unsubscribe := h.Bus.Subscribe(func(ev bus.Event) {
		if update, err := bus.Decode[activityapi.ProjectionUpdated](ev); err == nil && ev.Type == activityapi.TypeProjectionUpdated && update.Projection == activityapi.ProjectionPipeline {
			select {
			case updates <- update:
			default:
			}
		}
	})
	t.Cleanup(unsubscribe)

	err := h.DB.Write(context.Background(), func(tx *gorm.DB) error {
		return h.Bus.Publish(tx, bus.EventOf(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindMovie, Origin: domain.OriginManual}))
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case update := <-updates:
		if update.Title == nil || *update.Title != title {
			t.Errorf("update = %+v, want the title", update)
		}
	case <-time.After(waitForHandler):
		t.Fatal("no pipeline update announced")
	}
	deadline := time.Now().Add(waitForHandler)
	for time.Now().Before(deadline) {
		rows, err := reader.Pipeline(context.Background(), title)
		if err != nil {
			t.Fatal(err)
		}
		timeline, err := reader.Timeline(context.Background(), title, activityapi.Page{})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 1 && rows[0].Stage == activityapi.StageRequested && len(timeline) == 1 {
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatal("the title never reached the pipeline and timeline")
}
