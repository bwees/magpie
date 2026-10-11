// Copyright (C) Magpie contributors.
// Portions ported from Radarr (https://github.com/Radarr/Radarr), copyright its contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strconv"
	"strings"

	"magpie/internal/domain"
)

const editionPattern = `\(?\b(?<edition>(((Recut.|Extended.|Ultimate.)?(Director.?s|Collector.?s|Theatrical|Ultimate|Extended|Despecialized|(Special|Rouge|Final|Assembly|Imperial|Diamond|Signature|Hunter|Rekall)(?=(.(Cut|Edition|Version)))|\d{2,3}(th)?.Anniversary)(?:.(Cut|Edition|Version))?(.(Extended|Uncensored|Remastered|Unrated|Uncut|Open.?Matte|IMAX|Fan.?Edit))?|((Uncensored|Remastered|Unrated|Uncut|Open?.Matte|IMAX|Fan.?Edit|Restored|((2|3|4)in1))))))\b\)?`

const (
	movieTitlePlaceholder       = "A Movie"
	movieTitlePlaceholderDotted = "A.Movie"
	releaseGroupPlaceholder     = "RlsGrp"
	movieTitleOpenParen         = "("
)

var (
	reportEditionRegex = mustPattern(`^.+?`+editionPattern, ignoreCase).gated(editionGate)

	// movieTitleRegexes is Radarr's ReportMovieTitleRegex list. Order matters: the first match wins.
	movieTitleRegexes = []pattern{
		// Anime [Subgroup] with year explicitly in parentheses, e.g. [Sub] Title 1993 (1993) [VHS]
		mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>(?![(\[]).+?)\s*\((?<year>(1(8|9)|20)\d{2})\).*?(?<hash>\[\w{8}\])?(?:$|\.)`, ignoreCase).gated(startsWith("[")),
		// Anime [Subgroup] and Year
		mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>(?![(\[]).+?)?(?:(?:[-_\W](?<![)\[!]))*(?<year>(1(8|9)|20)\d{2}(?!p|i|x|\d+|\]|\W\d+)))+.*?(?<hash>\[\w{8}\])?(?:$|\.)`, ignoreCase).gated(startsWith("[")),
		// Anime [Subgroup] no year, versioned title, hash
		mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>(?![(\[]).+?)((v)(?:\d{1,2})(?:([-_. ])))(\[.*)?(?:[\[(][^])])?.*?(?<hash>\[\w{8}\])(?:$|\.)`, ignoreCase).gated(startsWith("[")),
		// Anime [Subgroup] no year, info in double sets of brackets, hash
		mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>(?![(\[]).+?)(\[.*).*?(?<hash>\[\w{8}\])(?:$|\.)`, ignoreCase).gated(startsWith("[")),
		// Anime [Subgroup] no year, info in parentheses or brackets, hash
		mustPattern(`^(?:\[(?<subgroup>.+?)\][-_. ]?)(?<title>(?![(\[]).+)(?:[\[(][^])]).*?(?<hash>\[\w{8}\])(?:$|\.)`, ignoreCase).gated(startsWith("[")),
		// Some German or TrueFrench tracker formats (missing year, ...)
		mustPattern(`^(?<title>(?![(\[]).+?)((\W|_))(`+editionPattern+`.{1,3})?(?:(?<!(19|20)\d{2}.*?)(?<!(?:Good|The)[_ .-])(German|TrueFrench))(.+?)(?=((19|20)\d{2}|$))(?<year>(19|20)\d{2}(?!p|i|\d+|\]|\W\d+))?(\W+|_|$)(?!\\)`, ignoreCase).gated(foldAny("german", "truefrench")),
		// Special, Despecialized, etc. Edition Movies, e.g: Mission.Impossible.3.Special.Edition.2011
		mustPattern(`^(?<title>(?![(\[]).+?)?(?:(?:[-_\W](?<![)\[!]))*`+editionPattern+`.{1,3}(?<year>(1(8|9)|20)\d{2}(?!p|i|\d+|\]|\W\d+)))+(\W+|_|$)(?!\\)`, ignoreCase).gated(editionGate),
		// Normal movie format, e.g: Mission.Impossible.3.2011
		mustPattern(`^(?<title>(?![(\[]).+?)?(?:(?:[-_\W](?<![)\[!]))*(?<year>(1(8|9)|20)\d{2}(?!p|i|(1(8|9)|20)\d{2}|\]|\W(1(8|9)|20)\d{2})))+(\W+|_|$)(?!\\)`, ignoreCase),
		// PassThePopcorn Torrent names: Star.Wars[PassThePopcorn]
		mustPattern(`^(?<title>.+?)?(?:(?:[-_\W](?<![()\[!]))*(?<year>(\[\w *\])))+(\W+|_|$)(?!\\)`, ignoreCase),
		// That did not work? Maybe some tool uses [] for years. Who would do that?
		mustPattern(`^(?<title>(?![(\[]).+?)?(?:(?:[-_\W](?<![)!]))*(?<year>(1(8|9)|20)\d{2}(?!p|i|\d+|\W\d+)))+(\W+|_|$)(?!\\)`, ignoreCase),
		// As a last resort for movies that have ( or [ in their title.
		mustPattern(`^(?<title>.+?)?(?:(?:[-_\W](?<![)\[!]))*(?<year>(1(8|9)|20)\d{2}(?!p|i|\d+|\]|\W\d+)))+(\W+|_|$)(?!\\)`, ignoreCase),
	}

	// movieFolderTitleRegexes handle folders named with the year first.
	movieFolderTitleRegexes = []pattern{
		mustPattern(`^(?:(?:[-_\W](?<![)!]))*(?<year>(19|20)\d{2}(?!p|i|\d+|\W\d+)))+(\W+|_|$)(?<title>.+?)?$`, caseSensitive),
	}

	movieRejectHashedRegexes = []pattern{
		mustPattern(`^[0-9a-zA-Z]{32}`, caseSensitive).gated(leadingAlnum(32)),
		mustPattern(`^[a-z0-9]{24}$`, caseSensitive).gated(leadingAlnum(24)),
		mustPattern(`^[A-Z]{11}\d{3}$`, caseSensitive).gated(leadingAlnum(14)),
		mustPattern(`^[a-z]{12}\d{3}$`, caseSensitive).gated(leadingAlnum(15)),
		mustPattern(`^Backup_\d{5,}S\d{2}-\d{2}$`, caseSensitive).gated(startsWith("Backup_")),
		mustPattern(`^123$`, caseSensitive).gated(startsWith("123")),
		mustPattern(`^abc$`, ignoreCase).gated(foldPrefix("abc")),
		mustPattern(`^abc[-_. ]xyz`, ignoreCase).gated(foldPrefix("abc")),
		mustPattern(`^b00bs$`, ignoreCase).gated(foldPrefix("b00bs")),
	}

	movieReversedTitleRegex        = mustPattern(`(?:^|[-._ ])(p027|p0801)[-._ ]`, caseSensitive)
	alternativeTitleRegex          = mustPattern(`[ ]+(?:AKA|\/)[ ]+`, ignoreCase).gated(foldAny("aka", "/"))
	bracketedAlternativeTitleRegex = mustPattern(`(.*) \([ ]*AKA[ ]+(.*)\)`, ignoreCase).gated(foldAny("aka"))
	normalizeAlternativeTitleRegex = mustPattern(`[ ]+(?:A\.K\.A\.)[ ]+`, ignoreCase).gated(foldAny("a.k.a."))
	movieSimpleTitleRegex          = mustPattern(`(?:(480|540|576|720|1080|2160)[ip]|[xh][\W_]?26[45]|DD\W?5\W1|[<>?*]|848x480|1280x720|1920x1080|3840x2160|4096x2160|(8|10)b(it)?|10-bit)\s*?(?![a-b0-9])`, ignoreCase)
	simpleReleaseTitleRegex        = mustPattern(`\s*(?:[<>?*|])`, ignoreCase)
	movieCleanQualityBracketsRegex = mustPattern(`\[[a-z0-9 ._-]+\]$`, ignoreCase).gated(endsWithBracket)
)

// movieInfo is Radarr's ParsedMovieInfo.
type movieInfo struct {
	titles       []string
	year         int
	edition      string
	languages    languages
	quality      qualityResult
	releaseGroup string
	releaseHash  string
	releaseTitle string
}

func (m *movieInfo) primaryTitle() string {
	if len(m.titles) == 0 {
		return ""
	}
	return m.titles[0]
}

// parseMovieTitle ports Radarr's Parser.ParseMovieTitle. It returns nil when the name cannot be parsed.
func parseMovieTitle(title string, isDir bool) *movieInfo {
	if !validateMovieTitle(title) {
		return nil
	}

	if movieReversedTitleRegex.isMatch(title) {
		withoutExtension := removeFileExtension(title, domain.MediaKindMovie)
		title = reverseRunes(withoutExtension) + substringFrom(title, len([]rune(withoutExtension)))
	}

	releaseTitle := removeFileExtension(title, domain.MediaKindMovie)
	releaseTitle = strings.Trim(releaseTitle, "-_")
	releaseTitle = strings.NewReplacer("【", "[", "】", "]").Replace(releaseTitle)

	simpleTitle := movieSimpleTitleRegex.replace(releaseTitle, "")
	simpleTitle = websitePrefixRegex.replace(simpleTitle, "")
	simpleTitle = websitePostfixRegex.replace(simpleTitle, "")
	simpleTitle = cleanTorrentSuffixRegex.replace(simpleTitle, "")
	simpleTitle = movieCleanQualityBracketsRegex.replaceFunc(simpleTitle, func(value string) string {
		if parseQualityName(value, movieQualityRules).id != QualityUnknown {
			return ""
		}
		return value
	})

	regexes := movieTitleRegexes
	if isDir {
		regexes = append(append([]pattern{}, movieTitleRegexes...), movieFolderTitleRegexes...)
	}

	for _, re := range regexes {
		matches := re.matches(simpleTitle)
		if len(matches) == 0 {
			continue
		}
		result := parseMovieMatch(matches[0])
		if result == nil {
			continue
		}
		return finishMovie(result, matches[0], title, releaseTitle)
	}
	return nil
}

func finishMovie(result *movieInfo, m *match, title, releaseTitle string) *movieInfo {
	simpleReleaseTitle := simpleReleaseTitleRegex.replace(releaseTitle, "")

	replaceWith := result.primaryTitle()
	if m.success("title") {
		replaceWith = m.value("title")
	}
	if strings.TrimSpace(replaceWith) != "" {
		placeholder := movieTitlePlaceholder
		if strings.Contains(replaceWith, ".") {
			placeholder = movieTitlePlaceholderDotted
		}
		if m.success("title") {
			runes := []rune(simpleReleaseTitle)
			start, length := m.index("title"), m.length("title")
			if start+length > len(runes) {
				return nil
			}
			simpleReleaseTitle = string(runes[:start]) + placeholder + string(runes[start+length:])
		} else {
			simpleReleaseTitle = strings.ReplaceAll(simpleReleaseTitle, replaceWith, placeholder)
		}
	}

	result.releaseGroup = parseReleaseGroup(simpleReleaseTitle, movieReleaseGroupRules)
	if sub := m.value("subgroup"); strings.TrimSpace(sub) != "" {
		result.releaseGroup = sub
	}

	languageTitle := simpleReleaseTitle
	if strings.TrimSpace(result.releaseGroup) != "" {
		languageTitle = strings.ReplaceAll(simpleReleaseTitle, result.releaseGroup, releaseGroupPlaceholder)
	}
	result.languages = parseMovieLanguages(languageTitle)
	result.quality = parseQuality(title, movieQualityRules)
	if strings.TrimSpace(result.edition) == "" {
		result.edition = parseEdition(simpleReleaseTitle)
	}
	result.releaseHash = movieReleaseHash(m)
	result.releaseTitle = releaseTitle
	return result
}

func validateMovieTitle(title string) bool {
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
	withoutExtension := removeFileExtension(title, domain.MediaKindMovie)
	for _, re := range movieRejectHashedRegexes {
		if re.isMatch(withoutExtension) {
			return false
		}
	}
	return true
}

func movieReleaseHash(m *match) string {
	if !m.success("hash") {
		return ""
	}
	hash := strings.Trim(m.value("hash"), "[]")
	if hash == "1280x720" {
		return ""
	}
	return hash
}

func parseEdition(title string) string {
	m := reportEditionRegex.match(title)
	if m == nil || strings.TrimSpace(m.value("edition")) == "" {
		return ""
	}
	return strings.ReplaceAll(m.value("edition"), ".", " ")
}

// parseMovieMatch ports Radarr's ParseMovieMatchCollection.
func parseMovieMatch(m *match) *movieInfo {
	if !m.success("title") || m.value("title") == movieTitleOpenParen {
		return nil
	}

	name := strings.ReplaceAll(m.value("title"), "_", " ")
	name = normalizeAlternativeTitleRegex.replace(name, " AKA ")
	name = strings.Trim(requestInfoRegex.replace(name, ""), " ")
	name = joinTitleParts(strings.Split(name, "."))

	result := &movieInfo{}
	result.year, _ = strconv.Atoi(m.value("year"))
	if m.success("edition") {
		result.edition = strings.ReplaceAll(m.value("edition"), ".", " ")
	}

	result.titles = []string{name}
	unbracketed := bracketedAlternativeTitleRegex.replace(name, "$1 AKA $2")
	for _, alternative := range alternativeTitleRegex.split(unbracketed) {
		if strings.TrimSpace(alternative) != "" && alternative != name {
			result.titles = append(result.titles, alternative)
		}
	}
	return result
}

// joinTitleParts rebuilds a dotted title, keeping acronyms such as "U.N.C.L.E." and "Dr." dotted.
func joinTitleParts(parts []string) string {
	var b strings.Builder
	previousAcronym := false
	for n, part := range parts {
		nextPart := ""
		if len(parts) >= n+2 {
			nextPart = parts[n+1]
		}
		lower := strings.ToLower(part)
		switch {
		case len([]rune(part)) == 1 && lower != "a" && !isInteger(part) &&
			(previousAcronym || n < len(parts)-1) &&
			(previousAcronym || len([]rune(nextPart)) != 1 || !isInteger(nextPart)):
			b.WriteString(part + ".")
			previousAcronym = true
		case lower == "a" && (previousAcronym || len([]rune(nextPart)) == 1):
			b.WriteString(part + ".")
			previousAcronym = true
		case lower == "dr":
			b.WriteString(part + ".")
			previousAcronym = true
		default:
			if previousAcronym {
				b.WriteString(" ")
				previousAcronym = false
			}
			b.WriteString(part + " ")
		}
	}
	return strings.Trim(b.String(), " ")
}

func isInteger(s string) bool {
	_, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil
}

// parseMoviePath ports Radarr's Parser.ParseMoviePath.
func parseMoviePath(path string) *movieInfo {
	dir, fileName := splitPath(path)
	dirName := baseName(dir)

	if result := parseMovieTitle(fileName, true); result != nil {
		return result
	}
	if result := parseMovieTitle(dirName+" "+fileName, false); result != nil {
		return result
	}
	return parseMovieTitle(dirName+pathExtension(fileName), false)
}
