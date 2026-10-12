package fakes

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const (
	JellyfinLibraryRefreshPath scripted.Method = "JellyfinLibrary.RefreshPath"
	JellyfinLibraryItems       scripted.Method = "JellyfinLibrary.Items"
	JellyfinLibraryChanges     scripted.Method = "JellyfinLibrary.Changes"
	JellyfinLibraryItemsSince  scripted.Method = "JellyfinLibrary.ItemsSince"

	changesBuffer = 64
)

// savedItem is a Jellyfin item with the time Jellyfin last saved it.
type savedItem struct {
	library.JellyfinItem
	saved time.Time
}

// JellyfinLibrary holds Jellyfin items by ID and records refreshes. Tests push LibraryChanged messages
// to every open Changes stream with PushChange.
type JellyfinLibrary struct {
	scripted.Script

	mu        sync.Mutex
	items     map[string]savedItem
	refreshed []string
	streams   map[chan library.LibraryChange]struct{}
}

var _ library.JellyfinLibrary = (*JellyfinLibrary)(nil)

func NewJellyfinLibrary() *JellyfinLibrary {
	return &JellyfinLibrary{
		items:   map[string]savedItem{},
		streams: map[chan library.LibraryChange]struct{}{},
	}
}

// AddItem makes Jellyfin know item, last saved at saved.
func (j *JellyfinLibrary) AddItem(item library.JellyfinItem, saved time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.items[item.ID] = savedItem{JellyfinItem: item, saved: saved}
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

// Streams reports how many Changes streams are open.
func (j *JellyfinLibrary) Streams() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.streams)
}

// DropStreams closes every open Changes stream, as a dropped socket would.
func (j *JellyfinLibrary) DropStreams() {
	j.mu.Lock()
	defer j.mu.Unlock()
	for stream := range j.streams {
		delete(j.streams, stream)
		close(stream)
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

func (j *JellyfinLibrary) Items(ctx context.Context, ids []string) ([]library.JellyfinItem, error) {
	if err := j.Enter(ctx, JellyfinLibraryItems); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	items := []library.JellyfinItem{}
	for _, id := range ids {
		if item, ok := j.items[id]; ok {
			items = append(items, item.JellyfinItem)
		}
	}
	return items, nil
}

// Changes streams pushed changes until ctx ends or DropStreams is called.
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
		if _, open := j.streams[stream]; open {
			delete(j.streams, stream)
			close(stream)
		}
	}()
	return stream, nil
}

func (j *JellyfinLibrary) ItemsSince(ctx context.Context, t time.Time) ([]library.JellyfinItem, error) {
	if err := j.Enter(ctx, JellyfinLibraryItemsSince); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	saved := []savedItem{}
	for _, item := range j.items {
		if item.saved.After(t) {
			saved = append(saved, item)
		}
	}
	slices.SortFunc(saved, func(a, b savedItem) int {
		return cmp.Or(a.saved.Compare(b.saved), cmp.Compare(a.ID, b.ID))
	})
	items := make([]library.JellyfinItem, 0, len(saved))
	for _, item := range saved {
		items = append(items, item.JellyfinItem)
	}
	return items, nil
}
