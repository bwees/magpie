package acquisition

import (
	"sync"

	"magpie/internal/domain"
)

// SearchRegistry holds the searches whose results are still cached. Searching adds to it; grabbing
// and the fallback after a rejection read from it.
type SearchRegistry struct {
	mu       sync.Mutex
	searches map[domain.SearchID]*search
}

func NewSearchRegistry() *SearchRegistry {
	return &SearchRegistry{searches: map[domain.SearchID]*search{}}
}

func (r *SearchRegistry) add(st *search) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searches[st.id] = st
}

func (r *SearchRegistry) remove(id domain.SearchID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.searches, id)
}

// find returns ErrSearchNotFound once a search expired.
func (r *SearchRegistry) find(id domain.SearchID) (*search, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.searches[id]
	if !ok {
		return nil, ErrSearchNotFound
	}
	return st, nil
}

func (r *SearchRegistry) all() []*search {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]*search, 0, len(r.searches))
	for _, st := range r.searches {
		list = append(list, st)
	}
	return list
}
