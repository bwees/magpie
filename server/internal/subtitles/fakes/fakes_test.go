package fakes

import (
	"context"
	"errors"
	"testing"
	"time"

	"magpie/internal/subtitles"
)

const (
	testFileID  = 4242
	testVideo   = "/media/Film.mkv"
	testSidecar = "/media/Film.en.srt"
	testOffset  = 1500 * time.Millisecond
)

var testSRT = []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")

func TestOpenSubtitlesLoginSearchAndDownload(t *testing.T) {
	ctx := context.Background()
	f := NewOpenSubtitles()
	query := subtitles.SearchQuery{MovieHash: "8e245d9679d31e12", Tmdb: 974576, Language: "en"}
	f.AddCandidates(query, subtitles.Candidate{FileID: testFileID, Language: "en", HashMatch: true})
	f.SetContent(testFileID, testSRT)

	if _, _, _, err := f.Download(ctx, testFileID); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("download before login err = %v, want ErrAuth", err)
	}
	if token, err := f.Login(ctx); err != nil || token.Value == "" || token.AllowedDownloads != FreeDailyDownloads {
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

func TestOpenSubtitlesRejectsLogin(t *testing.T) {
	f := NewOpenSubtitles()
	f.RejectLogin()
	if _, err := f.Login(context.Background()); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("Login err = %v, want ErrAuth", err)
	}
}

func TestOpenSubtitlesQuotaRunsOut(t *testing.T) {
	f := NewOpenSubtitles()
	f.SetContent(testFileID, testSRT)
	if _, err := f.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
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

func TestSidecarFilesWriteAndRemove(t *testing.T) {
	f := NewSidecarFiles()
	if err := f.Write(testSidecar, testSRT); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.Read(testSidecar); !ok || string(got) != string(testSRT) {
		t.Fatalf("Read = %q, %v", got, ok)
	}
	if err := f.Remove(testSidecar); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove(testSidecar); err != nil {
		t.Errorf("removing a missing file = %v, want nil", err)
	}
	if paths := f.Paths(); len(paths) != 0 {
		t.Errorf("Paths = %v, want none", paths)
	}
}
