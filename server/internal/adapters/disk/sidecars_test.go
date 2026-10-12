package disk

import (
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
