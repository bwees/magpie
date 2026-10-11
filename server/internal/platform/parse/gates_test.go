// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/parse/fixtures"
)

func allFixtureFiles() []fixtures.File {
	return []fixtures.File{
		fixtures.SonarrSingleEpisodeParserFixture, fixtures.SonarrMultiEpisodeParserFixture,
		fixtures.SonarrSeasonParserFixture, fixtures.SonarrDailyEpisodeParserFixture,
		fixtures.SonarrQualityParserFixture, fixtures.SonarrLanguageParserFixture,
		fixtures.SonarrReleaseGroupParserFixture, fixtures.SonarrParserFixture,
		fixtures.SonarrCrapParserFixture, fixtures.SonarrMiniSeriesEpisodeParserFixture,
		fixtures.SonarrPathParserFixture, fixtures.SonarrExtendedQualityParserRegex,
		fixtures.SonarrUrlFixture, fixtures.SonarrUnicodeReleaseParserFixture,
		fixtures.SonarrAbsoluteEpisodeNumberParserFixture, fixtures.SonarrAnimeMetadataParserFixture,
		fixtures.SonarrAnimeVersionFixture, fixtures.SonarrSubGroupParserFixture,
		fixtures.SonarrSeasonTitleParserFixture, fixtures.RadarrParserFixture,
		fixtures.RadarrEditionParserFixture, fixtures.RadarrQualityParserFixture,
		fixtures.RadarrLanguageParserFixture, fixtures.RadarrReleaseGroupParserFixture,
		fixtures.RadarrCrapParserFixture, fixtures.RadarrUrlFixture,
		fixtures.RadarrExtendedQualityParserRegex, fixtures.RadarrAnimeVersionFixture,
	}
}

// gateCorpus is every string in the vendored fixtures plus the intermediate forms the parser
// feeds its regexes, and random mutations of them.
func gateCorpus() []string {
	var base []string
	for _, f := range allFixtureFiles() {
		for _, c := range f.Cases {
			for _, a := range c.Args {
				switch v := a.(type) {
				case string:
					base = append(base, v)
				case []string:
					base = append(base, v...)
				}
			}
		}
	}
	for _, h := range append(fixtures.SonarrHashedReleases, fixtures.RadarrHashedReleases...) {
		base = append(base, h.Path, osAgnostic(h.Path), baseName(osAgnostic(h.Path)))
	}
	for _, r := range typicalReleases() {
		base = append(base, r.name)
	}

	seen := map[string]bool{}
	var corpus []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			corpus = append(corpus, s)
		}
	}
	for _, s := range base {
		release := removeFileExtension(s, domain.MediaKindSeries)
		for _, v := range []string{s, release, strings.ReplaceAll(s, "_", " "),
			episodeSimpleTitleRegex.replace(release, ""), movieSimpleTitleRegex.replace(release, "")} {
			add(v)
		}
	}
	if raceEnabled {
		return everyNth(corpus, raceCorpusStride)
	}
	for _, m := range mutations(corpus) {
		add(m)
	}
	return corpus
}

const raceCorpusStride = 40

func everyNth(in []string, n int) []string {
	var out []string
	for i := 0; i < len(in); i += n {
		out = append(out, in[i])
	}
	return out
}

const (
	mutationSeed     = 10
	mutationsPerBase = 1
	mutationAlphabet = "sexSEX0123456789 ._-[]()/pPrR"
)

func mutations(corpus []string) []string {
	rng := rand.New(rand.NewPCG(mutationSeed, mutationSeed))
	var out []string
	for _, s := range corpus {
		if s == "" {
			continue
		}
		for range mutationsPerBase {
			b := []byte(s)
			i := rng.IntN(len(b))
			b[i] = mutationAlphabet[rng.IntN(len(mutationAlphabet))]
			out = append(out, string(b))
		}
	}
	return out
}

func gatedPatterns() map[string]pattern {
	patterns := map[string]pattern{
		"rawHD": rawHDRegex, "mpeg2": mpeg2Regex, "proper": properRegex, "repack": repackRegex,
		"real": realRegex, "altRes": altResRegex, "otherSource": otherSourceRegex,
		"animeBluray": animeBlurayRegex, "animeWebDL": animeWebDLRegex, "highDefPdtv": highDefPdtvRegex,
		"series version": seriesQualityRules.version, "series codec": seriesQualityRules.codec,
		"series remux": seriesQualityRules.remux, "movie version": movieQualityRules.version,
		"movie codec": movieQualityRules.codec, "movie remux": movieQualityRules.remux,
		"germanRemux": movieQualityRules.germanRemux, "brDisk": movieQualityRules.brDisk,
		"series language": seriesLanguageRegex, "series case-sensitive language": seriesCaseSensitiveLanguageRegex,
		"movie language": movieLanguageRegex, "movie case-sensitive language": movieCaseSensitiveLanguageRegex,
		"series language clean": seriesLanguageCleanRegex,
		"websitePrefix":         websitePrefixRegex, "websitePostfix": websitePostfixRegex,
		"cleanTorrentSuffix": cleanTorrentSuffixRegex, "cleanQualityBrackets": cleanQualityBracketsRegex,
		"movieCleanQualityBrackets": movieCleanQualityBracketsRegex, "edition": reportEditionRegex,
	}
	patterns["series source"] = seriesQualityRules.source
	patterns["movie source"] = movieQualityRules.source
	patterns["series resolution"] = seriesQualityRules.resolution
	patterns["movie resolution"] = movieQualityRules.resolution
	patterns["six digit air date"] = sixDigitAirDateRegex
	patterns["alternative title"] = alternativeTitleRegex
	patterns["bracketed alternative title"] = bracketedAlternativeTitleRegex
	patterns["normalize alternative title"] = normalizeAlternativeTitleRegex
	patterns["title components"] = titleComponentsRegex
	patterns["series year in title"] = seriesYearInTitleRegex
	patterns["request info"] = requestInfoRegex
	for i, re := range episodeRejectHashedRegexes {
		patterns["series hashed "+string(rune('A'+i))] = re
	}
	for i, re := range movieRejectHashedRegexes {
		patterns["movie hashed "+string(rune('A'+i))] = re
	}
	for i, re := range seriesReleaseGroupRules.clean {
		patterns["series group clean "+string(rune('A'+i))] = re
	}
	for i, re := range movieReleaseGroupRules.clean {
		patterns["movie group clean "+string(rune('A'+i))] = re
	}
	for i, re := range movieTitleRegexes {
		patterns["movie title "+string(rune('A'+i))] = re
	}
	for i, sub := range seriesPreSubstitutions {
		patterns["pre-substitution "+string(rune('A'+i))] = sub.regex
	}
	for i, re := range seriesReleaseGroupRules.exceptions {
		patterns["series group exception "+string(rune('A'+i))] = re
	}
	for i, re := range movieReleaseGroupRules.exceptions {
		patterns["movie group exception "+string(rune('A'+i))] = re
	}
	return patterns
}

func ungatedMatch(p pattern, s string) bool {
	ok, err := p.re.MatchString(s)
	return err == nil && ok
}

// spans lists successive matches as "index:length" for comparing two ways of matching.
func spans(ms []*match) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "%d:%d ", m.m.RuneIndex, m.m.RuneLength)
	}
	return b.String()
}

func ungatedMatches(p pattern, s string) []*match {
	return pattern{re: p.re}.matches(s)
}

// TestGatesAreSound checks that gates, start gates and hints never change a result.
func TestGatesAreSound(t *testing.T) {
	corpus := gateCorpus()
	for name, p := range gatedPatterns() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, s := range corpus {
				if got, want := p.isMatch(s), ungatedMatch(p, s); got != want {
					t.Errorf("isMatch(%q) = %v, regex says %v", s, got, want)
				}
				if p.starts == nil {
					continue
				}
				if got, want := spans(p.matches(s)), spans(ungatedMatches(p, s)); got != want {
					t.Errorf("matches(%q) = %s, regex says %s", s, got, want)
				}
			}
		})
	}
	for i, entry := range episodeTitleRegexes {
		t.Run(fmt.Sprintf("episode regex %d", i), func(t *testing.T) {
			t.Parallel()
			for _, s := range corpus {
				if !entry.gate(newTitleShape(s)) && ungatedMatch(entry.regex, s) {
					t.Errorf("gate closed for %q, but the regex matches", s)
				}
			}
		})
	}
}

// Sonarr's release-group clean-up is one regex; the port runs its two branches in turn.
func TestSplitCleanRegexMatchesSonarr(t *testing.T) {
	sonarr := mustPattern(`^(.*?[-._ ](S\d+E\d+)[-._ ])|(-(RP|1|NZBGeek|\[N-Z-B\]|Obfuscated|Scrambled|sample|Pre|postbot|xpost|Rakuv[a-z0-9]*|WhiteRev|BUYMORE|AsRequested|AlternativeToRequested|GEROV|Z0iDS3N|Chamele0n|4P|4Planet|AlteZachen|RePACKPOST))+$`, ignoreCase)
	for _, s := range gateCorpus() {
		split := s
		for _, clean := range seriesReleaseGroupRules.clean {
			split = clean.replace(split, "")
		}
		if want := sonarr.replace(s, ""); split != want {
			t.Errorf("clean(%q) = %q, Sonarr gives %q", s, split, want)
		}
	}
}

// The case-sensitive language regexes rely on .NET scoping (?i) to its group; check regexp2 does too.
func TestCaseSensitiveLanguageRegexIgnoresLowercase(t *testing.T) {
	for _, s := range []string{"series.s01e01.lt.720p", "movie.2019.de.1080p", "movie.2019.en.es.1080p"} {
		if ungatedMatch(seriesCaseSensitiveLanguageRegex, s) || ungatedMatch(movieCaseSensitiveLanguageRegex, s) {
			t.Errorf("case-sensitive language regex matched %q", s)
		}
	}
}
