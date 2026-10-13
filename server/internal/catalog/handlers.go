package catalog

import (
	"context"

	acquisitionapi "magpie/internal/acquisition/api"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/jobs"
	requestsapi "magpie/internal/requests/api"

	"gorm.io/gorm"
)

const (
	handlerRequestApproved = "catalog.add-title-on-request-approved"
	handlerTmdbHealth      = "catalog.resume-refresh-on-tmdb-healthy"
	handlerFolderFound     = "catalog.match-on-folder-found"
	handlerProfileDeleted  = "catalog.reassign-titles-on-profile-deleted"
)

// newFolderFoundHandler queues matching a folder a library scan found; the match calls TMDB.
func newFolderFoundHandler(queue jobs.Enqueuer) bus.Handler {
	return bus.Handler{
		Name:       handlerFolderFound,
		EventTypes: []string{libraryapi.TypeFolderFound},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			found, err := bus.Decode[libraryapi.FolderFound](ev)
			if err != nil {
				return err
			}
			return queue.EnqueueTx(tx, newMatchFolderTask(found))
		},
	}
}

// newRequestApprovedHandler monitors a tracked Title at once, or queues the TMDB fetch: handlers can't call
// TMDB inside their transaction.
func newRequestApprovedHandler(s *Service, queue jobs.Enqueuer) bus.Handler {
	return bus.Handler{
		Name:       handlerRequestApproved,
		EventTypes: []string{requestsapi.TypeRequestApproved},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			req, err := bus.Decode[requestsapi.RequestApproved](ev)
			if err != nil {
				return err
			}
			handled, err := s.MonitorTracked(tx, req)
			if err != nil || handled {
				return err
			}
			return queue.EnqueueTx(tx, addTitleTask{Request: req, Retries: addTitleRetries})
		},
	}
}

// newProfileDeletedHandler moves a deleted profile's titles to the profile Acquisition named in its place.
func newProfileDeletedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerProfileDeleted,
		EventTypes: []string{acquisitionapi.TypeProfileDeleted},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			deleted, err := bus.Decode[acquisitionapi.ProfileDeleted](ev)
			if err != nil {
				return err
			}
			return s.ReplaceProfile(tx, deleted)
		},
	}
}

// newTmdbHealthHandler resumes metadata refresh once the platform sees a working TMDB key.
func newTmdbHealthHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerTmdbHealth,
		EventTypes: []string{platformapi.TypeServiceHealthChanged},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			health, err := bus.Decode[platformapi.ServiceHealthChanged](ev)
			if err != nil || health.Service != platformapi.ServiceTMDB || !health.Healthy {
				return err
			}
			return s.ResumeRefresh(tx)
		},
	}
}
