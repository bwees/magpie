package subtitles

import (
	"context"
	"errors"
	"io/fs"
	"slices"

	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"

	"gorm.io/gorm"
)

const handlerWhisper = "subtitles.store-whisper-results"

// maxWhisperAttempts is how many Whisper jobs one track asks for before it stays MISSING.
const maxWhisperAttempts = 3

// wantsWhisper: Whisper writes the regular track of the title's original language when nothing else
// was found, as long as its earlier attempts haven't run out.
func wantsWhisper(cfg Settings, row wantedRow, original domain.Language) bool {
	return cfg.WhisperEnabled && !row.Forced && row.Language == original && row.WhisperAttempts < maxWhisperAttempts
}

// requestWhisper asks Transcode for a Whisper job and waits for its reply.
func (s *Service) requestWhisper(ctx context.Context, row wantedRow, cfg Settings) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row.Status, row.NextAttemptAt, row.Candidate = StatusWaitingWhisper, nil, nil
		row.WhisperAttempts++
		if err := saveWanted(tx, []wantedRow{row}); err != nil {
			return err
		}
		run := transcodeapi.RunAgentJob{
			Kind: transcodeapi.JobKindWhisper, MediaFile: row.MediaFileID, Language: row.Language, Model: cfg.WhisperModel,
		}
		if err := s.publish(tx, run); err != nil {
			return err
		}
		return s.settle(tx, row.MediaFileID)
	})
}

// newWhisperHandler takes the replies to RunAgentJob.
func newWhisperHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerWhisper,
		EventTypes: []string{transcodeapi.TypeWhisperCompleted, transcodeapi.TypeJobFailed},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			if ev.Type == transcodeapi.TypeWhisperCompleted {
				e, err := bus.Decode[transcodeapi.WhisperCompleted](ev)
				if err != nil {
					return err
				}
				return s.storeWhisper(ctx, tx, e)
			}
			e, err := bus.Decode[transcodeapi.JobFailed](ev)
			if err != nil || e.Kind != transcodeapi.JobKindWhisper || !e.Final {
				return err
			}
			return s.whisperFailed(tx, e.MediaFile, e.Language)
		},
	}
}

// storeWhisper moves the staged subtitle to the track's sidecar path, unsynced. A redelivered result
// changes nothing; one nobody waits for any more, because the track was found meanwhile or the file is
// gone, is dropped. The video's path is read now, so a rename that ran while Whisper worked is followed.
func (s *Service) storeWhisper(ctx context.Context, tx *gorm.DB, e transcodeapi.WhisperCompleted) error {
	st, known, err := loadState(tx, e.MediaFile)
	if err != nil {
		return err
	}
	if known && slices.ContainsFunc(st.Subtitles, func(sub subtitleRow) bool { return sub.JobID != nil && *sub.JobID == e.Job }) {
		return nil
	}
	file, err := s.deps.Library.File(ctx, e.MediaFile)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return s.deps.Sidecars.Remove(e.StagedPath)
	}
	if err != nil {
		return err
	}
	i := slices.IndexFunc(st.Wanted, func(r wantedRow) bool { return r.Language == e.Language && !r.Forced })
	if !known || i < 0 || st.Wanted[i].Status == StatusSatisfied {
		return s.deps.Sidecars.Remove(e.StagedPath)
	}
	path := sidecarPath(file.Path, e.Language, false, false)
	err = s.deps.Sidecars.Move(e.StagedPath, path)
	if errors.Is(err, fs.ErrNotExist) {
		return s.whisperFailed(tx, e.MediaFile, e.Language)
	}
	if err != nil {
		return err
	}
	job := e.Job
	sub := subtitleRow{
		ID: newSubtitleID(), MediaFileID: e.MediaFile, Language: e.Language, Source: api.SourceWhisper,
		Path: path, AddedAt: s.now(), JobID: &job,
	}
	return s.keep(tx, st, sub, func() error { return nil })
}

// whisperFailed marks a track that waited for Whisper MISSING. The sweep searches it again and may ask
// Whisper once more. SubtitleMissing goes out the first time Whisper fails it.
func (s *Service) whisperFailed(tx *gorm.DB, file domain.MediaFileID, language domain.Language) error {
	rows, err := wantedOf(tx, file)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(rows, func(r wantedRow) bool { return r.Language == language && !r.Forced })
	if i < 0 || rows[i].Status != StatusWaitingWhisper {
		return nil
	}
	row := rows[i]
	cfg, err := SettingsKey.GetTx(tx)
	if err != nil {
		return err
	}
	now := s.now()
	next := nextMissingAttempt(cfg, row.CreatedAt, now)
	failedBefore := row.Reason == api.MissingWhisperFailed
	row.Status, row.Reason, row.NextAttemptAt = StatusMissing, api.MissingWhisperFailed, &next
	if err := saveWanted(tx, []wantedRow{row}); err != nil {
		return err
	}
	if !failedBefore {
		missing := api.SubtitleMissing{MediaFile: file, Language: language, Reason: api.MissingWhisperFailed}
		if err := s.publish(tx, missing); err != nil {
			return err
		}
	}
	return s.settle(tx, file)
}
