// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"fmt"
	"strings"
	"testing"

	"magpie/internal/platform/parse/fixtures"
)

func revisionVersion(proper bool) int {
	if proper {
		return properRevisionVersion
	}
	return defaultRevisionVersion
}

func optionalProper(args fixtures.Args) bool {
	return len(args) > 1 && argBool(args, 1)
}

// sonarrQualityCheck is Sonarr's ParseAndVerifyQuality for one expected quality.
func sonarrQualityCheck(quality string) checker {
	return func(t *testing.T, args fixtures.Args) {
		r := parseQuality(argString(args, 0), seriesQualityRules)
		requireEqual(t, "quality", r.id, sonarrQuality(t, quality))
		requireEqual(t, "version", r.revision.Version, revisionVersion(optionalProper(args)))
	}
}

func detectionCheck(rules qualityRules, source, resolution detectionSource) checker {
	return func(t *testing.T, args fixtures.Args) {
		r := parseQuality(argString(args, 0), rules)
		requireEqual(t, "source detection", r.sourceDetection, source)
		requireEqual(t, "resolution detection", r.resolutionDetection, resolution)
	}
}

func repackCheck(rules qualityRules) checker {
	return func(t *testing.T, args fixtures.Args) {
		r := parseQuality(argString(args, 0), rules)
		requireEqual(t, "version", r.revision.Version, argInt(args, 2))
		requireEqual(t, "repack", r.revision.Repack, argBool(args, 1))
	}
}

func versionCheck(rules qualityRules) checker {
	return func(t *testing.T, args fixtures.Args) {
		requireEqual(t, "version", parseQuality(argString(args, 0), rules).revision.Version, argInt(args, 1))
	}
}

func realCheck(rules qualityRules) checker {
	return func(t *testing.T, args fixtures.Args) {
		requireEqual(t, "real", parseQuality(argString(args, 0), rules).revision.Real, argInt(args, 1))
	}
}

func detectionChecks(rules qualityRules) map[string]checker {
	return map[string]checker{
		"should_parse_full_quality_from_name":                         detectionCheck(rules, detectedFromName, detectedFromName),
		"should_parse_resolution_from_name":                           detectionCheck(rules, detectedUnknown, detectedFromName),
		"should_parse_source_from_name":                               detectionCheck(rules, detectedFromName, detectedUnknown),
		"should_parse_quality_from_extension":                         detectionCheck(rules, detectedFromExtension, detectedFromExtension),
		"should_parse_resolution_from_name_and_source_from_extension": detectionCheck(rules, detectedFromExtension, detectedFromName),
		"should_be_able_to_parse_repack":                              repackCheck(rules),
	}
}

func sonarrQualitySuite() fixtureSuite {
	checks := detectionChecks(seriesQualityRules)
	for method, quality := range map[string]string{
		"should_parse_sdtv_quality":              "SDTV",
		"should_parse_dvd_quality":               "DVD",
		"should_parse_webdl480p_quality":         "WEBDL480p",
		"should_parse_bluray480p_quality":        "Bluray480p",
		"should_parse_webrip480p_quality":        "WEBRip480p",
		"should_parse_bluray576p_quality":        "Bluray576p",
		"should_parse_hdtv720p_quality":          "HDTV720p",
		"should_parse_hdtv1080p_quality":         "HDTV1080p",
		"should_parse_hdtv2160p_quality":         "HDTV2160p",
		"should_parse_webdl720p_quality":         "WEBDL720p",
		"should_parse_webrip720p_quality":        "WEBRip720p",
		"should_parse_webdl1080p_quality":        "WEBDL1080p",
		"should_parse_webrip1080p_quality":       "WEBRip1080p",
		"should_parse_webdl2160p_quality":        "WEBDL2160p",
		"should_parse_webrip2160p_quality":       "WEBRip2160p",
		"should_parse_bluray720p_quality":        "Bluray720p",
		"should_parse_bluray1080p_quality":       "Bluray1080p",
		"should_parse_bluray1080p_remux_quality": "Bluray1080pRemux",
		"should_parse_bluray2160p_quality":       "Bluray2160p",
		"should_parse_bluray2160p_remux_quality": "Bluray2160pRemux",
		"should_parse_raw_quality":               "RAWHD",
		"quality_parse":                          "Unknown",
	} {
		checks[method] = sonarrQualityCheck(quality)
	}
	checks["should_be_able_to_parse_anime_version"] = versionCheck(seriesQualityRules)
	return fixtureSuite{file: fixtures.SonarrQualityParserFixture, checks: checks}
}

// radarrExpectation is what Radarr's ParseAndVerifyQuality checks.
type radarrExpectation struct {
	source     QualitySource
	resolution Resolution
	modifier   QualityModifier
	// properArg is false for methods that take no proper argument.
	properArg bool
}

func radarrQualityCheck(want radarrExpectation) checker {
	return func(t *testing.T, args fixtures.Args) {
		verifyRadarrQuality(t, argString(args, 0), want, want.properArg && argBool(args, 1))
	}
}

func verifyRadarrQuality(t *testing.T, title string, want radarrExpectation, proper bool) {
	t.Helper()
	r := parseQuality(title, movieQualityRules)
	def := r.definition()
	if want.resolution != ResolutionUnknown {
		requireEqual(t, "resolution", r.resolution, want.resolution)
	}
	requireEqual(t, "source", def.Source, want.source)
	if want.modifier != ModifierNone {
		requireEqual(t, "modifier", def.Modifier, want.modifier)
	}
	requireEqual(t, "version", r.revision.Version, revisionVersion(proper))
}

func radarrQualitySuite() fixtureSuite {
	checks := detectionChecks(movieQualityRules)
	withProper := func(source QualitySource, resolution Resolution, modifier QualityModifier) radarrExpectation {
		return radarrExpectation{source, resolution, modifier, true}
	}
	withoutProper := func(source QualitySource, resolution Resolution, modifier QualityModifier) radarrExpectation {
		return radarrExpectation{source, resolution, modifier, false}
	}
	for method, want := range map[string]radarrExpectation{
		"should_parse_ts":                              withProper(SourceTelesync, Resolution720p, ModifierNone),
		"should_parse_cam":                             withProper(SourceCam, ResolutionUnknown, ModifierNone),
		"should_parse_sdtv_quality":                    withProper(SourceTV, Resolution480p, ModifierNone),
		"should_parse_dvd_quality":                     withProper(SourceDVD, ResolutionUnknown, ModifierNone),
		"should_parse_dvdr_quality":                    withProper(SourceDVD, Resolution480p, ModifierRemux),
		"should_parse_webdl480p_quality":               withProper(SourceWebDL, Resolution480p, ModifierNone),
		"should_parse_webrip480p_quality":              withProper(SourceWebRip, Resolution480p, ModifierNone),
		"should_parse_bluray480p_quality":              withProper(SourceBluray, Resolution480p, ModifierNone),
		"should_parse_hdtv720p_quality":                withProper(SourceTV, Resolution720p, ModifierNone),
		"should_parse_hdtv1080p_quality":               withProper(SourceTV, Resolution1080p, ModifierNone),
		"should_parse_hdtv2160p_quality":               withProper(SourceTV, Resolution2160p, ModifierNone),
		"should_parse_webdl720p_quality":               withProper(SourceWebDL, Resolution720p, ModifierNone),
		"should_parse_webrip720p_quality":              withProper(SourceWebRip, Resolution720p, ModifierNone),
		"should_parse_webdl1080p_quality":              withProper(SourceWebDL, Resolution1080p, ModifierNone),
		"should_parse_webrip1080p_quality":             withProper(SourceWebRip, Resolution1080p, ModifierNone),
		"should_parse_webdl2160p_quality":              withProper(SourceWebDL, Resolution2160p, ModifierNone),
		"should_parse_webrip2160p_quality":             withProper(SourceWebRip, Resolution2160p, ModifierNone),
		"should_parse_bluray720p_quality":              withProper(SourceBluray, Resolution720p, ModifierNone),
		"should_parse_bluray1080p_quality":             withProper(SourceBluray, Resolution1080p, ModifierNone),
		"should_parse_bluray2160p_quality":             withProper(SourceBluray, Resolution2160p, ModifierNone),
		"should_parse_bluray576p_quality":              withoutProper(SourceBluray, Resolution576p, ModifierNone),
		"should_parse_remux720p_as_bluray720p_quality": withoutProper(SourceBluray, Resolution720p, ModifierNone),
		"should_parse_remux1080p_quality":              withoutProper(SourceBluray, Resolution1080p, ModifierRemux),
		"should_parse_remux2160p_quality":              withoutProper(SourceBluray, Resolution2160p, ModifierRemux),
		"should_parse_brdisk_1080p_quality":            withoutProper(SourceBluray, Resolution1080p, ModifierBRDisk),
		"should_parse_raw_quality":                     withProper(SourceTV, Resolution1080p, ModifierRawHD),
		"quality_parse":                                withProper(SourceUnknown, ResolutionUnknown, ModifierNone),
	} {
		checks[method] = radarrQualityCheck(want)
	}
	return fixtureSuite{file: fixtures.RadarrQualityParserFixture, checks: checks}
}

func qualitySuites() []fixtureSuite {
	return []fixtureSuite{
		sonarrQualitySuite(),
		radarrQualitySuite(),
		{
			file: fixtures.SonarrExtendedQualityParserRegex,
			checks: map[string]checker{
				"should_parse_reality_from_title": realCheck(seriesQualityRules),
				"should_parse_version_from_title": versionCheck(seriesQualityRules),
			},
		},
		{
			file: fixtures.RadarrExtendedQualityParserRegex,
			checks: map[string]checker{
				"should_parse_reality_from_title": realCheck(movieQualityRules),
				"should_parse_version_from_title": versionCheck(movieQualityRules),
				"should_parse_ultrahd_from_title": func(t *testing.T, args fixtures.Args) {
					requireEqual(t, "resolution", parseQuality(argString(args, 0), movieQualityRules).resolution, Resolution2160p)
				},
			},
		},
	}
}

func TestQualityFixtures(t *testing.T) {
	runSuites(t, qualitySuites())
}

func TestSonarrQualityParsesItsOwnNames(t *testing.T) {
	for _, c := range fixtures.SonarrSelfQualities {
		t.Run(c.Name, func(t *testing.T) {
			r := parseQuality(fmt.Sprintf("My series S01E01 [%s]", c.Name), seriesQualityRules)
			requireEqual(t, "quality", r.id, sonarrQuality(t, c.Quality))
		})
	}
}

// otherSourceSeparators are the separators Sonarr and Radarr substitute for spaces in these cases.
func otherSourceSeparators() []string {
	return []string{"-", ".", " ", "_"}
}

func TestSonarrQualityFromOtherSource(t *testing.T) {
	for _, c := range fixtures.SonarrOtherSourceQualities {
		for _, sep := range otherSourceSeparators() {
			title := "My series S01E01 " + strings.ReplaceAll(c.Text, " ", sep)
			t.Run(title, func(t *testing.T) {
				r := parseQuality(title, seriesQualityRules)
				requireEqual(t, "quality", r.id, sonarrQuality(t, c.Quality))
				requireEqual(t, "version", r.revision.Version, defaultRevisionVersion)
			})
		}
	}
}

func TestRadarrQualityFromOtherSource(t *testing.T) {
	resolutions := map[string]Resolution{
		"Unknown": ResolutionUnknown, "R480p": Resolution480p, "R720p": Resolution720p,
		"R1080p": Resolution1080p, "R2160p": Resolution2160p,
	}
	for _, c := range fixtures.RadarrOtherSourceQualities {
		for _, sep := range otherSourceSeparators() {
			title := "My series S01E01 " + strings.ReplaceAll(c.Text, " ", sep)
			t.Run(title, func(t *testing.T) {
				want := radarrExpectation{QualitySource(c.Source), resolutions[c.Resolution], QualityModifier(c.Modifier), false}
				verifyRadarrQuality(t, title, want, false)
			})
		}
	}
}
