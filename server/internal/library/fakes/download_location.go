package fakes

import (
	"context"
	"path"
	"strings"
	"sync"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const (
	DownloadLocationSavePath  scripted.Method = "DownloadLocation.SavePath"
	DownloadLocationLocalPath scripted.Method = "DownloadLocation.LocalPath"
)

// DownloadLocation answers the save path set with Set; "" until then. LocalPath maps paths under the
// remote folder set with Map, and returns others unchanged.
type DownloadLocation struct {
	scripted.Script

	mu            sync.RWMutex
	path          string
	remote, local string
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

// Map sets the remote path mapping: paths under remote are seen under local.
func (d *DownloadLocation) Map(remote, local string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.remote, d.local = remote, local
}

func (d *DownloadLocation) SavePath(ctx context.Context) (string, error) {
	if err := d.Enter(ctx, DownloadLocationSavePath); err != nil {
		return "", err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.path, nil
}

func (d *DownloadLocation) LocalPath(ctx context.Context, p string) (string, error) {
	if err := d.Enter(ctx, DownloadLocationLocalPath); err != nil {
		return "", err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	rest, under := strings.CutPrefix(p, d.remote)
	if d.remote == "" || !under || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return p, nil
	}
	return path.Join(d.local, rest), nil
}
