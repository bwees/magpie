package library

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"

	"magpie/internal/domain"
	"magpie/internal/platform/jobs"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	restampQueueName = "library.restamp-etags"
	// unstampedETag marks a file whose ETag a migration cleared because the formula changed.
	unstampedETag = ""
)

// restampTask stamps the files that have no ETag; the empty column says which still need it.
type restampTask struct{}

func (restampTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: restampQueueName, MaxAttempts: importQueueAttempts, Backoff: importQueueBackoff}
}

func newRestampQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ restampTask) error { return s.RestampETags(ctx) })
}

func restampOnStart(lc fx.Lifecycle, q jobs.Enqueuer) {
	lc.Append(fx.StartHook(func(ctx context.Context) error {
		return q.Enqueue(ctx, restampTask{})
	}))
}

// RestampETags gives each file without an ETag the one its file on disk has now. A file gone from disk
// keeps none, so a swap of it is rejected.
func (s *Service) RestampETags(ctx context.Context) error {
	files, err := s.deps.Repo.files(ctx, "etag = ?", unstampedETag)
	if err != nil || len(files) == 0 {
		return err
	}
	etags := make(map[domain.MediaFileID]string, len(files))
	for _, f := range files {
		info, err := s.deps.Files.Stat(f.Path)
		if errors.Is(err, fs.ErrNotExist) {
			slog.WarnContext(ctx, "library: a file to restamp is gone from disk", "path", f.Path)
			continue
		}
		if err != nil {
			return err
		}
		etags[f.ID] = etagOf(info)
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		for id, etag := range etags {
			err := tx.Model(&mediaFileRow{}).Where("id = ? AND etag = ?", id, unstampedETag).Update("etag", etag).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}
