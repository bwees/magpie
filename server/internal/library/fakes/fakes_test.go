package fakes

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"testing"

	"magpie/internal/library"
	"magpie/internal/library/api"
)

const (
	downloadsDevice uint64 = 2
	mediaDevice     uint64 = 3
)

func newMountedFS(t *testing.T) *FileSystem {
	t.Helper()
	f := NewFileSystem()
	f.Mount("/downloads", downloadsDevice)
	f.Mount("/media", mediaDevice)
	if err := f.WriteFile("/downloads/movie.mkv", []byte("video")); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestHardlinkSharesTheInodeOnOneDevice(t *testing.T) {
	f := newMountedFS(t)
	if err := f.MkdirAll("/downloads/linked"); err != nil {
		t.Fatal(err)
	}
	if err := f.Hardlink("/downloads/movie.mkv", "/downloads/linked/movie.mkv"); err != nil {
		t.Fatal(err)
	}
	src, _ := f.Stat("/downloads/movie.mkv")
	dst, _ := f.Stat("/downloads/linked/movie.mkv")
	if src.Inode != dst.Inode || src.Device != downloadsDevice {
		t.Errorf("src %+v, dst %+v: want one inode on the downloads device", src, dst)
	}

	if err := f.Hardlink("/downloads/movie.mkv", "/media/movie.mkv"); !errors.Is(err, library.ErrCrossDevice) {
		t.Errorf("cross-device hardlink err = %v, want ErrCrossDevice", err)
	}
}

func TestRenameIsSameDeviceOnlyAndMoveCopiesAcross(t *testing.T) {
	f := newMountedFS(t)
	if err := f.Rename("/downloads/movie.mkv", "/media/movie.mkv"); !errors.Is(err, library.ErrCrossDevice) {
		t.Fatalf("cross-device rename err = %v", err)
	}
	if err := f.Move("/downloads/movie.mkv", "/media/movie.mkv"); err != nil {
		t.Fatal(err)
	}
	if f.Exists("/downloads/movie.mkv") {
		t.Error("source still exists after a move")
	}
	got, err := f.ReadFile("/media/movie.mkv")
	if err != nil || string(got) != "video" {
		t.Errorf("moved content = %q, %v", got, err)
	}
	if info, _ := f.Stat("/media/movie.mkv"); info.Device != mediaDevice {
		t.Errorf("moved file on device %d", info.Device)
	}
}

func TestCreatingNeedsAParentAndMissingPathsAreNotExist(t *testing.T) {
	f := newMountedFS(t)
	if err := f.Copy(context.Background(), "/downloads/movie.mkv", "/media/missing/movie.mkv"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("copy without parent err = %v", err)
	}
	if _, err := f.Stat("/nowhere"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat err = %v", err)
	}
}

func TestWalkIsLexicalPerDirectoryAndHonoursSkipDir(t *testing.T) {
	f := NewFileSystem()
	for _, dir := range []string{"/r/a", "/r/b"} {
		if err := f.MkdirAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"/r/a/z.mkv", "/r/a.txt", "/r/b/x.mkv"} {
		if err := f.WriteFile(file, nil); err != nil {
			t.Fatal(err)
		}
	}

	var visited []string
	err := f.Walk("/r", func(path string, info library.FileInfo) error {
		visited = append(visited, path)
		if path == "/r/b" {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/r", "/r/a", "/r/a/z.mkv", "/r/a.txt", "/r/b"}
	if !slices.Equal(visited, want) {
		t.Errorf("visited %v, want %v", visited, want)
	}
}

func TestHasherFollowsContent(t *testing.T) {
	f := newMountedFS(t)
	h := NewHasher(f)
	before, err := h.SHA256("/downloads/movie.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.WriteFile("/downloads/movie.mkv", []byte("transcoded")); err != nil {
		t.Fatal(err)
	}
	if after, _ := h.SHA256("/downloads/movie.mkv"); after == before {
		t.Error("SHA256 did not change with the content")
	}
}

func TestProberReturnsSetInfoAndFailsOtherwise(t *testing.T) {
	p := NewMediaProber()
	p.Set("/media/movie.mkv", api.MediaInfo{Container: "matroska"})
	if info, err := p.Probe(context.Background(), "/media/movie.mkv"); err != nil || info.Container != "matroska" {
		t.Errorf("Probe = %+v, %v", info, err)
	}
	if _, err := p.Probe(context.Background(), "/media/other.mkv"); !errors.Is(err, library.ErrProbeFailed) {
		t.Errorf("unknown path err = %v", err)
	}
}
