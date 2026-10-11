package fakes

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/domain"
)

const (
	duneTmdb   domain.TmdbID = 693134
	unknownID  domain.TmdbID = 1
	duneTitle                = "Dune: Part Two"
	posterPath               = "/poster.jpg"
	language                 = "en-US"
	extraItems               = 5
)

var posterBytes = []byte("jpeg")

func TestTMDBServesStoredMoviesAndImages(t *testing.T) {
	ctx := context.Background()
	f := NewTMDB()
	f.PutMovie(catalog.TmdbMovie{ID: duneTmdb, Title: duneTitle, Images: catalog.TmdbImages{Poster: posterPath}})
	f.PutImage(posterPath, posterBytes)

	if m, err := f.Movie(ctx, duneTmdb, language); err != nil || m.Title != duneTitle {
		t.Fatalf("Movie = %+v, %v", m, err)
	}
	if _, err := f.Movie(ctx, unknownID, language); !errors.Is(err, catalog.ErrTmdbNotFound) {
		t.Errorf("unknown movie err = %v", err)
	}
	img, err := f.Image(ctx, posterPath, catalog.TmdbImageOriginal)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = img.Close() }()
	if data, _ := io.ReadAll(img); string(data) != string(posterBytes) {
		t.Errorf("image = %q", data)
	}
}

func TestTMDBSearchPages(t *testing.T) {
	ctx := context.Background()
	f := NewTMDB()
	for i := range SearchPageSize + extraItems {
		f.PutMovie(catalog.TmdbMovie{ID: domain.TmdbID(i + 1), Title: duneTitle})
	}

	page, err := f.Search(ctx, "dune", 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalResults != SearchPageSize+extraItems || page.TotalPages != 2 || len(page.Items) != extraItems {
		t.Errorf("page 2 = %+v", page)
	}
}

func TestTMDBChangesSince(t *testing.T) {
	f := NewTMDB()
	now := time.Now()
	f.MarkChanged(domain.MediaKindMovie, duneTmdb, now)
	f.MarkChanged(domain.MediaKindMovie, unknownID, now.Add(-48*time.Hour))

	ids, err := f.Changes(context.Background(), domain.MediaKindMovie, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != duneTmdb {
		t.Errorf("Changes = %v", ids)
	}
}

func TestTMDBScriptedErrors(t *testing.T) {
	f := NewTMDB()
	f.FailNext(TMDBSeries, catalog.ErrTmdbUnauthorized)

	if _, err := f.Series(context.Background(), duneTmdb, language); !errors.Is(err, catalog.ErrTmdbUnauthorized) {
		t.Errorf("err = %v, want ErrTmdbUnauthorized", err)
	}
	if _, err := f.Series(context.Background(), duneTmdb, language); !errors.Is(err, catalog.ErrTmdbNotFound) {
		t.Errorf("second call err = %v, want ErrTmdbNotFound", err)
	}
}
