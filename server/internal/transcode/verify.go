package transcode

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/transcode/api"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	verifyQueueName = "transcode.verify-upload"
	// Probe errors retry through backlite before the attempt fails.
	verifyQueueAttempts = 3
	verifyQueueBackoff  = 30 * time.Second
)

// verifyTask runs the server's checks on an uploaded output.
type verifyTask struct {
	Job domain.JobID `json:"job"`
}

func (verifyTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: verifyQueueName, MaxAttempts: verifyQueueAttempts, Backoff: verifyQueueBackoff}
}

func newVerifyQueue(s *JobService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task verifyTask) error {
		return s.Verify(ctx, task.Job)
	})
}

// verdict is where a job that passed the server's checks goes next, and what it publishes.
type verdict struct {
	state api.JobState
	event domain.Message
}

// Verify checks an uploaded output. A transcode must match the digest the agent reported, have the
// codec, bit depth and duration ffprobe reads, and come from the source still leased; it then publishes
// TranscodeVerified for Library to swap in. A Whisper subtitle must parse with at least one cue; it then
// publishes WhisperCompleted and the job is done.
func (s *JobService) Verify(ctx context.Context, id domain.JobID) error {
	row, err := jobIn(s.deps.DB.Read(ctx), id)
	if errors.Is(err, ErrJobNotFound) || (err == nil && row.State != api.StateVerifying) {
		return nil
	}
	if err != nil {
		return err
	}
	next, f, err := s.check(ctx, row)
	if err != nil {
		return err
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		if f != nil {
			return s.failAttempt(tx, row, *f)
		}
		now := s.now()
		changes := map[string]any{"state": next.state, "progress": 1, "updated_at": now}
		if next.state == api.StateCompleted {
			changes["finished_at"] = now
		}
		moved, err := moveJob(tx, row.ID, []api.JobState{api.StateVerifying}, changes)
		if err != nil || !moved {
			return err
		}
		return s.deps.Bus.Publish(tx, bus.EventOf(next.event))
	})
}

// check returns where the job goes next, or the failure that ends the attempt. err is for trouble a
// retry of the task may get past.
func (s *JobService) check(ctx context.Context, row jobRow) (verdict, *failure, error) {
	if row.Kind == api.JobKindWhisper {
		return s.checkWhisper(row)
	}
	verified, f, err := s.checkTranscode(ctx, row)
	return verdict{state: api.StateVerified, event: verified}, f, err
}

func (s *JobService) checkTranscode(ctx context.Context, row jobRow) (api.TranscodeVerified, *failure, error) {
	fail := func(f failure) (api.TranscodeVerified, *failure, error) { return api.TranscodeVerified{}, &f, nil }
	result, err := row.result()
	if err != nil {
		return api.TranscodeVerified{}, nil, err
	}
	spec, err := row.spec()
	if err != nil || spec.Transcode == nil {
		return api.TranscodeVerified{}, nil, errors.Join(err, fmt.Errorf("job %s has no transcode spec", row.ID))
	}
	if result.Verify.SHA256 != row.OutputSHA256 {
		return fail(retryable(ReasonDigestMismatch))
	}

	source, err := s.deps.Library.SourceForProcessing(ctx, row.MediaFileID)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return fail(final(ReasonSourceGone))
	}
	if err != nil {
		return api.TranscodeVerified{}, nil, err
	}
	info, err := s.deps.Files.Stat(source.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return fail(final(ReasonSourceMissing))
	}
	if err != nil {
		return api.TranscodeVerified{}, nil, err
	}
	if info.ETag() != row.TransferETag || source.ETag != row.SourceETag {
		return fail(final(ReasonSourceChanged))
	}

	output, err := s.deps.Prober.Probe(ctx, row.StagingPath)
	if err != nil {
		if ctx.Err() != nil {
			return api.TranscodeVerified{}, nil, err
		}
		return fail(retryable("ffprobe can't read the output: " + err.Error()))
	}
	video, ok := firstVideo(output)
	if !ok || video.Codec != spec.Transcode.Codec || video.BitDepth != spec.Transcode.BitDepth {
		return fail(final(fmt.Sprintf("The output is %s %d-bit, not %s %d-bit", video.Codec, video.BitDepth, spec.Transcode.Codec, spec.Transcode.BitDepth)))
	}
	if delta := output.Duration - source.MediaInfo.Duration; delta.Abs() > maxDurationDelta {
		return fail(retryable(fmt.Sprintf("The output runs %.1f s off the source", delta.Seconds())))
	}

	return api.TranscodeVerified{
		Job:        row.ID,
		MediaFile:  row.MediaFileID,
		StagedPath: row.StagingPath,
		SHA256:     row.OutputSHA256,
		SourceETag: row.SourceETag,
		Stats: api.TranscodeStats{
			OldSize:      row.SourceSize,
			NewSize:      row.OutputSize,
			VideoCodec:   video.Codec,
			BitDepth:     video.BitDepth,
			VMAFMean:     result.Verify.VMAFMean,
			DurationSec:  output.Duration.Seconds(),
			Agent:        agentOf(row),
			Encoder:      spec.Transcode.Encoder,
			DroppedAudio: dropped(streamsOf(source.MediaInfo, libraryapi.StreamAudio), spec.Transcode.KeepAudio),
			DroppedSubs:  dropped(streamsOf(source.MediaInfo, libraryapi.StreamSubtitle), spec.Transcode.KeepSubs),
		},
	}, nil, nil
}

func agentOf(row jobRow) domain.AgentID {
	if row.AgentID == nil {
		return ""
	}
	return *row.AgentID
}

func dropped(all, kept []int) []int {
	return slices.DeleteFunc(slices.Clone(all), func(i int) bool { return slices.Contains(kept, i) })
}
