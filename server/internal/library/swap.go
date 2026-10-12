package library

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
	transcodeapi "magpie/internal/transcode/api"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	handlerTranscodeVerified = "library.swap-on-transcode-verified"

	swapQueueName = "library.transcode-swap"
	// Disk and database errors retry through backlite; a check that fails rejects the swap at once.
	swapQueueAttempts = 3
	swapQueueBackoff  = time.Minute

	// transcodedExtension is the container every transcode produces.
	transcodedExtension = ".mkv"
	// maxDurationDrift is how far the staged file's runtime may stray from the original's.
	maxDurationDrift = time.Second
)

// swapTask swaps one verified transcode into the library.
type swapTask struct {
	Verified transcodeapi.TranscodeVerified `json:"verified"`
}

func (swapTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: swapQueueName, MaxAttempts: swapQueueAttempts, Backoff: swapQueueBackoff}
}

func newSwapQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task swapTask) error {
		return s.Swap(ctx, task.Verified)
	})
}

// newTranscodeVerifiedHandler queues the swap: the checks read and rename files, which a handler can't
// do inside its transaction.
func newTranscodeVerifiedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerTranscodeVerified,
		EventTypes: []string{transcodeapi.TypeTranscodeVerified},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			verified, err := bus.Decode[transcodeapi.TranscodeVerified](ev)
			if err != nil {
				return err
			}
			return s.deps.Jobs.EnqueueTx(tx, swapTask{Verified: verified})
		},
	}
}

// swapRejection stops a swap: the staged file is dropped and Transcode is told why.
type swapRejection struct {
	reason api.SwapRejectReason
	detail string
}

func (r *swapRejection) Error() string { return string(r.reason) + ": " + r.detail }

func reject(reason api.SwapRejectReason, detail string) error {
	return &swapRejection{reason: reason, detail: detail}
}

// Swap replaces a media file with its verified transcode. The file must be the one the job leased, the
// staged file must be the upload Transcode checked, and ffprobe must agree it is the same video. The
// rename is atomic; the old file goes only if the extension changed.
func (s *Service) Swap(ctx context.Context, v transcodeapi.TranscodeVerified) error {
	file, err := s.deps.Repo.file(ctx, v.MediaFile)
	if errors.Is(err, api.ErrNotFound) {
		return s.rejectSwap(ctx, v, &swapRejection{reason: api.SwapFileGone, detail: "the media file is no longer in the library"})
	}
	if err != nil {
		return err
	}
	if _, err := s.deps.Files.Stat(v.StagedPath); errors.Is(err, fs.ErrNotExist) && file.Transcoded {
		return nil
	}
	final, media, err := s.checkSwap(ctx, file, v)
	var rejection *swapRejection
	if errors.As(err, &rejection) {
		return s.rejectSwap(ctx, v, rejection)
	}
	if err != nil {
		return err
	}

	if err := s.deps.Files.Rename(v.StagedPath, final); err != nil {
		return err
	}
	if final != file.Path {
		if err := s.deps.Files.Remove(file.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.WarnContext(ctx, "library: removing the file a transcode replaced", "path", file.Path, "error", err)
		}
	}
	// The new file is in place by now, so its record must be written even during shutdown.
	if err := s.commitSwap(context.WithoutCancel(ctx), file, final, media); err != nil {
		return err
	}
	s.refreshFolders(ctx, []string{final})
	return nil
}

// checkSwap returns the path the transcode goes to and its media info, or a *swapRejection.
func (s *Service) checkSwap(ctx context.Context, file storedFile, v transcodeapi.TranscodeVerified) (string, api.MediaInfo, error) {
	fail := func(err error) (string, api.MediaInfo, error) { return "", api.MediaInfo{}, err }
	info, err := s.deps.Files.Stat(file.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return fail(reject(api.SwapFileGone, "the media file is gone from disk"))
	}
	if err != nil {
		return fail(err)
	}
	if file.ETag != v.SourceETag || etagOf(info) != v.SourceETag {
		return fail(reject(api.SwapSourceChanged, "the file changed after the transcode started"))
	}
	if filepath.Dir(v.StagedPath) != filepath.Join(file.RootFolder, StagingDirName) {
		return fail(reject(api.SwapHashMismatch, "the staged file isn't in the file's staging folder"))
	}
	sum, err := s.deps.Hasher.SHA256(v.StagedPath)
	if errors.Is(err, fs.ErrNotExist) {
		return fail(reject(api.SwapHashMismatch, "the staged file is missing"))
	}
	if err != nil {
		return fail(err)
	}
	if sum != v.SHA256 {
		return fail(reject(api.SwapHashMismatch, "the staged file isn't the verified upload"))
	}
	staged, err := s.deps.Prober.Probe(ctx, v.StagedPath)
	if errors.Is(err, ErrProbeFailed) {
		return fail(reject(api.SwapProbeFailed, err.Error()))
	}
	if err != nil {
		return fail(err)
	}
	if drift := staged.Duration - file.MediaInfo.Duration; drift.Abs() > maxDurationDrift {
		return fail(reject(api.SwapProbeFailed, "the transcode runs "+drift.String()+" off the original"))
	}
	if want := len(file.MediaInfo.Streams) - len(v.Stats.DroppedAudio) - len(v.Stats.DroppedSubs); len(staged.Streams) != want {
		return fail(reject(api.SwapProbeFailed, "the transcode doesn't have the streams the job kept"))
	}
	return strings.TrimSuffix(file.Path, filepath.Ext(file.Path)) + transcodedExtension, staged, nil
}

func (s *Service) rejectSwap(ctx context.Context, v transcodeapi.TranscodeVerified, rejection *swapRejection) error {
	slog.WarnContext(ctx, "library: rejecting a transcode", "file", v.MediaFile, "job", v.Job, "reason", rejection.reason, "detail", rejection.detail)
	if err := s.deps.Files.Remove(v.StagedPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		rejected := api.TranscodeSwapRejected{Job: v.Job, MediaFile: v.MediaFile, Reason: rejection.reason}
		return s.deps.Bus.Publish(tx, bus.EventOf(rejected))
	})
}

func (s *Service) commitSwap(ctx context.Context, file storedFile, final string, media api.MediaInfo) error {
	info, err := s.deps.Files.Stat(final)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(media)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		err := tx.Model(&mediaFileRow{}).Where("id = ?", file.ID).Updates(map[string]any{
			"path": final, "size": info.Size, "media_info": string(encoded), "etag": etagOf(info),
			"transcoded": true, "transcoded_at": now, "original_size": gorm.Expr("COALESCE(original_size, ?)", file.Size),
		}).Error
		if err != nil {
			return err
		}
		transcoded := api.FileTranscoded{MediaFile: file.ID, Path: final, OldSize: file.Size, NewSize: info.Size}
		return s.deps.Bus.Publish(tx, bus.EventOf(transcoded))
	})
}
