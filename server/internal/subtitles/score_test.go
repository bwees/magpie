package subtitles

import (
	"testing"

	"magpie/internal/domain"
)

const (
	showTmdb     domain.TmdbID = 1399
	showYear                   = 2011
	showRelease                = "Show.2011.S01E02.1080p.WEB-DL.DDP5.1.H.264-GRP"
	movieTmdb    domain.TmdbID = 438631
	movieYear                  = 2021
	movieRelease               = "Dune.2021.1080p.WEB-DL.DDP5.1.H.264-FLUX"

	episodeMax = 360
	movieMax   = 180
)

var (
	episodeVideo = video{
		Kind: domain.MediaKindSeries, Tmdb: showTmdb, Imdb: "tt0944947", Year: showYear, Season: 1, Episode: 2,
		Release: traitsOf(showRelease, domain.MediaKindSeries),
	}
	movieVideo = video{
		Kind: domain.MediaKindMovie, Tmdb: movieTmdb, Year: movieYear,
		Release: traitsOf(movieRelease, domain.MediaKindMovie),
	}
)

func TestMaxScoreIsHashPlusOne(t *testing.T) {
	if got := episodeWeights.max(); got != episodeMax || got != episodeWeights.hash+1 {
		t.Errorf("episode max = %d, want %d", got, episodeMax)
	}
	if got := movieWeights.max(); got != movieMax || got != movieWeights.hash+1 {
		t.Errorf("movie max = %d, want %d", got, movieMax)
	}
}

func TestHashMatchWithMatchingIdentityIsPerfect(t *testing.T) {
	c := Candidate{HashMatch: true, Tmdb: showTmdb, Season: 1, Episode: 2, ReleaseName: "something else"}
	if got := score(episodeVideo, c, false); got.Points != episodeMax {
		t.Errorf("score = %+v, want %d", got, episodeMax)
	}
}

// Seed 3: a hash match for the wrong episode doesn't count.
func TestHashForTheWrongEpisodeIsNotCounted(t *testing.T) {
	right := Candidate{Tmdb: showTmdb, Season: 1, Episode: 3, ReleaseName: showRelease}
	wrong := right
	wrong.HashMatch = true
	withHash, without := score(episodeVideo, wrong, false), score(episodeVideo, right, false)
	if withHash != without {
		t.Errorf("hash-matched wrong episode scores %+v, want the same as without the hash %+v", withHash, without)
	}
	if withHash.Points >= episodeWeights.hash {
		t.Errorf("score %d includes the hash weight", withHash.Points)
	}
}

func TestReleaseAttributesComeFromTheReleaseName(t *testing.T) {
	c := Candidate{Tmdb: showTmdb, Season: 1, Episode: 2, ReleaseName: showRelease}
	// series 160 + year 90 + season 30 + episode 30 + source 25 + group 20 + audio, video, resolution, HI.
	const want = 160 + 90 + 30 + 30 + 25 + 20 + 4
	if got := score(episodeVideo, c, false); got.Points != want {
		t.Errorf("score = %d, want %d", got.Points, want)
	}
}

func TestMovieScoreAndThreshold(t *testing.T) {
	c := Candidate{Tmdb: movieTmdb, ReleaseName: "Dune.2021.1080p.WEB-DL.x264-OTHER"}
	// title 60 + year 40 + source 30 + video codec, resolution, HI.
	const want = 60 + 40 + 30 + 3
	got := score(movieVideo, c, false)
	if got.Points != want || got.Max != movieMax {
		t.Fatalf("score = %+v, want %d/%d", got, want, movieMax)
	}
	if !got.Meets(DefaultMinScoreMovie) || got.Meets(DefaultMinScoreEpisode) {
		t.Errorf("%d%% should meet %d%% and miss %d%%", got.Percent(), DefaultMinScoreMovie, DefaultMinScoreEpisode)
	}
	if !got.Upgradable() {
		t.Errorf("%+v should stay upgradable", got)
	}
}

func TestRankPrefersNonHIUnlessHIIsFine(t *testing.T) {
	hi := Candidate{FileID: 1, Language: "en", Tmdb: movieTmdb, HearingImpaired: true}
	plain := Candidate{FileID: 2, Language: "en", Tmdb: movieTmdb}
	machine := Candidate{FileID: 3, Language: "en", Tmdb: movieTmdb, HashMatch: true, MachineTranslated: true}
	forced := Candidate{FileID: 4, Language: "en", Tmdb: movieTmdb, Forced: true}
	blocked := Candidate{FileID: 5, Language: "en", Tmdb: movieTmdb, HashMatch: true}
	all := []Candidate{hi, plain, machine, forced, blocked}

	ranked := rank(movieVideo, wantedTrack{Language: "en"}, all, map[int64]bool{blocked.FileID: true})
	if len(ranked) != 2 || ranked[0].FileID != plain.FileID {
		t.Fatalf("ranked = %+v, want plain first and no machine, forced or blocked results", ranked)
	}
	ranked = rank(movieVideo, wantedTrack{Language: "en", HearingImpairedOK: true}, all, nil)
	if ranked[0].FileID != blocked.FileID || ranked[1].Score != ranked[2].Score {
		t.Errorf("with HI fine: %+v, want the hash match first and HI tied with plain", ranked)
	}
}

func TestSidecarPath(t *testing.T) {
	const video = "/tv/Show (2011)/Season 01/Show S01E02.mkv"
	for _, tc := range []struct {
		forced, sdh bool
		want        string
	}{
		{false, false, "/tv/Show (2011)/Season 01/Show S01E02.en.srt"},
		{true, false, "/tv/Show (2011)/Season 01/Show S01E02.en.forced.srt"},
		{false, true, "/tv/Show (2011)/Season 01/Show S01E02.en.sdh.srt"},
	} {
		if got := sidecarPath(video, "en", tc.forced, tc.sdh); got != tc.want {
			t.Errorf("sidecarPath(forced=%v, sdh=%v) = %q, want %q", tc.forced, tc.sdh, got, tc.want)
		}
	}
}
