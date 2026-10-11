package fakes

import (
	"context"
	"errors"
	"testing"
	"time"

	"magpie/internal/subtitles"
)

const (
	testUser     = "alice"
	testPassword = "hunter2"
	testFileID   = 4242
	testVideo    = "/media/Film.mkv"
	testOffset   = 1500 * time.Millisecond
)

var testSRT = []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")

func TestOpenSubtitlesLoginSearchAndDownload(t *testing.T) {
	ctx := context.Background()
	f := NewOpenSubtitles()
	f.SetCredentials(testUser, testPassword)
	query := subtitles.SearchQuery{MovieHash: "8e245d9679d31e12", Tmdb: 974576, Language: "en"}
	f.AddCandidates(query, subtitles.Candidate{FileID: testFileID, Language: "en", HashMatch: true})
	f.SetContent(testFileID, testSRT)

	if _, err := f.Login(ctx, testUser, "wrong"); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("bad login err = %v", err)
	}
	if token, err := f.Login(ctx, testUser, testPassword); err != nil || token.Value == "" {
		t.Fatalf("Login = %+v, %v", token, err)
	}
	cands, err := f.Search(ctx, query)
	if err != nil || len(cands) != 1 || cands[0].FileID != testFileID {
		t.Fatalf("Search = %+v, %v", cands, err)
	}
	content, remaining, _, err := f.Download(ctx, testFileID)
	if err != nil || string(content) != string(testSRT) || remaining != FreeDailyDownloads-1 {
		t.Fatalf("Download = %q, %d, %v", content, remaining, err)
	}
}

func TestOpenSubtitlesQuotaRunsOut(t *testing.T) {
	f := NewOpenSubtitles()
	f.SetContent(testFileID, testSRT)
	resetAt := time.Now().Add(time.Hour)
	f.SetQuota(1, resetAt)

	if _, _, _, err := f.Download(context.Background(), testFileID); err != nil {
		t.Fatal(err)
	}
	_, _, gotReset, err := f.Download(context.Background(), testFileID)
	if !errors.Is(err, subtitles.ErrQuotaExceeded) || !gotReset.Equal(resetAt) {
		t.Errorf("second download = %v, %v; want ErrQuotaExceeded at %v", gotReset, err, resetAt)
	}
}

func TestSyncerOffsetsAndNoSpeech(t *testing.T) {
	ctx := context.Background()
	f := NewSyncer()
	f.SetOffset(testVideo, testOffset)

	synced, offset, err := f.Sync(ctx, testVideo, testSRT)
	if err != nil || offset != testOffset || string(synced) != string(testSRT) {
		t.Fatalf("Sync = %q, %v, %v", synced, offset, err)
	}
	f.SetNoSpeech(testVideo)
	if _, _, err := f.Sync(ctx, testVideo, testSRT); !errors.Is(err, subtitles.ErrNoSpeech) {
		t.Errorf("err = %v, want ErrNoSpeech", err)
	}
}
