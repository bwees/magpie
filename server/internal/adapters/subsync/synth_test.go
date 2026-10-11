package subsync

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"time"
)

// Synthetic clips: formant-synthesised speech in noise, with music and sound effects in some
// pauses, plus a correctly timed subtitle whose cues jitter around the utterances like real ones.

const (
	synthNoiseDB       = -55
	synthMusicDB       = -30
	synthEffectDB      = -26
	synthMinSpeechDB   = -28
	synthMaxSpeechDB   = -16
	synthCueJitter     = 150 * time.Millisecond
	synthMissingCue    = 0.08
	synthMergeCue      = 0.2
	synthMergeMaxGap   = time.Second
	synthLongPause     = 0.15
	synthPausedMusic   = 0.4
	synthPausedEffect  = 0.3
	synthFricative     = 0.35
	synthEnvelopeRamp  = 0.025
	synthFormantBWHz   = 90
	synthMaxSyllables  = 3
	synthMusicVoices   = 3
	synthMusicBaseHz   = 220
	synthMusicSpreadHz = 440
)

var synthVowels = [][3]float64{
	{730, 1090, 2440}, {270, 2290, 3010}, {300, 870, 2240}, {530, 1840, 2480}, {570, 840, 2410},
}

type span struct{ start, end time.Duration }

type synthClip struct {
	pcm        []byte
	cues       []synthCue
	utterances []span
}

type synthCue struct {
	span
	text string
}

type synth struct {
	rng     *rand.Rand
	samples []float64
}

func newSynth(seed uint64, duration time.Duration) *synth {
	s := &synth{
		rng:     rand.New(rand.NewPCG(seed, ^seed)),
		samples: make([]float64, int(duration.Seconds()*SampleRate)),
	}
	s.addNoise(dbToAmplitude(synthNoiseDB))
	return s
}

func synthesiseClip(seed uint64, duration time.Duration) synthClip {
	s := newSynth(seed, duration)
	var utterances []span
	t := s.seconds(1, 3)
	for t < duration-3*time.Second {
		u := span{t, min(t+s.seconds(0.8, 4.5), duration-time.Second)}
		s.addUtterance(u)
		utterances = append(utterances, u)

		pause := s.seconds(0.4, 2.5)
		if s.rng.Float64() < synthLongPause {
			pause = s.seconds(5, 12)
			gap := span{u.end + 500*time.Millisecond, u.end + pause - 500*time.Millisecond}
			switch r := s.rng.Float64(); {
			case r < synthPausedMusic:
				s.addMusic(gap)
			case r < synthPausedMusic+synthPausedEffect:
				s.addEffect(gap)
			}
		}
		t = u.end + pause
	}
	return synthClip{pcm: encodePCM(s.samples), cues: s.cuesFor(utterances), utterances: utterances}
}

// ambienceClip has the noise, music and sound effects of a synthetic clip but nobody speaking.
func ambienceClip(seed uint64, duration time.Duration) []byte {
	s := newSynth(seed, duration)
	for t := time.Duration(0); t < duration; {
		gap := span{t, t + s.seconds(2, 8)}
		if s.rng.Float64() < synthPausedMusic {
			s.addMusic(gap)
		} else {
			s.addEffect(gap)
		}
		t = gap.end + s.seconds(1, 6)
	}
	return encodePCM(s.samples)
}

func silentClip(duration time.Duration) []byte {
	return make([]byte, int(duration.Seconds()*SampleRate)*bytesPerSample)
}

func (s *synth) seconds(lo, hi float64) time.Duration {
	return time.Duration((lo + s.rng.Float64()*(hi-lo)) * float64(time.Second))
}

func (s *synth) uniform(lo, hi float64) float64 {
	return lo + s.rng.Float64()*(hi-lo)
}

func (s *synth) index(d time.Duration) int {
	return min(int(d.Seconds()*SampleRate), len(s.samples))
}

func (s *synth) addNoise(amplitude float64) {
	for i := range s.samples {
		s.samples[i] += amplitude * s.rng.NormFloat64()
	}
}

func (s *synth) addUtterance(u span) {
	level := dbToAmplitude(s.uniform(synthMinSpeechDB, synthMaxSpeechDB))
	baseF0 := s.uniform(95, 220)
	t := u.start
	for t < u.end {
		for range 1 + s.rng.IntN(synthMaxSyllables) {
			syllable := span{t, min(t+s.seconds(0.12, 0.28), u.end)}
			s.addSyllable(syllable, baseF0*s.uniform(0.85, 1.2), level)
			t = syllable.end
		}
		t += s.seconds(0.03, 0.15)
	}
}

func (s *synth) addSyllable(sy span, f0, level float64) {
	start, end := s.index(sy.start), s.index(sy.end)
	if end <= start {
		return
	}
	vowel := synthVowels[s.rng.IntN(len(synthVowels))]
	voiced := make([]float64, end-start)
	period := SampleRate / f0
	glide := s.uniform(-0.15, 0.15)
	phase := 0.0
	for i := range voiced {
		phase++
		if phase >= period*(1+glide*float64(i)/float64(len(voiced))) {
			voiced[i], phase = 1, 0
		}
	}
	for _, formant := range vowel {
		resonate(voiced, formant, synthFormantBWHz)
	}
	if s.rng.Float64() < synthFricative {
		burst := min(len(voiced), int(s.uniform(0.03, 0.07)*SampleRate))
		hiss := highPassNoise(s.rng, burst)
		for i := range burst {
			voiced[i] += hiss[i]
		}
	}
	normalise(voiced, level)
	ramp := int(synthEnvelopeRamp * SampleRate)
	for i, v := range voiced {
		s.samples[start+i] += v * envelope(i, len(voiced), ramp)
	}
}

func (s *synth) addMusic(gap span) {
	start, end := s.index(gap.start), s.index(gap.end)
	amplitude := dbToAmplitude(synthMusicDB) / synthMusicVoices
	for range synthMusicVoices {
		freq := synthMusicBaseHz + s.rng.Float64()*synthMusicSpreadHz
		for i := start; i < end; i++ {
			t := float64(i-start) / SampleRate
			s.samples[i] += amplitude * math.Sin(2*math.Pi*freq*t) * envelope(i-start, end-start, SampleRate/10)
		}
	}
}

func (s *synth) addEffect(gap span) {
	start := s.index(gap.start)
	end := min(s.index(gap.end), start+int(s.uniform(0.3, 1.5)*SampleRate))
	if end <= start {
		return
	}
	burst := highPassNoise(s.rng, end-start)
	normalise(burst, dbToAmplitude(synthEffectDB))
	for i, v := range burst {
		s.samples[start+i] += v * envelope(i, len(burst), SampleRate/50)
	}
}

func (s *synth) cuesFor(utterances []span) []synthCue {
	var cues []synthCue
	for i := 0; i < len(utterances); i++ {
		if s.rng.Float64() < synthMissingCue {
			continue
		}
		u := utterances[i]
		for i+1 < len(utterances) && utterances[i+1].start-u.end < synthMergeMaxGap && s.rng.Float64() < synthMergeCue {
			i++
			u.end = utterances[i].end
		}
		jitter := func() time.Duration {
			return time.Duration((s.rng.Float64()*2 - 1) * float64(synthCueJitter))
		}
		cues = append(cues, synthCue{
			span: span{max(u.start+jitter(), 0), u.end + jitter()},
			text: fmt.Sprintf("<i>Line %d</i>\nsays something.", len(cues)+1),
		})
	}
	return cues
}

// srt renders the cues scaled by ratio and shifted by offset, dropping any that would start before zero.
func renderSRT(cues []synthCue, ratio float64, offset time.Duration) []byte {
	var b bytes.Buffer
	index := 0
	for _, c := range cues {
		start := time.Duration(float64(c.start)*ratio) + offset
		end := time.Duration(float64(c.end)*ratio) + offset
		if start < 0 {
			continue
		}
		index++
		fmt.Fprintf(&b, "%d\r\n%s --> %s\r\n%s\r\n\r\n", index,
			formatTimestamp(start, ','), formatTimestamp(end, ','),
			bytes.ReplaceAll([]byte(c.text), []byte("\n"), []byte("\r\n")))
	}
	return b.Bytes()
}

func resonate(x []float64, freqHz, bandwidthHz float64) {
	c := -math.Exp(-2 * math.Pi * bandwidthHz / SampleRate)
	b := 2 * math.Exp(-math.Pi*bandwidthHz/SampleRate) * math.Cos(2*math.Pi*freqHz/SampleRate)
	a := 1 - b - c
	var y1, y2 float64
	for i, v := range x {
		y := a*v + b*y1 + c*y2
		x[i], y1, y2 = y, y, y1
	}
}

func highPassNoise(rng *rand.Rand, n int) []float64 {
	out := make([]float64, n)
	prev := 0.0
	for i := range out {
		v := rng.NormFloat64()
		out[i], prev = v-prev, v
	}
	return out
}

func normalise(x []float64, rms float64) {
	var sum float64
	for _, v := range x {
		sum += v * v
	}
	if sum == 0 {
		return
	}
	scale := rms / math.Sqrt(sum/float64(len(x)))
	for i := range x {
		x[i] *= scale
	}
}

func envelope(i, n, ramp int) float64 {
	ramp = min(ramp, n/2)
	if ramp == 0 {
		return 1
	}
	edge := min(i, n-1-i)
	if edge >= ramp {
		return 1
	}
	return 0.5 - 0.5*math.Cos(math.Pi*float64(edge)/float64(ramp))
}

func dbToAmplitude(db float64) float64 {
	return math.Pow(10, db/20)
}

func encodePCM(samples []float64) []byte {
	out := make([]byte, len(samples)*bytesPerSample)
	for i, v := range samples {
		clipped := max(min(v*fullScale, math.MaxInt16), math.MinInt16)
		binary.LittleEndian.PutUint16(out[i*bytesPerSample:], uint16(int16(clipped)))
	}
	return out
}

// fixtureDecoder serves PCM fixtures by video path, as ffmpeg would.
type fixtureDecoder map[string][]byte

func (f fixtureDecoder) DecodeAudio(_ context.Context, videoPath string) (io.ReadCloser, error) {
	pcm, ok := f[videoPath]
	if !ok {
		return nil, fmt.Errorf("no fixture for %s", videoPath)
	}
	return io.NopCloser(bytes.NewReader(pcm)), nil
}
