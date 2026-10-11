// This package is a Go port of ffsubsync (https://github.com/smacke/ffsubsync), used under
// the MIT licence below. Magpie as a whole is GPL-3.0.
//
// Copyright 2019 Stephen Macke
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of this software
// and associated documentation files (the "Software"), to deal in the Software without
// restriction, including without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all copies or
// substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING
// BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// Package subsync aligns subtitle timing to a video's speech. It implements the Subtitles
// context's Syncer port.
//
// The algorithm is ffsubsync's:
//   - ffmpeg decodes the first audio track to 16 kHz mono PCM.
//   - A voice activity detector marks each 10 ms frame as speech or not.
//   - The subtitle is drawn into the same frames: speech while a dialogue cue is on screen.
//   - An FFT cross-correlation scores every shift within ±MaxOffset at once, for each
//     framerate ratio ffsubsync tries (23.976, 24 and 25 fps conversions).
//   - The best scale and shift are applied to the SRT timestamps. Nothing else in the file changes.
//
// The voice activity detector is energy based, not WebRTC's GMM: a frame is speech when its
// 300–3400 Hz energy is 9 dB above the local noise floor (10th percentile over 10 s). It needs
// no cgo or model files, and the cross-correlation tolerates its false positives on music.
//
// Confidence is the peak-to-sidelobe ratio: how many standard deviations the best score stands
// above the scores of all other shifts more than 2 s away. Below MinPeakRatio, or with less than
// MinSpeech of detected speech, Sync returns ErrNoSpeech. On the test corpus aligned clips score
// 9 to 15 and mismatched or speechless ones 2.5 to 4.
//
// Unlike ffsubsync, a framerate change must beat the unchanged subtitle by 1%, so a 0.1%
// change can't win on noise.
//
// Speed (BenchmarkSyncEpisode, Apple M2 Pro): a 45-minute episode takes 0.84 s for speech
// detection and alignment. ffmpeg's decode of a 45-minute 5.1 AC-3 track to 16 kHz mono adds
// about 5 s.
package subsync
