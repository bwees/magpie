package acquisition

import (
	"context"
	"slices"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"

	"gorm.io/gorm"
)

// wantedTitle is a monitored title with what it wants and the files it has.
type wantedTitle struct {
	title    catalogapi.Title
	episodes []catalogapi.Episode // empty for a movie
	files    []libraryapi.MediaFile
}

// sweepPick returns the wanted episodes to search, or for a movie whether to search it.
type sweepPick func(ctx context.Context, w wantedTitle) (episodes []catalogapi.Episode, movie bool, err error)

// SweepMissing queues an AUTO search for every wanted movie and episode without a file, one per season.
func (s *SearchService) SweepMissing(ctx context.Context) error {
	return s.sweep(ctx, func(_ context.Context, w wantedTitle) ([]catalogapi.Episode, bool, error) {
		withFile := map[domain.EpisodeID]bool{}
		for _, f := range w.files {
			for _, ref := range f.Episodes {
				withFile[ref.Episode] = true
			}
		}
		missing := slices.DeleteFunc(slices.Clone(w.episodes), func(ep catalogapi.Episode) bool { return withFile[ep.ID] })
		return missing, len(w.files) == 0, nil
	})
}

// SweepCutoffUnmet queues an AUTO search for every wanted item whose file is below its profile's cutoff and
// may still be upgraded, whether it was downloaded, scanned or migrated.
func (s *SearchService) SweepCutoffUnmet(ctx context.Context) error {
	return s.sweep(ctx, func(ctx context.Context, w wantedTitle) ([]catalogapi.Episode, bool, error) {
		below, err := upgradableFileScores(s.deps.DB.Read(ctx), w.title.ID)
		if err != nil {
			return nil, false, err
		}
		unmet := map[domain.EpisodeID]bool{}
		for _, f := range below {
			for _, ep := range f.Episodes {
				unmet[ep] = true
			}
		}
		episodes := slices.DeleteFunc(slices.Clone(w.episodes), func(ep catalogapi.Episode) bool { return !unmet[ep.ID] })
		return episodes, len(below) > 0, nil
	})
}

func (s *SearchService) sweep(ctx context.Context, pick sweepPick) error {
	monitored := true
	titles, err := s.deps.Catalog.Titles(ctx, catalogapi.TitleFilter{Monitored: &monitored})
	if err != nil {
		return err
	}
	tasks := []searchTask{}
	for _, t := range titles {
		episodes, wanted, err := s.deps.Catalog.Wanted(ctx, t.ID)
		if err != nil {
			return err
		}
		if !wanted {
			continue
		}
		files, err := s.deps.Library.FilesForTitle(ctx, t.ID)
		if err != nil {
			return err
		}
		picked, movie, err := pick(ctx, wantedTitle{title: t, episodes: episodes, files: files})
		if err != nil {
			return err
		}
		if t.Kind == domain.MediaKindSeries {
			tasks = append(tasks, seasonSearches(t.ID, picked)...)
		} else if movie {
			tasks = append(tasks, searchTask{Title: t.ID, Trigger: api.TriggerAuto})
		}
	}
	return s.queueSpread(ctx, tasks)
}

// seasonSearches batches episodes into one search per season.
func seasonSearches(title domain.TitleID, episodes []catalogapi.Episode) []searchTask {
	tasks := []searchTask{}
	seasons := []int{}
	for _, ep := range episodes {
		i := slices.Index(seasons, ep.Season)
		if i < 0 {
			seasons = append(seasons, ep.Season)
			tasks = append(tasks, searchTask{Title: title, Episodes: []domain.EpisodeID{}, Trigger: api.TriggerAuto})
			i = len(tasks) - 1
		}
		tasks[i].Episodes = append(tasks[i].Episodes, ep.ID)
	}
	return tasks
}

// queueSpread queues searches MinSweepGap apart, so no indexer gets more than one query per IndexerQueryGap.
func (s *SearchService) queueSpread(ctx context.Context, tasks []searchTask) error {
	start := s.now().UTC()
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		at := start
		for _, t := range tasks {
			if err := queueSearchAt(tx, s.deps.Jobs, t, at); err != nil {
				return err
			}
			at = at.Add(MinSweepGap)
		}
		return nil
	})
}

// profileFor is the title's profile, or the default one when it has none.
func (s *SearchService) profileFor(ctx context.Context, title catalogapi.Title) (ScoringProfile, error) {
	id := title.Profile
	if id == "" {
		var err error
		if id, err = DefaultProfileKey.Get(ctx, s.deps.Store); err != nil {
			return ScoringProfile{}, err
		}
	}
	return s.deps.Quality.ScoringProfile(ctx, id)
}
