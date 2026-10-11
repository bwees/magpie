package fakes

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition"
)

const (
	nyaa         = 1
	slowIndexer  = 2
	shortTimeout = 20 * time.Millisecond
	testMagnet   = "magnet:?xt=urn:btih:abc"
	testCategory = "magpie"
)

func TestProwlarrAnswersPerIndexerAndRecordsQueries(t *testing.T) {
	ctx := context.Background()
	p := NewProwlarr()
	p.AddIndexer(acquisition.ProwlarrIndexer{ID: nyaa, Name: "nyaa"})
	p.AddIndexer(acquisition.ProwlarrIndexer{ID: slowIndexer, Name: "slow"})
	p.SetResultsFunc(nyaa, func(q acquisition.TorznabQuery) []acquisition.TorznabItem {
		if q.Imdb == "" {
			return nil
		}
		return []acquisition.TorznabItem{{GUID: "g1"}}
	})

	if got, err := p.Search(ctx, nyaa, acquisition.TorznabQuery{Imdb: "tt1"}); err != nil || len(got) != 1 {
		t.Fatalf("ID search = %v, %v", got, err)
	}
	if got, _ := p.Search(ctx, nyaa, acquisition.TorznabQuery{Query: "Dune"}); len(got) != 0 {
		t.Errorf("title search = %v, want none", got)
	}
	if got := p.Queries(nyaa); len(got) != 2 || got[0].Imdb != "tt1" {
		t.Errorf("queries = %+v", got)
	}

	p.DelayIndexer(slowIndexer, time.Hour)
	timeout, cancel := context.WithTimeout(ctx, shortTimeout)
	defer cancel()
	if _, err := p.Search(timeout, slowIndexer, acquisition.TorznabQuery{}); !errors.Is(err, acquisition.ErrTimeout) {
		t.Errorf("slow indexer err = %v, want ErrTimeout", err)
	}

	failure := &acquisition.ErrIndexerFailed{Code: 100, Message: "bad key"}
	p.FailIndexer(nyaa, failure)
	var indexerErr *acquisition.ErrIndexerFailed
	if _, err := p.Search(ctx, nyaa, acquisition.TorznabQuery{}); !errors.As(err, &indexerErr) {
		t.Errorf("failing indexer err = %v", err)
	}
}

func TestQBittorrentMagnetLifecycle(t *testing.T) {
	ctx := context.Background()
	q := NewQBittorrent()
	req := acquisition.AddRequest{Magnet: testMagnet, Category: testCategory, StopCondition: acquisition.StopMetadataReceived}

	hash, err := q.AddTorrent(ctx, req)
	if err != nil || hash != HashOf(req) {
		t.Fatalf("AddTorrent = %q, %v", hash, err)
	}
	if files, _ := q.Files(ctx, hash); len(files) != 0 {
		t.Errorf("files before metadata = %v", files)
	}
	q.SetFiles(hash, acquisition.TorrentFile{Path: "Show.S01E01.rar", Priority: 1})
	if state, _ := q.Torrent(hash); !state.HasMetadata {
		t.Error("no metadata after SetFiles")
	}

	if err := q.Delete(ctx, true, hash); err != nil {
		t.Fatal(err)
	}
	if q.Started(hash) {
		t.Error("deleted torrent was started")
	}
	if got := q.Deletions(); !slices.Equal(got, []Deletion{{Hash: hash, DeleteFiles: true}}) {
		t.Errorf("deletions = %v", got)
	}
	if _, err := q.Files(ctx, hash); !errors.Is(err, acquisition.ErrTorrentNotFound) {
		t.Errorf("files of deleted torrent err = %v", err)
	}
}

func TestQBittorrentSyncReflectsUpdates(t *testing.T) {
	ctx := context.Background()
	q := NewQBittorrent()
	hash, _ := q.AddTorrent(ctx, acquisition.AddRequest{Torrent: []byte("d4:info"), Stopped: true})
	if err := q.Start(ctx, hash); err != nil {
		t.Fatal(err)
	}
	q.Update(hash, func(tr *acquisition.Torrent) {
		tr.Progress = 1
		tr.Status = acquisition.TorrentStalledUp
	})

	state, err := q.Sync(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Torrents[hash]; got.Progress != 1 || got.Status != acquisition.TorrentStalledUp {
		t.Errorf("synced torrent = %+v", got)
	}
}

func TestQBittorrentRejectsOldWebAPI(t *testing.T) {
	q := NewQBittorrent()
	q.SetVersion(acquisition.WebAPIVersion{Major: 2, Minor: 11})
	if _, err := q.Version(context.Background()); !errors.Is(err, acquisition.ErrUnsupportedVersion) {
		t.Errorf("err = %v, want ErrUnsupportedVersion", err)
	}
}

func TestTrashGuidesServesPerApp(t *testing.T) {
	ctx := context.Background()
	g := NewTrashGuides()
	g.SetCustomFormats(acquisition.TrashRadarr, acquisition.TrashCustomFormat{TrashID: "x", Name: "DV"})

	if got, _ := g.CustomFormats(ctx, acquisition.TrashRadarr); len(got) != 1 {
		t.Errorf("radarr formats = %v", got)
	}
	if got, _ := g.CustomFormats(ctx, acquisition.TrashSonarr); len(got) != 0 {
		t.Errorf("sonarr formats = %v", got)
	}
	g.FailAlways(TrashGuidesQualitySizes, acquisition.ErrUnavailable)
	if _, err := g.QualitySizes(ctx, acquisition.TrashRadarr); !errors.Is(err, acquisition.ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}
