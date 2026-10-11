package subsync

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

const (
	millisecondDigits  = 3
	srtTimestampFormat = "%02d:%02d:%02d%c%03d"
)

var (
	utf8BOM = []byte("\xef\xbb\xbf")

	timingLine = regexp.MustCompile(
		`^\s*(\d+):(\d{1,2}):(\d{1,2})([,.])(\d{1,3})\b\s*-->\s*(\d+):(\d{1,2}):(\d{1,2})([,.])(\d{1,3})\b`,
	)
)

// Submatch groups of timingLine, per timestamp.
const (
	groupHours = iota
	groupMinutes
	groupSeconds
	groupSeparator
	groupFraction
	groupsPerTimestamp
)

type cue struct {
	start time.Duration
	end   time.Duration
	text  string
}

type timestampSpan struct {
	from, to  int
	value     time.Duration
	separator byte
}

// timing maps an original subtitle time to its synced time: scaled by ratio, then shifted by offset.
type timing struct {
	ratio  float64
	offset time.Duration
}

func (t timing) identity() bool {
	return t.ratio == 1 && t.offset == 0
}

func (t timing) apply(d time.Duration) time.Duration {
	shifted := time.Duration(math.Round(float64(d)*t.ratio)) + t.offset
	return max(shifted, 0)
}

// parseCues reads the timed cues of an SRT file. Unparseable blocks are skipped.
func parseCues(srt []byte) []cue {
	var cues []cue
	var text [][]byte
	inCue := false
	flush := func() {
		if inCue {
			cues[len(cues)-1].text = string(bytes.Join(text, []byte("\n")))
		}
		inCue, text = false, nil
	}

	for _, line := range splitLines(srt) {
		content := trimLineEnding(line)
		if spans, ok := parseTimingLine(content); ok {
			flush()
			cues = append(cues, cue{start: spans[0].value, end: spans[1].value})
			inCue = true
			continue
		}
		if len(bytes.TrimSpace(content)) == 0 {
			flush()
			continue
		}
		if inCue {
			text = append(text, content)
		}
	}
	flush()
	return cues
}

// shiftSRT rewrites only the timestamps of each timing line; every other byte is copied unchanged.
func shiftSRT(srt []byte, t timing) []byte {
	if t.identity() {
		return bytes.Clone(srt)
	}
	out := make([]byte, 0, len(srt))
	for _, line := range splitLines(srt) {
		spans, ok := parseTimingLine(trimLineEnding(line))
		if !ok {
			out = append(out, line...)
			continue
		}
		cursor := 0
		for _, span := range spans {
			out = append(out, line[cursor:span.from]...)
			out = append(out, formatTimestamp(t.apply(span.value), span.separator)...)
			cursor = span.to
		}
		out = append(out, line[cursor:]...)
	}
	return out
}

func splitLines(data []byte) [][]byte {
	return bytes.SplitAfter(data, []byte("\n"))
}

func trimLineEnding(line []byte) []byte {
	return bytes.TrimRight(line, "\r\n")
}

func parseTimingLine(line []byte) ([2]timestampSpan, bool) {
	var spans [2]timestampSpan
	base := 0
	if bytes.HasPrefix(line, utf8BOM) {
		base = len(utf8BOM)
	}
	match := timingLine.FindSubmatchIndex(line[base:])
	if match == nil {
		return spans, false
	}
	for i := range spans {
		group := func(g int) []byte {
			idx := 2 * (1 + i*groupsPerTimestamp + g)
			return line[base+match[idx] : base+match[idx+1]]
		}
		value, err := timestampValue(group(groupHours), group(groupMinutes), group(groupSeconds), group(groupFraction))
		if err != nil {
			return spans, false
		}
		first := 2 * (1 + i*groupsPerTimestamp + groupHours)
		last := 2 * (1 + i*groupsPerTimestamp + groupFraction)
		spans[i] = timestampSpan{
			from:      base + match[first],
			to:        base + match[last+1],
			value:     value,
			separator: group(groupSeparator)[0],
		}
	}
	return spans, true
}

func timestampValue(hours, minutes, seconds, fraction []byte) (time.Duration, error) {
	fields := []struct {
		digits []byte
		unit   time.Duration
	}{
		{hours, time.Hour},
		{minutes, time.Minute},
		{seconds, time.Second},
		{padFraction(fraction), time.Millisecond},
	}
	var total time.Duration
	for _, f := range fields {
		n, err := strconv.Atoi(string(f.digits))
		if err != nil {
			return 0, err
		}
		total += time.Duration(n) * f.unit
	}
	return total, nil
}

func padFraction(fraction []byte) []byte {
	padded := bytes.Clone(fraction)
	for len(padded) < millisecondDigits {
		padded = append(padded, '0')
	}
	return padded
}

func formatTimestamp(d time.Duration, separator byte) string {
	d = d.Round(time.Millisecond)
	hours := int64(d / time.Hour)
	minutes := int64(d % time.Hour / time.Minute)
	seconds := int64(d % time.Minute / time.Second)
	millis := int64(d % time.Second / time.Millisecond)
	return fmt.Sprintf(srtTimestampFormat, hours, minutes, seconds, separator, millis)
}
