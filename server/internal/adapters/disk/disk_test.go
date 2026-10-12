package disk

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"magpie/internal/library"
)

func write(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHardlinkSharesTheInode(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "dst.mkv")
	write(t, src, []byte("video"))
	files := NewFileSystem()

	if err := files.Hardlink(src, dst); err != nil {
		t.Fatal(err)
	}
	a, _ := files.Stat(src)
	b, _ := files.Stat(dst)
	if a.Inode == 0 || a.Inode != b.Inode || a.Device != b.Device {
		t.Errorf("src %+v, dst %+v: want one inode", a, b)
	}
	if same, err := files.SameDevice(src, dir); err != nil || !same {
		t.Errorf("SameDevice = %v, %v", same, err)
	}
}

func TestCopyKeepsTheSourceAndRefusesAnExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "dst.mkv")
	content := make([]byte, copyChunk+1)
	content[copyChunk] = 7
	write(t, src, content)
	files := NewFileSystem()

	if err := files.Copy(context.Background(), src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || !slices.Equal(got, content) {
		t.Errorf("copied %d bytes, %v; want the same content", len(got), err)
	}
	if err := files.Copy(context.Background(), src, dst); !errors.Is(err, fs.ErrExist) {
		t.Errorf("copy over an existing file err = %v, want ErrExist", err)
	}
}

func TestACancelledCopyLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.mkv"), filepath.Join(dir, "dst.mkv")
	write(t, src, []byte("video"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewFileSystem().Copy(ctx, src, dst); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want Canceled", err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dst left behind: %v", err)
	}
}

func TestMoveAndRenameOnOneDevice(t *testing.T) {
	dir := t.TempDir()
	src, moved, renamed := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	write(t, src, []byte("video"))
	files := NewFileSystem()

	if err := files.Move(src, moved); err != nil {
		t.Fatal(err)
	}
	if err := files.Rename(moved, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the source is still there")
	}
	if _, err := files.Stat(renamed); err != nil {
		t.Error(err)
	}
}

func TestWalkListsFilesInOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b/2.mkv", "a.srt", "b/1.mkv"} {
		write(t, filepath.Join(dir, name), []byte(name))
	}
	var files []string
	err := NewFileSystem().Walk(dir, func(path string, info library.FileInfo) error {
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil || !slices.Equal(files, []string{"a.srt", "b/1.mkv", "b/2.mkv"}) {
		t.Errorf("walked %v, %v", files, err)
	}
}

func TestOSHashAddsTheSizeAndBothEnds(t *testing.T) {
	const size = 200000
	content := make([]byte, size)
	content[0] = 1
	content[size-osHashChunk] = 2
	path := filepath.Join(t.TempDir(), "movie.mkv")
	write(t, path, content)

	got, err := NewHasher().OSHash(path)
	if want := "0000000000030d43"; err != nil || got != want {
		t.Errorf("OSHash = %q, %v; want %q (size 200000 + 1 + 2)", got, err, want)
	}
}

func TestSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x")
	write(t, path, []byte("abc"))
	got, err := NewHasher().SHA256(path)
	if want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"; err != nil || got != want {
		t.Errorf("SHA256 = %q, %v", got, err)
	}
}
