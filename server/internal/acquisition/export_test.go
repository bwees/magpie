package acquisition

import (
	"context"
	"time"

	"magpie/internal/domain"
)

var NewIndexerSyncPeriodic = newIndexerSyncPeriodic

var (
	NewTitleAddedHandler        = newTitleAddedHandler
	NewMonitoringChangedHandler = newMonitoringChangedHandler
	NewWantedReleasedHandler    = newWantedReleasedHandler
	NewTitleRemovedHandler      = newTitleRemovedHandler
)

func SetDownloadClock(s *DownloadService, now func() time.Time) { s.now = now }

func SetSearchClock(s *SearchService, now func() time.Time) { s.now = now }

// DownloadCheckTarget is the download a queued check job acts on.
func DownloadCheckTarget(task any) (string, bool) {
	check, ok := task.(downloadCheckTask)
	return string(check.Download), ok
}

// SearchTaskOf is the search a queued search job starts.
func SearchTaskOf(task any) (SearchRequest, bool) {
	s, ok := task.(searchTask)
	return SearchRequest(s), ok
}

// SettleTaskOf is the search a queued automatic grab is for.
func SettleTaskOf(task any) (domain.SearchID, bool) {
	s, ok := task.(settleTask)
	return s.Search, ok
}

// TitleRemoval is a queued job removing a title's downloads.
type TitleRemoval = removeTitleTask

func TitleRemovalOf(task any) (TitleRemoval, bool) {
	r, ok := task.(removeTitleTask)
	return r, ok
}

// RunSearchJob runs a queued search as its job would.
func (s *SearchService) RunSearchJob(ctx context.Context, req SearchRequest) error {
	return s.runSearchJob(ctx, searchTask(req))
}
