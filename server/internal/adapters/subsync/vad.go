package subsync

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"slices"
)

// PCM format the decoder must produce.
const (
	SampleRate     = 16000
	bytesPerSample = 2
)

// Energy VAD tuning. Speech is the 300–3400 Hz band rising clearly above the local noise floor.
const (
	samplesPerFrame = SampleRate / frameRate

	speechBandLowHz  = 300
	speechBandHighHz = 3400
	butterworthQ     = math.Sqrt2 / 2

	fullScale = math.MaxInt16 + 1

	// Frames quieter than this are silence whatever the noise floor.
	silenceFloorDB = -60
	// Speech must rise this far above the local noise floor.
	speechMarginDB = 9
	// The noise floor is this percentile of frame energies within the surrounding window.
	noiseFloorPercentile   = 0.1
	noiseFloorWindowFrames = 10 * frameRate
	noiseFloorBlockFrames  = frameRate

	// Pauses between words shorter than this stay speech, as a subtitle stays on screen through them.
	maxPauseFrames = frameRate / 4
	// Speech bursts shorter than this are clicks or noise.
	minBurstFrames = frameRate / 10

	readBufferBytes = 64 * 1024
)

// biquad is an RBJ cookbook second-order filter in transposed direct form II.
type biquad struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             float64
}

func newHighPass(cutoffHz float64) *biquad {
	cos, alpha := biquadAngles(cutoffHz)
	return normalised(biquad{b0: (1 + cos) / 2, b1: -(1 + cos), b2: (1 + cos) / 2}, cos, alpha)
}

func newLowPass(cutoffHz float64) *biquad {
	cos, alpha := biquadAngles(cutoffHz)
	return normalised(biquad{b0: (1 - cos) / 2, b1: 1 - cos, b2: (1 - cos) / 2}, cos, alpha)
}

func biquadAngles(cutoffHz float64) (cos, alpha float64) {
	w0 := 2 * math.Pi * cutoffHz / SampleRate
	return math.Cos(w0), math.Sin(w0) / (2 * butterworthQ)
}

func normalised(f biquad, cos, alpha float64) *biquad {
	a0 := 1 + alpha
	return &biquad{
		b0: f.b0 / a0, b1: f.b1 / a0, b2: f.b2 / a0,
		a1: -2 * cos / a0, a2: (1 - alpha) / a0,
	}
}

func (f *biquad) process(x float64) float64 {
	y := f.b0*x + f.z1
	f.z1 = f.b1*x - f.a1*y + f.z2
	f.z2 = f.b2*x - f.a2*y
	return y
}

// detectSpeech reads 16 kHz mono s16le PCM and returns one speech flag per frame.
func detectSpeech(ctx context.Context, pcm io.Reader) ([]bool, error) {
	energies, err := frameEnergies(ctx, pcm)
	if err != nil {
		return nil, err
	}
	floors := noiseFloors(energies)
	speech := make([]bool, len(energies))
	for i, e := range energies {
		speech[i] = e > silenceFloorDB && e > floors[i]+speechMarginDB
	}
	fillShortRuns(speech, false, maxPauseFrames)
	fillShortRuns(speech, true, minBurstFrames)
	return speech, nil
}

// frameEnergies returns the speech-band energy of each frame in dBFS.
func frameEnergies(ctx context.Context, pcm io.Reader) ([]float64, error) {
	highPass, lowPass := newHighPass(speechBandLowHz), newLowPass(speechBandHighHz)
	chunk := make([]byte, readBufferBytes)
	var energies []float64
	var sumSquares float64
	inFrame := 0

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := io.ReadFull(pcm, chunk)
		for i := 0; i+bytesPerSample <= n; i += bytesPerSample {
			sample := float64(int16(binary.LittleEndian.Uint16(chunk[i:]))) / fullScale
			filtered := lowPass.process(highPass.process(sample))
			sumSquares += filtered * filtered
			inFrame++
			if inFrame == samplesPerFrame {
				energies = append(energies, decibels(sumSquares/samplesPerFrame))
				sumSquares, inFrame = 0, 0
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return energies, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func decibels(power float64) float64 {
	const minPower = 1e-12
	return 10 * math.Log10(max(power, minPower))
}

// noiseFloors estimates the noise floor around each frame, one estimate per block of frames.
func noiseFloors(energies []float64) []float64 {
	floors := make([]float64, len(energies))
	window := make([]float64, 0, noiseFloorWindowFrames+noiseFloorBlockFrames)
	for start := 0; start < len(energies); start += noiseFloorBlockFrames {
		end := min(start+noiseFloorBlockFrames, len(energies))
		center := (start + end) / 2
		lo := max(0, center-noiseFloorWindowFrames/2)
		hi := min(len(energies), center+noiseFloorWindowFrames/2)
		window = append(window[:0], energies[lo:hi]...)
		slices.Sort(window)
		floor := window[int(noiseFloorPercentile*float64(len(window)-1))]
		for i := start; i < end; i++ {
			floors[i] = floor
		}
	}
	return floors
}

// fillShortRuns flips interior runs of value shorter than maxLen to the opposite value.
func fillShortRuns(flags []bool, value bool, maxLen int) {
	for start := 0; start < len(flags); {
		if flags[start] != value {
			start++
			continue
		}
		end := start
		for end < len(flags) && flags[end] == value {
			end++
		}
		interior := start > 0 && end < len(flags)
		if interior && end-start < maxLen {
			for i := start; i < end; i++ {
				flags[i] = !value
			}
		}
		start = end
	}
}
