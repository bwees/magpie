package subsync

import (
	"bytes"
	"testing"
	"time"
)

var trickySRT = []byte("\xef\xbb\xbf1\r\n" +
	"00:00:01,500 --> 00:00:03,250 X1:100 X2:200 Y1:10 Y2:20\r\n" +
	"<i>Caf\xe9 au lait</i>\r\n" +
	"{\\an8}second line\r\n" +
	"\r\n" +
	"2\r\n" +
	" 0:00:04.5 --> 0:00:06.75\r\n" +
	"- Dot separators.\r\n" +
	"- And short fractions.\r\n" +
	"\r\n" +
	"3\r\n" +
	"01:02:03,004 --> 01:02:05,000\r\n" +
	"No trailing newline")

func TestParseCues(t *testing.T) {
	want := []cue{
		{start: 1500 * time.Millisecond, end: 3250 * time.Millisecond, text: "<i>Caf\xe9 au lait</i>\n{\\an8}second line"},
		{start: 4500 * time.Millisecond, end: 6750 * time.Millisecond, text: "- Dot separators.\n- And short fractions."},
		{start: time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond, end: time.Hour + 2*time.Minute + 5*time.Second, text: "No trailing newline"},
	}
	got := parseCues(trickySRT)
	if len(got) != len(want) {
		t.Fatalf("got %d cues, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cue %d = %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

func TestShiftSRTRoundTripsAtZero(t *testing.T) {
	if got := shiftSRT(trickySRT, timing{ratio: unitRatio}); !bytes.Equal(got, trickySRT) {
		t.Errorf("zero shift changed the subtitle:\n%q", got)
	}
}

func TestShiftSRTRewritesOnlyTimestamps(t *testing.T) {
	want := []byte("\xef\xbb\xbf1\r\n" +
		"00:00:00,000 --> 00:00:01,000 X1:100 X2:200 Y1:10 Y2:20\r\n" +
		"<i>Caf\xe9 au lait</i>\r\n" +
		"{\\an8}second line\r\n" +
		"\r\n" +
		"2\r\n" +
		" 00:00:02.250 --> 00:00:04.500\r\n" +
		"- Dot separators.\r\n" +
		"- And short fractions.\r\n" +
		"\r\n" +
		"3\r\n" +
		"01:02:00,754 --> 01:02:02,750\r\n" +
		"No trailing newline")
	got := shiftSRT(trickySRT, timing{ratio: unitRatio, offset: -2250 * time.Millisecond})
	if !bytes.Equal(got, want) {
		t.Errorf("shifted subtitle:\n%q\nwant:\n%q", got, want)
	}
}

func TestTimingScalesThenShifts(t *testing.T) {
	got := timing{ratio: 2, offset: -time.Second}.apply(3 * time.Second)
	if got != 5*time.Second {
		t.Errorf("apply = %v, want 5s", got)
	}
}
