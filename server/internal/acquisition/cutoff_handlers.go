package acquisition

import (
	"context"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	handlerFileScores = "acquisition.file-scores"

	rescoreQueue    = "acquisition.rescore-files"
	rescoreAttempts = 3
	rescoreBackoff  = time.Minute
)

// newFileScoresHandler keeps file_scores in one handler, so a title's imports, deletions and profile changes
// are applied in order.
func newFileScoresHandler(s *CutoffService) bus.Handler {
	return bus.Handler{
		Name: handlerFileScores,
		EventTypes: []string{
			libraryapi.TypeFileImported, libraryapi.TypeFileTranscoded, libraryapi.TypeFileDeleted,
			catalogapi.TypeProfileChanged,
		},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			switch ev.Type {
			case libraryapi.TypeFileImported:
				e, err := bus.Decode[libraryapi.FileImported](ev)
				if err != nil {
					return err
				}
				return s.RecordImport(ctx, tx, e)
			case libraryapi.TypeFileTranscoded:
				e, err := bus.Decode[libraryapi.FileTranscoded](ev)
				if err != nil {
					return err
				}
				return s.RecordTranscode(tx, e)
			case libraryapi.TypeFileDeleted:
				e, err := bus.Decode[libraryapi.FileDeleted](ev)
				if err != nil {
					return err
				}
				return s.Forget(tx, e)
			default:
				e, err := bus.Decode[catalogapi.ProfileChanged](ev)
				if err != nil {
					return err
				}
				return s.RescoreTitle(ctx, tx, e.Title)
			}
		},
	}
}

// rescoreTask scores files again after a profile or custom format changed. An empty Profile rescores every file.
type rescoreTask struct {
	Profile domain.ProfileID `json:"profile"`
}

func (rescoreTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: rescoreQueue, MaxAttempts: rescoreAttempts, Backoff: rescoreBackoff}
}

func newRescoreQueue(s *CutoffService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t rescoreTask) error {
		return s.Rescore(ctx, t.Profile)
	})
}
