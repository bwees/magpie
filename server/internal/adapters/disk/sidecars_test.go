package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestSidecarWriteReplacesAndRemoveForgivesMissingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Film (2021).en.srt")
	files := NewSidecarFiles()

	for _, content := range []string{"first", "second"} {
		if err := files.Write(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("read %q, %v; want %q", got, err, content)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("folder holds %v, %v; want only the sidecar", entries, err)
	}

	if err := files.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := files.Remove(path); err != nil {
		t.Errorf("removing a missing sidecar = %v, want nil", err)
	}
}

func TestSidecarMoveReplacesTheTargetAndReportsAMissingSource(t *testing.T) {
	dir := t.TempDir()
	staged, path := filepath.Join(dir, "job.srt"), filepath.Join(dir, "Film (2021).en.srt")
	files := NewSidecarFiles()
	for name, content := range map[string]string{staged: "whisper", path: "old"} {
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := files.Move(staged, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "whisper" {
		t.Fatalf("read %q, %v; want the moved subtitle", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != sidecarPerm {
		t.Fatalf("mode = %v, %v; want %v", info.Mode(), err, os.FileMode(sidecarPerm))
	}
	if err := files.Move(staged, path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("moving a missing file = %v, want fs.ErrNotExist", err)
	}
}
