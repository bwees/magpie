package library

import (
	"context"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"go.uber.org/fx"
)

type Deps struct {
	fx.In

	DB        *db.DB
	Repo      *Repository
	Catalog   catalogapi.Reader
	Files     FileSystem
	Prober    MediaProber
	Hasher    Hasher
	Downloads DownloadLocation
	Jellyfin  JellyfinLibrary
	Bus       *bus.Bus
	Jobs      jobs.Enqueuer
	Store     *settings.Store
}

// Service imports downloads into the library, keeps the root folders and serves the files of a title.
type Service struct {
	deps Deps
	now  func() time.Time
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps, now: time.Now}
}

// FilesForTitle lists a title's media files, newest first.
func (s *Service) FilesForTitle(ctx context.Context, title domain.TitleID) ([]api.MediaFile, error) {
	return s.deps.Repo.FilesForTitle(ctx, title)
}

// FilesForTitles lists the files of several titles for the library tables.
func (s *Service) FilesForTitles(ctx context.Context, titles []domain.TitleID, f api.FileFilter) ([]api.MediaFile, error) {
	return s.deps.Repo.FilesForTitles(ctx, titles, f)
}

// Coverage counts each title's episodes with a file and those available.
func (s *Service) Coverage(ctx context.Context, titles []domain.TitleID) ([]api.TitleCoverage, error) {
	return s.deps.Repo.Coverage(ctx, titles)
}

// File returns one media file, or api.ErrNotFound.
func (s *Service) File(ctx context.Context, id domain.MediaFileID) (api.MediaFile, error) {
	return s.deps.Repo.File(ctx, id)
}
