// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"slices"
	"strings"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/parse/fixtures"
)

const languageMethodPrefix = "should_parse_language_"

// containsLanguageCheck is a "Languages.Should().Contain(X)" method, X taken from the method name.
func containsLanguageCheck(method string, parse func(string) (languages, bool)) checker {
	name := strings.TrimPrefix(method, languageMethodPrefix)
	return func(t *testing.T, args fixtures.Args) {
		l, ok := parse(argString(args, 0))
		if !ok {
			t.Fatalf("unparsed %q", argString(args, 0))
		}
		if name == "unknown" || name == "polish_subbed" {
			requireEqual(t, "unknown language", isUnknownLanguage(l), true)
			return
		}
		for _, part := range expectedLanguageParts(name) {
			if want := languageByName(t, part); !slices.Contains(l.list, want) {
				t.Errorf("languages = %v, want to contain %s", l.list, want)
			}
		}
	}
}

// expectedLanguageParts splits combined method names such as french_english.
func expectedLanguageParts(name string) []string {
	if _, ok := languageNames[name]; ok {
		return []string{name}
	}
	return strings.Split(name, "_")
}

// exactLanguagesCheck is "Should().BeEquivalentTo(...)" or a Count check plus Contains.
func exactLanguagesCheck(parse func(string) (languages, bool), original bool, want ...domain.Language) checker {
	return func(t *testing.T, args fixtures.Args) {
		l, ok := parse(argString(args, 0))
		if !ok {
			t.Fatalf("unparsed %q", argString(args, 0))
		}
		requireEqual(t, "languages", strings.Join(languageStrings(sortedCopy(l.list)), ","), strings.Join(languageStrings(sortedCopy(want)), ","))
		requireEqual(t, "original", l.original, original)
	}
}

func languageStrings(in []domain.Language) []string {
	out := make([]string, len(in))
	for i, l := range in {
		out[i] = string(l)
	}
	return out
}

func seriesLanguagesDirect(title string) (languages, bool) {
	return parseSeriesLanguages(title), true
}

func seriesLanguagesViaTitle(title string) (languages, bool) {
	r := parseEpisodeTitle(title, now())
	if r == nil {
		return languages{}, false
	}
	return r.languages, true
}

func movieLanguagesDirect(title string) (languages, bool) {
	return parseMovieLanguages(title), true
}

func movieLanguagesViaTitle(isDir bool) func(string) (languages, bool) {
	return func(title string) (languages, bool) {
		r := parseMovieTitle(title, isDir)
		if r == nil {
			return languages{}, false
		}
		return r.languages, true
	}
}

func languageSuite(file fixtures.File, contains func(method string) func(string) (languages, bool), exact map[string]checker, skipped map[string]string) fixtureSuite {
	checks := map[string]checker{}
	for _, c := range file.Cases {
		if _, ok := exact[c.Method]; ok || skipped[c.Method] != "" || !strings.HasPrefix(c.Method, languageMethodPrefix) {
			continue
		}
		checks[c.Method] = containsLanguageCheck(c.Method, contains(c.Method))
	}
	for method, check := range exact {
		checks[method] = check
	}
	return fixtureSuite{file: file, checks: checks, skipped: skipped}
}

func subtitleSkips() map[string]string {
	return map[string]string{
		"should_parse_subtitle_language":         reasonSubtitles,
		"should_parse_subtitle_language_english": reasonSubtitles,
		"should_parse_subtitle_language_unknown": reasonSubtitles,
		"should_parse_title_and_tags":            reasonSubtitles,
		"should_not_parse_false_title":           reasonSubtitles,
	}
}

func germanOriginalChecks(parse func(string) (languages, bool)) map[string]checker {
	return map[string]checker{
		"should_add_original_language_to_german_release_with_dl_tag":                    exactLanguagesCheck(parse, true, langGerman),
		"should_not_add_original_language_to_german_release_when_title_contains_web_dl": exactLanguagesCheck(parse, false, langGerman),
		"should_add_original_language_and_english_to_german_release_with_ml_tag":        exactLanguagesCheck(parse, true, langGerman, langEnglish),
		"should_parse_original_title_from_release_name":                                 exactLanguagesCheck(parse, true),
	}
}

func sonarrLanguageSuite() fixtureSuite {
	exact := germanOriginalChecks(seriesLanguagesViaTitle)
	exact["should_parse_english_and_ukranian"] = exactLanguagesCheck(seriesLanguagesViaTitle, false, langUkrainian, langEnglish)
	exact["should_parse_turkish_and_ukranian"] = exactLanguagesCheck(seriesLanguagesViaTitle, false, langUkrainian, langTurkish)
	exact["should_parse_azerbaijani"] = exactLanguagesCheck(seriesLanguagesViaTitle, false, langAzerbaijani)
	exact["should_parse_uzbek"] = exactLanguagesCheck(seriesLanguagesViaTitle, false, langUzbek)
	exact["should_parse_hindi_and_english"] = exactLanguagesCheck(seriesLanguagesDirect, false, langHindi, langEnglish)
	exact["should_parse_spanish_and_catalan"] = exactLanguagesCheck(seriesLanguagesDirect, false, langSpanish, langCatalan)
	exact["should_parse_english_spanish_and_catalan"] = exactLanguagesCheck(seriesLanguagesDirect, false, langEnglish, langSpanish, langCatalan)
	exact["should_parse_language_russian_and_georgian"] = exactLanguagesCheck(seriesLanguagesDirect, false, langRussian, langGeorgian)
	exact["should_parse_language_english_french_german_italian_portuguese_spanish"] = exactLanguagesCheck(seriesLanguagesDirect, false,
		langEnglish, langFrench, langGerman, langItalian, langPortuguese, langSpanish)
	return languageSuite(fixtures.SonarrLanguageParserFixture,
		func(string) func(string) (languages, bool) { return seriesLanguagesDirect },
		exact, subtitleSkips())
}

func radarrLanguageSuite() fixtureSuite {
	direct := []string{"spanish_latino", "malayalam", "kannada", "albanian", "afrikaans", "marathi", "tagalog", "urdu", "romansh", "mongolian", "georgian"}
	fileTitle := []string{"arabic", "persian", "bengali", "telugu", "slovak", "latvian"}
	exact := germanOriginalChecks(movieLanguagesViaTitle(false))
	exact["should_parse_language_spanish_latino"] = func(t *testing.T, args fixtures.Args) {
		l := parseMovieLanguages(argString(args, 0))
		if len(l.list) == 0 {
			t.Fatal("no languages")
		}
		requireEqual(t, "first language", l.list[0], langSpanishLatino)
	}
	return languageSuite(fixtures.RadarrLanguageParserFixture,
		func(method string) func(string) (languages, bool) {
			name := strings.TrimPrefix(method, languageMethodPrefix)
			switch {
			case slices.Contains(direct, name):
				return movieLanguagesDirect
			case slices.Contains(fileTitle, name):
				return movieLanguagesViaTitle(false)
			}
			return movieLanguagesViaTitle(true)
		},
		exact, subtitleSkips())
}

func TestLanguageFixtures(t *testing.T) {
	runSuites(t, []fixtureSuite{sonarrLanguageSuite(), radarrLanguageSuite()})
}
