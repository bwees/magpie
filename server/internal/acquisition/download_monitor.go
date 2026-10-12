package acquisition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/jobs"

	"gorm.io/gorm"
)

// missingGrace keeps a just-added torrent that qBittorrent doesn't list yet from counting as removed.
const missingGrace = time.Minute

// downloadStep is what a check does for a download, given its torrent's state.
type downloadStep string

const (
	stepNone            downloadStep = ""
	stepGone            downloadStep = "GONE"
	stepMetadata        downloadStep = "METADATA"
	stepMetadataTimeout downloadStep = "METADATA_TIMEOUT"
	stepFailed          downloadStep = "FAILED"
	stepCompleted       downloadStep = "COMPLETED"
	stepStalled         downloadStep = "STALLED"
)

var (
	failedStatuses   = []TorrentStatus{TorrentError, TorrentMissingFiles}
	finishedStatuses = []TorrentStatus{TorrentUploading, TorrentStalledUp, TorrentStoppedUp, TorrentQueuedUp, TorrentForcedUp}
	seedingStatuses  = []TorrentStatus{TorrentUploading, TorrentStalledUp, TorrentQueuedUp, TorrentForcedUp}
	// finishedStates are downloads done downloading that may still seed.
	finishedStates = []api.DownloadState{api.StateCompleted, api.StateImported}
)

// PollInterval is the qBittorrent poll's pace: quick while anything downloads, slow when idle.
func (s *DownloadService) PollInterval(ctx context.Context) (time.Duration, error) {
	active, err := s.deps.Repo.hasActiveDownloads(ctx)
	if err != nil {
		return 0, err
	}
	if active {
		return ActivePollInterval, nil
	}
	return IdlePollInterval, nil
}

// Poll syncs with qBittorrent, emits DownloadProgressed for each download that changed, and queues a
// check job for each one that needs action. Torrents Magpie didn't grab are ignored.
func (s *DownloadService) Poll(ctx context.Context, q jobs.Enqueuer) error {
	downloads, err := s.deps.Repo.activeDownloads(ctx)
	if err != nil || len(downloads) == 0 {
		return err
	}
	state, err := s.sync(ctx)
	if err != nil {
		return err
	}
	options, err := DownloadOptionsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	now := s.now()
	for _, d := range downloads {
		t, found := state.Torrents[d.QbitHash]
		if found {
			s.observe(d, t, now)
		}
		if s.nextStep(d, t, found, now, options) == stepNone || !s.claim(d.ID) {
			continue
		}
		if err := q.Enqueue(ctx, downloadCheckTask{Download: d.ID}); err != nil {
			s.unclaim(d.ID)
			return err
		}
	}
	return nil
}

func (s *DownloadService) sync(ctx context.Context) (SyncState, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	state, err := s.deps.QBittorrent.Sync(ctx, s.rid)
	if err != nil {
		return SyncState{}, err
	}
	s.rid = state.RID
	return state, nil
}

func (s *DownloadService) observe(d download, t Torrent, now time.Time) {
	progress := api.DownloadProgressed{
		Download: d.ID, State: displayState(d, t), Progress: t.Progress, SpeedBps: t.DownloadBps, ETA: t.ETA, Ratio: t.Ratio,
	}
	s.mu.Lock()
	if mark, seen := s.marks[d.ID]; !seen || t.Progress > mark.progress {
		s.marks[d.ID] = progressMark{progress: t.Progress, at: now}
	}
	last, seen := s.live[d.ID]
	s.live[d.ID] = progress
	s.mu.Unlock()
	if !seen || last != progress {
		s.deps.Bus.Emit(bus.EventOf(progress))
	}
}

// displayState is the state the Downloads tab shows, from the stored state and qBittorrent's.
func displayState(d download, t Torrent) api.DownloadState {
	switch {
	case d.State == api.StateAwaitingMetadata:
		return api.StateAwaitingMetadata
	case slices.Contains(finishedStates, d.State) && slices.Contains(seedingStatuses, t.Status):
		return api.StateSeeding
	case slices.Contains(finishedStates, d.State):
		return d.State
	case t.Status == TorrentStalledDL:
		return api.StateStalled
	}
	return api.StateDownloading
}

func (s *DownloadService) nextStep(d download, t Torrent, found bool, now time.Time, o DownloadOptions) downloadStep {
	if !found {
		if now.Sub(d.GrabbedAt) > missingGrace {
			return stepGone
		}
		return stepNone
	}
	switch d.State {
	case api.StateAwaitingMetadata:
		switch {
		case t.HasMetadata:
			return stepMetadata
		case now.Sub(d.GrabbedAt) > o.MetadataTimeout.Std():
			return stepMetadataTimeout
		}
	case api.StateDownloading:
		switch {
		case slices.Contains(failedStatuses, t.Status):
			return stepFailed
		case t.Progress >= 1 && slices.Contains(finishedStatuses, t.Status):
			return stepCompleted
		case s.stalledFor(d.ID, now) > o.StalledLimit.Std():
			return stepStalled
		}
	}
	return stepNone
}

func (s *DownloadService) stalledFor(id domain.DownloadID, now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	mark, seen := s.marks[id]
	if !seen {
		return 0
	}
	return now.Sub(mark.at)
}

func (s *DownloadService) claim(id domain.DownloadID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checking[id] {
		return false
	}
	s.checking[id] = true
	return true
}

func (s *DownloadService) unclaim(id domain.DownloadID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.checking, id)
}

// forget drops what the poll tracked for a download that left qBittorrent.
func (s *DownloadService) forget(id domain.DownloadID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, id)
	delete(s.marks, id)
}

// Check acts on one download: the metadata check, completion with its backstop, a failure, a timeout
// or a stall. It reads the state afresh, so a repeated check changes nothing.
func (s *DownloadService) Check(ctx context.Context, id domain.DownloadID) error {
	defer s.unclaim(id)
	d, err := s.deps.Repo.Download(ctx, id)
	if errors.Is(err, ErrDownloadNotFound) {
		return nil
	}
	if err != nil || d.RemovedAt != nil {
		return err
	}
	state, err := s.sync(ctx)
	if err != nil {
		return err
	}
	cfg, err := s.settings(ctx)
	if err != nil {
		return err
	}
	t, found := state.Torrents[d.QbitHash]
	switch s.nextStep(d, t, found, s.now(), cfg.options) {
	case stepGone:
		return s.markRemoved(ctx, d)
	case stepMetadata:
		return s.checkMetadata(ctx, d, cfg)
	case stepMetadataTimeout:
		return s.drop(ctx, d, rejection{Reason: api.RejectMetadataTimeout, Temporary: true}, cfg)
	case stepFailed:
		failure := fmt.Sprintf("qBittorrent reports %s", t.Status)
		return s.drop(ctx, d, rejection{Reason: api.RejectDownloadFailed, Failure: failure}, cfg)
	case stepCompleted:
		return s.complete(ctx, d, t, cfg)
	case stepStalled:
		failure := fmt.Sprintf("No progress for over %s", cfg.options.StalledLimit.Std())
		return s.drop(ctx, d, rejection{Reason: api.RejectStalled, Temporary: true, Failure: failure}, cfg)
	}
	return nil
}

// checkMetadata runs the blocked-file check on a magnet whose file list just arrived, then starts it.
func (s *DownloadService) checkMetadata(ctx context.Context, d download, cfg grabSettings) error {
	files, err := s.deps.QBittorrent.Files(ctx, d.QbitHash)
	if err != nil || len(files) == 0 {
		return ignoreGone(err)
	}
	if file, blocked := cfg.blocked.firstMatch(filePaths(files)); blocked {
		return s.drop(ctx, d, rejection{Reason: api.RejectBlockedFile, BlockedFile: &file}, cfg)
	}
	if err := s.deps.QBittorrent.Start(ctx, d.QbitHash); err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		_, err := moveDownload(tx, d.ID, []api.DownloadState{api.StateAwaitingMetadata}, map[string]any{"state": api.StateDownloading})
		return err
	})
}

// complete re-checks the final file list and publishes DownloadCompleted once it passes.
func (s *DownloadService) complete(ctx context.Context, d download, t Torrent, cfg grabSettings) error {
	files, err := s.deps.QBittorrent.Files(ctx, d.QbitHash)
	if err != nil {
		return ignoreGone(err)
	}
	wanted := slices.DeleteFunc(files, func(f TorrentFile) bool { return f.Priority <= 0 })
	if file, blocked := cfg.blocked.firstMatch(filePaths(wanted)); blocked {
		return s.drop(ctx, d, rejection{Reason: api.RejectBlockedFile, BlockedFile: &file}, cfg)
	}
	completed := api.DownloadCompleted{
		Download: d.ID, Title: d.Title, Episodes: nonNil(d.Episodes), ContentPath: t.ContentPath,
		Files: make([]api.DownloadFile, 0, len(wanted)), ReleaseName: d.ReleaseName, InfoHash: d.InfoHash, Quality: d.Quality,
		CFScore: d.CFScore, SeedingMode: d.SeedingMode,
	}
	for _, f := range wanted {
		completed.Files = append(completed.Files, api.DownloadFile{Path: f.Path, Size: f.Size})
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		moved, err := moveDownload(tx, d.ID, []api.DownloadState{api.StateDownloading}, map[string]any{
			"state": api.StateCompleted, "progress": t.Progress, "completed_at": s.now().UTC(),
		})
		if err != nil || !moved {
			return err
		}
		return s.deps.Bus.Publish(tx, bus.EventOf(completed))
	})
}

// drop deletes a rejected download with its data, blocklists it and grabs the next cached release.
func (s *DownloadService) drop(ctx context.Context, d download, r rejection, cfg grabSettings) error {
	if err := s.deps.QBittorrent.Delete(ctx, true, d.QbitHash); ignoreGone(err) != nil {
		return err
	}
	r.Title, r.Episodes, r.Release, r.InfoHash, r.Indexer, r.Download = d.Title, d.Episodes, d.ReleaseName, d.InfoHash, d.Indexer, &d
	out := grabOutcome{rejections: []rejection{r}}
	if d.Search != nil {
		out.search, _ = s.deps.Searches.find(*d.Search)
	}
	s.fallback(ctx, &out, cfg)
	return s.commit(ctx, out)
}

// markRemoved records a download that left qBittorrent, removed by the user here or in qBittorrent.
func (s *DownloadService) markRemoved(ctx context.Context, d download) error {
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		return tx.Model(&downloadRow{}).Where("id = ? AND removed_at IS NULL", d.ID).
			Updates(map[string]any{"state": api.StateRemoved, "removed_at": s.now().UTC()}).Error
	})
	s.forget(d.ID)
	return err
}

func filePaths(files []TorrentFile) []string {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths
}

func ignoreGone(err error) error {
	if errors.Is(err, ErrTorrentNotFound) {
		return nil
	}
	return err
}
