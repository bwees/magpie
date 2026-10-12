package acquisition

import (
	"context"
	"errors"

	"magpie/internal/domain"

	"gorm.io/gorm"
)

// Settle runs a search's automatic grab: the best accepted release first, then the best for each item no
// grab covers yet, each through the blocked-file check and its fallback. Releases waiting only on the delay
// go to the pending queue. The search then completes, if every indexer answered.
func (s *DownloadService) Settle(ctx context.Context, id domain.SearchID) error {
	st, err := s.deps.Searches.find(id)
	if errors.Is(err, ErrSearchNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.isCancelled() {
		return s.finishSettle(ctx, st, false)
	}
	if err := s.CheckClient(ctx); err != nil {
		return err
	}
	covered, err := s.grabEachItem(ctx, st)
	if err != nil {
		return err
	}
	held, err := s.hold(ctx, st, covered)
	if err != nil {
		return err
	}
	return s.finishSettle(ctx, st, held)
}

func (s *DownloadService) finishSettle(ctx context.Context, st *search, held bool) error {
	if step := st.finishSettle(held); step.completed {
		return recordCompletion(context.WithoutCancel(ctx), s.deps.DB, s.deps.Bus, st, step.done, s.now().UTC())
	}
	return nil
}

func (s *DownloadService) grabEachItem(ctx context.Context, st *search) (itemSet, error) {
	cfg, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	covered := itemSet{}
	uncovered := func(d decidedRelease) bool { return !covered.overlaps(episodeIDs(d.Candidate)) }
	for !st.isCancelled() {
		out := grabOutcome{search: st}
		s.fallbackWhere(ctx, &out, cfg, uncovered)
		if out.added == nil && len(out.rejections) == 0 {
			break
		}
		if err := s.commit(ctx, out); err != nil {
			return nil, err
		}
		if out.added == nil {
			break
		}
		covered.add(out.added.Episodes)
	}
	return covered, nil
}

// hold stores the releases waiting on the delay for items no grab covers, and drops the held releases
// a grab made obsolete. It reports whether anything waits.
func (s *DownloadService) hold(ctx context.Context, st *search, covered itemSet) (bool, error) {
	waiting := st.heldReleases(func(d decidedRelease) bool { return !covered.overlaps(episodeIDs(d.Candidate)) })
	now := s.now().UTC()
	held := make([]PendingRelease, 0, len(waiting))
	for _, d := range waiting {
		c := d.Candidate
		held = append(held, PendingRelease{
			Title: st.input.Title.ID, Episodes: episodeIDs(c), IndexerID: c.Indexer.ProwlarrID, Release: c.Item,
			Trigger: st.trigger, Reason: d.Rejections[0].Reason, PublishAt: c.Item.PublishedAt, AddedAt: now,
		})
	}
	err := s.deps.DB.Write(context.WithoutCancel(ctx), func(tx *gorm.DB) error {
		if len(covered) > 0 {
			if err := dropPending(tx, st.input.Title.ID, covered); err != nil {
				return err
			}
		}
		return holdReleases(tx, held)
	})
	return len(held) > 0, err
}

// RemoveTitle takes a removed title's downloads out of qBittorrent, with their data when deleteFiles is set.
func (s *DownloadService) RemoveTitle(ctx context.Context, title domain.TitleID, deleteFiles bool) error {
	downloads, err := s.deps.Repo.titleDownloads(ctx, title)
	if err != nil {
		return err
	}
	for _, d := range downloads {
		if d.RemovedAt != nil {
			continue
		}
		if _, err := s.Remove(ctx, d.ID, deleteFiles); err != nil {
			return err
		}
	}
	return nil
}
