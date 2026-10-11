package subsync

import (
	"math"
	"slices"
	"testing"
	"time"
)

func TestIsDialogue(t *testing.T) {
	cases := map[string]struct {
		c    cue
		want bool
	}{
		"plain line":         {cue{start: 0, end: time.Second, text: "Hello there."}, true},
		"italic line":        {cue{start: 0, end: time.Second, text: "<i>Hello.</i>"}, true},
		"sound description":  {cue{start: 0, end: time.Second, text: "[door slams]"}, false},
		"CJK description":    {cue{start: 0, end: time.Second, text: "（笑）"}, false},
		"music notes":        {cue{start: 0, end: time.Second, text: "♪ ♫"}, false},
		"sung line":          {cue{start: 0, end: time.Second, text: "♪ Happy birthday ♪"}, true},
		"empty after markup": {cue{start: 0, end: time.Second, text: "<i></i>{\\an8}"}, false},
		"long sign":          {cue{start: 0, end: 11 * time.Second, text: "Downtown, 1985"}, false},
		"zero length":        {cue{start: time.Second, end: time.Second, text: "Hi."}, false},
	}
	for name, tc := range cases {
		if got := isDialogue(tc.c); got != tc.want {
			t.Errorf("%s: isDialogue = %v, want %v", name, got, tc.want)
		}
	}
}

func TestRasteriseMarksDialogueFrames(t *testing.T) {
	cues := []cue{
		{start: 20 * time.Millisecond, end: 50 * time.Millisecond, text: "Hi."},
		{start: 60 * time.Millisecond, end: 80 * time.Millisecond, text: "[laughs]"},
	}
	const noLimit, limit = 100, 4
	want := []float64{-1, -1, 1, 1, 1, -1, -1, -1, -1}
	if got := rasterise(cues, unitRatio, noLimit); !slices.Equal(got, want) {
		t.Errorf("rasterise = %v, want %v", got, want)
	}
	if got := rasterise(cues, unitRatio, limit); !slices.Equal(got, want[:limit]) {
		t.Errorf("rasterise with limit = %v, want %v", got, want[:limit])
	}
}

func TestCrossCorrelateFindsDelay(t *testing.T) {
	const maxOffset, delay = 5, 3
	subtitle := []float64{-1, 1, 1, -1, 1, -1, -1, 1}
	speech := make([]float64, len(subtitle)+delay)
	for i := range speech {
		speech[i] = nonSpeechValue
	}
	copy(speech[delay:], subtitle)

	size := nextPowerOfTwo(len(speech) + len(subtitle))
	scores := make([]float64, 2*maxOffset+1)
	crossCorrelate(spectrum(speech, size), subtitle, make([]complex128, size), scores, maxOffset)

	if got := argmax(scores) - maxOffset; got != delay {
		t.Errorf("best delay = %d, want %d (scores %v)", got, delay, scores)
	}
}

func TestPeakToSidelobe(t *testing.T) {
	scores := make([]float64, 1000)
	for i := range scores {
		scores[i] = float64(i % 2)
	}
	scores[500] = 10.5
	const tolerance = 0.01
	if got := peakToSidelobe(scores, 500); math.Abs(got-20) > tolerance {
		t.Errorf("peakToSidelobe = %v, want 20 (sidelobes have mean 0.5, deviation 0.5)", got)
	}
}
