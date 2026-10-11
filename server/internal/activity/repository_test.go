package activity

import (
	"context"
	"slices"
	"testing"

	"magpie/internal/activity/api"
	"magpie/internal/activity/api/contracttest"
	"magpie/internal/domain"
	"magpie/internal/platform/db/dbtest"

	"gorm.io/gorm"
)

// Seed 21: the real repository passes the same contract suite as the fake.
func TestRepositoryPassesTheReaderContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, Migrations())
		repo := NewRepository(d)
		if err := d.Write(context.Background(), func(tx *gorm.DB) error { return seedFixture(tx, f) }); err != nil {
			t.Fatal(err)
		}
		return repo
	})
}

func TestFakePassesTheReaderContract(t *testing.T) {
	contracttest.Run(t, func(_ *testing.T, f contracttest.Fixture) api.Reader { return contracttest.NewFake(f) })
}

func seedFixture(tx *gorm.DB, f contracttest.Fixture) error {
	var rows []any
	for _, e := range f.Timeline {
		row, err := newTimelineRow(e.Title, e.Entry)
		if err != nil {
			return err
		}
		rows = append(rows, &row)
	}
	for _, e := range f.History {
		row, err := newHistoryRow(e)
		if err != nil {
			return err
		}
		rows = append(rows, &row)
	}
	for _, p := range f.Pipeline {
		row, err := newPipelineRowFromAPI(p.Row, p.Season, p.Number)
		if err != nil {
			return err
		}
		rows = append(rows, &row)
	}
	for _, n := range f.Notifications {
		row := newNotificationRow(n.User, n.Notification)
		rows = append(rows, &row)
	}
	for _, item := range f.Attention {
		row := newAttentionRowFromAPI(item)
		rows = append(rows, &row)
	}
	for _, row := range rows {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
	}
	return nil
}

func newPipelineRowFromAPI(r api.PipelineRow, season, number int) (pipelineRow, error) {
	row := pipelineRow{
		TitleID: string(r.Title), Season: season, Number: number, Wanted: true,
		File: fileNone, LastEventID: string(r.Title), UpdatedAt: r.Updated,
	}
	if r.Episode != nil {
		row.EpisodeID = string(*r.Episode)
	}
	err := row.setFacets(facets{Wanted: true, File: fileNone, Subs: r.Subs, Transcode: r.Transcode})
	row.Stage, row.Failed, row.Detail = r.Stage, r.Failed, r.Detail
	return row, err
}

func newNotificationRow(user domain.UserID, n api.Notification) notificationRow {
	row := notificationRow{
		ID: n.ID, UserID: string(user), EventID: n.ID, Kind: n.Kind, Title: n.Title,
		Body: n.Body, Link: n.Link, CreatedAt: n.CreatedAt.UTC(),
	}
	if n.Read {
		readAt := n.CreatedAt.UTC()
		row.ReadAt = &readAt
	}
	return row
}

func newAttentionRowFromAPI(item api.AttentionItem) attentionRow {
	row := attentionRow{
		ID: item.ID, Kind: item.Kind, Ref: item.Ref, Text: item.Text, Action: item.Action,
		OpenedAt: item.OpenedAt.UTC(), OpenedEventID: item.ID,
	}
	if item.Title != nil {
		title := string(*item.Title)
		row.TitleID = &title
	}
	return row
}

func TestHandlersSubscribeToEveryDurableEvent(t *testing.T) {
	want := DurableEventTypes()
	if len(want) == 0 {
		t.Fatal("no durable event types")
	}
	for _, excluded := range []string{api.TypeAttentionChanged, api.TypeNotificationCreated} {
		if slices.Contains(want, excluded) {
			t.Errorf("Activity subscribes to its own %s", excluded)
		}
	}
	h := newHarness(t)
	for _, handler := range h.handlers {
		if !slices.Equal(handler.EventTypes, want) {
			t.Errorf("%s subscribes to %d types, want all %d durable events", handler.Name, len(handler.EventTypes), len(want))
		}
	}
}
