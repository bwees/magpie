package fakes

import (
	"context"
	"path/filepath"
	"sync"

	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/platform/scripted"
)

const MediaProberProbe scripted.Method = "MediaProber.Probe"

// MediaProber returns the media info set for each path. Unknown paths fail with ErrProbeFailed.
type MediaProber struct {
	scripted.Script

	mu    sync.RWMutex
	infos map[string]api.MediaInfo
}

var _ library.MediaProber = (*MediaProber)(nil)

func NewMediaProber() *MediaProber {
	return &MediaProber{infos: map[string]api.MediaInfo{}}
}

func (p *MediaProber) Set(path string, info api.MediaInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.infos[filepath.Clean(path)] = info
}

func (p *MediaProber) Probe(ctx context.Context, path string) (api.MediaInfo, error) {
	if err := p.Enter(ctx, MediaProberProbe); err != nil {
		return api.MediaInfo{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	info, ok := p.infos[filepath.Clean(path)]
	if !ok {
		return api.MediaInfo{}, library.ErrProbeFailed
	}
	return info, nil
}
