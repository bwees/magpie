package subsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"testing"
	"time"

	"magpie/internal/subtitles"
)

const (
	clipDuration = 180 * time.Second
	tolerance    = 100 * time.Millisecond
	palSpeedup   = 25 / 23.976
)

var _ subtitles.Syncer = (*Syncer)(nil)

var anyTimestamp = regexp.MustCompile(`\d+:\d{2}:\d{2},\d{3}`)

// The corpus: each clip's subtitle is displaced by a known offset; Sync must undo it.
func TestSyncRecoversKnownOffsets(t *testing.T) {
	corpus := []struct {
		seed      uint64
		displaced time.Duration
	}{
		{seed: 1, displaced: 0},
		{seed: 2, displaced: 2500 * time.Millisecond},
		{seed: 3, displaced: -2500 * time.Millisecond},
		{seed: 4, displaced: 17 * time.Second},
		{seed: 5, displaced: -45 * time.Second},
	}
	for _, tc := range corpus {
		t.Run(fmt.Sprintf("clip %d displaced %v", tc.seed, tc.displaced), func(t *testing.T) {
			t.Parallel()
			clip := synthesiseClip(tc.seed, clipDuration)
			syncer := New(fixtureDecoder{"clip.mkv": clip.pcm})
			input := renderSRT(clip.cues, 1, tc.displaced)

			synced, offset, err := syncer.Sync(context.Background(), "clip.mkv", input)
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			t.Logf("offset %v, want %v", offset, -tc.displaced)
			if diff := (offset + tc.displaced).Abs(); diff > tolerance {
				t.Errorf("offset = %v, want %v ± %v", offset, -tc.displaced, tolerance)
			}
			assertTimestampsNear(t, synced, renderSRT(clip.cues, 1, 0), tc.displaced < 0)
		})
	}
}

func TestSyncCorrectsFramerate(t *testing.T) {
	clip := synthesiseClip(6, clipDuration)
	syncer := New(fixtureDecoder{"clip.mkv": clip.pcm})
	input := renderSRT(clip.cues, palSpeedup, 3*time.Second)

	synced, _, err := syncer.Sync(context.Background(), "clip.mkv", input)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	assertTimestampsNear(t, synced, renderSRT(clip.cues, 1, 0), false)
}

func TestSyncReturnsErrNoSpeech(t *testing.T) {
	clip := synthesiseClip(7, clipDuration)
	other := synthesiseClip(8, clipDuration)
	decoder := fixtureDecoder{
		"clip.mkv":     clip.pcm,
		"silent.mkv":   silentClip(clipDuration),
		"ambience.mkv": ambienceClip(9, clipDuration),
	}
	cases := []struct {
		name  string
		video string
		srt   []byte
	}{
		{"silent clip", "silent.mkv", renderSRT(clip.cues, 1, 0)},
		{"music and effects only", "ambience.mkv", renderSRT(clip.cues, 1, 0)},
		{"subtitle for another video", "clip.mkv", renderSRT(other.cues, 1, 0)},
		{"subtitle without dialogue", "clip.mkv", []byte("1\n00:00:01,000 --> 00:00:03,000\n[door slams]\n\n2\n00:00:05,000 --> 00:00:07,000\n♪ ♪\n")},
		{"empty subtitle", "clip.mkv", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := New(decoder).Sync(context.Background(), tc.video, tc.srt)
			t.Log(err)
			if !errors.Is(err, subtitles.ErrNoSpeech) {
				t.Errorf("err = %v, want subtitles.ErrNoSpeech", err)
			}
		})
	}
}

func TestSyncRoundTripsWhenAlreadyAligned(t *testing.T) {
	clip := synthesiseClip(10, clipDuration)
	speech, err := detectSpeech(context.Background(), bytes.NewReader(clip.pcm))
	if err != nil {
		t.Fatal(err)
	}
	input := append([]byte("\xef\xbb\xbf"), srtFromSpeech(speech)...)
	syncer := New(fixtureDecoder{"clip.mkv": clip.pcm})

	synced, offset, err := syncer.Sync(context.Background(), "clip.mkv", input)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if offset != 0 {
		t.Errorf("offset = %v, want 0", offset)
	}
	if !bytes.Equal(synced, input) {
		t.Error("synced subtitle differs from the input")
	}
}

func TestSyncKeepsCueText(t *testing.T) {
	clip := synthesiseClip(2, clipDuration)
	input := renderSRT(clip.cues, 1, 2500*time.Millisecond)
	synced, _, err := New(fixtureDecoder{"clip.mkv": clip.pcm}).Sync(context.Background(), "clip.mkv", input)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	stripped := func(srt []byte) []byte { return anyTimestamp.ReplaceAll(srt, nil) }
	if !bytes.Equal(stripped(synced), stripped(input)) {
		t.Error("synced subtitle changed more than its timestamps")
	}
}

func TestSyncHonoursCancellation(t *testing.T) {
	clip := synthesiseClip(1, clipDuration)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := New(fixtureDecoder{"clip.mkv": clip.pcm}).Sync(ctx, "clip.mkv", renderSRT(clip.cues, 1, 0))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestSyncReportsDecodeFailure(t *testing.T) {
	decodeErr := errors.New("corrupt stream")
	srt := []byte("1\n00:00:01,000 --> 00:00:03,000\nHello.\n")
	cases := map[string]AudioDecoder{
		"on open":  failingDecoder{openErr: decodeErr},
		"on close": failingDecoder{closeErr: decodeErr},
	}
	for name, decoder := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := New(decoder).Sync(context.Background(), "clip.mkv", srt)
			if !errors.Is(err, decodeErr) {
				t.Errorf("err = %v, want %v", err, decodeErr)
			}
		})
	}
}

type failingDecoder struct {
	openErr, closeErr error
}

func (f failingDecoder) DecodeAudio(context.Context, string) (io.ReadCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return failingCloser{Reader: bytes.NewReader(silentClip(time.Second)), err: f.closeErr}, nil
}

type failingCloser struct {
	io.Reader
	err error
}

func (f failingCloser) Close() error { return f.err }

// assertTimestampsNear checks every cue of got lands within tolerance of want.
// dropped skips the leading cues of want that the displaced input no longer had.
func assertTimestampsNear(t *testing.T, got, want []byte, dropped bool) {
	t.Helper()
	gotCues, wantCues := parseCues(got), parseCues(want)
	if dropped {
		wantCues = wantCues[len(wantCues)-len(gotCues):]
	}
	if len(gotCues) != len(wantCues) {
		t.Fatalf("got %d cues, want %d", len(gotCues), len(wantCues))
	}
	for i := range gotCues {
		if d := (gotCues[i].start - wantCues[i].start).Abs(); d > tolerance {
			t.Errorf("cue %d starts at %v, want %v", i+1, gotCues[i].start, wantCues[i].start)
		}
		if d := (gotCues[i].end - wantCues[i].end).Abs(); d > tolerance {
			t.Errorf("cue %d ends at %v, want %v", i+1, gotCues[i].end, wantCues[i].end)
		}
	}
}

// srtFromSpeech writes one cue per detected speech run, so the subtitle matches the audio exactly.
func srtFromSpeech(speech []bool) []byte {
	var b bytes.Buffer
	index := 0
	for start := 0; start < len(speech); start++ {
		if !speech[start] {
			continue
		}
		end := start
		for end < len(speech) && speech[end] {
			end++
		}
		index++
		fmt.Fprintf(&b, "%d\n%s --> %s\n<b>Speaker %d</b>\n\n", index,
			formatTimestamp(time.Duration(start)*frameDuration, ','),
			formatTimestamp(time.Duration(end)*frameDuration, ','), index)
		start = end
	}
	return b.Bytes()
}
