// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strconv"
	"strings"

	"magpie/internal/domain"
)

// Revision is a release's proper/repack/real/version marker.
type Revision struct {
	Version int
	Real    int
	Repack  bool
}

const (
	defaultRevisionVersion = 1
	properRevisionVersion  = 2

	// The remux regexes allow up to "Hybrid-UHD-" before "remux".
	remuxMaxLead = len("hybrid-uhd-")
)

// detectionSource records whether a quality part came from the name or the file extension.
type detectionSource int

const (
	detectedUnknown detectionSource = iota
	detectedFromName
	detectedFromExtension
)

type qualityResult struct {
	id                  QualityID
	resolution          Resolution
	revision            Revision
	sourceDetection     detectionSource
	resolutionDetection detectionSource
}

func (q *qualityResult) set(id QualityID) {
	q.id = id
	q.resolution = mustQuality(id).Resolution
}

func (q qualityResult) definition() QualityDefinition {
	return mustQuality(q.id)
}

func (q qualityResult) domain() domain.Quality {
	d := q.definition()
	d.Resolution = q.resolution
	return d.Quality(q.revision.Version)
}

// qualityRules holds what differs between Sonarr's and Radarr's QualityParser.
type qualityRules struct {
	kind        domain.MediaKind
	source      pattern
	version     pattern
	resolution  pattern
	codec       pattern
	remux       pattern
	germanRemux pattern
	brDisk      pattern
	bdRip576p   QualityID
	findQuality func(source QualityID, remux bool, resolution Resolution) QualityID
}

var (
	rawHDRegex       = mustPattern(`\b(?<rawhd>RawHD|Raw[-_. ]HD)\b`, ignoreCase).gated(foldAny("raw"))
	mpeg2Regex       = mustPattern(`\b(?<mpeg2>MPEG[-_. ]?2)\b`, caseSensitive).gated(exactAny("MPEG"))
	properRegex      = mustPattern(`\b(?<proper>proper)\b`, ignoreCase).gated(foldAny("proper"))
	repackRegex      = mustPattern(`\b(?<repack>repack\d?|rerip\d?)\b`, ignoreCase).gated(foldAny("repack", "rerip"))
	realRegex        = mustPattern(`\b(?<real>REAL)\b`, caseSensitive).gated(exactAny("REAL"))
	altResRegex      = mustPattern(`\b(?<R2160p>UHD)\b|(?<R2160p>\[4K\])`, ignoreCase).gated(foldAny("uhd", "[4k]"))
	otherSourceRegex = mustPattern(`(?<hdtv>HD[-_. ]TV)|(?<sdtv>SD[-_. ]TV)`, ignoreCase).gated(foldAny("tv"))
	animeBlurayRegex = mustPattern(`bd(?:720|1080|2160)|(?<=[-_. (\[])bd(?=[-_. )\]])`, ignoreCase).gated(foldAny("bd"))
	animeWebDLRegex  = mustPattern(`\[WEB\]|[\[\(]WEB[ .]`, ignoreCase).gated(foldAny("[web", "(web"))
	highDefPdtvRegex = mustPattern(`hr[-_. ]ws`, ignoreCase).gated(foldAny("hr"))
)

var seriesQualityRules = qualityRules{
	kind: domain.MediaKindSeries,
	source: mustScanPattern(`\b(?:
		(?<bluray>BluRay|Blu-Ray|HD-?DVD|BDMux|BD(?!$))|
		(?<webdl>WEB[-_. ]DL(?:mux)?|WEBDL|AmazonHD|AmazonSD|iTunesHD|MaxdomeHD|NetflixU?HD|WebHD|HBOMaxHD|DisneyHD|[. ]WEB[. ](?:[xh][ .]?26[456]|AVC|HEVC|EAC3|DDP?[ .]?5[. ]1)|[. ](?-i:WEB)$|(?:720|1080|2160)p[-. ]WEB[-. ]|[-. ]WEB[-. ](?:720|1080|2160)p|\b\s\/\sWEB\s\/\s\b|(?:AMZN|ATVP|DP|NF)[. -]WEB[. -](?!Rip))|
		(?<webrip>WebRip|Web-Rip|WEBMux)|
		(?<hdtv>HDTV)|
		(?<bdrip>BDRip|BDLight)|
		(?<brrip>BRRip)|
		(?<dvd>DVD|DVDRip|NTSC|PAL|xvidvd)|
		(?<dsr>WS[-_. ]DSR|DSR)|
		(?<pdtv>PDTV)|
		(?<sdtv>SDTV)|
		(?<tvrip>TVRip)
		)(?:\b|$|[ .])`, ignoreCase|ignoreWhitespace, seriesSourceStarts),
	version:     mustPattern(`\d[-._ ]?v(?<version>\d)[-._ ]|\[v(?<version>\d)\]|repack(?<version>\d)|rerip(?<version>\d)|(?:480|576|720|1080|2160)p[._ ]v(?<version>\d)`, ignoreCase).gated(vDigitOrRepack),
	resolution:  mustScanPattern(`\b(?:(?<R360p>360p)|(?<R480p>480p|480i|640x480|848x480)|(?<R540p>540p)|(?<R576p>576p)|(?<R720p>720p|1280x720|960p)|(?<R1080p>1080p|1920x1080|1440p|FHD|1080i|4kto1080p)|(?<R2160p>2160p|3840x2160|4k[-_. ](?:UHD|HEVC|BD|H26[56])|(?:UHD|HEVC|BD|H26[56])[-_. ]4k))\b`, ignoreCase, resolutionStarts),
	codec:       mustPattern(`\b(?:(?<x264>x264)|(?<h264>h264)|(?<xvidhd>XvidHD)|(?<xvid>Xvid)|(?<divx>divx))\b`, ignoreCase).gated(foldAny("264", "vid", "divx")),
	remux:       mustPattern(`(?:[_. ]|\d{4}p-|\bHybrid-)(?<remux>(?:(BD|UHD)[-_. ]?)?Remux)\b|(?<remux>(?:(BD|UHD)[-_. ]?)?Remux[_. ]\d{4}p)`, ignoreCase).hinted(literalHint("remux", remuxMaxLead)),
	bdRip576p:   QualityBluray480p,
	findQuality: sonarrFindQuality,
}

var movieQualityRules = qualityRules{
	kind: domain.MediaKindMovie,
	source: mustScanPattern(`\b(?:
		(?<bluray>M?Blu[-_. ]?Ray|HD[-_. ]?DVD|BD(?!$)|UHD2?BD|BDISO|BDMux|BD25|BD50|BR[-_. ]?DISK)|
		(?<webdl>WEB[-_. ]?DL(?:mux)?|AmazonHD|AmazonSD|iTunesHD|MaxdomeHD|NetflixU?HD|WebHD|HBOMaxHD|DisneyHD|[. ]WEB[. ](?:[xh][ .]?26[45]|AVC|HEVC|DDP?5[. ]1)|[. ](?-i:WEB)$|(?:\d{3,4}0p)[-. ](?:Hybrid[-_. ]?)?WEB[-. ]|[-. ]WEB[-. ]\d{3,4}0p|\b\s\/\sWEB\s\/\s\b|(?:AMZN|NF|DP)[. -]WEB[. -](?!Rip))|
		(?<webrip>WebRip|Web-Rip|WEBMux)|
		(?<hdtv>HDTV)|
		(?<bdrip>BDRip|BDLight|HD[-_. ]?DVDRip|UHDBDRip)|
		(?<brrip>BRRip)|
		(?<dvdr>\d?x?M?DVD-?[R59])|
		(?<dvd>DVD(?!-R)|DVDRip|xvidvd)|
		(?<dsr>WS[-_. ]DSR|DSR)|
		(?<regional>R[0-9]{1}|REGIONAL)|
		(?<scr>SCR|SCREENER|DVDSCR|DVDSCREENER)|
		(?<ts>TS[-_. ]|TELESYNCH?|HD-TS|HDTS|PDVD|TSRip|HDTSRip)|
		(?<tc>TC|TELECINE|HD-TC|HDTC)|
		(?<cam>CAMRIP|(?:NEW)?CAM|HD-?CAM(?:Rip)?|HQCAM)|
		(?<wp>WORKPRINT|WP)|
		(?<pdtv>PDTV)|
		(?<sdtv>SDTV)|
		(?<tvrip>TVRip)
		)(?:\b|$|[ .])`, ignoreCase|ignoreWhitespace, movieSourceStarts),
	version:     mustPattern(`\d[-._ ]?v(?<version>\d)[-._ ]|\[v(?<version>\d)\]|repack(?<version>\d)|rerip(?<version>\d)`, ignoreCase).gated(vDigitOrRepack),
	resolution:  mustScanPattern(`\b(?:(?<R360p>360p)|(?<R480p>480p|480i|640x480|848x480)|(?<R540p>540p)|(?<R576p>576p)|(?<R720p>720p|1280x720|960p)|(?<R1080p>1080p|1920x1080|1440p|FHD|1080i|4kto1080p)|(?<R2160p>2160p|3840x2160|4k[-_. ](?:UHD|HEVC|BD|H\.?265)|(?:UHD|HEVC|BD|H\.?265)[-_. ]4k))\b`, ignoreCase, resolutionStarts),
	codec:       mustPattern(`\b(?:(?<x264>x264)|(?<h264>h264)|(?<xvidhd>XvidHD)|(?<xvid>X-?vid)|(?<divx>divx))\b`, ignoreCase).gated(foldAny("264", "vid", "divx")),
	remux:       mustPattern(`(?:[_. \[]|\d{4}p-|\bHybrid-)(?<remux>(?:(BD|UHD)[-_. ]?)?Remux)\b|(?<remux>(?:(BD|UHD)[-_. ]?)?Remux[_. ]\d{4}p)`, ignoreCase).hinted(literalHint("remux", remuxMaxLead)),
	germanRemux: mustPattern(`((?<=\d{4}).*German.*([DM]L)?)(?=.*\b(AVC|HEVC|VC[_. -]?1|MVC|MPEG[_. -]?2))(?=.*Blu-?ray)`, ignoreCase).gated(foldAny("german")),
	brDisk:      mustPattern(`^(?!.*\b((?<!HD[._ -]|HD)DVD|BDRip|720p|MKV|XviD|WMV|d3g|(BD)?REMUX|^(?=.*1080p)(?=.*HEVC)|[xh][-_. ]?26[45]|German.*[DM]L|((?<=\d{4}).*German.*([DM]L)?)(?=.*\b(AVC|HEVC|VC[-_. ]?1|MVC|MPEG[-_. ]?2)\b))\b)(((?=.*\b(Blu[-_. ]?ray|BD|HD[-_. ]?DVD)\b)(?=.*\b(AVC|HEVC|VC[-_. ]?1|MVC|MPEG[-_. ]?2|BDMV|ISO)\b))|^((?=.*\b(((?=.*\b((.*_)?COMPLETE.*|Dis[ck])\b)(?=.*(Blu[-_. ]?ray|HD[-_. ]?DVD)))|3D[-_. ]?BD|BR[-_. ]?DISK|Full[-_. ]?Blu[-_. ]?ray|^((?=.*((BD|UHD)[-_. ]?(25|50|66|100|ISO)))))))).*`, ignoreCase).gated(brDiskGate),
	bdRip576p:   QualityBluray576p,
	findQuality: radarrFindQuality,
}

func parseQuality(name string, rules qualityRules) qualityResult {
	if strings.TrimSpace(name) == "" {
		return newQualityResult()
	}
	name = strings.TrimSpace(name)
	result := parseQualityName(name, rules)
	if result.id == QualityUnknown && !containsInvalidPathChars(name) {
		id, _ := extensionQuality(pathExtension(name), rules.kind)
		result.set(id)
		result.sourceDetection = detectedFromExtension
		result.resolutionDetection = detectedFromExtension
	}
	return result
}

func newQualityResult() qualityResult {
	return qualityResult{id: QualityUnknown, revision: Revision{Version: defaultRevisionVersion}}
}

func parseQualityName(name string, rules qualityRules) qualityResult {
	normalized := strings.TrimSpace(strings.ReplaceAll(name, "_", " "))
	result := parseQualityModifiers(name, normalized, rules)

	brDisk := rules.brDisk.isMatch(normalized)
	if rawHDRegex.isMatch(normalized) && !brDisk {
		result.sourceDetection = detectedFromName
		result.resolutionDetection = detectedFromName
		result.set(QualityRawHD)
		return result
	}

	sourceMatch := rules.source.lastMatch(normalized)
	resolution := parseResolution(normalized, rules)
	codec := rules.codec.match(normalized)
	remux := rules.remux.isMatch(normalized) || rules.germanRemux.isMatch(normalized)

	if resolution != ResolutionUnknown {
		result.resolutionDetection = detectedFromName
	}

	if sourceMatch != nil {
		result.sourceDetection = detectedFromName
		if id, ok := qualityFromSource(sourceMatch, name, normalized, codec, resolution, remux, brDisk, rules); ok {
			result.set(id)
			if id == QualityTelesync {
				result.resolution = resolution
			}
			if id == QualityHDTV720p && isTVRipSource(sourceMatch) && resolution != Resolution720p && !containsIgnoreCase(normalized, "720p") {
				result.resolutionDetection = detectedFromName
			}
			return result
		}
	}

	if sourceMatch == nil && remux && resolution != ResolutionUnknown {
		result.sourceDetection = detectedUnknown
		switch resolution {
		case Resolution480p:
			result.set(QualityBluray480p)
			return result
		case Resolution720p:
			result.set(QualityBluray720p)
			return result
		case Resolution2160p:
			result.set(QualityRemux2160p)
			return result
		case Resolution1080p:
			result.set(QualityRemux1080p)
			return result
		}
	}

	if animeBlurayRegex.isMatch(normalized) {
		result.sourceDetection = detectedFromName
		switch {
		case isSDResolution(resolution) || containsIgnoreCase(normalized, "480p"):
			result.resolutionDetection = detectedFromName
			result.set(QualityDVD)
		case resolution == Resolution1080p || containsIgnoreCase(normalized, "1080p"):
			result.resolutionDetection = detectedFromName
			result.set(pick(remux, QualityRemux1080p, QualityBluray1080p))
		case resolution == Resolution2160p || containsIgnoreCase(normalized, "2160p"):
			result.resolutionDetection = detectedFromName
			result.set(pick(remux, QualityRemux2160p, QualityBluray2160p))
		case remux && resolution != Resolution720p:
			result.set(QualityRemux1080p)
		default:
			result.set(QualityBluray720p)
		}
		return result
	}

	if animeWebDLRegex.isMatch(normalized) {
		result.sourceDetection = detectedFromName
		switch {
		case isSDResolution(resolution) || containsIgnoreCase(normalized, "480p"):
			result.resolutionDetection = detectedFromName
			result.set(QualityWEBDL480p)
		case resolution == Resolution1080p || containsIgnoreCase(normalized, "1080p"):
			result.resolutionDetection = detectedFromName
			result.set(QualityWEBDL1080p)
		case resolution == Resolution2160p || containsIgnoreCase(normalized, "2160p"):
			result.resolutionDetection = detectedFromName
			result.set(QualityWEBDL2160p)
		default:
			result.set(QualityWEBDL720p)
		}
		return result
	}

	if resolution != ResolutionUnknown {
		source := QualityUnknown
		if remux {
			result.sourceDetection = detectedFromName
		} else if id, ok := extensionQuality(pathExtension(name), rules.kind); ok && id != QualityUnknown {
			result.sourceDetection = detectedFromExtension
			source = id
		}
		bucket := resolution
		if isSDResolution(resolution) {
			bucket = Resolution480p
		}
		result.resolutionDetection = detectedFromName
		if source == QualityUnknown && !remux {
			result.set(unknownSourceQuality(bucket))
		} else {
			result.set(rules.findQuality(source, remux, bucket))
		}
		return result
	}

	if codec != nil && codec.success("x264") {
		result.set(QualitySDTV)
		return result
	}

	if containsIgnoreCase(normalized, "848x480") {
		result.resolutionDetection = detectedFromName
		switch {
		case strings.Contains(normalized, "dvd"):
			result.sourceDetection = detectedFromName
			result.set(QualityDVD)
		case containsIgnoreCase(normalized, "bluray"):
			result.sourceDetection = detectedFromName
			result.set(QualityBluray480p)
		default:
			result.set(QualitySDTV)
		}
		return result
	}

	if containsIgnoreCase(normalized, "1280x720") {
		result.resolutionDetection = detectedFromName
		if containsIgnoreCase(normalized, "bluray") {
			result.sourceDetection = detectedFromName
			result.set(QualityBluray720p)
		} else {
			result.set(QualityHDTV720p)
		}
		return result
	}

	if containsIgnoreCase(normalized, "1920x1080") {
		result.resolutionDetection = detectedFromName
		if containsIgnoreCase(normalized, "bluray") {
			result.sourceDetection = detectedFromName
			result.set(QualityBluray1080p)
		} else {
			result.set(QualityHDTV1080p)
		}
		return result
	}

	for _, fixed := range []struct {
		token string
		id    QualityID
	}{
		{"bluray720p", QualityBluray720p},
		{"bluray1080p", QualityBluray1080p},
		{"bluray2160p", QualityBluray2160p},
	} {
		if containsIgnoreCase(normalized, fixed.token) {
			result.sourceDetection = detectedFromName
			result.resolutionDetection = detectedFromName
			result.set(fixed.id)
			return result
		}
	}

	if m := otherSourceRegex.match(normalized); m != nil {
		switch {
		case m.success("sdtv"):
			result.sourceDetection = detectedFromName
			result.set(QualitySDTV)
		case m.success("hdtv"):
			result.sourceDetection = detectedFromName
			result.set(QualityHDTV720p)
		}
	}
	return result
}

func qualityFromSource(m *match, name, normalized string, codec *match, resolution Resolution, remux, brDisk bool, rules qualityRules) (QualityID, bool) {
	switch {
	case m.success("bluray"):
		switch {
		case brDisk:
			return QualityBRDisk, true
		case codec != nil && (codec.success("xvid") || codec.success("divx")):
			return QualityBluray480p, true
		case resolution == Resolution2160p:
			return pick(remux, QualityRemux2160p, QualityBluray2160p), true
		case resolution == Resolution1080p:
			return pick(remux, QualityRemux1080p, QualityBluray1080p), true
		case resolution == Resolution720p:
			return QualityBluray720p, true
		case resolution == Resolution576p:
			return QualityBluray576p, true
		case resolution == Resolution360p || resolution == Resolution480p || resolution == Resolution540p:
			return QualityBluray480p, true
		case remux:
			return QualityRemux1080p, true
		}
		return QualityBluray720p, true

	case m.success("webdl"):
		switch {
		case resolution == Resolution2160p:
			return QualityWEBDL2160p, true
		case resolution == Resolution1080p:
			return QualityWEBDL1080p, true
		case resolution == Resolution720p || strings.Contains(name, "[WEBDL]"):
			return QualityWEBDL720p, true
		}
		return QualityWEBDL480p, true

	case m.success("webrip"):
		switch resolution {
		case Resolution2160p:
			return QualityWEBRip2160p, true
		case Resolution1080p:
			return QualityWEBRip1080p, true
		case Resolution720p:
			return QualityWEBRip720p, true
		}
		return QualityWEBRip480p, true

	case m.success("scr"):
		return QualityDVDSCR, true
	case m.success("cam"):
		return QualityCam, true
	case m.success("ts"):
		return QualityTelesync, true
	case m.success("tc"):
		return QualityTelecine, true
	case m.success("wp"):
		return QualityWorkprint, true
	case m.success("regional"):
		return QualityRegional, true

	case m.success("hdtv"):
		switch {
		case mpeg2Regex.isMatch(normalized):
			return QualityRawHD, true
		case resolution == Resolution2160p:
			return QualityHDTV2160p, true
		case resolution == Resolution1080p:
			return QualityHDTV1080p, true
		case resolution == Resolution720p || strings.Contains(name, "[HDTV]"):
			return QualityHDTV720p, true
		}
		return QualitySDTV, true

	case m.success("bdrip") || m.success("brrip"):
		switch resolution {
		case Resolution720p:
			return QualityBluray720p, true
		case Resolution1080p:
			return QualityBluray1080p, true
		case Resolution2160p:
			return QualityBluray2160p, true
		case Resolution576p:
			return rules.bdRip576p, true
		}
		return QualityBluray480p, true

	case m.success("dvdr"):
		return QualityDVDR, true
	case m.success("dvd"):
		return QualityDVD, true

	case isTVRipSource(m):
		switch {
		case resolution == Resolution1080p || containsIgnoreCase(normalized, "1080p"):
			return QualityHDTV1080p, true
		case resolution == Resolution720p || containsIgnoreCase(normalized, "720p"):
			return QualityHDTV720p, true
		case highDefPdtvRegex.isMatch(normalized):
			return QualityHDTV720p, true
		}
		return QualitySDTV, true
	}
	return QualityUnknown, false
}

func parseResolution(name string, rules qualityRules) Resolution {
	m := rules.resolution.match(name)
	if m != nil {
		for _, r := range []struct {
			group      string
			resolution Resolution
		}{
			{"R360p", Resolution360p},
			{"R480p", Resolution480p},
			{"R540p", Resolution540p},
			{"R576p", Resolution576p},
			{"R720p", Resolution720p},
			{"R1080p", Resolution1080p},
			{"R2160p", Resolution2160p},
		} {
			if m.success(r.group) {
				return r.resolution
			}
		}
	}
	if alt := altResRegex.match(name); alt != nil && alt.success("R2160p") {
		return Resolution2160p
	}
	return ResolutionUnknown
}

func parseQualityModifiers(name, normalized string, rules qualityRules) qualityResult {
	result := newQualityResult()

	version := rules.version.match(normalized)
	parsedVersion := 0
	if version != nil {
		parsedVersion, _ = strconv.Atoi(version.value("version"))
		result.revision.Version = parsedVersion
	}

	bumped := properRevisionVersion
	if version != nil {
		bumped = parsedVersion + 1
	}
	if properRegex.isMatch(normalized) {
		result.revision.Version = bumped
	}
	if repackRegex.isMatch(normalized) {
		result.revision.Version = bumped
		result.revision.Repack = true
	}

	if reals := realRegex.matches(name); len(reals) > 0 {
		result.revision.Real = len(reals)
	}
	return result
}

func isTVRipSource(m *match) bool {
	return m.success("pdtv") || m.success("sdtv") || m.success("dsr") || m.success("tvrip")
}

func isSDResolution(r Resolution) bool {
	switch r {
	case Resolution360p, Resolution480p, Resolution540p, Resolution576p:
		return true
	}
	return false
}

func pick(cond bool, yes, no QualityID) QualityID {
	if cond {
		return yes
	}
	return no
}

// unknownSourceQuality is what both parsers pick for a bare resolution.
func unknownSourceQuality(r Resolution) QualityID {
	switch r {
	case Resolution2160p:
		return QualityHDTV2160p
	case Resolution1080p:
		return QualityHDTV1080p
	case Resolution720p:
		return QualityHDTV720p
	}
	return QualitySDTV
}
