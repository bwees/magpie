package subtitles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/subtitles/api"

	"gorm.io/gorm"
)

const (
	// unavailableRetry is the wait before searching again after OpenSubtitles failed.
	unavailableRetry = 15 * time.Minute
	// authPause stops OpenSubtitles calls after the login was rejected twice in a row.
	authPause = 6 * time.Hour
	// frequentRetries is how long after import a missing track is retried on every sweep; later, daily.
	frequentRetries = 7 * day
	// quotaPeriod is OpenSubtitles' quota day, used when it doesn't say when the quota resets.
	quotaPeriod = day

	srtExt    = ".srt"
	forcedTag = "forced"
	sdhTag    = "sdh"
	nameSep   = "."

	authProblem = "OpenSubtitles rejected the login. Check it under Settings → Connections."
)

var errNoFileOnDisk = errors.New("subtitles: the media file is gone")

// pausedError: OpenSubtitles calls are on hold until a time.
type pausedError struct{ until time.Time }

func (e *pausedError) Error() string {
	return fmt.Sprintf("subtitles: OpenSubtitles paused until %s", e.until.Format(time.RFC3339))
}

// quotaWaitError: no downloads are left until the quota resets.
type quotaWaitError struct{ until time.Time }

func (e *quotaWaitError) Error() string {
	return fmt.Sprintf("subtitles: no OpenSubtitles downloads left until %s", e.until.Format(time.RFC3339))
}

// sidecarPath is <video basename>.<lang>[.forced|.sdh].srt.
func sidecarPath(video string, lang domain.Language, forced, sdh bool) string {
	parts := []string{strings.TrimSuffix(video, filepath.Ext(video)), string(lang)}
	switch {
	case forced:
		parts = append(parts, forcedTag)
	case sdh:
		parts = append(parts, sdhTag)
	}
	return strings.Join(parts, nameSep) + srtExt
}

// target is the media file a search is for, with what scoring and the query need.
type target struct {
	File  libraryapi.MediaFile
	Title catalogapi.Title
	Video video
	Query SearchQuery
}

func (t target) query(lang domain.Language) SearchQuery {
	q := t.Query
	q.Language = lang
	return q
}

func (t target) minScore(cfg Settings) int {
	if t.Title.Kind == domain.MediaKindSeries {
		return cfg.MinScoreEpisode
	}
	return cfg.MinScoreMovie
}

func (s *Service) target(ctx context.Context, id domain.MediaFileID) (target, error) {
	file, err := s.deps.Library.File(ctx, id)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return target{}, errNoFileOnDisk
	}
	if err != nil {
		return target{}, err
	}
	title, err := s.deps.Catalog.Title(ctx, file.Title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return target{}, errNoFileOnDisk
	}
	if err != nil {
		return target{}, err
	}
	season, episode := 0, 0
	if title.Kind == domain.MediaKindSeries && len(file.Episodes) > 0 {
		first := file.Episodes[0]
		season, episode = first.Season, first.Number
		if ep, err := s.deps.Catalog.Episode(ctx, first.Episode); err == nil {
			season, episode = ep.AiredSeason, ep.AiredNumber
		}
	}
	imdb := ""
	if title.External.Imdb != nil {
		imdb = *title.External.Imdb
	}
	name := file.ReleaseName
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
	}
	return target{
		File:  file,
		Title: title,
		Video: video{
			Kind: title.Kind, Tmdb: title.External.Tmdb, Imdb: imdb, Year: title.Year,
			Season: season, Episode: episode, Release: traitsOf(name, title.Kind),
		},
		Query: SearchQuery{MovieHash: file.OSHash, Tmdb: title.External.Tmdb, Imdb: imdb, Season: season, Episode: episode},
	}, nil
}

// searcher runs one pass's searches, asking OpenSubtitles once per language.
type searcher struct {
	s     *Service
	cache map[SearchQuery][]Candidate
}

func (s *Service) newSearcher() *searcher {
	return &searcher{s: s, cache: map[SearchQuery][]Candidate{}}
}

func (sr *searcher) search(ctx context.Context, q SearchQuery) ([]Candidate, error) {
	if cands, ok := sr.cache[q]; ok {
		return cands, nil
	}
	var cands []Candidate
	err := sr.s.withLogin(ctx, func() error {
		var err error
		cands, err = sr.s.deps.OpenSubtitles.Search(ctx, q)
		return err
	})
	if err != nil {
		return nil, err
	}
	sr.cache[q] = cands
	return cands, nil
}

// searchFile searches every open track of a file, downloads what scores high enough, and marks the rest missing.
func (s *Service) searchFile(ctx context.Context, file domain.MediaFileID, priority Priority) error {
	unlock := s.lockFile(file)
	defer unlock()
	st, known, err := s.deps.Repo.state(ctx, file)
	if err != nil || !known {
		return err
	}
	now := s.now()
	var due []wantedRow
	for _, row := range st.Wanted {
		open := row.Status == StatusPending || row.Status == StatusMissing
		if open && row.Candidate == nil && (row.NextAttemptAt == nil || !now.Before(*row.NextAttemptAt)) {
			due = append(due, row)
		}
	}
	if len(due) == 0 {
		return nil
	}
	t, err := s.target(ctx, file)
	if errors.Is(err, errNoFileOnDisk) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	sr := s.newSearcher()
	for i, row := range due {
		err := s.searchTrack(ctx, sr, t, row, st.Blocked, cfg, priority)
		if until, retry := retryTime(err, s.now()); retry {
			return s.retryLater(ctx, file, due[i:], priority, until)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// retryTime says when to try again after an OpenSubtitles failure. Other errors aren't retried here.
func retryTime(err error, now time.Time) (time.Time, bool) {
	var paused *pausedError
	var rateLimited *RetryAfterError
	switch {
	case err == nil:
		return time.Time{}, false
	case errors.As(err, &paused):
		return paused.until, true
	case errors.As(err, &rateLimited):
		return now.Add(rateLimited.Wait), true
	case errors.Is(err, ErrRateLimited), errors.Is(err, ErrUnavailable):
		return now.Add(unavailableRetry), true
	}
	return time.Time{}, false
}

// retryLater keeps the rows' status and books another search at until.
func (s *Service) retryLater(ctx context.Context, file domain.MediaFileID, rows []wantedRow, priority Priority, until time.Time) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		for _, row := range rows {
			row.NextAttemptAt = &until
			if err := saveWanted(tx, []wantedRow{row}); err != nil {
				return err
			}
		}
		return s.queueSearch(tx, file, priority, until)
	})
}

// searchTrack searches one wanted track and records the outcome.
func (s *Service) searchTrack(ctx context.Context, sr *searcher, t target, row wantedRow, blocked map[int64]bool, cfg Settings, priority Priority) error {
	cands, err := sr.search(ctx, t.query(row.Language))
	if err != nil {
		return err
	}
	ranked := rank(t.Video, row.track(), cands, blocked)
	if len(ranked) == 0 || !ranked[0].Score.Meets(t.minScore(cfg)) {
		if wantsWhisper(cfg, row, t.Title.OriginalLanguage) {
			return s.requestWhisper(ctx, row, cfg)
		}
		return s.markMissing(ctx, t, row, cfg)
	}
	return s.fetchAndStore(ctx, t, row, pendingDownload{Candidate: ranked[0].Candidate, Score: ranked[0].Score}, cfg, priority)
}

// missingReason is why nothing was found and Whisper won't be asked.
func missingReason(cfg Settings, row wantedRow, original domain.Language) api.MissingReason {
	switch {
	case !cfg.WhisperEnabled:
		return api.MissingNoMatch
	case row.Language != original:
		return api.MissingNotOriginalLanguage
	case row.WhisperAttempts >= maxWhisperAttempts:
		return api.MissingWhisperFailed
	}
	return api.MissingNoMatch
}

// nextMissingAttempt retries a missing track on every sweep for a week after import, then daily.
func nextMissingAttempt(cfg Settings, created, now time.Time) time.Time {
	if now.Sub(created) < frequentRetries {
		return now.Add(cfg.UpgradeCheckInterval.Std())
	}
	return now.Add(day)
}

// markMissing moves a track to MISSING. SubtitleMissing is published only when it wasn't missing already.
func (s *Service) markMissing(ctx context.Context, t target, row wantedRow, cfg Settings) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		now := s.now()
		next := nextMissingAttempt(cfg, row.CreatedAt, now)
		reason := missingReason(cfg, row, t.Title.OriginalLanguage)
		wasMissing := row.Status == StatusMissing
		row.Status, row.Reason, row.NextAttemptAt, row.Source, row.Candidate = StatusMissing, reason, &next, nil, nil
		if err := saveWanted(tx, []wantedRow{row}); err != nil {
			return err
		}
		if !wasMissing {
			missing := api.SubtitleMissing{MediaFile: row.MediaFileID, Language: row.Language, Forced: row.Forced, Reason: reason}
			if err := s.publish(tx, missing); err != nil {
				return err
			}
		}
		return s.settle(tx, row.MediaFileID)
	})
}

// fetchAndStore downloads an accepted candidate and writes it as the track's sidecar. Without quota the
// candidate waits on the row until the quota resets.
func (s *Service) fetchAndStore(ctx context.Context, t target, row wantedRow, pick pendingDownload, cfg Settings, priority Priority) error {
	got, err := s.fetch(ctx, t, pick.Candidate.FileID, cfg)
	var wait *quotaWaitError
	if errors.As(err, &wait) {
		return s.waitForQuota(ctx, row, pick, priority, wait.until)
	}
	if err != nil {
		return err
	}
	return s.store(ctx, t, row.track(), pick, got)
}

// waitForQuota keeps the accepted candidate on the row and books the waiting downloads for the reset.
func (s *Service) waitForQuota(ctx context.Context, row wantedRow, pick pendingDownload, priority Priority, until time.Time) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		current, err := wantedOf(tx, row.MediaFileID)
		if err != nil {
			return err
		}
		for _, r := range current {
			if r.Language != row.Language || r.Forced != row.Forced {
				continue
			}
			if err := r.setCandidate(pick); err != nil {
				return err
			}
			r.NextAttemptAt, r.Priority = &until, priority
			if r.Status != StatusSatisfied {
				r.Status = StatusPending
			}
			if err := saveWanted(tx, []wantedRow{r}); err != nil {
				return err
			}
		}
		if err := s.deps.Jobs.EnqueueTxAt(tx, downloadsTask{}, until); err != nil {
			return err
		}
		return s.settle(tx, row.MediaFileID)
	})
}

// fetched is a downloaded subtitle, synced when auto-sync could align it.
type fetched struct {
	Content []byte
	Synced  bool
	Offset  time.Duration
}

func (s *Service) fetch(ctx context.Context, t target, fileID int64, cfg Settings) (fetched, error) {
	content, err := s.download(ctx, fileID)
	if err != nil {
		return fetched{}, err
	}
	if !cfg.AutoSync {
		return fetched{Content: content}, nil
	}
	synced, offset, err := s.deps.Syncer.Sync(ctx, t.File.Path, content)
	switch {
	case err == nil:
		return fetched{Content: synced, Synced: true, Offset: offset}, nil
	case ctx.Err() != nil:
		return fetched{}, ctx.Err()
	case !errors.Is(err, ErrNoSpeech):
		slog.WarnContext(ctx, "subtitle sync failed; keeping it unsynced", "file", t.File.ID, "error", err)
	}
	return fetched{Content: content}, nil
}

// store writes a downloaded subtitle as the track's sidecar, replacing any provider or Whisper subtitle of
// the track and anything else at that path, and publishes SubtitleAdded.
func (s *Service) store(ctx context.Context, t target, track wantedTrack, pick pendingDownload, got fetched) error {
	cfg, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		st, known, err := loadState(tx, t.File.ID)
		if err != nil || !known {
			return err
		}
		now := s.now()
		c := pick.Candidate
		path := sidecarPath(t.File.Path, track.Language, track.Forced, c.HearingImpaired)
		points, max, fileID := pick.Score.Points, pick.Score.Max, c.FileID
		row := subtitleRow{
			ID: newSubtitleID(), MediaFileID: t.File.ID, Language: track.Language, Forced: track.Forced,
			SDH: c.HearingImpaired && !track.Forced, Source: api.SourceProvider, ProviderFileID: &fileID,
			ReleaseName: c.ReleaseName, Score: &points, MaxScore: &max, Synced: got.Synced, Path: path, AddedAt: now,
		}
		if got.Synced {
			ms := got.Offset.Milliseconds()
			row.SyncOffsetMs = &ms
		}
		if window := cfg.UpgradeWindow(); pick.Score.Upgradable() && window > 0 {
			until := now.Add(window)
			row.UpgradeUntil = &until
		}
		return s.keep(tx, st, row, func() error { return s.deps.Sidecars.Write(path, got.Content) })
	})
}

// keep stores sub as its track's sidecar, replacing the track's provider or Whisper subtitle and anything
// else at its path, and publishes SubtitleAdded. write puts the file at sub.Path.
func (s *Service) keep(tx *gorm.DB, st fileState, sub subtitleRow, write func() error) error {
	track := wantedTrack{Language: sub.Language, Forced: sub.Forced}
	var replaced []SubtitleID
	var stalePaths []string
	replaces := false
	for _, old := range st.Subtitles {
		sameTrack := old.Language == track.Language && old.Forced == track.Forced &&
			(old.Source == api.SourceProvider || old.Source == api.SourceWhisper)
		if !sameTrack && (old.Path != sub.Path || old.Source == api.SourceEmbedded) {
			continue
		}
		replaced = append(replaced, old.ID)
		replaces = replaces || sameTrack
		if old.Path != sub.Path {
			stalePaths = append(stalePaths, old.Path)
		}
	}
	if err := deleteSubtitles(tx, replaced); err != nil {
		return err
	}
	if err := saveSubtitle(tx, sub); err != nil {
		return err
	}
	if err := s.markSatisfied(tx, st.Wanted, track, sub.Source); err != nil {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	for _, stale := range stalePaths {
		used, err := pathInUse(tx, stale)
		if err != nil {
			return err
		}
		if used {
			continue
		}
		if err := s.deps.Sidecars.Remove(stale); err != nil {
			return err
		}
	}
	added := api.SubtitleAdded{
		MediaFile: sub.MediaFileID, Title: st.File.TitleID, Episodes: append([]domain.EpisodeID{}, st.File.EpisodeIDs...),
		Language: sub.Language, Forced: sub.Forced, SDH: sub.SDH,
		Source: sub.Source, Synced: sub.Synced, Replaces: replaces,
	}
	if err := s.publish(tx, added); err != nil {
		return err
	}
	return s.settle(tx, sub.MediaFileID)
}

// markSatisfied closes the wanted row of a track, if the track is wanted.
func (s *Service) markSatisfied(tx *gorm.DB, rows []wantedRow, track wantedTrack, source api.Source) error {
	for _, row := range rows {
		if row.Language == track.Language && row.Forced == track.Forced {
			row.satisfy(source)
			return saveWanted(tx, []wantedRow{row})
		}
	}
	return nil
}

// download spends one download of the daily quota. Without quota it returns a *quotaWaitError and makes
// no call.
func (s *Service) download(ctx context.Context, fileID int64) ([]byte, error) {
	sess, err := session(s.deps.DB.Read(ctx))
	if err != nil {
		return nil, err
	}
	if ok, until := sess.quotaLeft(s.now()); !ok {
		return nil, &quotaWaitError{until: until}
	}
	var content []byte
	var remaining int
	var resetAt time.Time
	err = s.withLogin(ctx, func() error {
		var err error
		content, remaining, resetAt, err = s.deps.OpenSubtitles.Download(ctx, fileID)
		return err
	})
	if errors.Is(err, ErrQuotaExceeded) {
		if resetAt.IsZero() {
			resetAt = s.now().Add(quotaPeriod)
		}
		return nil, errors.Join(s.recordQuota(ctx, 0, resetAt), &quotaWaitError{until: resetAt})
	}
	if err != nil {
		return nil, err
	}
	return content, s.recordQuota(ctx, remaining, resetAt)
}

func (s *Service) recordQuota(ctx context.Context, remaining int, resetAt time.Time) error {
	return s.updateSession(ctx, func(sess *sessionRow) {
		sess.DownloadsRemaining = &remaining
		if !resetAt.IsZero() {
			sess.ResetAt = &resetAt
		}
	})
}

func (s *Service) updateSession(ctx context.Context, change func(*sessionRow)) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		sess, err := session(tx)
		if err != nil {
			return err
		}
		change(&sess)
		return saveSession(tx, sess)
	})
}

// withLogin runs an OpenSubtitles call. On ErrAuth it logs in once and retries; a second rejection pauses
// every call for authPause and is shown on the quota badge.
func (s *Service) withLogin(ctx context.Context, call func() error) error {
	sess, err := session(s.deps.DB.Read(ctx))
	if err != nil {
		return err
	}
	if paused, until := sess.paused(s.now()); paused {
		return &pausedError{until: until}
	}
	err = call()
	if !errors.Is(err, ErrAuth) {
		return err
	}
	if err := s.login(ctx); err != nil {
		if errors.Is(err, ErrAuth) {
			return s.pause(ctx, err)
		}
		return err
	}
	err = call()
	if errors.Is(err, ErrAuth) {
		return s.pause(ctx, err)
	}
	return err
}

func (s *Service) login(ctx context.Context) error {
	token, err := s.deps.OpenSubtitles.Login(ctx)
	if err != nil {
		return err
	}
	return s.updateSession(ctx, func(sess *sessionRow) {
		expires := token.ExpiresAt
		sess.VIP, sess.TokenExpiresAt, sess.PausedUntil, sess.Problem = token.VIP, &expires, nil, ""
		if sess.DownloadsRemaining == nil && token.AllowedDownloads > 0 {
			allowed, reset := token.AllowedDownloads, nextUTCMidnight(s.now())
			sess.DownloadsRemaining, sess.ResetAt = &allowed, &reset
		}
	})
}

// nextUTCMidnight is when OpenSubtitles resets the daily quota.
func nextUTCMidnight(now time.Time) time.Time {
	return now.UTC().Truncate(day).Add(day)
}

func (s *Service) pause(ctx context.Context, cause error) error {
	until := s.now().Add(authPause)
	slog.WarnContext(ctx, "OpenSubtitles rejected the login; pausing subtitle downloads", "until", until, "error", cause)
	if err := s.updateSession(ctx, func(sess *sessionRow) { sess.PausedUntil, sess.Problem = &until, authProblem }); err != nil {
		return err
	}
	return &pausedError{until: until}
}
