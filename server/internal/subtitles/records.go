package subtitles

import (
	"context"
	"errors"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	requestsapi "magpie/internal/requests/api"
	"magpie/internal/subtitles/api"

	"gorm.io/gorm"
)

// undetermined is ffprobe's tag for a stream without a language.
const undetermined domain.Language = "und"

// recompute works through files in batches of this size after a wanted-language change.
const recomputeBatch = 100

func newSubtitleID() SubtitleID { return domain.NewID[SubtitleID]() }

// embeddedRows are the subtitle tracks inside the video. Image tracks count; untagged ones can't satisfy a language.
func embeddedRows(file domain.MediaFileID, info libraryapi.MediaInfo, now time.Time) []subtitleRow {
	var rows []subtitleRow
	for _, stream := range info.Streams {
		if stream.Kind != libraryapi.StreamSubtitle || stream.Language == "" || stream.Language == undetermined {
			continue
		}
		index := stream.Index
		rows = append(rows, subtitleRow{
			ID: newSubtitleID(), MediaFileID: file, Language: stream.Language, Forced: stream.Forced,
			Source: api.SourceEmbedded, StreamIndex: &index, AddedAt: now,
		})
	}
	return rows
}

// wantedList reads a title's effective wanted languages. A title Catalog no longer knows wants nothing.
func (s *Service) wantedList(ctx context.Context, title domain.TitleID) ([]catalogapi.WantedLanguage, error) {
	list, err := s.deps.Catalog.WantedLanguages(ctx, title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return nil, nil
	}
	return list, err
}

// recordImport stores a new file's embedded tracks and release sidecars and opens a wanted row for every
// wanted track. Replaces is ignored: the old file is cleaned up on its FileDeleted.
func (s *Service) recordImport(ctx context.Context, tx *gorm.DB, e libraryapi.FileImported) error {
	if _, known, err := loadState(tx, e.MediaFile); err != nil || known {
		return err
	}
	list, err := s.wantedList(ctx, e.Title)
	if err != nil {
		return err
	}
	now := s.now()
	if err := insertFile(tx, fileRow{MediaFileID: e.MediaFile, TitleID: e.Title, Settled: true, ImportedAt: now}); err != nil {
		return err
	}
	subs := embeddedRows(e.MediaFile, e.MediaInfo, now)
	for _, sc := range e.Sidecars {
		subs = append(subs, subtitleRow{
			ID: newSubtitleID(), MediaFileID: e.MediaFile, Language: sc.Language, Forced: sc.Forced, SDH: sc.SDH,
			Source: api.SourceRelease, Path: sc.Path, AddedAt: now,
		})
	}
	for _, sub := range subs {
		if err := saveSubtitle(tx, sub); err != nil {
			return err
		}
	}
	return s.applyWanted(tx, e.MediaFile, nil, list, subs, PriorityImport)
}

// applyWanted saves the reconciled wanted rows, books a search when a track is open, and settles the file.
func (s *Service) applyWanted(tx *gorm.DB, file domain.MediaFileID, existing []wantedRow, list []catalogapi.WantedLanguage, subs []subtitleRow, priority Priority) error {
	keep, drop := reconcile(file, existing, list, subs, priority, s.now())
	if err := deleteWanted(tx, drop); err != nil {
		return err
	}
	if err := saveWanted(tx, keep); err != nil {
		return err
	}
	if hasOpenTrack(keep) {
		if err := s.queueSearch(tx, file, priority, time.Time{}); err != nil {
			return err
		}
	}
	return s.settle(tx, file)
}

// recordTranscode re-reads the embedded tracks after a transcode, which may have dropped some.
func (s *Service) recordTranscode(ctx context.Context, tx *gorm.DB, e libraryapi.FileTranscoded) error {
	st, known, err := loadState(tx, e.MediaFile)
	if err != nil || !known {
		return err
	}
	file, err := s.deps.Library.File(ctx, e.MediaFile)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	list, err := s.wantedList(ctx, st.File.TitleID)
	if err != nil {
		return err
	}
	var stale []SubtitleID
	var subs []subtitleRow
	for _, sub := range st.Subtitles {
		if sub.Source == api.SourceEmbedded {
			stale = append(stale, sub.ID)
		} else {
			subs = append(subs, sub)
		}
	}
	if err := deleteSubtitles(tx, stale); err != nil {
		return err
	}
	embedded := embeddedRows(e.MediaFile, file.MediaInfo, s.now())
	for _, sub := range embedded {
		if err := saveSubtitle(tx, sub); err != nil {
			return err
		}
	}
	return s.applyWanted(tx, e.MediaFile, st.Wanted, list, append(subs, embedded...), PriorityImport)
}

// forgetFile drops a deleted file's rows and books the removal of its sidecars.
func (s *Service) forgetFile(tx *gorm.DB, e libraryapi.FileDeleted) error {
	paths, err := deleteFile(tx, e.MediaFile)
	if err != nil || len(paths) == 0 {
		return err
	}
	return s.deps.Jobs.EnqueueTx(tx, removeSidecarsTask{Paths: paths})
}

// followRename points the file's stored sidecars at the paths Library moved them to.
func (s *Service) followRename(tx *gorm.DB, e libraryapi.FileRenamed) error {
	return moveSidecars(tx, e.MediaFile, e.Sidecars)
}

// rerun drops a file's provider and Whisper subtitles, blocklists the provider's files for it and searches again.
func (s *Service) rerun(ctx context.Context, tx *gorm.DB, e requestsapi.IssueActionRequested) error {
	if e.Action != requestsapi.IssueActionRerunSubtitles {
		return nil
	}
	files, err := s.issueFiles(ctx, e)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := s.rerunFile(ctx, tx, file); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) issueFiles(ctx context.Context, e requestsapi.IssueActionRequested) ([]domain.MediaFileID, error) {
	if e.Episode != nil {
		file, err := s.deps.Library.FileForEpisode(ctx, *e.Episode)
		if errors.Is(err, libraryapi.ErrNotFound) {
			return nil, nil
		}
		return []domain.MediaFileID{file.ID}, err
	}
	files, err := s.deps.Library.FilesForTitle(ctx, e.Title)
	ids := make([]domain.MediaFileID, 0, len(files))
	for _, f := range files {
		ids = append(ids, f.ID)
	}
	return ids, err
}

func (s *Service) rerunFile(ctx context.Context, tx *gorm.DB, file domain.MediaFileID) error {
	st, known, err := loadState(tx, file)
	if err != nil || !known {
		return err
	}
	list, err := s.wantedList(ctx, st.File.TitleID)
	if err != nil {
		return err
	}
	var dropped []SubtitleID
	var paths []string
	var kept []subtitleRow
	for _, sub := range st.Subtitles {
		if sub.Source != api.SourceProvider && sub.Source != api.SourceWhisper {
			kept = append(kept, sub)
			continue
		}
		dropped = append(dropped, sub.ID)
		paths = append(paths, sub.Path)
		if sub.ProviderFileID != nil {
			if err := block(tx, file, *sub.ProviderFileID); err != nil {
				return err
			}
		}
	}
	if err := deleteSubtitles(tx, dropped); err != nil {
		return err
	}
	if len(paths) > 0 {
		if err := s.deps.Jobs.EnqueueTx(tx, removeSidecarsTask{Paths: paths}); err != nil {
			return err
		}
	}
	return s.applyWanted(tx, file, st.Wanted, list, kept, PriorityManual)
}

// recompute brings one batch of files in line with their current wanted languages and books the next batch.
func (s *Service) recompute(ctx context.Context, task recomputeTask) error {
	files, err := filesAfter(s.deps.DB.Read(ctx), task.Title, task.After, recomputeBatch)
	if err != nil {
		return err
	}
	lists := map[domain.TitleID][]catalogapi.WantedLanguage{}
	for _, f := range files {
		list, cached := lists[f.TitleID]
		if !cached {
			if list, err = s.wantedList(ctx, f.TitleID); err != nil {
				return err
			}
			lists[f.TitleID] = list
		}
		if err := s.recomputeFile(ctx, f.MediaFileID, list); err != nil {
			return err
		}
	}
	if len(files) < recomputeBatch {
		return nil
	}
	next := recomputeTask{Title: task.Title, After: files[len(files)-1].MediaFileID}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.deps.Jobs.EnqueueTx(tx, next) })
}

func (s *Service) recomputeFile(ctx context.Context, file domain.MediaFileID, list []catalogapi.WantedLanguage) error {
	unlock := s.lockFile(file)
	defer unlock()
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		st, known, err := loadState(tx, file)
		if err != nil || !known {
			return err
		}
		return s.applyWanted(tx, file, st.Wanted, list, st.Subtitles, PriorityBacklog)
	})
}

// removeSidecars deletes sidecar files no stored subtitle points at any more. The check and the removal
// share a write transaction, so a subtitle written to the same path meanwhile keeps its file.
func (s *Service) removeSidecars(ctx context.Context, paths []string) error {
	for _, path := range paths {
		err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
			used, err := pathInUse(tx, path)
			if err != nil || used {
				return err
			}
			return s.deps.Sidecars.Remove(path)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
