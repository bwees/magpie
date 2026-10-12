package contracttest

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const ImporterImportTimeline scripted.Method = "activity.Importer.ImportTimeline"

// ImporterFactory returns an empty Activity whose Catalog knows title.
type ImporterFactory func(t *testing.T, title domain.TitleID) api.Importer

var importedHistoryAt = time.Date(2023, 6, 1, 20, 0, 0, 0, time.UTC)

// RunImporter is the activity.Importer contract: every history row can be imported again.
func RunImporter(t *testing.T, factory ImporterFactory) {
	t.Run("history rows can be imported again", func(t *testing.T) {
		title := domain.NewID[domain.TitleID]()
		importer := factory(t, title)
		rows := []api.ImportTimelineEvent{
			{Source: "radarr:history:1", Title: title, Kind: api.ImportGrabbed, ReleaseName: "Arrival.2016.1080p-GRP", At: importedHistoryAt},
			{Source: "radarr:history:2", Title: title, Kind: api.ImportImported, ReleaseName: "Arrival.2016.1080p-GRP", At: importedHistoryAt.Add(time.Hour)},
		}
		for range 2 {
			if err := importer.ImportTimeline(context.Background(), rows); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// ImporterFake is the in-memory activity.Importer the migration job tests against. It passes RunImporter.
type ImporterFake struct {
	scripted.Script

	mu   sync.Mutex
	rows []api.ImportTimelineEvent
}

var _ api.Importer = (*ImporterFake)(nil)

func NewImporterFake() *ImporterFake {
	return &ImporterFake{}
}

func (f *ImporterFake) ImportTimeline(ctx context.Context, in []api.ImportTimelineEvent) error {
	if err := f.Enter(ctx, ImporterImportTimeline); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range in {
		if !slices.ContainsFunc(f.rows, func(r api.ImportTimelineEvent) bool { return r.Source == row.Source }) {
			f.rows = append(f.rows, row)
		}
	}
	return nil
}

// Rows lists the imported history rows, one per source.
func (f *ImporterFake) Rows() []api.ImportTimelineEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.rows)
}
