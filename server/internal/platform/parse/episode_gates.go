// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import "strings"

// episodeRegex is one of Sonarr's title regexes with the gate that lets the parser skip it.
type episodeRegex struct {
	gate  episodeGate
	regex pattern
}

// episodeGate is false when an episode-title regex cannot match the title; see gates.go.
// Each gate below names the text its regex cannot match without.
type episodeGate func(t *titleShape) bool

func always(*titleShape) bool {
	return true
}

func bracketStart(t *titleShape) bool {
	return !t.ascii || strings.HasPrefix(t.lower, "[")
}

func containsGate(subs ...string) episodeGate {
	return func(t *titleShape) bool { return t.contains(subs...) }
}

func bothGates(a, b episodeGate) episodeGate {
	return func(t *titleShape) bool { return a(t) && b(t) }
}

var (
	needsParen         = containsGate("(")
	needsBracket       = containsGate("[")
	needsParenOrSquare = containsGate("[", "(")
	needsSpecialSuffix = containsGate("[sp]")
	needsSlash         = containsGate("/")
	needsEpisodeWord   = containsGate("episode")
	needsEpisodioWord  = containsGate("episod")
	needsPartWord      = containsGate("part")
	needsSeasonBracket = containsGate("[s")
	needsSeasonParen   = containsGate("(s")
	needsSpanishCap    = containsGate("[cap")
	needsTurkishWord   = containsGate("blm", "bolum")
	needsDutchWord     = containsGate("afl.")
	needsTemporadaWord = containsGate("temporada")
	needsEpisodeAndSq  = bothGates(needsEpisodeWord, needsBracket)
	needsParenAndSq    = bothGates(needsParen, needsBracket)
	needsSeasonEpisode = bothGates(containsGate("season"), needsEpisodeWord)
)

func needsHashAfterDash(t *titleShape) bool {
	return t.contains(" - ") && needsParenOrSquare(t)
}

func needsNonASCII(t *titleShape) bool {
	return !t.ascii
}

func needsSplitEpisode(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		return t.inSet(r.end, "abcd") && t.inSet(r.end+1, " _.")
	})
}

func needsTwoSeasonMarkers(t *titleShape) bool {
	const repetitions = 2
	return t.countRuns(t.afterS) >= repetitions
}

func needsTwoCrossMarkers(t *titleShape) bool {
	const repetitions = 2
	return t.countRuns(func(r digitRun) bool { return t.at(r.end) == 'x' }) >= repetitions
}

func needsDate(t *titleShape) bool {
	return t.datePrefix(isNonWordByte)
}

func needsDateAndBracket(t *titleShape) bool {
	return needsDate(t) && needsBracket(t)
}

func needsSeparatedDate(t *titleShape) bool {
	return t.datePrefix(isSeparatorByte)
}

func needsSeparatedDateAndPart(t *titleShape) bool {
	return needsSeparatedDate(t) && needsPartWord(t)
}

func needsSeasonEpisodeDash(t *titleShape) bool {
	return t.seasonEpisodeDash()
}

func needsSeasonEpisodeMarker(t *titleShape) bool {
	return t.runThenMarker(1, 2, "ex")
}

func needsFourDigitSeason(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool { return t.afterS(r) && r.len() == 4 })
}

func needsFourDigitCross(t *titleShape) bool {
	return t.runThenMarker(4, 4, "x")
}

func needsDottedEpisode(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		return t.afterS(r) && r.len() == 2 && t.at(r.end) == '.' && isDigitByte(t.at(r.end+1))
	})
}

// seasonWords are the words Sonarr accepts in place of "S" before a season number.
func seasonWords() []string {
	return []string{"season", "saison", "series", "stagione"}
}

// afterSeasonMarker is true when run r follows "s" or a season word plus one of [_. ].
func (t *titleShape) afterSeasonMarker(r digitRun) bool {
	if t.afterS(r) {
		return true
	}
	if !t.inSet(r.start-1, "_. ") {
		return false
	}
	for _, w := range seasonWords() {
		if strings.HasSuffix(t.lower[:r.start-1], w) {
			return true
		}
	}
	return false
}

// needsMultiSeason: two season numbers of one or two digits, the second after one or three
// separators and an optional season marker.
func needsMultiSeason(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		if !lenBetween(r, 1, 2) || !t.afterSeasonMarker(r) {
			return false
		}
		for _, seps := range []int{1, 3} {
			if t.skipBytes(r.end, isSeparatorByte)-r.end < seps {
				continue
			}
			for _, start := range t.afterOptionalSeasonMarker(r.end + seps) {
				if next, ok := t.runAt(start); ok && lenBetween(next, 1, 2) {
					return true
				}
			}
		}
		return false
	})
}

// afterOptionalSeasonMarker lists where a season number can start at or after i: right at i,
// after an "s", or after a season word and one of [_. ].
func (t *titleShape) afterOptionalSeasonMarker(i int) []int {
	starts := []int{i}
	if t.at(i) == 's' {
		starts = append(starts, i+1)
	}
	for _, w := range seasonWords() {
		if strings.HasPrefix(t.lower[i:], w) && t.inSet(i+len(w), "_. ") {
			starts = append(starts, i+len(w)+1)
		}
	}
	return starts
}

// needsPartialSeason: "s<1-2 digits>", non-word bytes, then part/vol/e/p and a 1-2 digit number.
func needsPartialSeason(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		if !t.afterS(r) || !lenBetween(r, 1, 2) {
			return false
		}
		i := t.skipBytes(r.end, isNonWordByte)
		if i == r.end {
			return false
		}
		var starts []int
		for _, word := range []string{"part", "vol"} {
			if strings.HasPrefix(t.lower[i:], word) {
				starts = append(starts, i+len(word))
				if isNonWordByte(t.at(i + len(word))) {
					starts = append(starts, i+len(word)+1)
				}
			}
		}
		if t.inSet(i, "ep") {
			starts = append(starts, i+1)
		}
		for _, start := range starts {
			if part, ok := t.runAt(start); ok && lenBetween(part, 1, 2) {
				return true
			}
		}
		return false
	})
}

// needsSeasonThenEpisode: "s<1, 2 or 4 digits>", then a separator and/or e/x, then a 1-2 digit run.
func needsSeasonThenEpisode(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		if !t.afterS(r) || !lenBetween(r, 1, 2) && r.len() != 4 {
			return false
		}
		i := r.end
		if isSeparatorByte(t.at(i)) {
			i++
		}
		if t.inSet(i, "ex") {
			i++
		}
		next, ok := t.runAt(i)
		return i > r.end && ok && lenBetween(next, 1, 2)
	})
}

func needsTwoEpisodeMarkers(t *titleShape) bool {
	const repetitions = 2
	return t.countRuns(func(r digitRun) bool { return lenBetween(r, 2, 3) && t.at(r.start-1) == 'e' }) >= repetitions
}

func needsFiveDigits(t *titleShape) bool {
	return t.hasRun(5)
}

func needsFiveDigitsAndEp(t *titleShape) bool {
	return t.contains("ep") && needsFiveDigits(t)
}

func needsFourDigits(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool { return r.len() >= 4 })
}

func needsThreeDigitSeason(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool { return t.afterS(r) && r.len() == 3 && t.at(r.end) == 'e' })
}

func needsOfCount(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		return t.at(r.end) == 'o' && t.at(r.end+1) == 'f' && isDigitByte(t.at(r.end+2))
	})
}

func needsCompactSeasonEpisode(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		if !t.afterS(r) || r.len() != 2 || t.at(r.end) != 'e' {
			return false
		}
		episode, ok := t.runAt(r.end + 1)
		return ok && episode.len() == 2
	})
}

func needsSeasonThenE(t *titleShape) bool {
	return t.seasonThenE(1, 2)
}

func needsThreeDigitSeasonThenE(t *titleShape) bool {
	return t.seasonThenE(3, 3)
}

func needsSpacedDash(t *titleShape) bool {
	if !t.ascii {
		return true
	}
	for i := 1; i+2 < len(t.lower); i++ {
		if t.lower[i] == '-' && isNonWordByte(t.lower[i-1]) && isNonWordByte(t.lower[i+1]) && t.lower[i+2] == 'e' {
			return true
		}
	}
	return false
}

func needsSeparatorRun(t *titleShape) bool {
	const minSeparators = 3
	if !t.ascii {
		return true
	}
	run := 0
	for i := range len(t.lower) {
		if !isSeparatorByte(t.lower[i]) {
			run = 0
			continue
		}
		run++
		if run >= minSeparators {
			return true
		}
	}
	return false
}

func needsSeasonYear(t *titleShape) bool {
	return t.seasonYearLookahead()
}

func needsThreeDigitCode(t *titleShape) bool {
	return t.separatedRun(3, 3, func(b byte) bool { return b == '_' || b == '.' || b == '-' }, func(b byte) bool {
		return !isLetterByte(b) && !isDigitByte(b)
	})
}

func needsFourDigitEpisode(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool { return r.len() == 4 && notIorP(t.at(r.end)) })
}

func needsAmbiguousDate(t *titleShape) bool {
	return t.ambiguousDate()
}

func needsEightDigits(t *titleShape) bool {
	return t.hasRun(8)
}

func needsLongEpisodeMarker(t *titleShape) bool {
	return t.eThenRun(3)
}

func needsShortMonth(t *titleShape) bool {
	return t.shortMonth() && t.contains("th", "st", "rd")
}

func needsFourDigitCode(t *titleShape) bool {
	return t.anyRun(func(r digitRun) bool {
		next := t.at(r.end)
		return r.len() == 4 && next != 'p' && next != 'i' && next != ')' && next != ']'
	})
}

func needsShortEpisodeMarker(t *titleShape) bool {
	return t.digitMarkerDigit("-ex")
}

func needsPartOrEpisodeMarker(t *titleShape) bool {
	if needsPartWord(t) || !t.ascii {
		return true
	}
	return t.anyRun(func(r digitRun) bool { return t.at(r.start-1) == 'e' && isNonWordByte(t.at(r.start-2)) })
}

func needsEpisodeRunMarker(t *titleShape) bool {
	return t.eThenRun(2)
}

func needsDigitRange(t *titleShape) bool {
	return t.digitMarkerDigit("-")
}

func needsSeparatedAbsolute(t *titleShape) bool {
	return t.separatedRun(2, 4, isSeparatorByte, notIorP)
}

func needsBoundedAbsolute(t *titleShape) bool {
	return t.separatedRun(2, 3, func(b byte) bool { return isNonWordByte(b) || b == '_' }, notIorP)
}

func needsExtantNumber(t *titleShape) bool {
	return t.separatedRun(4, 6, isSeparatorByte, isSeparatorByte)
}

func needsThreeDigitSeasonMarker(t *titleShape) bool {
	return t.runThenMarker(3, 3, "ex")
}
