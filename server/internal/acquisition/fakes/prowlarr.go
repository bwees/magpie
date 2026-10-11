// Package fakes holds in-memory fakes of Acquisition's ports, for tests and for booting without real adapters.
package fakes

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/platform/scripted"
)

const (
	ProwlarrIndexers scripted.Method = "Prowlarr.Indexers"
	ProwlarrSearch   scripted.Method = "Prowlarr.Search"
	ProwlarrRSS      scripted.Method = "Prowlarr.RSS"
	ProwlarrDownload scripted.Method = "Prowlarr.Download"
	ProwlarrWebURL   scripted.Method = "Prowlarr.WebURL"
)

type indexer struct {
	info    acquisition.ProwlarrIndexer
	results func(acquisition.TorznabQuery) []acquisition.TorznabItem
	rss     []acquisition.TorznabItem
	err     error
	delay   time.Duration
	queries []acquisition.TorznabQuery
}

// Prowlarr holds indexers with per-indexer results, failures and answer delays, and records every query.
// A delayed answer that outlives ctx returns ErrTimeout, as the adapter does.
type Prowlarr struct {
	scripted.Script

	mu        sync.Mutex
	indexers  map[int]*indexer
	downloads map[string]acquisition.TorrentPayload
	webURL    string
}

var _ acquisition.Prowlarr = (*Prowlarr)(nil)

func NewProwlarr() *Prowlarr {
	return &Prowlarr{indexers: map[int]*indexer{}, downloads: map[string]acquisition.TorrentPayload{}}
}

// AddIndexer adds an indexer that answers every query with no results until SetResults.
// Adding a known ID replaces what Indexers reports for it and keeps its scripted answers.
func (p *Prowlarr) AddIndexer(info acquisition.ProwlarrIndexer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ix, ok := p.indexers[info.ID]; ok {
		ix.info = info
		return
	}
	p.indexers[info.ID] = &indexer{info: info, results: func(acquisition.TorznabQuery) []acquisition.TorznabItem { return nil }}
}

// RemoveIndexer drops an indexer, as when it is deleted or disabled in Prowlarr.
func (p *Prowlarr) RemoveIndexer(indexerID int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.indexers, indexerID)
}

func (p *Prowlarr) SetWebURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.webURL = url
}

// SetResults makes the indexer answer every query with items.
func (p *Prowlarr) SetResults(indexerID int, items ...acquisition.TorznabItem) {
	p.SetResultsFunc(indexerID, func(acquisition.TorznabQuery) []acquisition.TorznabItem { return items })
}

// SetResultsFunc makes the indexer's answer depend on the query, e.g. results for an ID search only.
func (p *Prowlarr) SetResultsFunc(indexerID int, fn func(acquisition.TorznabQuery) []acquisition.TorznabItem) {
	p.with(indexerID, func(ix *indexer) { ix.results = fn })
}

func (p *Prowlarr) SetRSS(indexerID int, items ...acquisition.TorznabItem) {
	p.with(indexerID, func(ix *indexer) { ix.rss = items })
}

// FailIndexer makes Search and RSS on one indexer return err, e.g. an *ErrIndexerFailed. Nil clears it.
func (p *Prowlarr) FailIndexer(indexerID int, err error) {
	p.with(indexerID, func(ix *indexer) { ix.err = err })
}

// DelayIndexer makes one indexer answer after d.
func (p *Prowlarr) DelayIndexer(indexerID int, d time.Duration) {
	p.with(indexerID, func(ix *indexer) { ix.delay = d })
}

// SetDownload makes Download(link) return payload.
func (p *Prowlarr) SetDownload(link string, payload acquisition.TorrentPayload) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.downloads[link] = payload
}

// Queries returns every query sent to one indexer, in order.
func (p *Prowlarr) Queries(indexerID int) []acquisition.TorznabQuery {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ix, ok := p.indexers[indexerID]; ok {
		return slices.Clone(ix.queries)
	}
	return nil
}

func (p *Prowlarr) Indexers(ctx context.Context) ([]acquisition.ProwlarrIndexer, error) {
	if err := p.Enter(ctx, ProwlarrIndexers); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	list := []acquisition.ProwlarrIndexer{}
	for _, ix := range p.indexers {
		list = append(list, ix.info)
	}
	slices.SortFunc(list, func(a, b acquisition.ProwlarrIndexer) int { return a.ID - b.ID })
	return list, nil
}

func (p *Prowlarr) WebURL(ctx context.Context) (string, error) {
	if err := p.Enter(ctx, ProwlarrWebURL); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.webURL, nil
}

func (p *Prowlarr) Search(ctx context.Context, indexerID int, q acquisition.TorznabQuery) ([]acquisition.TorznabItem, error) {
	if err := p.Enter(ctx, ProwlarrSearch); err != nil {
		return nil, err
	}
	p.mu.Lock()
	ix, ok := p.indexers[indexerID]
	if !ok {
		p.mu.Unlock()
		return nil, acquisition.ErrUnavailable
	}
	ix.queries = append(ix.queries, q)
	delay, failure, results := ix.delay, ix.err, ix.results
	p.mu.Unlock()

	if err := wait(ctx, delay); err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	return results(q), nil
}

func (p *Prowlarr) RSS(ctx context.Context, indexerID int, _ []int) ([]acquisition.TorznabItem, error) {
	if err := p.Enter(ctx, ProwlarrRSS); err != nil {
		return nil, err
	}
	p.mu.Lock()
	ix, ok := p.indexers[indexerID]
	if !ok {
		p.mu.Unlock()
		return nil, acquisition.ErrUnavailable
	}
	delay, failure, rss := ix.delay, ix.err, slices.Clone(ix.rss)
	p.mu.Unlock()

	if err := wait(ctx, delay); err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	return rss, nil
}

func (p *Prowlarr) Download(ctx context.Context, link string) (acquisition.TorrentPayload, error) {
	if err := p.Enter(ctx, ProwlarrDownload); err != nil {
		return acquisition.TorrentPayload{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	payload, ok := p.downloads[link]
	if !ok {
		return acquisition.TorrentPayload{}, acquisition.ErrUnavailable
	}
	return payload, nil
}

func (p *Prowlarr) with(indexerID int, fn func(*indexer)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ix, ok := p.indexers[indexerID]; ok {
		fn(ix)
	}
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return acquisition.ErrTimeout
		}
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
