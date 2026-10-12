package library

import (
	"context"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

const handlerDownloadCompleted = "library.import-on-download-completed"

// newDownloadCompletedHandler queues the import of a finished download. Handlers can't touch the disk
// inside their transaction, so the import runs as a job.
func newDownloadCompletedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerDownloadCompleted,
		EventTypes: []string{acquisitionapi.TypeDownloadCompleted},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			completed, err := bus.Decode[acquisitionapi.DownloadCompleted](ev)
			if err != nil {
				return err
			}
			return s.QueueImport(tx, completed)
		},
	}
}
