package srt

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestParseReadsCuesWithWindowsLineEndingsAndABOM(t *testing.T) {
	data := []byte(byteOrderMark + "1\r\n00:00:01,000 --> 00:00:02,500\r\n Hello\r\nthere \r\n\r\n2\r\n01:02:03.004 --> 01:02:04.000\r\nBye\r\n")
	cues, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []Cue{
		{Start: time.Second, End: 2500 * time.Millisecond, Text: "Hello\nthere"},
		{Start: time.Hour + 2*time.Minute + 3*time.Second + 4*time.Millisecond, End: time.Hour + 2*time.Minute + 4*time.Second, Text: "Bye"},
	}
	if !slices.Equal(cues, want) {
		t.Fatalf("cues = %+v, want %+v", cues, want)
	}
}

func TestParseOfNothingHasNoCues(t *testing.T) {
	cues, err := Parse([]byte("\n\n"))
	if err != nil || len(cues) != 0 {
		t.Fatalf("Parse = %+v, %v; want no cues", cues, err)
	}
}

func TestParseRejectsABlockWithoutTiming(t *testing.T) {
	if _, err := Parse([]byte("1\nnot a timing line\nHello\n")); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}

func TestShiftAndFormatRenumberTheCues(t *testing.T) {
	cues := []Cue{{Start: time.Second, End: 2 * time.Second, Text: "Hello"}, {Start: 3 * time.Second, End: 4 * time.Second, Text: "Bye"}}
	got := string(Format(Shift(cues, 4*time.Second)))
	want := "1\n00:00:05,000 --> 00:00:06,000\nHello\n\n2\n00:00:07,000 --> 00:00:08,000\nBye\n\n"
	if got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
}
