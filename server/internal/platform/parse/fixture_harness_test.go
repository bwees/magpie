// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/parse/fixtures"
)

// checker asserts one vendored [TestCase] the way its NUnit test method does.
type checker func(t *testing.T, args fixtures.Args)

// fixtureSuite pairs a vendored fixture file with a checker per NUnit method.
// Every method must have a checker or a skip reason, so new upstream methods can't pass silently.
type fixtureSuite struct {
	file    fixtures.File
	checks  map[string]checker
	skipped map[string]string
}

const (
	reasonAnime     = "anime and absolute episode numbering are out of scope for v1"
	reasonSubtitles = "subtitle file names are parsed by the Subtitles context, not the release parser"
	reasonNotAField = "not a Release field in Magpie v1"
	reasonNotPorted = "helper outside the release parser; not ported"
)

func runSuites(t *testing.T, suites []fixtureSuite) {
	for _, s := range suites {
		t.Run(s.file.Source, func(t *testing.T) {
			for _, c := range s.file.Cases {
				t.Run(fmt.Sprintf("%s/line_%d", c.Method, c.Line), func(t *testing.T) {
					if reason, ok := s.skipped[c.Method]; ok {
						t.Skip(reason)
					}
					check, ok := s.checks[c.Method]
					if !ok {
						t.Fatalf("no checker or skip reason for NUnit method %s", c.Method)
					}
					check(t, c.Args)
				})
			}
		})
	}
}

func argString(args fixtures.Args, i int) string {
	if i >= len(args) || args[i] == nil {
		return ""
	}
	return args[i].(string)
}

func argInt(args fixtures.Args, i int) int {
	if i >= len(args) {
		return 0
	}
	return args[i].(int)
}

func argBool(args fixtures.Args, i int) bool {
	return args[i].(bool)
}

func argInts(args fixtures.Args, i int) []int {
	return args[i].([]int)
}

func argStrings(args fixtures.Args, i int) []string {
	return args[i].([]string)
}

// osAgnostic is NzbDrone.Test.Common's AsOsAgnostic on a non-Windows host.
func osAgnostic(path string) string {
	if len(path) > 2 && path[1] == ':' {
		path = "/" + strings.ReplaceAll(path, ":", "")
	}
	return strings.ReplaceAll(path, `\`, "/")
}

// sonarrQualityIDs maps Sonarr's C# Quality property names to Sonarr quality IDs.
var sonarrQualityIDs = map[string]int{
	"Unknown": 0, "SDTV": 1, "DVD": 2, "WEBDL1080p": 3, "HDTV720p": 4, "WEBDL720p": 5, "Bluray720p": 6,
	"Bluray1080p": 7, "WEBDL480p": 8, "HDTV1080p": 9, "RAWHD": 10, "WEBRip480p": 12, "Bluray480p": 13,
	"WEBRip720p": 14, "WEBRip1080p": 15, "HDTV2160p": 16, "WEBRip2160p": 17, "WEBDL2160p": 18,
	"Bluray2160p": 19, "Bluray1080pRemux": 20, "Bluray2160pRemux": 21, "Bluray576p": 22,
}

func sonarrQuality(t *testing.T, name string) QualityID {
	t.Helper()
	id, ok := sonarrQualityIDs[name]
	if !ok {
		t.Fatalf("unknown Sonarr quality %q", name)
	}
	return SonarrToMagpieQuality(id)
}

// radarrQualityIDs maps Radarr's C# Quality property names to Radarr (Magpie) quality IDs.
var radarrQualityIDs = map[string]QualityID{
	"Unknown": QualityUnknown, "SDTV": QualitySDTV, "DVD": QualityDVD, "WEBDL1080p": QualityWEBDL1080p,
	"HDTV720p": QualityHDTV720p, "WEBDL720p": QualityWEBDL720p, "Bluray720p": QualityBluray720p,
	"Bluray1080p": QualityBluray1080p, "WEBDL480p": QualityWEBDL480p, "HDTV1080p": QualityHDTV1080p,
}

func radarrQuality(t *testing.T, name string) QualityID {
	t.Helper()
	id, ok := radarrQualityIDs[name]
	if !ok {
		t.Fatalf("unknown Radarr quality %q", name)
	}
	return id
}

// languageNames maps Sonarr/Radarr Language names, as used in method names and casts, to Magpie codes.
var languageNames = map[string]domain.Language{
	"english": langEnglish, "french": langFrench, "spanish": langSpanish, "german": langGerman,
	"italian": langItalian, "danish": langDanish, "dutch": langDutch, "japanese": langJapanese,
	"icelandic": langIcelandic, "chinese": langChinese, "russian": langRussian, "polish": langPolish,
	"vietnamese": langVietnamese, "swedish": langSwedish, "norwegian": langNorwegian, "finnish": langFinnish,
	"turkish": langTurkish, "portuguese": langPortuguese, "flemish": langFlemish, "greek": langGreek,
	"korean": langKorean, "hungarian": langHungarian, "hebrew": langHebrew, "lithuanian": langLithuanian,
	"czech": langCzech, "arabic": langArabic, "hindi": langHindi, "bulgarian": langBulgarian,
	"malayalam": langMalayalam, "ukrainian": langUkrainian, "slovak": langSlovak, "thai": langThai,
	"portuguese_brazil": langPortugueseBrazil, "brazilian_portuguese": langPortugueseBrazil,
	"spanish_latino": langSpanishLatino, "romanian": langRomanian, "latvian": langLatvian,
	"persian": langPersian, "catalan": langCatalan, "tamil": langTamil, "azerbaijani": langAzerbaijani,
	"uzbek": langUzbek, "urdu": langUrdu, "romansh": langRomansh, "georgian": langGeorgian,
	"irish": langIrish, "bengali": langBengali, "telugu": langTelugu, "kannada": langKannada,
	"albanian": langAlbanian, "afrikaans": langAfrikaans, "marathi": langMarathi, "tagalog": langTagalog,
	"mongolian": langMongolian, "ukrainian_multi": langUkrainian,
}

func languageByName(t *testing.T, name string) domain.Language {
	t.Helper()
	l, ok := languageNames[strings.ToLower(name)]
	if !ok {
		t.Fatalf("unknown language %q", name)
	}
	return l
}

// languageCount is Sonarr's Languages.Count, where Original counts as a language.
func languageCount(l languages) int {
	if l.original {
		return len(l.list) + 1
	}
	return len(l.list)
}

func isUnknownLanguage(l languages) bool {
	return languageCount(l) == 0
}

func requireNoAbsolute(t *testing.T, r *episodeInfo) {
	t.Helper()
	if len(r.absoluteEpisodeNumbers) != 0 {
		t.Errorf("absolute episodes = %v, want none", r.absoluteEpisodeNumbers)
	}
}

func requireEqual[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func requireInts(t *testing.T, field string, got, want []int) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func sortedCopy[T ~string](in []T) []T {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

// now is the clock the daily fixtures were written against: the real one.
func now() time.Time {
	return time.Now()
}
