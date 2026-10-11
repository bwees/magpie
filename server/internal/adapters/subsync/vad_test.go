package subsync

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"
)

const (
	minSpeechRecall    = 0.85
	maxPauseFalseAlarm = 0.25
)

func TestDetectSpeechFollowsUtterances(t *testing.T) {
	clip := synthesiseClip(11, clipDuration)
	speech, err := detectSpeech(context.Background(), bytes.NewReader(clip.pcm))
	if err != nil {
		t.Fatal(err)
	}
	if want := int(clipDuration / frameDuration); len(speech) != want {
		t.Fatalf("got %d frames, want %d", len(speech), want)
	}

	truth := make([]bool, len(speech))
	for _, u := range clip.utterances {
		for f := int(u.start / frameDuration); f < int(u.end/frameDuration); f++ {
			truth[f] = true
		}
	}
	var hits, speechFrames, falseAlarms, pauseFrames int
	for i := range speech {
		switch {
		case truth[i]:
			speechFrames++
			if speech[i] {
				hits++
			}
		default:
			pauseFrames++
			if speech[i] {
				falseAlarms++
			}
		}
	}
	recall := float64(hits) / float64(speechFrames)
	falseAlarm := float64(falseAlarms) / float64(pauseFrames)
	t.Logf("recall %.2f, false alarms in pauses %.2f", recall, falseAlarm)
	if recall < minSpeechRecall {
		t.Errorf("recall = %.2f, want ≥ %.2f", recall, minSpeechRecall)
	}
	if falseAlarm > maxPauseFalseAlarm {
		t.Errorf("false alarm rate = %.2f, want ≤ %.2f", falseAlarm, maxPauseFalseAlarm)
	}
}

func TestDetectSpeechIgnoresSilence(t *testing.T) {
	speech, err := detectSpeech(context.Background(), bytes.NewReader(silentClip(10*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(speech, true) {
		t.Error("speech detected in digital silence")
	}
}

func TestFillShortRuns(t *testing.T) {
	flags := []bool{true, false, false, true, false, false, false, true, true, false}
	fillShortRuns(flags, false, 3)
	want := []bool{true, true, true, true, false, false, false, true, true, false}
	if !slices.Equal(flags, want) {
		t.Errorf("fillShortRuns = %v, want %v", flags, want)
	}
}
