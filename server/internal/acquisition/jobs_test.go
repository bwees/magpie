package acquisition_test

import (
	"context"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"
)

func TestIndexerSyncScheduleFollowsSettings(t *testing.T) {
	f := newFixture(t)
	periodic := acquisition.NewIndexerSyncPeriodic(f.service)
	ctx := context.Background()

	schedule, err := periodic.Schedule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if schedule != jobs.Every(time.Hour) {
		t.Errorf("default schedule = %v, want every hour", schedule)
	}

	options := acquisition.IndexerOptionsKey.Default
	options.SyncInterval = settings.Duration(6 * time.Hour)
	if err := f.service.SetIndexerOptions(ctx, options); err != nil {
		t.Fatal(err)
	}
	if schedule, _ = periodic.Schedule(ctx); schedule != jobs.Every(6*time.Hour) {
		t.Errorf("schedule = %v, want every 6 h", schedule)
	}
}
