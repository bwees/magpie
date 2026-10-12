package subtitles

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
	"magpie/internal/subtitles/api"

	"gorm.io/gorm"
)

var ErrQuotaExhausted = errors.New("subtitles: no OpenSubtitles downloads left today")

// QuotaExhaustedError says when downloads are possible again.
type QuotaExhaustedError struct{ ResetAt time.Time }

func (e *QuotaExhaustedError) Error() string { return ErrQuotaExhausted.Error() }
func (e *QuotaExhaustedError) Unwrap() error { return ErrQuotaExhausted }

// SearchNow reopens every track that isn't satisfied and searches the file now, skipping the backoff and
// any pause after a rejected login.
func (s *Service) SearchNow(ctx context.Context, file domain.MediaFileID) (FileView, error) {
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		st, known, err := loadState(tx, file)
		if err != nil {
			return err
		}
		if !known {
			return ErrNotFound
		}
		sess, err := session(tx)
		if err != nil {
			return err
		}
		sess.PausedUntil, sess.Problem = nil, ""
		if err := saveSession(tx, sess); err != nil {
			return err
		}
		for _, row := range st.Wanted {
			if row.Status != StatusSatisfied && row.Candidate == nil {
				row.reopen(PriorityManual)
				if err := saveWanted(tx, []wantedRow{row}); err != nil {
					return err
				}
			}
		}
		if err := s.queueSearch(tx, file, PriorityManual, time.Time{}); err != nil {
			return err
		}
		return s.settle(tx, file)
	})
	if err != nil {
		return FileView{}, err
	}
	return s.File(ctx, file)
}

// CandidateView is one scored OpenSubtitles result for the candidate picker.
type CandidateView struct {
	Candidate
	Score Score
	// Acceptable is whether the score reaches the minimum the automatic search accepts.
	Acceptable bool
}

// Candidates searches OpenSubtitles for one track of a file and returns every usable result, best first.
func (s *Service) Candidates(ctx context.Context, file domain.MediaFileID, lang domain.Language, forced bool) ([]CandidateView, error) {
	track := wantedTrack{Language: lang, Forced: forced}
	st, t, err := s.manualTarget(ctx, file, &track)
	if err != nil {
		return nil, err
	}
	cfg, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	cands, err := s.newSearcher().search(ctx, t.query(track.Language))
	if err != nil {
		return nil, err
	}
	ranked := rank(t.Video, track, cands, st.Blocked)
	out := make([]CandidateView, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, CandidateView{Candidate: r.Candidate, Score: r.Score, Acceptable: r.Score.Meets(t.minScore(cfg))})
	}
	return out, nil
}

// Download fetches a candidate the user picked, syncs it when auto-sync is on, and stores it as the track's
// sidecar. It fails with a *QuotaExhaustedError when no downloads are left.
func (s *Service) Download(ctx context.Context, file domain.MediaFileID, lang domain.Language, forced bool, fileID int64) (FileView, error) {
	track := wantedTrack{Language: lang, Forced: forced}
	unlock := s.lockFile(file)
	st, t, err := s.manualTarget(ctx, file, &track)
	if err != nil {
		unlock()
		return FileView{}, err
	}
	err = s.downloadPicked(ctx, st, t, track, fileID)
	unlock()
	if err != nil {
		return FileView{}, err
	}
	return s.File(ctx, file)
}

func (s *Service) downloadPicked(ctx context.Context, st fileState, t target, track wantedTrack, fileID int64) error {
	cfg, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	cands, err := s.newSearcher().search(ctx, t.query(track.Language))
	if err != nil {
		return err
	}
	for _, r := range rank(t.Video, track, cands, st.Blocked) {
		if r.FileID != fileID {
			continue
		}
		got, err := s.fetch(ctx, t, fileID, cfg)
		var wait *quotaWaitError
		if errors.As(err, &wait) {
			return &QuotaExhaustedError{ResetAt: wait.until}
		}
		if err != nil {
			return err
		}
		return s.store(ctx, t, track, pendingDownload{Candidate: r.Candidate, Score: r.Score}, got)
	}
	return ErrCandidateNotFound
}

// manualTarget loads a file for a manual action. The track's HI preference comes from its wanted row.
func (s *Service) manualTarget(ctx context.Context, file domain.MediaFileID, track *wantedTrack) (fileState, target, error) {
	st, known, err := s.deps.Repo.state(ctx, file)
	if err != nil {
		return fileState{}, target{}, err
	}
	if !known {
		return fileState{}, target{}, ErrNotFound
	}
	for _, row := range st.Wanted {
		if row.Language == track.Language && row.Forced == track.Forced {
			track.HearingImpairedOK = row.HearingImpaired
		}
	}
	t, err := s.target(ctx, file)
	if errors.Is(err, errNoFileOnDisk) {
		return fileState{}, target{}, ErrNotFound
	}
	return st, t, err
}

// DeleteSubtitle removes a sidecar and its record. A provider file is blocklisted for this media file, and
// a wanted track left without a subtitle is searched again.
func (s *Service) DeleteSubtitle(ctx context.Context, id SubtitleID) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		sub, err := subtitle(tx, id)
		if err != nil {
			return err
		}
		if sub.Source == api.SourceEmbedded {
			return ErrEmbedded
		}
		if err := deleteSubtitles(tx, []SubtitleID{id}); err != nil {
			return err
		}
		if sub.ProviderFileID != nil {
			if err := block(tx, sub.MediaFileID, *sub.ProviderFileID); err != nil {
				return err
			}
		}
		used, err := pathInUse(tx, sub.Path)
		if err != nil {
			return err
		}
		if !used {
			if err := s.deps.Sidecars.Remove(sub.Path); err != nil {
				return err
			}
		}
		st, _, err := loadState(tx, sub.MediaFileID)
		if err != nil {
			return err
		}
		var list []wantedRow
		for _, row := range st.Wanted {
			if row.Language == sub.Language && row.Forced == sub.Forced && covering(st.Subtitles, row.Language, row.Forced) == nil {
				row.reopen(PriorityManual)
				list = append(list, row)
			}
		}
		if len(list) == 0 {
			return nil
		}
		if err := saveWanted(tx, list); err != nil {
			return err
		}
		if err := s.queueSearch(tx, sub.MediaFileID, PriorityManual, time.Time{}); err != nil {
			return err
		}
		return s.settle(tx, sub.MediaFileID)
	})
}
