package fakes

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const (
	JellyfinLibraryRefreshPath scripted.Method = "JellyfinLibrary.RefreshPath"
	JellyfinLibraryFindByPath  scripted.Method = "JellyfinLibrary.FindByPath"
	JellyfinLibraryChanges     scripted.Method = "JellyfinLibrary.Changes"
	JellyfinLibraryItemsSince  scripted.Method = "JellyfinLibrary.ItemsSince"

	changesBuffer = 64
)

// JellyfinLibrary holds Jellyfin items by path and records refreshes. Tests push LibraryChanged messages
// to every open Changes stream with PushChange.
type JellyfinLibrary struct {
	scripted.Script

	mu        sync.Mutex
	items     map[string]library.JellyfinItem
	refreshed []string
	streams   map[chan library.LibraryChange]struct{}
}

var _ library.JellyfinLibrary = (*JellyfinLibrary)(nil)

func NewJellyfinLibrary() *JellyfinLibrary {
	return &JellyfinLibrary{
		items:   map[string]library.JellyfinItem{},
		streams: map[chan library.LibraryChange]struct{}{},
	}
}

// AddItem makes Jellyfin know an item at item.Path.
func (j *JellyfinLibrary) AddItem(item library.JellyfinItem) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.items[filepath.Clean(item.Path)] = item
}

// PushChange sends c to every open Changes stream. A stream whose buffer is full misses it, as a
// dropped socket message would.
func (j *JellyfinLibrary) PushChange(c library.LibraryChange) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for stream := range j.streams {
		select {
		case stream <- c:
		default:
		}
	}
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

func (j *JellyfinLibrary) FindByPath(ctx context.Context, path string) (library.JellyfinItem, error) {
	if err := j.Enter(ctx, JellyfinLibraryFindByPath); err != nil {
		return library.JellyfinItem{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	item, ok := j.items[filepath.Clean(path)]
	if !ok {
		return library.JellyfinItem{}, library.ErrItemNotFound
	}
	return item, nil
}

func (j *JellyfinLibrary) Changes(ctx context.Context) (<-chan library.LibraryChange, error) {
	if err := j.Enter(ctx, JellyfinLibraryChanges); err != nil {
		return nil, err
	}
	stream := make(chan library.LibraryChange, changesBuffer)
	j.mu.Lock()
	j.streams[stream] = struct{}{}
	j.mu.Unlock()
	go func() {
		<-ctx.Done()
		j.mu.Lock()
		defer j.mu.Unlock()
		delete(j.streams, stream)
		close(stream)
	}()
	return stream, nil
}

func (j *JellyfinLibrary) ItemsSince(ctx context.Context, t time.Time) ([]library.JellyfinItem, error) {
	if err := j.Enter(ctx, JellyfinLibraryItemsSince); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	items := []library.JellyfinItem{}
	for _, item := range j.items {
		if item.DateSaved.After(t) {
			items = append(items, item)
		}
	}
	slices.SortFunc(items, func(a, b library.JellyfinItem) int {
		return cmp.Or(a.DateSaved.Compare(b.DateSaved), cmp.Compare(a.ID, b.ID))
	})
	return items, nil
}
