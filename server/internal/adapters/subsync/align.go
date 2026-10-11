// Ported from ffsubsync (MIT, Copyright 2019 Stephen Macke). See doc.go for the licence.

package subsync

import (
	"context"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	// frameRate is the number of speech/no-speech frames per second (ffsubsync SAMPLE_RATE).
	frameRate     = 100
	frameDuration = time.Second / frameRate

	// Cues on screen longer than this are signs or credits, not speech (ffsubsync DEFAULT_MAX_SUBTITLE_SECONDS).
	maxCueDuration = 10 * time.Second

	// Frames this close to the best offset belong to its peak, not to the sidelobes.
	peakExclusionFrames = 2 * frameRate

	speechValue    = 1.0
	nonSpeechValue = -1.0

	unitRatio = 1.0
	// A framerate change must score this fraction higher than keeping the subtitle's scale.
	framerateChangeMinGain = 0.01
)

// framerateChanges are the subtitle time scales tried besides 1 (ffsubsync FRAMERATE_RATIOS and their inverses).
var framerateChanges = []float64{
	24 / 23.976, 25 / 23.976, 25 / 24.0,
	23.976 / 24, 23.976 / 25, 24 / 25.0,
}

var (
	markupTag = regexp.MustCompile(`<[^>]+>|\{[^}]*\}`)

	pairedBrackets = map[rune]rune{
		'(': ')', '[': ']',
		'（': '）', '【': '】', '「': '」',
	}

	nonDialogueSymbols = "♪♫♬♩🎵🎶"
)

type alignment struct {
	offsetFrames int
	ratio        float64
	score        float64
	// peakRatio is the peak-to-sidelobe ratio: how many standard deviations the best score
	// stands above the other offsets in the search range.
	peakRatio float64
}

func (a alignment) timing() timing {
	return timing{ratio: a.ratio, offset: time.Duration(a.offsetFrames) * frameDuration}
}

// align finds the subtitle scale and shift whose cue signal best matches the speech signal.
func align(ctx context.Context, speech []float64, cues []cue, maxOffsetFrames int) (alignment, error) {
	ratios := append([]float64{unitRatio}, framerateChanges...)
	subtitles := make([][]float64, len(ratios))
	longest := 0
	for i, ratio := range ratios {
		subtitles[i] = rasterise(cues, ratio, len(speech)+maxOffsetFrames)
		longest = max(longest, len(subtitles[i]))
	}

	size := nextPowerOfTwo(len(speech) + longest)
	speechSpectrum := spectrum(speech, size)
	work := make([]complex128, size)
	scores := make([]float64, 2*maxOffsetFrames+1)
	var bestScores []float64

	var best alignment
	for i, ratio := range ratios {
		if err := ctx.Err(); err != nil {
			return alignment{}, err
		}
		crossCorrelate(speechSpectrum, subtitles[i], work, scores, maxOffsetFrames)
		lag := argmax(scores)
		candidate := alignment{offsetFrames: lag - maxOffsetFrames, ratio: ratio, score: scores[lag]}
		if ratio == unitRatio || beatsUnitRatio(candidate, best) {
			best = candidate
			bestScores = append(bestScores[:0], scores...)
		}
	}
	best.peakRatio = peakToSidelobe(bestScores, best.offsetFrames+maxOffsetFrames)
	return best, nil
}

// beatsUnitRatio reports whether a framerate change scores clearly better than the current best.
// ffsubsync takes the plain maximum; on short clips that lets a 0.1% change win by noise.
func beatsUnitRatio(candidate, best alignment) bool {
	if best.ratio == unitRatio {
		return candidate.score > best.score+framerateChangeMinGain*math.Abs(best.score)
	}
	return candidate.score > best.score
}

// rasterise turns cues into one value per frame: speech while a dialogue cue is on screen.
// Frames from limit on are dropped; no shift in range can line them up with the audio.
func rasterise(cues []cue, ratio float64, limit int) []float64 {
	var last time.Duration
	for _, c := range cues {
		last = max(last, c.end)
	}
	signal := make([]float64, min(toFrame(last, ratio)+1, limit))
	for i := range signal {
		signal[i] = nonSpeechValue
	}
	// Stretched cues are down-weighted so stretching alone can't raise the score (ffsubsync).
	value := 2*min(1/ratio, 1) - 1
	for _, c := range cues {
		if !isDialogue(c) {
			continue
		}
		for f := toFrame(c.start, ratio); f < min(toFrame(c.end, ratio), len(signal)); f++ {
			signal[f] = value
		}
	}
	return signal
}

func toFrame(d time.Duration, ratio float64) int {
	return int(math.Round(d.Seconds() * ratio * frameRate))
}

// isDialogue drops cues that are not someone speaking: empty, bracketed sound descriptions,
// music notes, and long signs.
func isDialogue(c cue) bool {
	if c.end <= c.start || c.end-c.start > maxCueDuration {
		return false
	}
	text := strings.TrimSpace(markupTag.ReplaceAllString(c.text, ""))
	if text == "" {
		return false
	}
	runes := []rune(text)
	if closing, ok := pairedBrackets[runes[0]]; ok && runes[len(runes)-1] == closing {
		return false
	}
	return strings.ContainsFunc(text, func(r rune) bool {
		return !strings.ContainsRune(nonDialogueSymbols, r) && !unicode.IsSpace(r)
	})
}

func spectrum(signal []float64, size int) []complex128 {
	out := make([]complex128, size)
	for i, v := range signal {
		out[i] = complex(v, 0)
	}
	fft(out, false)
	return out
}

// crossCorrelate fills scores[k+maxOffset] with sum(speech[n] * subtitle[n-k]) for k in ±maxOffset:
// the match between speech and the subtitle delayed by k frames.
func crossCorrelate(speechSpectrum []complex128, subtitle []float64, work []complex128, scores []float64, maxOffset int) {
	clear(work)
	for i, v := range subtitle {
		work[i] = complex(v, 0)
	}
	fft(work, false)
	for i := range work {
		s := work[i]
		work[i] = speechSpectrum[i] * complex(real(s), -imag(s))
	}
	fft(work, true)
	size := len(work)
	for k := -maxOffset; k <= maxOffset; k++ {
		scores[k+maxOffset] = real(work[(k+size)%size])
	}
}

func argmax(values []float64) int {
	best := 0
	for i, v := range values {
		if v > values[best] {
			best = i
		}
	}
	return best
}

func peakToSidelobe(scores []float64, peak int) float64 {
	var sum, sumSquares float64
	n := 0
	for i, v := range scores {
		if abs(i-peak) <= peakExclusionFrames {
			continue
		}
		sum += v
		sumSquares += v * v
		n++
	}
	if n == 0 {
		return 0
	}
	mean := sum / float64(n)
	std := math.Sqrt(max(sumSquares/float64(n)-mean*mean, 0))
	if std == 0 {
		return 0
	}
	return (scores[peak] - mean) / std
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
