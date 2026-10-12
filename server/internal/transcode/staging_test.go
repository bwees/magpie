package transcode

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"

	"gorm.io/gorm"
)

const testStagingDir = ".magpie-staging"

var stagedBytes = []byte("staged output")

// stagedJob stores a job in state whose staged file sits in dir, and writes that file unless absent.
func (f *fixture) stagedJob(dir string, kind api.JobKind, state api.JobState, onDisk bool) string {
	f.t.Helper()
	id := domain.NewID[domain.JobID]()
	path := stagedPath(dir, id, kind)
	row := jobRow{
		ID: id, Kind: kind, MediaFileID: domain.NewID[domain.MediaFileID](), TitleID: domain.NewID[domain.TitleID](),
		Origin: domain.OriginDownload, State: state, Attempt: 1, ExcludedAgents: emptyAgents, StagingPath: path,
		QueuedAt: testStart, UpdatedAt: testStart,
	}
	if err := f.db.Write(context.Background(), func(tx *gorm.DB) error { return tx.Create(&row).Error }); err != nil {
		f.t.Fatal(err)
	}
	if onDisk {
		f.writeStaged(path)
	}
	return path
}

func (f *fixture) writeStaged(path string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), stagingDirPerm); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, stagedBytes, stagingFilePerm); err != nil {
		f.t.Fatal(err)
	}
}

// captureLogs sends slog's default logger to a buffer for the rest of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

func TestStagingSweepRemovesOrphansAndKeepsWhatJobsNeed(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, testStagingDir)
	kept := map[string]string{}
	for _, state := range []api.JobState{
		api.StateQueued, api.StateOffered, api.StateLeased, api.StateFetching, api.StateEncoding,
		api.StateUploading, api.StateVerifying, api.StateVerified,
	} {
		kept[string(state)] = f.stagedJob(dir, api.JobKindTranscode, state, true)
	}
	kept["COMPLETED Whisper subtitle Subtitles hasn't moved"] = f.stagedJob(dir, api.JobKindWhisper, api.StateCompleted, true)
	foreign := filepath.Join(dir, "not-a-job.mkv")
	f.writeStaged(foreign)
	kept["a file Transcode didn't name"] = foreign

	removed := map[string]string{}
	for _, state := range []api.JobState{api.StateSwapped, api.StateFailed, api.StateNotWorthIt, api.StateCancelled} {
		removed[string(state)] = f.stagedJob(dir, api.JobKindTranscode, state, true)
	}
	removed["FAILED Whisper subtitle"] = f.stagedJob(dir, api.JobKindWhisper, api.StateFailed, true)
	unknown := stagedPath(dir, domain.NewID[domain.JobID](), api.JobKindTranscode)
	f.writeStaged(unknown)
	removed["a job that no longer exists"] = unknown
	logs := captureLogs(t)

	if err := f.jobs.SweepStaging(context.Background()); err != nil {
		t.Fatal(err)
	}

	for what, path := range kept {
		if !exists(path) {
			t.Errorf("removed the staged file of %s", what)
		}
	}
	for what, path := range removed {
		if exists(path) {
			t.Errorf("kept the staged file of %s", what)
		}
		if !strings.Contains(logs.String(), "level=INFO") || !strings.Contains(logs.String(), path) {
			t.Errorf("no INFO log names the removed %s", path)
		}
	}
}

func TestStagingSweepSkipsAMissingStagingDir(t *testing.T) {
	f := newFixture(t)
	gone := filepath.Join(t.TempDir(), "unmounted", testStagingDir)
	f.stagedJob(gone, api.JobKindTranscode, api.StateFailed, false)
	present := filepath.Join(f.root, testStagingDir)
	orphan := f.stagedJob(present, api.JobKindTranscode, api.StateCancelled, true)

	if err := f.jobs.SweepStaging(context.Background()); err != nil {
		t.Fatal(err)
	}

	if exists(orphan) {
		t.Fatal("a missing staging dir stopped the sweep of the others")
	}
}
