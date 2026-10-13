package catalog_test

import (
	"context"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/domain"
)

const (
	anoraTmdb domain.TmdbID    = 1064213
	profileR  domain.ProfileID = "R"
)

func anora() catalog.TmdbMovie {
	movie := conclave()
	movie.ID, movie.Title, movie.OriginalTitle = anoraTmdb, "Anora", "Anora"
	return movie
}

func (h *harness) deliverProfileDeleted(m acquisitionapi.ProfileDeleted) {
	h.t.Helper()
	if err := h.handle(catalog.NewProfileDeletedHandler(h.service), m); err != nil {
		h.t.Fatalf("handling %s: %v", m.Type(), err)
	}
}

func (h *harness) titleProfile(id domain.TitleID) domain.ProfileID {
	h.t.Helper()
	title, err := h.repo.Title(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return title.Profile
}

// Seed catalog #21.
func TestProfileDeletedMovesItsTitlesToTheReplacement(t *testing.T) {
	h := newHarness(t)
	moved := addMovie(t, h, conclave())
	h.tmdb.PutMovie(anora())
	other := approved(profileQ)
	other.Tmdb = anoraTmdb
	h.deliver(other)
	h.runJobs()
	kept, err := h.repo.TitleByTmdb(context.Background(), domain.MediaKindMovie, anoraTmdb)
	if err != nil {
		t.Fatal(err)
	}
	deleted := acquisitionapi.ProfileDeleted{Profile: profileP, ReplacedBy: profileR}

	h.deliverProfileDeleted(deleted)
	h.deliverProfileDeleted(deleted)

	if got := h.titleProfile(moved.ID); got != profileR {
		t.Errorf("moved title profile = %s, want %s", got, profileR)
	}
	if got := h.titleProfile(kept.ID); got != profileQ {
		t.Errorf("other title profile = %s, want %s kept", got, profileQ)
	}
	changed := events[api.ProfileChanged](h)
	if len(changed) != 1 || changed[0] != (api.ProfileChanged{Title: moved.ID, From: profileP, To: profileR}) {
		t.Errorf("ProfileChanged = %+v, want one for the moved title", changed)
	}
}
