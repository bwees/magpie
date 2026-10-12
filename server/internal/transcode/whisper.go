package transcode

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"time"

	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/srt"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

// needs is the capability a device must have passed to run the job.
func (r jobRow) needs() agentproto.Capability {
	if r.Kind == api.JobKindWhisper {
		return agentproto.CapWhisperVulkan
	}
	return targetVideos[r.TargetCodec].needs
}

// whisperParams picks the audio Whisper listens to: the first track in the job's language, else the
// default track, else the first one. ok is false for a file without audio.
func whisperParams(row jobRow, info libraryapi.MediaInfo) (agentproto.WhisperParams, bool) {
	isAudio := func(s libraryapi.Stream) bool { return s.Kind == libraryapi.StreamAudio }
	preferences := []func(libraryapi.Stream) bool{
		func(s libraryapi.Stream) bool { return isAudio(s) && s.Language == row.Language },
		func(s libraryapi.Stream) bool { return isAudio(s) && s.Default },
		isAudio,
	}
	for _, prefer := range preferences {
		if i := slices.IndexFunc(info.Streams, prefer); i >= 0 {
			audio := info.Streams[i]
			return agentproto.WhisperParams{
				Language:       row.Language,
				AudioStream:    audio.Index,
				StartOffsetSec: startOffset(info, audio).Seconds(),
				Model:          row.WhisperModel,
			}, true
		}
	}
	return agentproto.WhisperParams{}, false
}

// startOffset is how much later than the video the audio starts. Whisper's cues count from the audio's
// first sample, so the agent adds this to every cue.
func startOffset(info libraryapi.MediaInfo, audio libraryapi.Stream) time.Duration {
	video, ok := firstVideo(info)
	if !ok {
		return 0
	}
	return audio.StartTime - video.StartTime
}

// checkWhisper accepts an uploaded subtitle that matches the agent's digest and parses with at least
// one cue. Subtitles moves the staged file to its sidecar path.
func (s *JobService) checkWhisper(row jobRow) (verdict, *failure, error) {
	fail := func(f failure) (verdict, *failure, error) { return verdict{}, &f, nil }
	result, err := row.result()
	if err != nil {
		return verdict{}, nil, err
	}
	if result.Verify.SHA256 != row.OutputSHA256 {
		return fail(retryable(ReasonDigestMismatch))
	}
	content, err := os.ReadFile(row.StagingPath)
	if errors.Is(err, fs.ErrNotExist) {
		return fail(retryable(ReasonUploadMissing))
	}
	if err != nil {
		return verdict{}, nil, err
	}
	cues, err := srt.Parse(content)
	if err != nil {
		return fail(final(ReasonBadSubtitle))
	}
	if len(cues) == 0 {
		return fail(final(ReasonNoCues))
	}
	completed := api.WhisperCompleted{Job: row.ID, MediaFile: row.MediaFileID, Language: row.Language, StagedPath: row.StagingPath}
	return verdict{state: api.StateCompleted, event: completed}, nil, nil
}
