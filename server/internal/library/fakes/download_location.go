package fakes

import (
	"context"
	"sync"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const DownloadLocationSavePath scripted.Method = "DownloadLocation.SavePath"

// DownloadLocation answers the save path set with Set; "" until then.
type DownloadLocation struct {
	scripted.Script

	mu   sync.RWMutex
	path string
}

var _ library.DownloadLocation = (*DownloadLocation)(nil)

func NewDownloadLocation() *DownloadLocation {
	return &DownloadLocation{}
}

func (d *DownloadLocation) Set(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.path = path
}

func (d *DownloadLocation) SavePath(ctx context.Context) (string, error) {
	if err := d.Enter(ctx, DownloadLocationSavePath); err != nil {
		return "", err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path, nil
}
