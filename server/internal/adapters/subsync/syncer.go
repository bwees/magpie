package subsync

import (
	"context"
	"fmt"
	"io"
	"time"

	"magpie/internal/subtitles"
)

const (
	// MaxOffset bounds the shift searched in either direction (ffsubsync DEFAULT_MAX_OFFSET_SECONDS).
	MaxOffset = 60 * time.Second

	// MinPeakRatio is the lowest peak-to-sidelobe ratio accepted as a real alignment.
	MinPeakRatio = 6.0

	// MinSpeech is the least detected speech an alignment can be based on.
	MinSpeech = 5 * time.Second
)

// AudioDecoder decodes a video's audio for the speech detector.
type AudioDecoder interface {
	// DecodeAudio streams the first audio track as 16 kHz mono signed 16-bit little-endian PCM.
	// Close reports any decode failure.
	DecodeAudio(ctx context.Context, videoPath string) (io.ReadCloser, error)
}

// Syncer aligns subtitle timing to a video's speech. It keeps no state between calls.
type Syncer struct {
	decoder AudioDecoder
}

func New(decoder AudioDecoder) *Syncer {
	return &Syncer{decoder: decoder}
}

// Sync returns srt with every timestamp moved to match the video's speech, and the shift applied.
// Only timestamps change; when the shift is zero the output equals the input.
func (s *Syncer) Sync(ctx context.Context, videoPath string, srt []byte) ([]byte, time.Duration, error) {
	cues := parseCues(srt)
	if !hasDialogue(cues) {
		return nil, 0, fmt.Errorf("%w: subtitle has no dialogue cues", subtitles.ErrNoSpeech)
	}

	speech, err := s.detectSpeech(ctx, videoPath)
	if err != nil {
		return nil, 0, err
	}
	if spoken := countSpeech(speech); spoken < int(MinSpeech/frameDuration) {
		return nil, 0, fmt.Errorf("%w: %v of speech detected", subtitles.ErrNoSpeech, time.Duration(spoken)*frameDuration)
	}

	result, err := align(ctx, speechSignal(speech), cues, int(MaxOffset/frameDuration))
	if err != nil {
		return nil, 0, err
	}
	if result.peakRatio < MinPeakRatio {
		return nil, 0, fmt.Errorf("%w: peak ratio %.1f below %.1f", subtitles.ErrNoSpeech, result.peakRatio, MinPeakRatio)
	}
	t := result.timing()
	return shiftSRT(srt, t), t.offset, nil
}

func (s *Syncer) detectSpeech(ctx context.Context, videoPath string) ([]bool, error) {
	pcm, err := s.decoder.DecodeAudio(ctx, videoPath)
	if err != nil {
		return nil, fmt.Errorf("decode audio: %w", err)
	}
	speech, detectErr := detectSpeech(ctx, pcm)
	closeErr := pcm.Close()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if detectErr != nil {
		return nil, fmt.Errorf("detect speech: %w", detectErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("decode audio: %w", closeErr)
	}
	return speech, nil
}

func hasDialogue(cues []cue) bool {
	for _, c := range cues {
		if isDialogue(c) {
			return true
		}
	}
	return false
}

func countSpeech(speech []bool) int {
	n := 0
	for _, s := range speech {
		if s {
			n++
		}
	}
	return n
}

func speechSignal(speech []bool) []float64 {
	signal := make([]float64, len(speech))
	for i, s := range speech {
		signal[i] = nonSpeechValue
		if s {
			signal[i] = speechValue
		}
	}
	return signal
}
