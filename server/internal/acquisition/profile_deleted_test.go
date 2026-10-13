package acquisition_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/parse"
)

func (f searchFixture) defaultProfile(t *testing.T) domain.ProfileID {
	t.Helper()
	id, err := acquisition.DefaultProfileKey.Get(context.Background(), f.store)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// onMissingProfile points the title at a profile that doesn't exist, as an older release could leave it.
func (f searchFixture) onMissingProfile() {
	f.title.Profile = domain.NewID[domain.ProfileID]()
	f.catalog.PutTitle(f.title)
}

// Seed acquisition #52.
func TestDeletingAProfilePublishesProfileDeletedNamingTheDefault(t *testing.T) {
	f := newSearchFixture(t)
	uhd := f.presetProfile(t, api.Preset2160p)

	if err := f.quality.DeleteProfile(context.Background(), uhd); err != nil {
		t.Fatal(err)
	}

	deleted := payload[api.ProfileDeleted](t, f.events.waitFor(t, api.TypeProfileDeleted, 1)[0])
	if want := (api.ProfileDeleted{Profile: uhd, ReplacedBy: f.defaultProfile(t)}); deleted != want {
		t.Errorf("ProfileDeleted = %+v, want %+v", deleted, want)
	}
}

func TestDeletingTheDefaultProfilePublishesNothing(t *testing.T) {
	f := newSearchFixture(t)

	if err := f.quality.DeleteProfile(context.Background(), f.defaultProfile(t)); err == nil {
		t.Fatal("deleted the default profile")
	}
	if got := f.events.ofType(api.TypeProfileDeleted); len(got) != 0 {
		t.Errorf("ProfileDeleted = %+v, want none", got)
	}
}

func TestASearchForATitleOnAMissingProfileUsesTheDefault(t *testing.T) {
	f := newSearchFixture(t)
	f.onMissingProfile()
	f.indexer(t, fastID, "Fast", idCaps, fastAnswer, release(acceptedName))

	started := f.start(t)

	if started.Profile.ID != f.defaultProfile(t) {
		t.Errorf("search profile = %s, want the default %s", started.Profile.ID, f.defaultProfile(t))
	}
	if done := f.completed(t); done.Accepted != 1 {
		t.Errorf("SearchCompleted = %+v, want the 1080p release accepted", done)
	}
}

func TestRSSForATitleOnAMissingProfileUsesTheDefault(t *testing.T) {
	f := newSearchFixture(t)
	f.onMissingProfile()
	f.indexer(t, fastID, "Fast", idCaps, 0)
	f.prowlarr.SetRSS(fastID, published(acceptedName, time.Hour, time.Now()))

	if err := f.searches.SyncFeed(context.Background(), fastID); err != nil {
		t.Fatal(err)
	}
	f.settle(t)

	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != acceptedName {
		t.Errorf("grabs = %+v, want %s grabbed under the default profile", grabs, acceptedName)
	}
}

func TestAFileOfATitleOnAMissingProfileIsScoredUnderTheDefault(t *testing.T) {
	f := newSearchFixture(t)
	f.onMissingProfile()

	f.deliverScores(t, f.fileImported(bluray1080p, parse.QualityBluray1080p, domain.OriginDownload))

	if met := f.cutoffMet(t); len(met) != 1 {
		t.Errorf("CutoffMet = %+v, want the file at the 1080p default's cutoff", met)
	}
}

func TestSearchingATitleWithNoProfileLeftIsAProfileNotFound(t *testing.T) {
	f := newSearchFixture(t)
	f.onMissingProfile()
	if err := acquisition.DefaultProfileKey.Set(context.Background(), f.store, domain.NewID[domain.ProfileID]()); err != nil {
		t.Fatal(err)
	}

	rec := do(serveSearches(f, admin()), http.MethodPost, "/api/acquisition/searches", startBody(f.title.ID))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("start = %d %s, want 404", rec.Code, rec.Body)
	}
	if got := decode[problemBody](t, rec); got.Type != string(acquisition.TypeProfileNotFound) || got.Detail == "" {
		t.Errorf("problem = %+v, want a profile-not-found with a detail", got)
	}
}
