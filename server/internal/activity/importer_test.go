package activity

import (
	"context"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	"magpie/internal/activity/api/contracttest"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

func (h *harness) importer() *Importer {
	return NewImporter(ServiceParams{Repository: h.repo, DB: h.db, Catalog: h.catalog}, h.service)
}

func TestImporterPassesTheContract(t *testing.T) {
	contracttest.RunImporter(t, func(t *testing.T, title domain.TitleID) api.Importer {
		h := newHarness(t)
		h.catalog.PutTitle(catalogapi.Title{ID: title, Kind: domain.MediaKindMovie, Name: movieName})
		return h.importer()
	})
}

func TestImportedHistoryLandsOnceWithItsOwnTimestampsAndNoNotifications(t *testing.T) {
	h := newHarness(t)
	title := h.movie()
	grabbedAt := ago(400 * 24 * time.Hour)
	failedAt := grabbedAt.Add(time.Hour)
	rows := []api.ImportTimelineEvent{
		{Source: "radarr:history:10", Title: title, Kind: api.ImportGrabbed, ReleaseName: "Dune.2021.1080p-GRP", At: grabbedAt},
		{Source: "radarr:history:11", Title: title, Kind: api.ImportDownloadFailed, ReleaseName: "Dune.2021.1080p-GRP", Reason: "stalled", At: failedAt},
	}
	for range 2 {
		if err := h.importer().ImportTimeline(context.Background(), rows); err != nil {
			t.Fatal(err)
		}
	}

	timeline := h.timeline(title)
	if len(timeline) != 2 {
		t.Fatalf("timeline = %+v, want 2 entries", timeline)
	}
	byType := map[string]api.TimelineEntry{}
	for _, e := range timeline {
		byType[e.Type] = e
	}
	grabbed, failed := byType[acquisitionapi.TypeReleaseGrabbed], byType[acquisitionapi.TypeDownloadFailed]
	if !grabbed.At.Equal(grabbedAt) || !failed.At.Equal(failedAt) || failed.Severity != api.SeverityError {
		t.Fatalf("timeline = %+v, want the rows at their own times with the failure as an error", timeline)
	}
	if history := h.history(); len(history) != 2 || history[0].TitleName != movieName {
		t.Fatalf("history = %+v, want 2 rows named %s", history, movieName)
	}
	if items := h.attention(); len(items) != 0 {
		t.Fatalf("attention = %+v, want none", items)
	}
	if sends := h.jobs.sends(); len(sends) != 0 {
		t.Fatalf("queued sends = %+v, want none", sends)
	}
}
