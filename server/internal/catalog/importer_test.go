package catalog_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
)

const (
	importedTvdb  = "123"
	refreshedTvdb = "456"
	sonarrSource  = domain.SourceKey("sonarr:12")
	dvdGroupID    = "dvd-order"
)

var importedAt = time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC)

func TestServicePassesTheImporterContract(t *testing.T) {
	contracttest.RunImporter(t, func(t *testing.T, w contracttest.ImporterWorld) api.Importer {
		h := newHarness(t)
		for _, id := range w.Movies {
			h.tmdb.PutMovie(catalog.TmdbMovie{ID: id, Title: "Movie"})
		}
		for _, s := range w.Series {
			var seasons []catalog.TmdbSeason
			for number, count := range s.Seasons {
				seasons = append(seasons, tmdbSeason(number, count, startOfTest.AddDate(-1, 0, 0)))
			}
			slices.SortFunc(seasons, func(a, b catalog.TmdbSeason) int { return a.Number - b.Number })
			putSeries(h, severanceSeries(seasons...), seasons...)
		}
		return h.service
	})
}

// idSource is where a title's TVDB and IMDb IDs came from.
func (h *harness) idSource(title domain.TitleID) string {
	h.t.Helper()
	var source string
	err := h.db.Read(context.Background()).Table("catalog_titles").Select("external_id_source").Where("id = ?", title).Take(&source).Error
	if err != nil {
		h.t.Fatal(err)
	}
	return source
}

func importSeries(h *harness, in api.ImportTitle) api.ImportedTitles {
	h.t.Helper()
	got, err := h.service.ImportTitles(context.Background(), []api.ImportTitle{in})
	if err != nil {
		h.t.Fatal(err)
	}
	return got
}

// Seed catalog #11.
func TestImportedTvdbIDIsKeptUntilTmdbHasOne(t *testing.T) {
	h := newHarness(t)
	seasons := airedSeasons()
	putSeries(h, severanceSeries(seasons...), seasons...)
	in := api.ImportTitle{
		Source: sonarrSource, Kind: domain.MediaKindSeries, Tmdb: severanceTmdb, Monitored: true,
		External: domain.ExternalIDs{Tmdb: severanceTmdb, Tvdb: ptr(importedTvdb)}, AddedAt: importedAt,
	}

	first := importSeries(h, in)
	id := first.Titles[sonarrSource]
	title, err := h.repo.Title(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if title.External.Tvdb == nil || *title.External.Tvdb != importedTvdb || h.idSource(id) != catalog.ExternalIDSourceImport || title.Origin != domain.OriginMigration {
		t.Fatalf("title = %+v, want tvdb %s from IMPORT with origin MIGRATION", title, importedTvdb)
	}
	added := events[api.TitleAdded](h)
	if len(added) != 1 || added[0].Origin != domain.OriginMigration || added[0].Title != id {
		t.Fatalf("TitleAdded = %+v, want one with origin MIGRATION", added)
	}

	again := importSeries(h, in)
	if again.Titles[sonarrSource] != id || len(events[api.TitleAdded](h)) != 1 {
		t.Fatalf("second import gave %s and %d TitleAdded, want %s and 1", again.Titles[sonarrSource], len(events[api.TitleAdded](h)), id)
	}

	series := severanceSeries(seasons...)
	series.External.Tvdb = ptr(refreshedTvdb)
	h.tmdb.PutSeries(series)
	if err := h.service.RefreshTitle(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	title, err = h.repo.Title(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if title.External.Tvdb == nil || *title.External.Tvdb != refreshedTvdb || h.idSource(id) != catalog.ExternalIDSourceTMDB {
		t.Fatalf("after refresh = %+v, want tvdb %s from TMDB", title.External, refreshedTvdb)
	}
}

func TestImportedSeriesKeepsEpisodeMonitoringAndIsNotOfferedForSearch(t *testing.T) {
	h := newHarness(t)
	seasons := airedSeasons()
	putSeries(h, severanceSeries(seasons...), seasons...)
	profile := domain.NewID[domain.ProfileID]()
	in := api.ImportTitle{
		Source: sonarrSource, Kind: domain.MediaKindSeries, Tmdb: severanceTmdb, Profile: profile, Monitored: true,
		AddedAt: importedAt,
		Episodes: []api.ImportEpisode{
			{Source: "sonarr:episode:1", Season: 1, Number: 1, Monitored: true},
			{Source: "sonarr:episode:2", Season: 1, Number: 2, Monitored: false},
			{Source: "sonarr:episode:4", Season: 2, Number: 1, Monitored: false},
		},
	}

	got := importSeries(h, in)
	view := h.view(got.Titles[sonarrSource])
	if view.Profile != profile || !view.AddedAt.Equal(importedAt) {
		t.Fatalf("title = profile %s added %s, want %s and %s", view.Profile, view.AddedAt, profile, importedAt)
	}
	monitoredIDs := episodeIDs(h.episodes(view.ID), monitored)
	if !slices.Equal(monitoredIDs, []domain.EpisodeID{got.Episodes["sonarr:episode:1"]}) {
		t.Fatalf("monitored = %v, want only S01E01 (%s)", monitoredIDs, got.Episodes["sonarr:episode:1"])
	}
	if flags := seasonMonitoring(view); !flags[1] || flags[2] {
		t.Fatalf("seasons = %v, want only season 1 monitored", flags)
	}
	added := events[api.TitleAdded](h)
	if len(added) != 1 || !slices.Equal(added[0].Monitored, monitoredIDs) || added[0].Profile != profile {
		t.Fatalf("TitleAdded = %+v, want monitored %v on profile %s", added, monitoredIDs, profile)
	}
}

func TestImportedSeriesUsesTheConfirmedEpisodeGroup(t *testing.T) {
	h := newHarness(t)
	seasons := airedSeasons()
	putSeries(h, severanceSeries(seasons...), seasons...)
	h.tmdb.PutEpisodeGroup(severanceTmdb, catalog.TmdbEpisodeGroup{
		ID: dvdGroupID, Type: catalog.TmdbGroupDVD,
		Groups: []catalog.TmdbEpisodeGroupSeason{{Order: 1, Episodes: []catalog.TmdbEpisodeGroupEntry{
			{Episode: 102, Order: 0}, {Episode: 101, Order: 1},
		}}},
	})
	in := api.ImportTitle{
		Source: sonarrSource, Kind: domain.MediaKindSeries, Tmdb: severanceTmdb, Monitored: true,
		EpisodeGroup: ptr(dvdGroupID),
		Episodes:     []api.ImportEpisode{{Source: "sonarr:episode:1", Season: 1, Number: 1, Monitored: true}},
	}

	got := importSeries(h, in)
	episode, err := h.repo.Episode(context.Background(), got.Episodes["sonarr:episode:1"])
	if err != nil {
		t.Fatal(err)
	}
	if episode.AiredSeason != 1 || episode.AiredNumber != 2 || !episode.Monitored {
		t.Fatalf("group S01E01 = %+v, want aired S01E02, monitored", episode)
	}
	if view := h.view(got.Titles[sonarrSource]); view.EpisodeGroupID != dvdGroupID {
		t.Fatalf("episode group = %q, want %q", view.EpisodeGroupID, dvdGroupID)
	}
}
