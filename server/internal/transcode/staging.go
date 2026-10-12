package transcode

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"magpie/internal/domain"
	"magpie/internal/transcode/api"
)

// stagedNeededStates are the states in which a job may still need its staged file: an upload that can
// resume, an output waiting for the server's check or Library's swap, or a Whisper subtitle that
// Subtitles hasn't moved yet.
var stagedNeededStates = append(slices.Clone(openStates), api.StateCompleted)

// SweepStaging removes the staged files no job needs any more. The cleanup after a job ends can be
// skipped when the server stops, so this runs at startup, before the scheduler offers anything. It only
// looks in staging dirs jobs were given and only touches files named like a job's output.
func (s *JobService) SweepStaging(ctx context.Context) error {
	files, err := s.deps.Jobs.stagedFiles(ctx)
	if err != nil {
		return err
	}
	needed := map[string]bool{}
	dirs := map[string]bool{}
	for _, file := range files {
		needed[file.Path] = needed[file.Path] || file.Needed
		dirs[filepath.Dir(file.Path)] = true
	}
	for dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		sweepStagingDir(ctx, dir, needed)
	}
	return nil
}

func sweepStagingDir(ctx context.Context, dir string, needed map[string]bool) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "transcode: reading a staging dir", "dir", dir, "error", err)
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if !entry.Type().IsRegular() || !isStagedName(entry.Name()) || needed[path] {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.WarnContext(ctx, "transcode: removing an orphaned staged file", "path", path, "error", err)
			continue
		}
		slog.InfoContext(ctx, "transcode: removed an orphaned staged file", "path", path)
	}
}

// isStagedName reports whether name is how a job names its staged output.
func isStagedName(name string) bool {
	for _, ext := range stagedExtensions {
		if stem, ok := strings.CutSuffix(name, ext); ok && domain.ValidID(domain.JobID(stem)) {
			return true
		}
	}
	return false
}
