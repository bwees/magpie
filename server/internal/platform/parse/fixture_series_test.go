// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"magpie/internal/platform/parse/fixtures"
)

func mustParseEpisode(t *testing.T, title string) *episodeInfo {
	t.Helper()
	r := parseEpisodeTitle(title, now())
	if r == nil {
		t.Fatalf("parseEpisodeTitle(%q) = nil", title)
	}
	return r
}

// checkSingleEpisode is the body shared by Sonarr's single-episode test methods.
func checkSingleEpisode(t *testing.T, r *episodeInfo, title string, season, episode int) {
	t.Helper()
	requireInts(t, "episodes", r.episodeNumbers, []int{episode})
	requireEqual(t, "season", r.seasonNumber(), season)
	requireEqual(t, "series title", r.seriesTitle, title)
	requireNoAbsolute(t, r)
	requireEqual(t, "full season", r.fullSeason, false)
}

func singleEpisode(t *testing.T, args fixtures.Args) {
	r := mustParseEpisode(t, argString(args, 0))
	checkSingleEpisode(t, r, argString(args, 1), argInt(args, 2), argInt(args, 3))
}

func checkSeasonPack(t *testing.T, args fixtures.Args) *episodeInfo {
	t.Helper()
	r := mustParseEpisode(t, argString(args, 0))
	requireEqual(t, "series title", r.seriesTitle, argString(args, 1))
	requireInts(t, "episodes", r.episodeNumbers, nil)
	requireNoAbsolute(t, r)
	return r
}

func dailyAirDate(year, month, day int) string {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC).Format(AirDateLayout)
}

func checkDaily(t *testing.T, args fixtures.Args) *episodeInfo {
	t.Helper()
	r := mustParseEpisode(t, argString(args, 0))
	requireEqual(t, "series title", r.seriesTitle, argString(args, 1))
	requireEqual(t, "air date", r.airDate, dailyAirDate(argInt(args, 2), argInt(args, 3), argInt(args, 4)))
	requireInts(t, "episodes", r.episodeNumbers, nil)
	requireNoAbsolute(t, r)
	requireEqual(t, "full season", r.fullSeason, false)
	return r
}

func expandDate(template string, year int, month, day string) string {
	return strings.NewReplacer("{year}", fmt.Sprint(year), "{month}", month, "{day}", day).Replace(template)
}

func talkShowTitle(airDate time.Time) string {
	return fmt.Sprintf("A.Talk.Show.%s.1080p.WEB-DL.H264-Sonarr", airDate.Format("2006.01.02"))
}

func requireUnparsed(t *testing.T, args fixtures.Args) {
	if r := parseEpisodeTitle(argString(args, 0), now()); r != nil {
		t.Errorf("parseEpisodeTitle(%q) = %+v, want nil", argString(args, 0), r)
	}
}

// seriesName is Sonarr's Parser.ParseSeriesName.
func seriesName(title string) string {
	if r := parseEpisodeTitle(title, now()); r != nil {
		return r.seriesTitle
	}
	return cleanSeriesTitle(title)
}

func checkReleaseGroup(rules releaseGroupRules) checker {
	return func(t *testing.T, args fixtures.Args) {
		requireEqual(t, "release group", parseReleaseGroup(argString(args, 0), rules), argString(args, 1))
	}
}

// xmlCleanUnicode is Sonarr's XmlCleaner.ReplaceUnicode: drop characters XML can't carry.
func xmlCleanUnicode(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r >= 0x20 && r <= 0xD7FF, r >= 0xE000 && r <= 0xFFFD:
			return r
		}
		return -1
	}, s)
}

func seriesSuites() []fixtureSuite {
	releaseGroup := checkReleaseGroup(seriesReleaseGroupRules)
	return []fixtureSuite{
		{
			file: fixtures.SonarrSingleEpisodeParserFixture,
			checks: map[string]checker{
				"should_parse_single_episode":               singleEpisode,
				"should_parse_japanese_variety_show_format": singleEpisode,
				"should_parse_decimal_number_as_special": func(t *testing.T, args fixtures.Args) {
					singleEpisode(t, args)
					requireEqual(t, "special", mustParseEpisode(t, argString(args, 0)).special, true)
				},
				"should_parse_split_episode": func(t *testing.T, args fixtures.Args) {
					singleEpisode(t, args)
					requireEqual(t, "split episode", mustParseEpisode(t, argString(args, 0)).splitEpisode, true)
				},
			},
		},
		{
			file: fixtures.SonarrMultiEpisodeParserFixture,
			checks: map[string]checker{
				"should_parse_multiple_episodes": func(t *testing.T, args fixtures.Args) {
					r := mustParseEpisode(t, argString(args, 0))
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 2))
					requireInts(t, "episodes", r.episodeNumbers, argInts(args, 3))
					requireEqual(t, "series title", r.seriesTitle, argString(args, 1))
					requireNoAbsolute(t, r)
					requireEqual(t, "full season", r.fullSeason, false)
				},
			},
		},
		{
			file: fixtures.SonarrSeasonParserFixture,
			checks: map[string]checker{
				"should_parse_full_season_release": func(t *testing.T, args fixtures.Args) {
					r := checkSeasonPack(t, args)
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 2))
					requireEqual(t, "full season", r.fullSeason, true)
				},
				"should_parse_season_extras": func(t *testing.T, args fixtures.Args) {
					r := checkSeasonPack(t, args)
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 2))
					requireEqual(t, "full season", r.fullSeason, true)
					requireEqual(t, "season extra", r.seasonExtra, true)
				},
				"should_parse_season_subpack": func(t *testing.T, args fixtures.Args) {
					r := checkSeasonPack(t, args)
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 2))
					requireEqual(t, "full season", r.fullSeason, true)
					requireEqual(t, "season extra", r.seasonExtra, true)
				},
				"should_parse_partial_season_release": func(t *testing.T, args fixtures.Args) {
					r := checkSeasonPack(t, args)
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 2))
					requireEqual(t, "full season", r.fullSeason, false)
					requireEqual(t, "partial season", r.partialSeason, true)
					requireEqual(t, "season part", r.seasonPart, argInt(args, 3))
				},
				"should_parse_multi_season_release": func(t *testing.T, args fixtures.Args) {
					r := checkSeasonPack(t, args)
					requireEqual(t, "full season", r.fullSeason, true)
					requireEqual(t, "partial season", r.partialSeason, false)
					requireEqual(t, "multi season", len(r.seasonNumbers) > 1, true)
					requireInts(t, "seasons", r.seasonNumbers, argInts(args, 2))
				},
				"should_not_be_multi_season_for_a_single_season_release": func(t *testing.T, args fixtures.Args) {
					r := mustParseEpisode(t, argString(args, 0))
					requireInts(t, "seasons", r.seasonNumbers, []int{argInt(args, 1)})
				},
			},
		},
		{
			file: fixtures.SonarrDailyEpisodeParserFixture,
			checks: map[string]checker{
				"should_parse_daily_episode": func(t *testing.T, args fixtures.Args) { checkDaily(t, args) },
				"should_parse_daily_episode_with_multiple_parts": func(t *testing.T, args fixtures.Args) {
					r := checkDaily(t, args)
					requireEqual(t, "daily part", r.dailyPart, argInt(args, 5))
				},
				"should_parse_daily_episode_using_short_month_format": func(t *testing.T, args fixtures.Args) { checkDaily(t, args) },
				"should_not_accept_ancient_daily_series": func(t *testing.T, args fixtures.Args) {
					title := expandDate(argString(args, 0), 1950, "10", "14")
					if r := parseEpisodeTitle(title, now()); r != nil {
						t.Errorf("parseEpisodeTitle(%q) = %+v, want nil", title, r)
					}
				},
				"should_accept_dates_within_future_limit": func(t *testing.T, args fixtures.Args) {
					twoDaysFromNow := now().AddDate(0, 0, 2)
					title := expandDate(argString(args, 0), twoDaysFromNow.Year(), twoDaysFromNow.Format("01"), twoDaysFromNow.Format("02"))
					r := mustParseEpisode(t, title)
					requireEqual(t, "air date", r.airDate, twoDaysFromNow.Format(AirDateLayout))
				},
				"should_parse_episodes_released_before_their_air_date": func(t *testing.T, args fixtures.Args) {
					airDate := now().AddDate(0, 0, argInt(args, 0))
					r := mustParseEpisode(t, talkShowTitle(airDate))
					requireEqual(t, "air date", r.airDate, airDate.Format(AirDateLayout))
				},
				"should_not_parse_dates_beyond_future_limit": func(t *testing.T, args fixtures.Args) {
					title := talkShowTitle(now().AddDate(0, 0, argInt(args, 0)))
					if r := parseEpisodeTitle(title, now()); r != nil {
						t.Errorf("parseEpisodeTitle(%q) = %+v, want nil", title, r)
					}
				},
				"should_not_parse_ambiguous_daily_episode": requireUnparsed,
			},
		},
		{
			file: fixtures.SonarrMiniSeriesEpisodeParserFixture,
			checks: map[string]checker{
				"should_parse_mini_series_episode": func(t *testing.T, args fixtures.Args) {
					checkSingleEpisode(t, mustParseEpisode(t, argString(args, 0)), argString(args, 1), 1, argInt(args, 2))
				},
				"should_parse_korean_series_episode": func(t *testing.T, args fixtures.Args) {
					checkSingleEpisode(t, mustParseEpisode(t, argString(args, 0)), argString(args, 1), 1, argInt(args, 2))
				},
			},
		},
		{
			file: fixtures.SonarrPathParserFixture,
			checks: map[string]checker{
				"should_parse_from_path": func(t *testing.T, args fixtures.Args) {
					r := parseEpisodePath(osAgnostic(argString(args, 0)), now())
					if r == nil {
						t.Fatalf("parseEpisodePath(%q) = nil", argString(args, 0))
					}
					requireInts(t, "episodes", r.episodeNumbers, []int{argInt(args, 2)})
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 1))
					requireNoAbsolute(t, r)
					requireEqual(t, "full season", r.fullSeason, false)
				},
				"should_parse_multi_episode_from_path": func(t *testing.T, args fixtures.Args) {
					r := parseEpisodePath(osAgnostic(argString(args, 0)), now())
					if r == nil {
						t.Fatalf("parseEpisodePath(%q) = nil", argString(args, 0))
					}
					requireEqual(t, "series title", r.seriesTitle, argString(args, 1))
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 2))
					requireInts(t, "episodes", r.episodeNumbers, argInts(args, 3))
					requireNoAbsolute(t, r)
					requireEqual(t, "full season", r.fullSeason, false)
				},
			},
		},
		{
			file: fixtures.SonarrParserFixture,
			checks: map[string]checker{
				"should_parse_series_name": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "clean series name", cleanSeriesTitle(seriesName(argString(args, 0))), cleanSeriesTitle(argString(args, 1)))
				},
				"should_parse_series_title_info": func(t *testing.T, args fixtures.Args) {
					r := mustParseEpisode(t, argString(args, 0))
					requireEqual(t, "title without year", r.titleInfo.titleWithoutYear, argString(args, 1))
					requireEqual(t, "year", r.titleInfo.year, argInt(args, 2))
				},
				"should_clean_up_invalid_path_characters": func(t *testing.T, args fixtures.Args) {
					parseEpisodeTitle(argString(args, 0), now())
				},
				"should_remove_request_info_from_title": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "series title", mustParseEpisode(t, argString(args, 0)).seriesTitle, argString(args, 1))
				},
				"should_parse_quality_from_extension": func(t *testing.T, args fixtures.Args) {
					q := mustParseEpisode(t, argString(args, 0)).quality
					if q.id == QualityUnknown {
						t.Error("quality is Unknown")
					}
					requireEqual(t, "source detection", q.sourceDetection, detectedFromExtension)
					requireEqual(t, "resolution detection", q.resolutionDetection, detectedFromExtension)
				},
				"should_parse_releasetitle": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "release title", mustParseEpisode(t, argString(args, 0)).releaseTitle, argString(args, 1))
				},
				"should_parse_multiple_series_titles": func(t *testing.T, args fixtures.Args) {
					var want []string
					for i := 1; i < len(args); i++ {
						want = append(want, argString(args, i))
					}
					got := mustParseEpisode(t, argString(args, 0)).titleInfo.allTitles
					requireEqual(t, "all titles", strings.Join(sortedCopy(got), "|"), strings.Join(sortedCopy(want), "|"))
				},
				"should_parse_language_after_parsing_title": func(t *testing.T, args fixtures.Args) {
					l := mustParseEpisode(t, argString(args, 0)).languages
					if argString(args, 1) == "Unknown" {
						requireEqual(t, "unknown language", isUnknownLanguage(l), true)
						return
					}
					requireEqual(t, "language count", languageCount(l), 1)
					requireEqual(t, "language", l.list[0], languageByName(t, argString(args, 1)))
				},
			},
		},
		{
			file: fixtures.SonarrCrapParserFixture,
			checks: map[string]checker{
				"should_not_parse_crap": requireUnparsed,
				"should_parse_unknown_formats_without_error": func(t *testing.T, args fixtures.Args) {
					mustParseEpisode(t, argString(args, 0))
				},
				"should_not_parse_random": func(t *testing.T, args fixtures.Args) {
					requireRandomUnparsed(t, argInt(args, 0), func(s string) bool { return parseEpisodeTitle(s, now()) == nil })
				},
				"should_not_parse_file_name_without_proper_spacing": requireUnparsed,
				"should_not_parse_invalid_release_name":             requireUnparsed,
				"should_not_parse_special_with_part_number":         requireUnparsed,
			},
		},
		{
			file: fixtures.SonarrReleaseGroupParserFixture,
			checks: map[string]checker{
				"should_parse_release_group":                      releaseGroup,
				"should_parse_exception_release_group":            releaseGroup,
				"should_not_include_language_in_release_group":    releaseGroup,
				"should_not_include_repost_in_release_group":      releaseGroup,
				"should_parse_anime_release_groups":               releaseGroup,
				"should_not_parse_anime_hash_as_release_group":    releaseGroup,
				"should_not_parse_episode_title_as_release_group": releaseGroup,
			},
		},
		{
			file: fixtures.SonarrUrlFixture,
			checks: map[string]checker{
				"should_not_parse_url_in_name": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "clean series name", cleanSeriesTitle(seriesName(argString(args, 0))), cleanSeriesTitle(argString(args, 1)))
				},
				"should_not_parse_url_in_group": releaseGroup,
			},
		},
		{
			file: fixtures.SonarrUnicodeReleaseParserFixture,
			checks: map[string]checker{
				"should_parse_full_season_releases": func(t *testing.T, args fixtures.Args) {
					r := mustParseEpisode(t, xmlCleanUnicode(argString(args, 0)))
					requireEqual(t, "release group", r.releaseGroup, argString(args, 2))
					requireInts(t, "episodes", r.episodeNumbers, nil)
					requireNoAbsolute(t, r)
					requireEqual(t, "series title", r.seriesTitle, argString(args, 1))
					requireEqual(t, "full season", r.fullSeason, true)
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 3))
				},
				"should_parse_multi_episode_release": func(t *testing.T, args fixtures.Args) {
					r := mustParseEpisode(t, xmlCleanUnicode(argString(args, 0)))
					requireEqual(t, "season", r.seasonNumber(), argInt(args, 3))
					requireInts(t, "episodes", r.episodeNumbers, argInts(args, 4))
					requireEqual(t, "series title", r.seriesTitle, argString(args, 1))
					requireNoAbsolute(t, r)
					requireEqual(t, "full season", r.fullSeason, false)
				},
			},
			skipped: map[string]string{
				"should_parse_chinese_anime_releases":                reasonAnime,
				"should_parse_chinese_anime_season_episode_releases": reasonAnime,
				"should_parse_unbracketed_chinese_anime_releases":    reasonAnime,
				"should_parse_chinese_multiepisode_releases":         reasonAnime,
				"should_parse_gm_team_releases_and_files":            reasonAnime,
				"should_parse_unicode_digits":                        reasonAnime,
				"should_parse_false_positive_chinese_anime_releases": reasonAnime,
			},
		},
		skipAll(fixtures.SonarrAbsoluteEpisodeNumberParserFixture, reasonAnime),
		skipAll(fixtures.SonarrAnimeMetadataParserFixture, reasonAnime),
		{file: fixtures.SonarrAnimeVersionFixture, checks: map[string]checker{"should_be_able_to_parse_repack": versionCheck(seriesQualityRules)}},
		skipAll(fixtures.SonarrSubGroupParserFixture, reasonAnime),
		skipAll(fixtures.SonarrSeasonTitleParserFixture, reasonAnime),
	}
}

func skipAll(file fixtures.File, reason string) fixtureSuite {
	skipped := map[string]string{}
	for _, c := range file.Cases {
		skipped[c.Method] = reason
	}
	return fixtureSuite{file: file, skipped: skipped}
}

const (
	randomTitleRepetitions = 500
	randomTitleCharset     = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	md5Repetitions         = 100
	md5Seed                = "CRAPPY TEST SEED"
)

func requireRandomUnparsed(t *testing.T, length int, unparsed func(string) bool) {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(length), uint64(length)))
	for range randomTitleRepetitions {
		var b strings.Builder
		for range length {
			b.WriteByte(randomTitleCharset[rng.IntN(len(randomTitleCharset))])
		}
		if !unparsed(b.String()) {
			t.Errorf("random title %q parsed", b.String())
		}
	}
}

func requireMD5Unparsed(t *testing.T, unparsed func(string) bool) {
	t.Helper()
	hash := md5Seed
	for range md5Repetitions {
		sum := md5.Sum([]byte(hash))
		hash = strings.ToUpper(hex.EncodeToString(sum[:]))
		if !unparsed(hash) {
			t.Errorf("md5 title %q parsed", hash)
		}
	}
}

func TestSonarrFixtures(t *testing.T) {
	runSuites(t, seriesSuites())
}

func TestSonarrHashedReleases(t *testing.T) {
	for _, c := range fixtures.SonarrHashedReleases {
		t.Run(c.Path, func(t *testing.T) {
			r := parseEpisodePath(osAgnostic(c.Path), now())
			if r == nil {
				t.Fatalf("parseEpisodePath(%q) = nil", c.Path)
			}
			requireEqual(t, "series title", r.seriesTitle, c.Title)
			requireEqual(t, "quality", r.quality.id, sonarrQuality(t, c.Quality))
			requireEqual(t, "release group", r.releaseGroup, c.Group)
		})
	}
}

func TestSonarrDoesNotParseMD5(t *testing.T) {
	requireMD5Unparsed(t, func(s string) bool { return parseEpisodeTitle(s, now()) == nil })
}

func TestSonarrDoesNotParseSeasonFolders(t *testing.T) {
	if r := parseEpisodeTitle("Season 3", now()); r != nil {
		t.Errorf("parsed season folder: %+v", r)
	}
}

func TestSonarrReleaseGroupExcludesExtension(t *testing.T) {
	r := parseEpisodePath(osAgnostic(`C:\Test\Doctor.Series.2005.s01e01.internal.bdrip.x264-archivist.mkv`), now())
	if r == nil {
		t.Fatal("unparsed")
	}
	requireEqual(t, "release group", r.releaseGroup, "archivist")
}
