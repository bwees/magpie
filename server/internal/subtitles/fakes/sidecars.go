package fakes

import (
	"context"
	"maps"
	"slices"
	"sync"

	"magpie/internal/platform/scripted"
	"magpie/internal/subtitles"
)

const (
	SidecarFilesWrite  scripted.Method = "SidecarFiles.Write"
	SidecarFilesRemove scripted.Method = "SidecarFiles.Remove"
)

// SidecarFiles keeps sidecar files in memory, keyed by path.
type SidecarFiles struct {
	scripted.Script

	mu    sync.Mutex
	files map[string][]byte
}

var _ subtitles.SidecarFiles = (*SidecarFiles)(nil)

func NewSidecarFiles() *SidecarFiles {
	return &SidecarFiles{files: map[string][]byte{}}
}

// Put places a file, as a release import would.
func (f *SidecarFiles) Put(path string, content []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = slices.Clone(content)
}

// Read returns a file's content and whether it exists.
func (f *SidecarFiles) Read(path string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	content, ok := f.files[path]
	return slices.Clone(content), ok
}

// Paths lists every stored path, sorted.
func (f *SidecarFiles) Paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(maps.Keys(f.files))
}

func (f *SidecarFiles) Write(path string, content []byte) error {
	if err := f.Enter(context.Background(), SidecarFilesWrite); err != nil {
		return err
	}
	f.Put(path, content)
	return nil
}

func (f *SidecarFiles) Remove(path string) error {
	if err := f.Enter(context.Background(), SidecarFilesRemove); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, path)
	return nil
}
