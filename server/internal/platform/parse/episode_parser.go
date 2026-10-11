// Copyright (C) Magpie contributors.
// Portions ported from Sonarr (https://github.com/Sonarr/Sonarr), copyright its contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"magpie/internal/domain"
)

// AirDateLayout is the format of Release.AirDate.
const AirDateLayout = "2006-01-02"

const (
	earliestPlausibleYear  = 1900
	twoDigitYearPrefix     = "20"
	zeroMonthOrDay         = "00"
	monthsInYear           = 12
	futureAirDateTolerance = 14 * 24 * time.Hour
	seasonRangeEndpoints   = 2
	minTitleCaptures       = 2
	episodeRangeCaptures   = 2
	shortMonthLength       = len("jan")
)

var earliestDailyAirDate = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)

// errAbortParse mirrors an exception inside Sonarr's ParseTitle: the whole name is unparseable.
var errAbortParse = errors.New("abort parse")

var (
	episodeRejectHashedRegexes = []pattern{
		mustPattern(`^[0-9a-zA-Z]{32}`, caseSensitive).gated(leadingAlnum(32)),
		mustPattern(`^[a-z0-9]{24}$`, caseSensitive).gated(leadingAlnum(24)),
		mustPattern(`^[A-Z]{11}\d{3}$`, caseSensitive).gated(leadingAlnum(14)),
		mustPattern(`^[a-z]{12}\d{3}$`, caseSensitive).gated(leadingAlnum(15)),
		mustPattern(`^Backup_\d{5,}S\d{2}-\d{2}$`, caseSensitive).gated(startsWith("Backup_")),
		mustPattern(`^123$`, caseSensitive).gated(startsWith("123")),
		mustPattern(`^abc$`, ignoreCase).gated(foldPrefix("abc")),
		mustPattern(`^abc[-_. ]xyz`, ignoreCase).gated(foldPrefix("abc")),
		mustPattern(`^b00bs$`, ignoreCase).gated(foldPrefix("b00bs")),
		mustPattern(`^\d{6}_\d{2}$`, caseSensitive).gated(leadingDigits(6)),
		mustPattern(`^[0-9a-zA-Z]{30}`, caseSensitive).gated(leadingAlnum(30)),
		mustPattern(`^[0-9a-zA-Z]{26}`, caseSensitive).gated(leadingAlnum(26)),
		mustPattern(`^[0-9a-zA-Z]{39}`, caseSensitive).gated(leadingAlnum(39)),
		mustPattern(`^[0-9a-zA-Z]{24}`, caseSensitive).gated(leadingAlnum(24)),
	}

	seasonFolderOnlyRegex = mustPattern(`^(Season[ ._-]*\d+|Specials)$`, caseSensitive)

	episodeReversedTitleRegex = mustPattern(`(?:^|[-._ ])(p027|p0801|\d{2,3}E-?\d{2}S)[-._ ]`, caseSensitive)
	episodeSimpleTitleRegex   = mustPattern(`(?:(480|540|576|720|1080|1440|2160)[ip]|[xh][\W_]?26[456]|DD\W?5\W1|[<>?*]|848x480|1280x720|1920x1080|3840x2160|4096x2160|(?<![a-f0-9])(8|10)[ -]?(b(?![a-z0-9])|bit))\s*?`, ignoreCase)
	sixDigitAirDateRegex      = mustPattern(`(?<=[_.-])(?<airdate>(?<!\d)(?<airyear>[1-9]\d{1})(?<airmonth>[0-1][0-9])(?<airday>[0-3][0-9]))(?=[_.-])`, ignoreCase).gated(digitRunOf(6))
	cleanQualityBracketsRegex = mustPattern(`\[[a-z0-9 ._-]+\]$`, ignoreCase).gated(endsWithBracket)
	seriesYearInTitleRegex    = mustPattern(`^(?<title>.+?)[-_. ]+?[\(\[]?(?<year>\d{4})[\]\)]?`, ignoreCase).gated(digitRunAtLeast(4))
	titleComponentsRegex      = mustPattern(`^(?:(?<title>.+?) \((?<title>.+?)\)|(?<title>.+?) \| (?<title>.+?)|(?<title>.+?) AKA (?<title>.+?)|(?<title>.+?) / (?<title>.+?))$`, ignoreCase).gated(foldAny(" (", " | ", " aka ", " / "))
	seasonFolderRegex         = mustPattern(`^(?:S|Season|Saison|Series|Stagione|Sezon)[-_. ]*(?<season>(?<!\d+)\d{1,4}(?!\d+))(?:[_. ]+(?!\d+)|$)`, ignoreCase)
	simpleEpisodeNumberRegex  = mustPattern(`^[ex]?(?<episode>(?<!\d+)\d{1,3}(?!\d+))(?:[ex-](?<episode>(?<!\d+)\d{1,3}(?!\d+)))?(?:[_. ](?!\d+)(?<remaining>.+)|$)`, ignoreCase)
	requestInfoRegex          = mustPattern(`^(?:\[.+?\])+`, caseSensitive).gated(startsWith("["))
)

// numberWords lets "Part One" style mini-series parse; the index is the number.
func numberWords() []string {
	return []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
}

func shortMonth(name string) (time.Month, bool) {
	for m := time.January; m <= time.December; m++ {
		if strings.EqualFold(m.String()[:shortMonthLength], name) {
			return m, true
		}
	}
	return 0, false
}

type seriesTitleInfo struct {
	title            string
	titleWithoutYear string
	year             int
	allTitles        []string
}

// episodeInfo is Sonarr's ParsedEpisodeInfo.
type episodeInfo struct {
	releaseTitle            string
	seriesTitle             string
	titleInfo               seriesTitleInfo
	seasonNumbers           []int
	episodeNumbers          []int
	absoluteEpisodeNumbers  []int
	specialAbsoluteEpisodes []float64
	airDate                 string
	dailyPart               int
	fullSeason              bool
	partialSeason           bool
	seasonPart              int
	seasonExtra             bool
	splitEpisode            bool
	miniSeries              bool
	special                 bool
	releaseTokens           string
	languages               languages
	quality                 qualityResult
	releaseGroup            string
	releaseHash             string
}

func (e *episodeInfo) seasonNumber() int {
	if len(e.seasonNumbers) == 0 {
		return 0
	}
	return e.seasonNumbers[0]
}

// parseEpisodeTitle ports Sonarr's Parser.ParseTitle. It returns nil when the name cannot be parsed.
func parseEpisodeTitle(title string, now time.Time) *episodeInfo {
	if !validateEpisodeTitle(title) {
		return nil
	}

	if episodeReversedTitleRegex.isMatch(title) {
		withoutExtension := removeFileExtension(title, domain.MediaKindSeries)
		title = reverseRunes(withoutExtension) + substringFrom(title, len([]rune(withoutExtension)))
	}

	releaseTitle := removeFileExtension(title, domain.MediaKindSeries)
	releaseTitle = strings.NewReplacer("【", "[", "】", "]").Replace(releaseTitle)
	for _, sub := range seriesPreSubstitutions {
		sub.regex.tryReplace(&releaseTitle, sub.replacement)
	}

	simpleTitle := episodeSimpleTitleRegex.replace(releaseTitle, "")
	simpleTitle = websitePrefixRegex.replace(simpleTitle, "")
	simpleTitle = websitePostfixRegex.replace(simpleTitle, "")
	simpleTitle = cleanTorrentSuffixRegex.replace(simpleTitle, "")
	simpleTitle = cleanQualityBracketsRegex.replaceFunc(simpleTitle, func(value string) string {
		if parseQualityName(value, seriesQualityRules).id != QualityUnknown {
			return ""
		}
		return value
	})

	if m := sixDigitAirDateRegex.match(simpleTitle); m != nil {
		month, day := m.value("airmonth"), m.value("airday")
		if month != zeroMonthOrDay || day != zeroMonthOrDay {
			fixed := twoDigitYearPrefix + m.value("airyear") + "." + month + "." + day
			simpleTitle = strings.ReplaceAll(simpleTitle, m.value("airdate"), fixed)
		}
	}

	shape := newTitleShape(simpleTitle)
	for _, entry := range episodeTitleRegexes {
		if !entry.gate(shape) {
			continue
		}
		matches := entry.regex.matches(simpleTitle)
		if len(matches) == 0 {
			continue
		}
		result, err := parseEpisodeMatches(matches, releaseTitle, now)
		if err != nil {
			return nil
		}
		if result == nil {
			continue
		}

		if result.fullSeason && containsIgnoreCase(result.releaseTokens, "Special") {
			result.fullSeason = false
			result.special = true
		}
		result.languages = parseSeriesLanguages(result.releaseTokens)
		result.quality = parseQuality(title, seriesQualityRules)
		result.releaseGroup = parseReleaseGroup(releaseTitle, seriesReleaseGroupRules)
		if sub := matches[0].value("subgroup"); strings.TrimSpace(sub) != "" {
			result.releaseGroup = sub
		}
		result.releaseHash = releaseHash(matches[0])
		return result
	}
	return nil
}

func validateEpisodeTitle(title string) bool {
	if len(title) > maxNameLength {
		return false
	}
	lower := strings.ToLower(title)
	if strings.Contains(lower, "password") && strings.Contains(lower, "yenc") {
		return false
	}
	if !hasLetterOrDigit(title) {
		return false
	}
	withoutExtension := removeFileExtension(title, domain.MediaKindSeries)
	for _, re := range episodeRejectHashedRegexes {
		if re.isMatch(withoutExtension) {
			return false
		}
	}
	return !seasonFolderOnlyRegex.isMatch(withoutExtension)
}

func releaseHash(m *match) string {
	if !m.success("hash") {
		return ""
	}
	hash := strings.Trim(m.value("hash"), "[]()")
	if hash == "1280x720" {
		return ""
	}
	return hash
}

// parseEpisodeMatches ports Sonarr's ParseMatchCollection. A nil result with no error means
// "try the next regex"; an error means the whole name is unparseable.
func parseEpisodeMatches(matches []*match, releaseTitle string, now time.Time) (*episodeInfo, error) {
	first := matches[0]
	seriesName := strings.NewReplacer(".", " ", "_", " ").Replace(first.value("title"))
	seriesName = strings.Trim(requestInfoRegex.replace(seriesName, ""), " ")

	airYear, _ := strconv.Atoi(first.value("airyear"))
	lastIndex := first.endIndex("title")

	var result *episodeInfo
	if airYear < earliestPlausibleYear {
		result = &episodeInfo{releaseTitle: releaseTitle, seasonNumbers: []int{0}}
		for _, m := range matches {
			episodes := m.captures("episode")
			absolutes := m.captures("absoluteepisode")

			if len(episodes) > 0 {
				firstEpisode, err := parseNumber(episodes[0].String())
				if err != nil {
					return nil, err
				}
				lastEpisode, err := parseNumber(episodes[len(episodes)-1].String())
				if err != nil {
					return nil, err
				}
				if firstEpisode > lastEpisode {
					return nil, nil
				}
				result.episodeNumbers = numberRange(firstEpisode, lastEpisode)
				lastIndex = max(lastIndex, captureEnd(episodes[len(episodes)-1]))
				if m.success("special") {
					result.special = true
				}
				if m.success("splitepisode") {
					result.splitEpisode = true
				}
			}

			if len(absolutes) > 0 {
				firstAbsolute, err := parseDecimal(absolutes[0].String())
				if err != nil {
					return nil, err
				}
				lastAbsolute, err := parseDecimal(absolutes[len(absolutes)-1].String())
				if err != nil {
					return nil, err
				}
				if firstAbsolute > lastAbsolute {
					return nil, nil
				}
				if !isWhole(firstAbsolute) || !isWhole(lastAbsolute) {
					if len(absolutes) != 1 {
						return nil, nil
					}
					result.specialAbsoluteEpisodes = []float64{firstAbsolute}
					result.special = true
					lastIndex = max(lastIndex, captureEnd(absolutes[0]))
				} else {
					result.absoluteEpisodeNumbers = numberRange(int(firstAbsolute), int(lastAbsolute))
					if m.success("special") {
						result.special = true
					}
					lastIndex = max(lastIndex, captureEnd(absolutes[len(absolutes)-1]))
				}
			}

			if len(episodes) == 0 && len(absolutes) == 0 {
				if strings.TrimSpace(first.value("extras")) != "" {
					result.seasonExtra = true
				}
				if part := first.value("seasonpart"); strings.TrimSpace(part) != "" {
					seasonPart, err := strconv.Atoi(part)
					if err != nil {
						return nil, errAbortParse
					}
					result.seasonPart = seasonPart
					result.partialSeason = true
				} else if first.success("special") {
					result.special = true
				} else {
					result.fullSeason = true
				}
			}

			if len(episodes) == episodeRangeCaptures && first.success("episodecount") && episodes[len(episodes)-1].String() == first.value("episodecount") {
				result.episodeNumbers = nil
				result.fullSeason = true
			}
		}

		var seasons []int
		for _, c := range first.captures("season") {
			if season, err := strconv.Atoi(c.String()); err == nil {
				seasons = append(seasons, season)
				lastIndex = max(lastIndex, captureEnd(c))
			}
		}
		slices.Sort(seasons)
		seasons = slices.Compact(seasons)

		switch {
		case len(seasons) == 1 || len(seasons) > seasonRangeEndpoints:
			result.seasonNumbers = seasons
		case len(seasons) == seasonRangeEndpoints:
			result.seasonNumbers = numberRange(seasons[0], seasons[1])
		case len(result.absoluteEpisodeNumbers) == 0 && len(result.episodeNumbers) > 0:
			result.seasonNumbers = []int{1}
			result.miniSeries = true
		}
	} else {
		airDate, err := parseAirDate(first, airYear, now)
		if err != nil {
			return nil, err
		}
		lastIndex = max(lastIndex, first.endIndex("airyear"), first.endIndex("airmonth"), first.endIndex("airday"))
		result = &episodeInfo{releaseTitle: releaseTitle, seasonNumbers: []int{0}, airDate: airDate.Format(AirDateLayout)}
		if first.success("part") {
			part, err := strconv.Atoi(first.value("part"))
			if err != nil {
				return nil, errAbortParse
			}
			result.dailyPart = part
		}
	}

	if lastIndex < len([]rune(releaseTitle)) {
		result.releaseTokens = substringFrom(releaseTitle, lastIndex)
	} else {
		result.releaseTokens = releaseTitle
	}
	result.seriesTitle = seriesName
	result.titleInfo = parseSeriesTitleInfo(seriesName, first)
	return result, nil
}

func parseAirDate(m *match, airYear int, now time.Time) (time.Time, error) {
	var month, day int
	var err error
	switch {
	case m.success("ambiguousairmonth") && m.success("ambiguousairday"):
		month, _ = strconv.Atoi(m.value("ambiguousairmonth"))
		day, _ = strconv.Atoi(m.value("ambiguousairday"))
		if day <= monthsInYear && month <= monthsInYear {
			return time.Time{}, errAbortParse
		}
	case m.success("shortairmonth"):
		short, ok := shortMonth(m.value("shortairmonth"))
		if !ok {
			return time.Time{}, errAbortParse
		}
		month = int(short)
		if day, err = strconv.Atoi(m.value("airday")); err != nil {
			return time.Time{}, errAbortParse
		}
	default:
		if month, err = strconv.Atoi(m.value("airmonth")); err != nil {
			return time.Time{}, errAbortParse
		}
		if day, err = strconv.Atoi(m.value("airday")); err != nil {
			return time.Time{}, errAbortParse
		}
	}

	if month > monthsInYear {
		month, day = day, month
	}

	airDate := time.Date(airYear, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if airDate.Year() != airYear || int(airDate.Month()) != month || airDate.Day() != day {
		return time.Time{}, errAbortParse
	}

	latest := now.Add(futureAirDateTolerance)
	latestDate := time.Date(latest.Year(), latest.Month(), latest.Day(), 0, 0, 0, 0, time.UTC)
	if airDate.After(latestDate) {
		return time.Time{}, errAbortParse
	}
	if airDate.Before(earliestDailyAirDate) && strings.TrimSpace(m.value("titleyear")) == "" {
		return time.Time{}, errAbortParse
	}
	return airDate, nil
}

func parseSeriesTitleInfo(title string, first *match) seriesTitleInfo {
	info := seriesTitleInfo{title: title, titleWithoutYear: title}
	if m := seriesYearInTitleRegex.match(title); m != nil {
		info.titleWithoutYear = m.value("title")
		info.year, _ = strconv.Atoi(m.value("year"))
	}

	if m := titleComponentsRegex.match(info.titleWithoutYear); m != nil {
		for _, c := range m.captures("title") {
			info.allTitles = append(info.allTitles, c.String())
		}
	} else if titles := first.captures("title"); len(titles) >= minTitleCaptures {
		for _, c := range titles {
			info.allTitles = append(info.allTitles, strings.NewReplacer(".", " ", "_", " ").Replace(c.String()))
		}
	}
	return info
}

func numberRange(first, last int) []int {
	out := make([]int, 0, last-first+1)
	for n := first; n <= last; n++ {
		out = append(out, n)
	}
	return out
}

func isWhole(f float64) bool {
	return math.Mod(f, 1) == 0
}

// parseNumber ports Sonarr's ParseNumber: digits in any script, or an English number word.
func parseNumber(value string) (int, error) {
	if n, err := strconv.Atoi(toASCIIDigits(value)); err == nil {
		return n, nil
	}
	if n := slices.Index(numberWords(), strings.ToLower(value)); n >= 0 {
		return n, nil
	}
	return 0, fmt.Errorf("%w: %q isn't a number", errAbortParse, value)
}

func parseDecimal(value string) (float64, error) {
	f, err := strconv.ParseFloat(toASCIIDigits(value), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q isn't a number", errAbortParse, value)
	}
	return f, nil
}

// toASCIIDigits rewrites Unicode decimal digits (fullwidth, Arabic-Indic, ...) as ASCII.
func toASCIIDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r <= unicode.MaxASCII || !unicode.IsDigit(r) {
			return r
		}
		zero := r
		for unicode.IsDigit(zero - 1) {
			zero--
		}
		return '0' + (r-zero)%10
	}, s)
}

// parseEpisodePath ports Sonarr's Parser.ParsePath.
func parseEpisodePath(path string, now time.Time) *episodeInfo {
	dir, fileName := splitPath(path)
	dirName := baseName(dir)

	pathSeasonEpisodeSpaceRegex.tryReplace(&fileName, pathSeasonEpisodeSpaceReplacement)

	result := parseEpisodeTitle(fileName, now)

	if episodeMatch := simpleEpisodeNumberRegex.match(fileName); episodeMatch != nil && dir != "" &&
		(result == nil || result.miniSeries || len(result.absoluteEpisodeNumbers) > 0) {
		if seasonMatch := seasonFolderRegex.match(dirName); seasonMatch != nil && seasonMatch.success("season") {
			episodes := episodeMatch.captures("episode")
			firstEpisode, err := parseNumber(episodes[0].String())
			if err != nil {
				return nil
			}
			lastEpisode, err := parseNumber(episodes[len(episodes)-1].String())
			if err != nil {
				return nil
			}
			pathTitle := fmt.Sprintf("S%sE%02d", seasonMatch.value("season"), firstEpisode)
			if firstEpisode != lastEpisode {
				pathTitle += fmt.Sprintf("-E%02d", lastEpisode)
			}
			if episodeMatch.success("remaining") {
				pathTitle += " " + episodeMatch.value("remaining")
			}
			return parseEpisodeTitle(pathTitle, now)
		}
	}

	if number, err := strconv.Atoi(strings.TrimSuffix(fileName, pathExtension(fileName))); result == nil && err == nil {
		result = parseEpisodeTitle(dirName, now)
		switch {
		case result != nil && slices.Contains(result.absoluteEpisodeNumbers, number):
			result.absoluteEpisodeNumbers = []int{number}
		case result != nil && slices.Contains(result.episodeNumbers, number):
			result.episodeNumbers = []int{number}
		default:
			result = nil
		}
	}

	if result == nil {
		result = parseEpisodeTitle(dirName+" "+fileName, now)
	}
	if result == nil {
		result = parseEpisodeTitle(dirName+pathExtension(fileName), now)
	}
	return result
}

// splitPath splits on the last / or \, so Windows paths from download clients work too.
func splitPath(path string) (dir, file string) {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

func baseName(path string) string {
	_, name := splitPath(path)
	return name
}
