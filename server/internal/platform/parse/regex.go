// Copyright (C) Magpie contributors.
// SPDX-License-Identifier: GPL-3.0-or-later

package parse

import (
	"strings"

	"github.com/dlclark/regexp2/v2"
)

// There is no match timeout: it would make results depend on machine load. regexp2's
// backtracking stack limit stops a pathological release name instead, as no match.
const (
	ignoreCase       = regexp2.IgnoreCase
	ignoreWhitespace = regexp2.IgnorePatternWhitespace
	caseSensitive    = regexp2.None
	replaceAll       = -1
	replaceFromStart = -1
	// .NET reports index and length 0 for a group that did not capture.
	unmatchedGroupSpan = 0
)

// pattern is a .NET-compatible regex with the helpers the ported code needs.
// The zero pattern never matches, for rules one parser has and the other lacks.
// A pattern with a gate skips the regex when the gate is closed; see gates.go.
type pattern struct {
	re       *regexp2.Regexp
	gate     gate
	anchored *regexp2.Regexp
	starts   startGate
	hint     hint
}

// hint returns the first index of a lowercased ASCII string where a match can start, or -1.
type hint func(lower string) int

// hinted makes isMatch start searching at the hint instead of the start of the text.
func (p pattern) hinted(h hint) pattern {
	p.hint = h
	return p
}

func mustPattern(expr string, opts regexp2.RegexOptions) pattern {
	return pattern{re: regexp2.MustCompile(expr, opts)}
}

func (p pattern) gated(g gate) pattern {
	p.gate = g
	return p
}

func (p pattern) skips(s string) bool {
	return p.re == nil || p.gate != nil && !p.gate(s)
}

func (p pattern) isMatch(s string) bool {
	if p.skips(s) {
		return false
	}
	if p.anchored != nil {
		return p.match(s) != nil
	}
	if p.hint != nil && isASCII(s) {
		start := p.hint(strings.ToLower(s))
		if start < 0 {
			return false
		}
		m, err := p.re.FindStringMatchStartingAt(s, start)
		return err == nil && m != nil
	}
	ok, err := p.re.MatchString(s)
	return err == nil && ok
}

func (p pattern) match(s string) *match {
	if p.skips(s) {
		return nil
	}
	if p.anchored != nil && isASCII(s) {
		if first := p.boundaryScan(s, true); len(first) > 0 {
			return first[0]
		}
		return nil
	}
	m, err := p.re.FindStringMatch(s)
	if err != nil || m == nil {
		return nil
	}
	return &match{m: m}
}

func (p pattern) matches(s string) []*match {
	if p.skips(s) {
		return nil
	}
	if p.anchored != nil && isASCII(s) {
		return p.boundaryMatches(s)
	}
	var out []*match
	m, err := p.re.FindStringMatch(s)
	for err == nil && m != nil {
		out = append(out, &match{m: m})
		m, err = p.re.FindNextMatch(m)
	}
	return out
}

// startGate is true at every index of a lowercased ASCII string where a match could start.
type startGate func(lower string, i int) bool

// mustScanPattern keeps a \G-anchored copy of the regex and tries it only where starts allows.
// A big alternation has no literal prefix for regexp2 to search for, so it would otherwise
// run every branch at every index. The scan keeps .NET's successive-match order: try each
// allowed index left to right and resume after each match.
func mustScanPattern(expr string, opts regexp2.RegexOptions, starts startGate) pattern {
	p := mustPattern(expr, opts)
	p.anchored = mustPattern(`\G`+expr, opts).re
	p.starts = starts
	return p
}

func (p pattern) boundaryMatches(s string) []*match {
	return p.boundaryScan(s, false)
}

func (p pattern) boundaryScan(s string, firstOnly bool) []*match {
	var out []*match
	var runes []rune
	lower := strings.ToLower(s)
	for i := 0; i <= len(s) && (!firstOnly || len(out) == 0); i++ {
		if !p.starts(lower, i) {
			continue
		}
		if runes == nil {
			runes = []rune(s)
		}
		m, err := p.anchored.FindRunesMatchStartingAt(runes, i)
		if err != nil {
			return out
		}
		if m == nil {
			continue
		}
		out = append(out, &match{m: m})
		if m.RuneLength > 0 {
			i = m.RuneIndex + m.RuneLength - 1
		}
	}
	return out
}

// isWordBoundary is \b at byte i of an ASCII string.
func isWordBoundary(s string, i int) bool {
	before := i > 0 && !isNonWordByte(s[i-1])
	after := i < len(s) && !isNonWordByte(s[i])
	return before != after
}

func (p pattern) lastMatch(s string) *match {
	all := p.matches(s)
	if len(all) == 0 {
		return nil
	}
	return all[len(all)-1]
}

func (p pattern) replace(s, replacement string) string {
	if p.skips(s) {
		return s
	}
	out, err := p.re.Replace(s, replacement, replaceFromStart, replaceAll)
	if err != nil {
		return s
	}
	return out
}

func (p pattern) replaceFunc(s string, eval func(value string) string) string {
	if p.skips(s) {
		return s
	}
	out, err := p.re.ReplaceFunc(s, func(m regexp2.Match) string { return eval(m.String()) }, replaceFromStart, replaceAll)
	if err != nil {
		return s
	}
	return out
}

// tryReplace mirrors Sonarr's RegexReplace.TryReplace: replace, and report whether it matched.
func (p pattern) tryReplace(s *string, replacement string) bool {
	matched := p.isMatch(*s)
	*s = p.replace(*s, replacement)
	return matched
}

func (p pattern) split(s string) []string {
	var parts []string
	runes := []rune(s)
	start := 0
	for _, m := range p.matches(s) {
		parts = append(parts, string(runes[start:m.m.RuneIndex]))
		start = m.m.RuneIndex + m.m.RuneLength
	}
	return append(parts, string(runes[start:]))
}

type match struct {
	m *regexp2.Match
}

func (m *match) group(name string) *regexp2.Group {
	return m.m.GroupByName(name)
}

func (m *match) success(name string) bool {
	g := m.group(name)
	return g != nil && len(g.Captures) > 0
}

func (m *match) value(name string) string {
	if !m.success(name) {
		return ""
	}
	return m.group(name).String()
}

func (m *match) captures(name string) []regexp2.Capture {
	g := m.group(name)
	if g == nil {
		return nil
	}
	return g.Captures
}

func (m *match) index(name string) int {
	if !m.success(name) {
		return unmatchedGroupSpan
	}
	return m.group(name).RuneIndex
}

func (m *match) length(name string) int {
	if !m.success(name) {
		return unmatchedGroupSpan
	}
	return m.group(name).RuneLength
}

// endIndex is .NET's Group.EndIndex(): zero when the group did not capture.
func (m *match) endIndex(name string) int {
	return m.index(name) + m.length(name)
}

func (m *match) String() string {
	return m.m.String()
}

func captureEnd(c regexp2.Capture) int {
	return c.RuneIndex + c.RuneLength
}
