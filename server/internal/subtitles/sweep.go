package subtitles

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// sweep is the periodic pass. Its quota order: downloads waiting since the last reset (imports and manual
// requests first), then upgrades, then the missing backlog.
func (s *Service) sweep(ctx context.Context) error {
	if err := s.drainDownloads(ctx); err != nil {
		return err
	}
	if err := s.upgradeAll(ctx); err != nil {
		return err
	}
	due, err := dueFiles(s.deps.DB.Read(ctx), s.now())
	if err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		for _, file := range due {
			if err := s.queueSearch(tx, file, PriorityBacklog, time.Time{}); err != nil {
				return err
			}
		}
		return nil
	})
}

// drainDownloads downloads accepted candidates that waited for quota, in quota order. Once the quota runs
// out again the rest wait for the next reset.
func (s *Service) drainDownloads(ctx context.Context) error {
	s.drainLock.Lock()
	defer s.drainLock.Unlock()
	rows, err := waitingDownloads(s.deps.DB.Read(ctx), s.now())
	if err != nil {
		return err
	}
	cfg, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.drainOne(ctx, row, cfg)
		if until, retry := retryTime(err, s.now()); retry {
			return s.postponeDownloads(ctx, rows, until)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) drainOne(ctx context.Context, row wantedRow, cfg Settings) error {
	unlock := s.lockFile(row.MediaFileID)
	defer unlock()
	pick, ok, err := row.candidate()
	if err != nil || !ok {
		return err
	}
	t, err := s.target(ctx, row.MediaFileID)
	if errors.Is(err, errNoFileOnDisk) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.fetchAndStore(ctx, t, row, pick, cfg, row.Priority)
}

// postponeDownloads moves the waiting downloads to a later time after OpenSubtitles failed.
func (s *Service) postponeDownloads(ctx context.Context, rows []wantedRow, until time.Time) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		for _, row := range rows {
			current, err := wantedOf(tx, row.MediaFileID)
			if err != nil {
				return err
			}
			for _, r := range current {
				if r.Language == row.Language && r.Forced == row.Forced && r.Candidate != nil {
					r.NextAttemptAt = &until
					if err := saveWanted(tx, []wantedRow{r}); err != nil {
						return err
					}
				}
			}
		}
		return s.deps.Jobs.EnqueueTxAt(tx, downloadsTask{}, until)
	})
}

// upgradeAll re-searches provider subtitles inside their upgrade window and replaces one only with a
// candidate at least one point better.
func (s *Service) upgradeAll(ctx context.Context) error {
	subs, err := upgradable(s.deps.DB.Read(ctx), s.now())
	if err != nil {
		return err
	}
	cfg, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	sr := s.newSearcher()
	for _, sub := range subs {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.upgrade(ctx, sr, sub, cfg)
		if _, retry := retryTime(err, s.now()); retry {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) upgrade(ctx context.Context, sr *searcher, sub subtitleRow, cfg Settings) error {
	old, ok := sub.score()
	if !ok || !old.Upgradable() {
		return nil
	}
	unlock := s.lockFile(sub.MediaFileID)
	defer unlock()
	st, known, err := s.deps.Repo.state(ctx, sub.MediaFileID)
	if err != nil || !known {
		return err
	}
	t, err := s.target(ctx, sub.MediaFileID)
	if errors.Is(err, errNoFileOnDisk) {
		return nil
	}
	if err != nil {
		return err
	}
	cands, err := sr.search(ctx, t.query(sub.Language))
	if err != nil {
		return err
	}
	blocked := map[int64]bool{}
	for id := range st.Blocked {
		blocked[id] = true
	}
	if sub.ProviderFileID != nil {
		blocked[*sub.ProviderFileID] = true
	}
	track := wantedTrack{Language: sub.Language, Forced: sub.Forced}
	row := wantedRow{MediaFileID: sub.MediaFileID, Language: sub.Language, Forced: sub.Forced}
	for _, r := range st.Wanted {
		if r.Language == sub.Language && r.Forced == sub.Forced {
			row, track = r, r.track()
		}
	}
	ranked := rank(t.Video, track, cands, blocked)
	if len(ranked) == 0 || ranked[0].Score.Points < old.Points+1 {
		return nil
	}
	pick := pendingDownload{Candidate: ranked[0].Candidate, Score: ranked[0].Score}
	return s.fetchAndStore(ctx, t, row, pick, cfg, PriorityUpgrade)
}
