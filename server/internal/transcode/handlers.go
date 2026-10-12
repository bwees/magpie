package transcode

import (
	"context"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

const (
	handlerCutoffMet    = "transcode.queue-on-cutoff-met"
	handlerFileSwapped  = "transcode.swapped-on-file-transcoded"
	handlerSwapRejected = "transcode.fail-on-swap-rejected"
	handlerFileDeleted  = "transcode.cancel-on-file-deleted"
	handlerTitleRemoved = "transcode.cancel-on-title-removed"
)

func handle[T any](name, eventType string, fn func(ctx context.Context, tx *gorm.DB, payload T) error) bus.Handler {
	return bus.Handler{
		Name:       name,
		EventTypes: []string{eventType},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			payload, err := bus.Decode[T](ev)
			if err != nil {
				return err
			}
			return fn(ctx, tx, payload)
		},
	}
}

func newCutoffMetHandler(s *JobService) bus.Handler {
	return handle(handlerCutoffMet, acquisitionapi.TypeCutoffMet, s.QueueOnCutoff)
}

func newFileTranscodedHandler(s *JobService) bus.Handler {
	return handle(handlerFileSwapped, libraryapi.TypeFileTranscoded, func(_ context.Context, tx *gorm.DB, e libraryapi.FileTranscoded) error {
		return s.Swapped(tx, e)
	})
}

func newSwapRejectedHandler(s *JobService) bus.Handler {
	return handle(handlerSwapRejected, libraryapi.TypeTranscodeSwapRejected, func(_ context.Context, tx *gorm.DB, e libraryapi.TranscodeSwapRejected) error {
		return s.SwapRejected(tx, e)
	})
}

func newFileDeletedHandler(s *JobService) bus.Handler {
	return handle(handlerFileDeleted, libraryapi.TypeFileDeleted, func(_ context.Context, tx *gorm.DB, e libraryapi.FileDeleted) error {
		reason := ReasonFileDeleted
		if e.Reason == libraryapi.DeleteUpgraded {
			reason = ReasonUpgraded
		}
		return s.CancelForFile(tx, e.MediaFile, reason)
	})
}

func newTitleRemovedHandler(s *JobService) bus.Handler {
	return handle(handlerTitleRemoved, catalogapi.TypeTitleRemoved, func(_ context.Context, tx *gorm.DB, e catalogapi.TitleRemoved) error {
		return s.CancelForTitle(tx, e.Title)
	})
}
