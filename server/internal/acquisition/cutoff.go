package acquisition

import (
	"context"
	"errors"
	"time"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

type CutoffDeps struct {
	fx.In

	DB      *db.DB
	Repo    *Repository
	Catalog catalogapi.Reader
	Bus     *bus.Bus
}

// CutoffService keeps Acquisition's scores of library files and says when a file is good enough. A file
// that first meets its profile's cutoff is published as CutoffMet, so Transcode can queue it.
type CutoffService struct {
	deps CutoffDeps
	now  func() time.Time
}

func NewCutoffService(deps CutoffDeps) *CutoffService {
	return &CutoffService{deps: deps, now: time.Now}
}

// fileScorer scores the files of one title under its profile.
type fileScorer struct {
	title   catalogapi.Title
	profile ScoringProfile
}

// scorerFor reports false when the title is gone.
func scorerFor(ctx context.Context, tx *gorm.DB, catalog catalogapi.Reader, id domain.TitleID) (fileScorer, bool, error) {
	title, err := catalog.Title(ctx, id)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return fileScorer{}, false, nil
	}
	if err != nil {
		return fileScorer{}, false, err
	}
	profile, err := loadTitleProfile(ctx, tx, title)
	if err != nil {
		return fileScorer{}, false, err
	}
	return fileScorer{title: title, profile: profile}, true, nil
}

// score evaluates a file again under the scorer's profile; what it was scored from stays.
func (s fileScorer) score(f FileScore) FileScore {
	media := libraryapi.MediaFile{ReleaseName: f.ReleaseName, Path: f.Path, Quality: f.Quality, Size: f.SizeBytes}
	eval := scoreFile(media, s.title, s.profile).Eval
	f.Quality, f.CFScore, f.CutoffMet = eval.Quality, eval.Score, eval.CutoffMet
	f.Profile = s.profile.Profile().ID
	return f
}

// RecordImport scores an imported file and publishes CutoffMet when it meets the cutoff. A redelivered
// import keeps what is known about the file, so CutoffMet goes out once.
func (s *CutoffService) RecordImport(ctx context.Context, tx *gorm.DB, e libraryapi.FileImported) error {
	scorer, found, err := scorerFor(ctx, tx, s.deps.Catalog, e.Title)
	if err != nil || !found {
		return err
	}
	before, _, err := loadFileScore(tx, e.MediaFile)
	if err != nil {
		return err
	}
	episodes := make([]domain.EpisodeID, 0, len(e.Episodes))
	for _, ref := range e.Episodes {
		episodes = append(episodes, ref.Episode)
	}
	after := scorer.score(FileScore{
		File: e.MediaFile, Title: e.Title, Episodes: episodes, Quality: e.Quality, ReleaseName: e.ReleaseName,
		Path: e.Path, SizeBytes: e.Size, Transcoded: before.Transcoded, Origin: e.Origin,
	})
	return s.save(tx, before, after)
}

// RecordTranscode stops upgrades for a transcoded file. Its score stays the original's.
func (s *CutoffService) RecordTranscode(tx *gorm.DB, e libraryapi.FileTranscoded) error {
	return markFileTranscoded(tx, e.MediaFile)
}

// Forget drops a deleted file's score, whatever the reason.
func (s *CutoffService) Forget(tx *gorm.DB, e libraryapi.FileDeleted) error {
	return deleteFileScore(tx, e.MediaFile)
}

// RescoreTitle evaluates a title's files against its current profile, publishing CutoffMet for files that
// newly meet it.
func (s *CutoffService) RescoreTitle(ctx context.Context, tx *gorm.DB, title domain.TitleID) error {
	scores, err := titleFileScores(tx, title)
	if err != nil || len(scores) == 0 {
		return err
	}
	scorer, found, err := scorerFor(ctx, tx, s.deps.Catalog, title)
	if err != nil || !found {
		return err
	}
	for _, before := range scores {
		if err := s.save(tx, before, scorer.score(before)); err != nil {
			return err
		}
	}
	return nil
}

// Rescore evaluates again every file scored under profile, or every file when profile is empty, one title
// per transaction.
func (s *CutoffService) Rescore(ctx context.Context, profile domain.ProfileID) error {
	titles, err := scoredTitles(s.deps.DB.Read(ctx), profile)
	if err != nil {
		return err
	}
	for _, title := range titles {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.RescoreTitle(ctx, tx, title) })
		if err != nil {
			return err
		}
	}
	return nil
}

// save stores a file's score. A file that now meets the cutoff and didn't before is published, unless it
// was transcoded, which only happens after it met the cutoff.
func (s *CutoffService) save(tx *gorm.DB, before, after FileScore) error {
	if err := saveFileScore(tx, after, s.now().UTC()); err != nil {
		return err
	}
	if before.CutoffMet || !after.CutoffMet || after.Transcoded {
		return nil
	}
	return s.deps.Bus.Publish(tx, bus.EventOf(api.CutoffMet{
		File: after.File, Title: after.Title, Episodes: nonNil(after.Episodes), Quality: after.Quality,
		CFScore: after.CFScore, Origin: after.Origin,
	}))
}

// CutoffStatuses lists the cutoff state of a title's scored files for the title page.
func (s *CutoffService) CutoffStatuses(ctx context.Context, title domain.TitleID) ([]api.CutoffStatus, error) {
	return s.deps.Repo.CutoffStatuses(ctx, title)
}
