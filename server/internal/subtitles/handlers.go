package subtitles

import (
	"context"

	catalogapi "magpie/internal/catalog/api"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"

	"gorm.io/gorm"
)

const (
	handlerLibrary          = "subtitles.track-library-files"
	handlerIssueAction      = "subtitles.rerun-on-issue-action"
	handlerLanguagesChanged = "subtitles.recompute-on-languages-changed"
)

// newLibraryHandler takes every library file event in one handler, so a file's import, transcode and
// deletion are handled in order.
func newLibraryHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerLibrary,
		EventTypes: []string{libraryapi.TypeFileImported, libraryapi.TypeFileTranscoded, libraryapi.TypeFileDeleted},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			switch ev.Type {
			case libraryapi.TypeFileImported:
				e, err := bus.Decode[libraryapi.FileImported](ev)
				if err != nil {
					return err
				}
				return s.recordImport(ctx, tx, e)
			case libraryapi.TypeFileTranscoded:
				e, err := bus.Decode[libraryapi.FileTranscoded](ev)
				if err != nil {
					return err
				}
				return s.recordTranscode(ctx, tx, e)
			default:
				e, err := bus.Decode[libraryapi.FileDeleted](ev)
				if err != nil {
					return err
				}
				return s.forgetFile(tx, e)
			}
		},
	}
}

func newIssueActionHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerIssueAction,
		EventTypes: []string{requestsapi.TypeIssueActionRequested},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			e, err := bus.Decode[requestsapi.IssueActionRequested](ev)
			if err != nil {
				return err
			}
			return s.rerun(ctx, tx, e)
		},
	}
}

// newLanguagesChangedHandler queues the recompute; it runs in batches as a job.
func newLanguagesChangedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerLanguagesChanged,
		EventTypes: []string{catalogapi.TypeLanguagesChanged},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			e, err := bus.Decode[catalogapi.LanguagesChanged](ev)
			if err != nil {
				return err
			}
			return s.deps.Jobs.EnqueueTx(tx, recomputeTask{Title: e.Title})
		},
	}
}
