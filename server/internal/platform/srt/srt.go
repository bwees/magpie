// Package srt reads and writes SubRip subtitles. The agent shifts Whisper's cues with it, and the
// transcode server checks the uploaded file before Subtitles stores it.
package srt

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrMalformed is a block without a timing line, or a timing line that doesn't parse.
var ErrMalformed = errors.New("srt: malformed subtitle")

// Cue is one subtitle: the text shown from Start to End.
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

const (
	byteOrderMark = string(rune(0xFEFF))
	blankLine     = "\n\n"
	newline       = "\n"
	crlf          = "\r\n"
	arrow         = " --> "
	timestampForm = "%02d:%02d:%02d,%03d"
)

var (
	timing    = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})[,.](\d{3})\s*-->\s*(\d+):(\d{2}):(\d{2})[,.](\d{3})`)
	cueNumber = regexp.MustCompile(`^\d+$`)
)

// Parse reads every cue. Empty input has no cues and no error.
func Parse(data []byte) ([]Cue, error) {
	text := strings.ReplaceAll(strings.TrimPrefix(string(data), byteOrderMark), crlf, newline)
	var cues []Cue
	for block := range strings.SplitSeq(strings.TrimSpace(text), blankLine) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		cue, err := parseBlock(strings.Split(block, newline))
		if err != nil {
			return nil, err
		}
		cues = append(cues, cue)
	}
	return cues, nil
}

func parseBlock(lines []string) (Cue, error) {
	if cueNumber.MatchString(strings.TrimSpace(lines[0])) {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return Cue{}, fmt.Errorf("%w: a cue number without timing", ErrMalformed)
	}
	match := timing.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if match == nil {
		return Cue{}, fmt.Errorf("%w: %q is not a timing line", ErrMalformed, lines[0])
	}
	text := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		text = append(text, strings.TrimSpace(line))
	}
	return Cue{
		Start: timestamp(match[1:5]),
		End:   timestamp(match[5:9]),
		Text:  strings.Join(text, newline),
	}, nil
}

// timestamp reads hours, minutes, seconds and milliseconds the timing pattern already checked are digits.
func timestamp(parts []string) time.Duration {
	units := []time.Duration{time.Hour, time.Minute, time.Second, time.Millisecond}
	var total time.Duration
	for i, part := range parts {
		n, _ := strconv.Atoi(part)
		total += time.Duration(n) * units[i]
	}
	return total
}

// Format writes cues numbered from 1.
func Format(cues []Cue) []byte {
	var out strings.Builder
	for i, cue := range cues {
		fmt.Fprintf(&out, "%d%s%s%s%s%s%s%s", i+1, newline, formatTimestamp(cue.Start), arrow, formatTimestamp(cue.End), newline, cue.Text, blankLine)
	}
	return []byte(out.String())
}

func formatTimestamp(d time.Duration) string {
	d = max(d, 0).Round(time.Millisecond)
	return fmt.Sprintf(timestampForm, d/time.Hour, d%time.Hour/time.Minute, d%time.Minute/time.Second, d%time.Second/time.Millisecond)
}

// Shift moves every cue by d.
func Shift(cues []Cue, d time.Duration) []Cue {
	shifted := make([]Cue, 0, len(cues))
	for _, cue := range cues {
		shifted = append(shifted, Cue{Start: cue.Start + d, End: cue.End + d, Text: cue.Text})
	}
	return shifted
}
