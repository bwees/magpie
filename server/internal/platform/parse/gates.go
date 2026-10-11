// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strings"
	"unicode"
)

// A gate is a cheap test that is true whenever its regex could match. Gates only skip work:
// a regex with a false gate cannot match, so results are the same as running every regex.
// TestGatesAreSound checks each gate against the fixture corpus.
//
// Gates read ASCII text only. Any non-ASCII character opens every gate, because .NET case
// folding and Unicode digits (\d) are not modelled here.

type gate func(s string) bool

// foldAny opens when s contains one of subs, ignoring ASCII case. subs must be lowercase.
func foldAny(subs ...string) gate {
	return func(s string) bool {
		if !isASCII(s) {
			return true
		}
		lower := strings.ToLower(s)
		for _, sub := range subs {
			if strings.Contains(lower, sub) {
				return true
			}
		}
		return false
	}
}

func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for j := range len(prefix) {
		if toLowerByte(s[j]) != prefix[j] {
			return false
		}
	}
	return true
}

func toLowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// exactAny opens when s contains one of subs as written.
func exactAny(subs ...string) gate {
	return func(s string) bool {
		if !isASCII(s) {
			return true
		}
		for _, sub := range subs {
			if strings.Contains(s, sub) {
				return true
			}
		}
		return false
	}
}

// wordGate opens when s has one of words as a whole word, or contains one of subs. Words are
// split on anything but letters and digits, so a regex word bounded by \b, "_" or punctuation
// is always a whole word here. With fold, s is lowercased and words and subs must be lowercase.
func wordGate(fold bool, words []string, subs ...string) gate {
	return func(s string) bool {
		if !isASCII(s) {
			return true
		}
		if fold {
			s = strings.ToLower(s)
		}
		for _, sub := range subs {
			if strings.Contains(s, sub) {
				return true
			}
		}
		for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !isLetterByte(byte(r)) && !isDigitByte(byte(r)) }) {
			for _, word := range words {
				if w == word {
					return true
				}
			}
		}
		return false
	}
}

func allOf(gates ...gate) gate {
	return func(s string) bool {
		for _, g := range gates {
			if !g(s) {
				return false
			}
		}
		return true
	}
}

func startsWith(prefix string) gate {
	return func(s string) bool {
		return !isASCII(s) || strings.HasPrefix(s, prefix)
	}
}

// endsWithBracket opens for text whose last character, before an optional final newline, is ']'.
func endsWithBracket(s string) bool {
	return !isASCII(s) || strings.HasSuffix(strings.TrimSuffix(s, "\n"), "]")
}

// wordStarts allows indexes where \b holds and the text starts with a digit, a whitespace
// character or one of prefixes, depending on what the regex's branches can start with.
func wordStarts(digits, spaces bool, prefixes ...string) startGate {
	return func(lower string, i int) bool {
		if i >= len(lower) || !isWordBoundary(lower, i) {
			return false
		}
		c := lower[i]
		if digits && isDigitByte(c) || spaces && isSpaceByte(c) {
			return true
		}
		for _, p := range prefixes {
			if strings.HasPrefix(lower[i:], p) {
				return true
			}
		}
		return false
	}
}

func isSpaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

var (
	seriesSourceStarts = wordStarts(false, true,
		"blu", "hd", "bd", "web", "amazon", "itunes", "maxdome", "netflix", "hbomax", "disney", ".web",
		" web", "-web", "720p", "1080p", "2160p", "amzn", "atvp", "dp", "nf", "br", "dvd", "ntsc", "pal",
		"xvidvd", "ws", "dsr", "pdtv", "sdtv", "tvrip")
	movieSourceWords = wordStarts(false, true,
		"mblu", "blu", "hd", "bd", "uhd", "br", "web", "amazon", "itunes", "maxdome", "netflix", "hbomax",
		"disney", ".web", " web", "-web", "amzn", "nf", "dp", "xdvd", "xmdvd", "xvidvd", "mdvd", "dvd", "ws",
		"dsr", "regional", "scr", "ts", "tele", "pdvd", "tc", "cam", "newcam", "hq", "workprint", "wp",
		"pdtv", "sdtv", "tvrip")
	resolutionStarts = wordStarts(true, false, "fhd", "uhd", "hevc", "bd", "h")
)

// The longest lead before "web" in Radarr's "<digits>0p-Hybrid-WEB" branch.
const movieSourceDigitLead = len("21600p-hybrid-")

// movieSourceStarts adds Radarr's branches that start with a digit or R<digit>: a digit must
// lead to "web" (<digits>0p WEB) or "dvd" (DVD-R with a disc count).
func movieSourceStarts(lower string, i int) bool {
	if movieSourceWords(lower, i) {
		return true
	}
	if i >= len(lower) || !isWordBoundary(lower, i) {
		return false
	}
	switch c := lower[i]; {
	case c == 'r':
		return i+1 < len(lower) && isDigitByte(lower[i+1])
	case isDigitByte(c):
		window := lower[i:min(len(lower), i+movieSourceDigitLead+len("web"))]
		return strings.Contains(window, "web") || strings.Contains(window, "dvd")
	}
	return false
}

// brDiskGate: every BR-DISK branch names a disc source and a codec, container or disc marker,
// and the regex's negative lookahead rejects any title with one of the encode words below.
var brDiskGate = allOf(
	foldAny("blu", "bd", "hd", "br"),
	foldAny("vc", "mpeg", "bdmv", "iso", "complete", "dis", "3d", "full", "25", "50", "66", "100"),
	noneOfWords("bdrip", "720p", "mkv", "xvid", "wmv", "d3g", "remux", "bdremux", "x264", "x265", "h264", "h265"),
)

// noneOfWords closes when s has one of words as a whole \b-delimited word, ignoring case.
// Unlike the other gates it reads \b exactly: word characters include the underscore.
func noneOfWords(words ...string) gate {
	return func(s string) bool {
		if !isASCII(s) {
			return true
		}
		for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return isNonWordByte(byte(r)) }) {
			for _, word := range words {
				if w == word {
					return false
				}
			}
		}
		return true
	}
}

// seasonEpisodeGate: "s<digits>e<digit>".
func seasonEpisodeGate(s string) bool {
	if !isASCII(s) {
		return true
	}
	t := newTitleShape(s)
	return t.anyRun(func(r digitRun) bool {
		return t.afterS(r) && t.at(r.end) == 'e' && isDigitByte(t.at(r.end+1))
	})
}

// cleanSuffixGate: the release-group clean-up suffix regexes end in "-<word>".
func cleanSuffixGate(s string) bool {
	if !isASCII(s) {
		return true
	}
	lower := strings.TrimSuffix(strings.ToLower(s), "\n")
	if strings.Contains(lower, "-rakuv") {
		return true
	}
	for _, w := range []string{
		"rp", "1", "nzbgeek", "[n-z-b]", "obfuscated", "obfuscation", "scrambled", "sample", "pre",
		"postbot", "xpost", "whiterev", "buymore", "asrequested", "alternativetorequested", "gerov",
		"z0ids3n", "chamele0n", "4p", "4planet", "altezachen", "repackpost",
	} {
		if strings.HasSuffix(lower, "-"+w) {
			return true
		}
	}
	return false
}

// leadingAlnum opens when s starts with at least n ASCII letters or digits.
func leadingAlnum(n int) gate {
	return func(s string) bool {
		if !isASCII(s) {
			return true
		}
		count := 0
		for count < len(s) && (isLetterByte(s[count]) || isDigitByte(s[count])) {
			count++
		}
		return count >= n
	}
}

// foldPrefix opens when s starts with prefix, ignoring ASCII case.
func foldPrefix(prefix string) gate {
	return func(s string) bool {
		return !isASCII(s) || hasPrefixFold(s, prefix)
	}
}

// leadingDigits opens when s starts with at least n digits.
func leadingDigits(n int) gate {
	return func(s string) bool {
		if !isASCII(s) {
			return true
		}
		count := 0
		for count < len(s) && isDigitByte(s[count]) {
			count++
		}
		return count >= n
	}
}

// digitRunAtLeast opens when s has n digits in a row.
func digitRunAtLeast(n int) gate {
	return func(s string) bool {
		return !isASCII(s) || newTitleShape(s).anyRun(func(r digitRun) bool { return r.len() >= n })
	}
}

// digitRunOf opens when s has a run of exactly n digits.
func digitRunOf(n int) gate {
	return func(s string) bool {
		return !isASCII(s) || newTitleShape(s).hasRun(n)
	}
}

// literalHint finds where a search can start: every match contains literal (ASCII,
// lowercase) at most maxLead bytes after its start. -1 means no match is possible.
func literalHint(literal string, maxLead int) hint {
	return func(lower string) int {
		i := strings.Index(lower, literal)
		if i < 0 {
			return -1
		}
		return max(0, i-maxLead)
	}
}

// editionGate lists a word from every alternative of Radarr's edition pattern.
var editionGate = foldAny(
	"recut", "extended", "ultimate", "director", "collector", "theatrical", "despecialized",
	"special", "rouge", "final", "assembly", "imperial", "diamond", "signature", "hunter", "rekall",
	"anniversary", "uncensored", "remastered", "unrated", "uncut", "matte", "imax", "fan", "restored", "in1",
)

// websitePrefixGate: the prefix ends in a closing bracket or two spaces/dashes.
var websitePrefixGate = exactAny("]", ")", "  ", " -", "- ", "--")

// seasonTokenGate: "s" followed by two digits.
func seasonTokenGate(s string) bool {
	if !isASCII(s) {
		return true
	}
	lower := strings.ToLower(s)
	for i := 0; i+2 < len(lower); i++ {
		if lower[i] == 's' && isDigitByte(lower[i+1]) && isDigitByte(lower[i+2]) {
			return true
		}
	}
	return false
}

func vDigitOrRepack(s string) bool {
	if !isASCII(s) {
		return true
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "repack") || strings.Contains(lower, "rerip") {
		return true
	}
	for i := 0; i+1 < len(lower); i++ {
		if lower[i] == 'v' && isDigitByte(lower[i+1]) {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func isDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func isLetterByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// isNonWordByte is .NET's \W for ASCII: not a letter, digit or underscore.
func isNonWordByte(b byte) bool {
	return !isLetterByte(b) && !isDigitByte(b) && b != '_'
}

func isSeparatorByte(b byte) bool {
	return b == '-' || b == '_' || b == '.' || b == ' '
}

// digitRun is a maximal run of ASCII digits, [start, end).
type digitRun struct {
	start, end int
}

func (r digitRun) len() int {
	return r.end - r.start
}

// titleShape is what the episode-title gates read: the lowercased title and its digit runs.
type titleShape struct {
	lower string
	ascii bool
	runs  []digitRun
}

func newTitleShape(s string) *titleShape {
	t := &titleShape{lower: strings.ToLower(s), ascii: isASCII(s)}
	for i := 0; i < len(t.lower); {
		if !isDigitByte(t.lower[i]) {
			i++
			continue
		}
		start := i
		for i < len(t.lower) && isDigitByte(t.lower[i]) {
			i++
		}
		t.runs = append(t.runs, digitRun{start, i})
	}
	return t
}

// at returns the byte at i, or 0 outside the title.
func (t *titleShape) at(i int) byte {
	if i < 0 || i >= len(t.lower) {
		return 0
	}
	return t.lower[i]
}

func (t *titleShape) contains(subs ...string) bool {
	if !t.ascii {
		return true
	}
	for _, sub := range subs {
		if strings.Contains(t.lower, sub) {
			return true
		}
	}
	return false
}

// anyRun opens when some digit run satisfies pred.
func (t *titleShape) anyRun(pred func(r digitRun) bool) bool {
	if !t.ascii {
		return true
	}
	for _, r := range t.runs {
		if pred(r) {
			return true
		}
	}
	return false
}

// countRuns counts digit runs satisfying pred; non-ASCII titles count as unlimited.
func (t *titleShape) countRuns(pred func(r digitRun) bool) int {
	if !t.ascii {
		return len(t.lower)
	}
	n := 0
	for _, r := range t.runs {
		if pred(r) {
			n++
		}
	}
	return n
}

func (t *titleShape) runAt(start int) (digitRun, bool) {
	for _, r := range t.runs {
		if r.start == start {
			return r, true
		}
	}
	return digitRun{}, false
}

func (t *titleShape) inSet(i int, set string) bool {
	b := t.at(i)
	return b != 0 && strings.IndexByte(set, b) >= 0
}

func (t *titleShape) afterS(r digitRun) bool {
	return t.at(r.start-1) == 's'
}

func lenBetween(r digitRun, lo, hi int) bool {
	return r.len() >= lo && r.len() <= hi
}

// skipBytes returns the first index at or after i whose byte fails pred.
func (t *titleShape) skipBytes(i int, pred func(byte) bool) int {
	for i < len(t.lower) && pred(t.lower[i]) {
		i++
	}
	return i
}

// datePrefix opens for four digits ending a run, then sep+, then a digit (airyear + month).
func (t *titleShape) datePrefix(sep func(byte) bool) bool {
	return t.anyRun(func(r digitRun) bool {
		if r.len() < 4 {
			return false
		}
		next := t.skipBytes(r.end, sep)
		return next > r.end && isDigitByte(t.at(next))
	})
}

// seasonEpisodeDash opens for "s<digits>e<digits>-<digit>".
func (t *titleShape) seasonEpisodeDash() bool {
	return t.anyRun(func(r digitRun) bool {
		if !t.afterS(r) || t.at(r.end) != 'e' {
			return false
		}
		episode, ok := t.runAt(r.end + 1)
		return ok && t.at(episode.end) == '-' && isDigitByte(t.at(episode.end+1))
	})
}

// runThenMarker opens for a run whose length is in [lo, hi] followed by a marker byte,
// optionally after one non-word byte.
func (t *titleShape) runThenMarker(lo, hi int, markers string) bool {
	return t.anyRun(func(r digitRun) bool {
		if !lenBetween(r, lo, hi) {
			return false
		}
		return t.inSet(r.end, markers) || isNonWordByte(t.at(r.end)) && t.at(r.end) != 0 && t.inSet(r.end+1, markers)
	})
}

// seasonThenE opens for "s<run>" with a run length in [lo, hi], then an optional non-word
// or underscore byte, then 'e'.
func (t *titleShape) seasonThenE(lo, hi int) bool {
	return t.anyRun(func(r digitRun) bool {
		if !t.afterS(r) || !lenBetween(r, lo, hi) {
			return false
		}
		next := t.at(r.end)
		return next == 'e' || next != 0 && (isNonWordByte(next) || next == '_') && t.at(r.end+1) == 'e'
	})
}

// separatedRun opens for a run of length [lo, hi] right after a byte in before, whose next
// byte passes after.
func (t *titleShape) separatedRun(lo, hi int, before func(byte) bool, after func(byte) bool) bool {
	return t.anyRun(func(r digitRun) bool {
		prev := t.at(r.start - 1)
		return lenBetween(r, lo, hi) && prev != 0 && before(prev) && after(t.at(r.end))
	})
}

// eThenRun opens for 'e' or "ep" right before a run of at least minDigits digits.
func (t *titleShape) eThenRun(minDigits int) bool {
	return t.anyRun(func(r digitRun) bool {
		prev := t.at(r.start - 1)
		return r.len() >= minDigits && (prev == 'e' || prev == 'p' && t.at(r.start-2) == 'e')
	})
}

func notIorP(b byte) bool {
	return b != 'i' && b != 'p'
}

// ambiguousDate opens for <digits>sep+<2 digits>sep+<4 digits>, the dd.mm.yyyy shapes.
func (t *titleShape) ambiguousDate() bool {
	return t.anyRun(func(r digitRun) bool {
		if r.len() < 2 {
			return false
		}
		dayStart := t.skipBytes(r.end, isSeparatorByte)
		day, ok := t.runAt(dayStart)
		if dayStart == r.end || !ok || day.len() != 2 {
			return false
		}
		yearStart := t.skipBytes(day.end, isSeparatorByte)
		year, ok := t.runAt(yearStart)
		return yearStart > day.end && ok && year.len() == 4
	})
}

// seasonYearLookahead opens for <digits><sep><4 digits><sep>, the "S01 2019 " shape.
func (t *titleShape) seasonYearLookahead() bool {
	return t.anyRun(func(r digitRun) bool {
		if !isSeparatorByte(t.at(r.end)) {
			return false
		}
		year, ok := t.runAt(r.end + 1)
		return ok && year.len() == 4 && isSeparatorByte(t.at(year.end))
	})
}

// digitMarkerDigit opens for a run followed by one or two marker bytes, then a digit.
func (t *titleShape) digitMarkerDigit(markers string) bool {
	return t.anyRun(func(r digitRun) bool {
		if !t.inSet(r.end, markers) {
			return false
		}
		next := r.end + 1
		if t.inSet(next, markers) {
			next++
		}
		return isDigitByte(t.at(next))
	})
}

func (t *titleShape) shortMonth() bool {
	return t.contains("jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec")
}

func (t *titleShape) hasRun(n int) bool {
	return t.anyRun(func(r digitRun) bool { return r.len() == n })
}
