package fakes

import (
	"context"
	"slices"
	"sync"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const JellyfinLibraryRefreshPath scripted.Method = "JellyfinLibrary.RefreshPath"

// JellyfinLibrary records refreshes.
type JellyfinLibrary struct {
	scripted.Script

	mu        sync.Mutex
	refreshed []string
}

var _ library.JellyfinLibrary = (*JellyfinLibrary)(nil)

func NewJellyfinLibrary() *JellyfinLibrary {
	return &JellyfinLibrary{}
}

// Refreshed returns every path passed to RefreshPath, in call order.
func (j *JellyfinLibrary) Refreshed() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Clone(j.refreshed)
}

func (j *JellyfinLibrary) RefreshPath(ctx context.Context, path string) error {
	if err := j.Enter(ctx, JellyfinLibraryRefreshPath); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.refreshed = append(j.refreshed, path)
	return nil
}
