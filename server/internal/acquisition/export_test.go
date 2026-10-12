package acquisition

import "time"

var NewIndexerSyncPeriodic = newIndexerSyncPeriodic

func SetDownloadClock(s *DownloadService, now func() time.Time) { s.now = now }

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
