package subsync

import (
	"context"
	"testing"
	"time"
)

const episodeDuration = 45 * time.Minute

// BenchmarkSyncEpisode times speech detection and alignment for a 45-minute episode.
// ffmpeg decoding is not included.
func BenchmarkSyncEpisode(b *testing.B) {
	clip := synthesiseClip(12, episodeDuration)
	syncer := New(fixtureDecoder{"episode.mkv": clip.pcm})
	input := renderSRT(clip.cues, 1, 17*time.Second)

	for b.Loop() {
		if _, _, err := syncer.Sync(context.Background(), "episode.mkv", input); err != nil {
			b.Fatal(err)
		}
	}
}
