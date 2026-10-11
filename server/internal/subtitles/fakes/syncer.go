package fakes

import (
	"context"
	"slices"
	"sync"
	"time"

	"magpie/internal/platform/scripted"
	"magpie/internal/subtitles"
)

const SyncerSync scripted.Method = "Syncer.Sync"

// Syncer returns the subtitle unchanged with a scripted offset per video, or ErrNoSpeech for videos
// marked speechless. Unknown videos sync with a zero offset.
type Syncer struct {
	scripted.Script

	mu         sync.Mutex
	offsets    map[string]time.Duration
	speechless map[string]bool
}

var _ subtitles.Syncer = (*Syncer)(nil)

func NewSyncer() *Syncer {
	return &Syncer{offsets: map[string]time.Duration{}, speechless: map[string]bool{}}
}

func (f *Syncer) SetOffset(videoPath string, offset time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offsets[videoPath] = offset
}

// SetNoSpeech makes Sync return ErrNoSpeech for videoPath.
func (f *Syncer) SetNoSpeech(videoPath string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.speechless[videoPath] = true
}

func (f *Syncer) Sync(ctx context.Context, videoPath string, srt []byte) ([]byte, time.Duration, error) {
	if err := f.Enter(ctx, SyncerSync); err != nil {
		return nil, 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.speechless[videoPath] {
		return nil, 0, subtitles.ErrNoSpeech
	}
	return slices.Clone(srt), f.offsets[videoPath], nil
}
