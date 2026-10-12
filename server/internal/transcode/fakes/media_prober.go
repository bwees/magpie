package fakes

import (
	"context"
	"errors"
	"sync"

	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/scripted"
	"magpie/internal/transcode"
)

const MediaProberProbe scripted.Method = "MediaProber.Probe"

var ErrUnreadable = errors.New("fake ffprobe: no media info set for this path")

// MediaProber returns the media info set per path; other paths are unreadable.
type MediaProber struct {
	scripted.Script

	mu    sync.Mutex
	infos map[string]libraryapi.MediaInfo
}

var _ transcode.MediaProber = (*MediaProber)(nil)

func NewMediaProber() *MediaProber {
	return &MediaProber{infos: map[string]libraryapi.MediaInfo{}}
}

func (p *MediaProber) Set(path string, info libraryapi.MediaInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.infos[path] = info
}

func (p *MediaProber) Probe(ctx context.Context, path string) (libraryapi.MediaInfo, error) {
	if err := p.Enter(ctx, MediaProberProbe); err != nil {
		return libraryapi.MediaInfo{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	info, ok := p.infos[path]
	if !ok {
		return libraryapi.MediaInfo{}, ErrUnreadable
	}
	return info, nil
}
