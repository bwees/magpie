// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr) and
// Radarr (https://github.com/Radarr/Radarr), copyright their contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strings"
	"unicode"

	"magpie/internal/domain"
)

var (
	fileExtensionRegex = mustPattern(`\.[a-z0-9]{2,4}$`, ignoreCase)

	websitePrefixRegex      = mustPattern(`^(?:(?:\[|\()\s*)?(?:www\.)?[-a-z0-9-]{1,256}\.(?<!Naruto-Kun\.)(?:[a-z]{2,6}\.[a-z]{2,6}|xn--[a-z0-9-]{4,}|[a-z]{2,})\b(?:\s*(?:\]|\))|[ -]{2,})[ -]*`, ignoreCase).gated(websitePrefixGate)
	websitePostfixRegex     = mustPattern(`(?:\[\s*)?(?:www\.)?[-a-z0-9-]{1,256}\.(?:xn--[a-z0-9-]{4,}|[a-z]{2,6})\b(?:\s*\])$`, ignoreCase).gated(endsWithBracket)
	cleanTorrentSuffixRegex = mustPattern(`\[(?:ettv|rartv|rarbg|cttv|publichd)\]$`, ignoreCase).gated(endsWithBracket)

	pathSeasonEpisodeSpaceRegex = mustPattern(`(?<separator>^|[-_. ])S(?<season>(?<!\d+)\d{1,2}(?!\d+)) (?<episode>(?<!\d+)\d{1,2}(?!\d+))(?!\d+)`, ignoreCase)
)

const pathSeasonEpisodeSpaceReplacement = "${separator}S${season}E${episode}"

// maxNameLength rejects longer names as unparseable. Some of Sonarr's and Radarr's regexes
// backtrack polynomially on bracket-heavy names: 300 bytes still parses in well under a
// second, while real release names stay under 200.
const maxNameLength = 300

type preSubstitution struct {
	regex       pattern
	replacement string
}

// seriesPreSubstitutions normalises Korean and Chinese release layouts before Sonarr's title regexes run.
var seriesPreSubstitutions = []preSubstitution{
	{mustPattern(`\.E(\d{2,4})\.\d{6}\.(.*-(F1RST|NEXT))$`, caseSensitive).gated(exactAny("F1RST", "NEXT")), ".S01E$1.$2"},
	{mustPattern(`^\[(?:(?<subgroup>[^\]]+?)(?:[\u4E00-\u9FCC]+)?)\]\[(?<title>[^\]]+?)(?:\s(?<chinesetitle>[\u4E00-\u9FCC][^\]]*?))\]\[(?:(?:[\u4E00-\u9FCC]+?)?(?<episode>\d{1,4})(?:[\u4E00-\u9FCC]+?)?)\]`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} - "},
	{mustPattern(`^\[(?<subgroup>[^\]]*?(?:LoliHouse|ZERO|Lilith-Raws|Skymoon-Raws|orion origin)[^\]]*?)\](?<title>[^\[\]]+?)(?: - (?<episode>[0-9-]+)\s*|\[第?(?<episode>[0-9]+(?:-[0-9]+)?)话?(?:END|完)?\])\[`, caseSensitive).gated(startsWith("[")), "[${subgroup}][${title}][${episode}]["},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s?★[^\[ -]+\s?)?\[?(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\]\[|\s*[_/·]\s*)){0,2}(?<title>[^\[\]]+?)(?:\s(?:S?(?<!\d+)((0)(?<season>\d)|(?<season>[1-9]\d))(?!\d+)))\]?(?:\[\d{4}\])?\[第?(?<episode>[0-9]+(?:-[0-9]+)?)(?:话|集)?(?: ?END|完| ?Fin)?\]`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} S${season} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\]\[(?<chinesetitle>(?<![^a-zA-Z0-9])[^a-zA-Z0-9]+)(?<title>[^\]]+?)\](?:\[\d{4}\])?\[第?(?<episode>[0-9]+(?:-[0-9]+)?)(?:话|集)?(?: ?END|完| ?Fin)?\]`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s?★[^\[ -]+\s?)?\[?(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\]\[|\s*[_/·]\s*)){0,2}(?<title>[^\]]+?)\]?(?:\[\d{4}\])?\[(?:第|全)?(?<episode>[0-9]{1,4}-[0-9]{1,4})(?:话|集)?\](?:\[(?<language>.+?)\+.+?\])[_. ](?<title>.+?)S(?<season>[0-9]{1,2})(?<rest>.+?)$`, caseSensitive).gated(startsWith("[")), "${title}S${season}E${episode}.${language}${rest}"},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s?★[^\[ -]+\s?)?\[?(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\]\[|\s*[_/·]\s*)){0,2}(?<title>[^\]]+?)\]?(?:\[\d{4}\])?\[(?:全)?(?<episode>[0-9]{1,4})(?:话|集)?\](?:\[(?<language>.+?)\+.+?\])[_. ](?<title>.+?)S(?<season>[0-9]{1,2})(?<rest>.+?)$`, caseSensitive).gated(startsWith("[")), "${title}S${season}.${language}${rest}"},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s?★[^\[ -]+\s?)?\[?(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\]\[|\s*[_/·]\s*)){0,2}(?<title>[^\]]+?)\]?(?:\[\d{4}\])?\[第?(?<episode>[0-9]{1,4}(?:-[0-9]{1,4})?)(?:话|集)?(?: ?END|完| ?Fin)?\]`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s)(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\s/\s))(?<title>[^\[\]]+?)(?:\s(?:S?(?<!\d+)((0)(?<season>\d)|(?<season>[1-9]\d))(?!\d+)))(?:[- ]+)(?<episode>[0-9]+(?:-[0-9]+)?)话?(?:END|完)?`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} S${season} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s)(?:(?<title>[^\]]+?)(?:\s/\s))(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:[- ]+)(?<episode>[0-9]+(?:-[0-9]+)?(?![a-z]))话?(?:END|完)?`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s)(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\s/\s))(?<title>[^\]]+?)(?:[- ]+)(?<episode>[0-9]+(?:-[0-9]+)?(?![a-z]))话?(?:END|完)?`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:(?<chinesubgroup>\[(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*\])+)\[(?<title>[^\]]+?)\](?<junk>\[[^\]]+\])*\[(?<episode>[0-9]+(?:-[0-9]+)?)( END| Fin)?\]`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} "},
	{mustPattern(`^\[(?<subgroup>[^\]]+)\](?:\s)(?:(?<chinesetitle>(?=[^\]]*?[\u4E00-\u9FCC])[^\]]*?)(?:\s\|\s))(?<title>[^\]]+?)(?:[- ]+)(?<episode>[0-9]+(?:-[0-9]+)?(?![a-z]))话?(?:END|完)?`, caseSensitive).gated(startsWith("[")), "[${subgroup}] ${title} - ${episode} "},
	{mustPattern(`^(?<title>.+?(?=[ ._-]\()).+?\((?<year>\d{4})\/(?<info>S[^\/]+)`, caseSensitive).gated(allOf(exactAny("("), exactAny("/"))), "${title} (${year}) - ${info} "},
}

// extensionQuality is Sonarr's and Radarr's MediaFileExtensions table. ok is false for unknown extensions.
func extensionQuality(ext string, kind domain.MediaKind) (id QualityID, ok bool) {
	movie := kind == domain.MediaKindMovie
	switch strings.ToLower(ext) {
	case ".webm":
		return QualityUnknown, true
	case ".m4v", ".3gp", ".nsv", ".ty", ".strm", ".rm", ".rmvb", ".m3u", ".ifo", ".mov", ".qt", ".divx",
		".xvid", ".bivx", ".nrg", ".pva", ".wmv", ".asf", ".asx", ".ogm", ".ogv", ".m2v", ".avi", ".bin",
		".dat", ".dvr-ms", ".mpg", ".mpeg", ".mp4", ".avc", ".vp3", ".svq3", ".nuv", ".viv", ".dv", ".fli",
		".flv", ".wpl":
		return QualitySDTV, true
	case ".img", ".iso", ".vob":
		return QualityDVD, true
	case ".mkv":
		if movie {
			return QualityWEBDL720p, true
		}
		return QualityHDTV720p, true
	case ".mk3d":
		if movie {
			return QualityWEBDL720p, true
		}
	case ".ts", ".wtv":
		if movie {
			return QualitySDTV, true
		}
		return QualityHDTV720p, true
	case ".m2ts":
		return QualityBluray720p, true
	}
	return QualityUnknown, false
}

func isUsenetExtension(ext string) bool {
	switch ext {
	case ".par2", ".nzb":
		return true
	}
	return false
}

// removeFileExtension strips a trailing media or usenet extension and leaves any other suffix.
func removeFileExtension(title string, kind domain.MediaKind) string {
	return fileExtensionRegex.replaceFunc(title, func(value string) string {
		ext := strings.ToLower(value)
		if _, ok := extensionQuality(ext, kind); ok || isUsenetExtension(ext) {
			return ""
		}
		return value
	})
}

// pathExtension is .NET's Path.GetExtension: the text from the last dot of the last path segment.
func pathExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		switch path[i] {
		case '.':
			if i == len(path)-1 {
				return ""
			}
			return path[i:]
		case '/', '\\':
			return ""
		}
	}
	return ""
}

func containsInvalidPathChars(s string) bool {
	return strings.ContainsRune(s, 0)
}

func hasLetterOrDigit(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// substringFrom slices by rune index, matching how regex indexes are counted.
func substringFrom(s string, index int) string {
	runes := []rune(s)
	if index >= len(runes) {
		return ""
	}
	return string(runes[index:])
}

func reverseRunes(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
