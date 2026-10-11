package catalog

import "time"

// Internals the catalog_test package drives directly.
var (
	AddTitleQueue             = addTitleQueue
	NewRefreshTitleQueue      = newRefreshTitleQueue
	NewMovieRefreshQueue      = newMovieRefreshQueue
	NewReleaseClockQueue      = newReleaseClockQueue
	NewRequestApprovedHandler = newRequestApprovedHandler
	NewTmdbHealthHandler      = newTmdbHealthHandler
	NewMovieRefreshPeriodic   = newMovieRefreshPeriodic
	AddTitleRetries           = addTitleRetries
	RefreshPausedSetting      = refreshPaused
)

type (
	ReleaseClockTask = releaseClockTask
	MovieRefreshTask = movieRefreshTask
)

func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}
