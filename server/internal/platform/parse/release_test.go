// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"magpie/internal/domain"
)

func mustParseRelease(t *testing.T, name string, kind domain.MediaKind) Release {
	t.Helper()
	r, err := ParseRelease(name, kind)
	if err != nil {
		t.Fatalf("ParseRelease(%q): %v", name, err)
	}
	return r
}

func TestRevisions(t *testing.T) {
	cases := []struct {
		name string
		want Revision
	}{
		{"Series.Title.S01E05.720p.HDTV.x264-GRP", Revision{Version: 1}},
		{"Series.Title.S01E05.PROPER.720p.HDTV.x264-GRP", Revision{Version: 2}},
		{"Series.Title.S01E05.REPACK.720p.HDTV.x264-GRP", Revision{Version: 2, Repack: true}},
		{"Series.Title.S01E05.REPACK2.720p.HDTV.x264-GRP", Revision{Version: 3, Repack: true}},
		{"Series.Title.S01E05.REAL.720p.HDTV.x264-GRP", Revision{Version: 1, Real: 1}},
		{"Series.Title.S01E05.REAL.PROPER.720p.HDTV.x264-GRP", Revision{Version: 2, Real: 1}},
		{"[SubGroup] Series Title - 05v2 [720p]", Revision{Version: 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mustParseRelease(t, c.name, domain.MediaKindSeries)
			requireEqual(t, "revision", r.Revision, c.want)
			requireEqual(t, "quality revision", r.Quality.Revision, c.want.Version)
		})
	}
}

func TestMultiEpisode(t *testing.T) {
	cases := []struct {
		name     string
		episodes []int
	}{
		{"Series.Title.S01E01E02.1080p.WEB-DL.DD5.1.H264-NTb", []int{1, 2}},
		{"Series.Title.S01E01-E03.1080p.WEB-DL.DD5.1.H264-NTb", []int{1, 2, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mustParseRelease(t, c.name, domain.MediaKindSeries)
			requireEqual(t, "title", r.Title, "Series Title")
			requireEqual(t, "season", r.Season, 1)
			requireInts(t, "episodes", r.Episodes, c.episodes)
			requireEqual(t, "full season", r.FullSeason, false)
		})
	}
}

func TestMultiSeasonPack(t *testing.T) {
	r := mustParseRelease(t, "Series.Title.S01-S03.1080p.BluRay.x264-GRP", domain.MediaKindSeries)
	requireInts(t, "seasons", r.Seasons, []int{1, 2, 3})
	requireEqual(t, "season", r.Season, 1)
	requireEqual(t, "multi season", r.MultiSeason, true)
	requireEqual(t, "full season", r.FullSeason, true)
	requireInts(t, "episodes", r.Episodes, nil)
}

func TestSeasonPack(t *testing.T) {
	r := mustParseRelease(t, "Series.Title.S02.1080p.BluRay.x264-GRP", domain.MediaKindSeries)
	requireInts(t, "seasons", r.Seasons, []int{2})
	requireEqual(t, "multi season", r.MultiSeason, false)
	requireEqual(t, "full season", r.FullSeason, true)
	requireInts(t, "episodes", r.Episodes, nil)
}

func TestSample(t *testing.T) {
	cases := []struct {
		name   string
		kind   domain.MediaKind
		sample bool
	}{
		{"Series.Title.S01E05.720p.HDTV.x264-GRP.sample", domain.MediaKindSeries, true},
		{"Series.Title.S01E05.720p.HDTV.x264-GRP-sample.mkv", domain.MediaKindSeries, true},
		{"Movie.Title.2019.1080p.BluRay.x264-GRP-Sample", domain.MediaKindMovie, true},
		{"Movie.Title.2019.1080p.BluRay.x264-GRP", domain.MediaKindMovie, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mustParseRelease(t, c.name, c.kind)
			requireEqual(t, "sample", r.Sample, c.sample)
			requireEqual(t, "group", r.Group, "GRP")
		})
	}
}

func TestTitlesContainingYears(t *testing.T) {
	cases := []struct {
		name  string
		kind  domain.MediaKind
		title string
		year  int
	}{
		{"1917.2019.2160p.UHD.BluRay.REMUX.HDR.HEVC.Atmos-EPSiLON", domain.MediaKindMovie, "1917", 2019},
		{"Blade.Runner.2049.2017.1080p.WEB-DL.DD5.1.H264-FGT", domain.MediaKindMovie, "Blade Runner 2049", 2017},
		{"2001.A.Space.Odyssey.1968.1080p.BluRay.x264-GRP", domain.MediaKindMovie, "2001 A Space Odyssey", 1968},
		{"Series.Title.2012.S02E17.720p.HDTV.X264-DIMENSION", domain.MediaKindSeries, "Series Title", 2012},
		{"1923.S01E01.1080p.WEB.h264-GRP", domain.MediaKindSeries, "1923", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mustParseRelease(t, c.name, c.kind)
			requireEqual(t, "title", r.Title, c.title)
			requireEqual(t, "year", r.Year, c.year)
		})
	}
}

// The parser reports the year in the name as-is; matching 2019 against a 2020 title is the caller's job.
func TestYearIsNotAdjusted(t *testing.T) {
	r := mustParseRelease(t, "Movie.Title.2019.1080p.BluRay.x264-GRP", domain.MediaKindMovie)
	requireEqual(t, "year", r.Year, 2019)
}

func TestMovieRelease(t *testing.T) {
	r := mustParseRelease(t, "Movie.Title.AKA.Other.Title.2019.Directors.Cut.German.DL.1080p.BluRay.x264-GRP", domain.MediaKindMovie)
	requireEqual(t, "title", r.Title, "Movie Title AKA Other Title")
	requireEqual(t, "aliases", slices.Equal(r.Aliases, []string{"Movie Title", "Other Title"}), true)
	requireEqual(t, "edition", r.Edition, "Directors Cut")
	requireEqual(t, "languages", slices.Equal(r.Languages, []domain.Language{langGerman}), true)
	requireEqual(t, "original language", r.OriginalLanguage, true)
	requireEqual(t, "quality", r.Quality, mustQuality(QualityBluray1080p).Quality(defaultRevisionVersion))
}

func TestDailyRelease(t *testing.T) {
	r := mustParseRelease(t, "Series.Title.2018.04.28.720p.HDTV.x264-GRP", domain.MediaKindSeries)
	requireEqual(t, "air date", r.AirDate, "2018-04-28")
	requireInts(t, "episodes", r.Episodes, nil)
}

func TestSpecial(t *testing.T) {
	r := mustParseRelease(t, "Series.Title.S00E05.Special.Episode.720p.HDTV.x264-GRP", domain.MediaKindSeries)
	requireEqual(t, "season", r.Season, 0)
	requireInts(t, "episodes", r.Episodes, []int{5})
}

func TestSeriesQualityUsesMagpieIDs(t *testing.T) {
	r := mustParseRelease(t, "Series.Title.S01E01.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-GRP", domain.MediaKindSeries)
	requireEqual(t, "quality", r.Quality.ID, int(QualityRemux1080p))
	requireEqual(t, "quality name", r.Quality.Name, "Remux-1080p")
}

func TestUnableToParse(t *testing.T) {
	for _, kind := range []domain.MediaKind{domain.MediaKindSeries, domain.MediaKindMovie} {
		_, err := ParseRelease("abc", kind)
		var unable *UnableToParse
		if !errors.As(err, &unable) {
			t.Fatalf("%s: err = %v, want UnableToParse", kind, err)
		}
		requireEqual(t, "name", unable.Name, "abc")
	}
}

func TestOverlongNameIsUnparsed(t *testing.T) {
	name := strings.Repeat("[a] ", maxNameLength)
	for _, kind := range []domain.MediaKind{domain.MediaKindSeries, domain.MediaKindMovie} {
		var unable *UnableToParse
		if _, err := ParseRelease(name, kind); !errors.As(err, &unable) {
			t.Errorf("%s: err = %v, want UnableToParse", kind, err)
		}
	}
}

func TestUnknownKind(t *testing.T) {
	_, err := ParseRelease("Series.Title.S01E01.720p.HDTV.x264-GRP", domain.MediaKind("MUSIC"))
	var unable *UnableToParse
	if err == nil || errors.As(err, &unable) {
		t.Fatalf("err = %v, want an unknown-kind error", err)
	}
}

func TestParseFile(t *testing.T) {
	r, err := ParseFile("/downloads/Series.Title.S02.1080p.WEB-DL-GRP/Season 2/05 - Episode Name.mkv", domain.MediaKindSeries)
	if err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "season", r.Season, 2)
	requireInts(t, "episodes", r.Episodes, []int{5})

	m, err := ParseFile(`D:\Movies\Movie.Title.2019.1080p.BluRay.x264-GRP\abc.mkv`, domain.MediaKindMovie)
	if err != nil {
		t.Fatal(err)
	}
	requireEqual(t, "title", m.Title, "Movie Title")
	requireEqual(t, "group", m.Group, "GRP")
}

func TestQualities(t *testing.T) {
	const radarrQualityCount = 30
	all := Qualities()
	requireEqual(t, "count", len(all), radarrQualityCount)
	for _, d := range all {
		got, ok := QualityByID(d.ID)
		requireEqual(t, "found", ok, true)
		requireEqual(t, "definition", got, d)
		if d.Name == "" || d.Source == "" || d.Modifier == "" {
			t.Errorf("incomplete definition %+v", d)
		}
	}
	if _, ok := QualityByID(11); ok {
		t.Error("quality 11 is unused in Radarr and must not exist")
	}
}

func TestSonarrToMagpieQuality(t *testing.T) {
	cases := map[int]QualityID{
		13: QualityBluray480p,
		22: QualityBluray576p,
		20: QualityRemux1080p,
		21: QualityRemux2160p,
		0:  QualityUnknown,
		1:  QualitySDTV,
		7:  QualityBluray1080p,
		19: QualityBluray2160p,
	}
	for sonarr, want := range cases {
		requireEqual(t, "magpie ID", SonarrToMagpieQuality(sonarr), want)
	}
}
