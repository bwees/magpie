// Package fakes holds in-memory fakes of Transcode's ports, for tests and for booting without real adapters.
package fakes

import (
	"context"
	"fmt"
	"io/fs"
	"sync"

	"magpie/internal/platform/scripted"
	"magpie/internal/transcode"
)

const FileStatStat scripted.Method = "FileStat.Stat"

// FileStat returns whatever infos were set; other paths don't exist.
type FileStat struct {
	scripted.Script

	mu    sync.Mutex
	infos map[string]transcode.FileInfo
}

var _ transcode.FileStat = (*FileStat)(nil)

func NewFileStat() *FileStat {
	return &FileStat{infos: map[string]transcode.FileInfo{}}
}

func (f *FileStat) Set(path string, info transcode.FileInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infos[path] = info
}

func (f *FileStat) Remove(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.infos, path)
}

func (f *FileStat) Stat(path string) (transcode.FileInfo, error) {
	if err := f.Enter(context.Background(), FileStatStat); err != nil {
		return transcode.FileInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	info, ok := f.infos[path]
	if !ok {
		return transcode.FileInfo{}, fmt.Errorf("stat %s: %w", path, fs.ErrNotExist)
	}
	return info, nil
}
