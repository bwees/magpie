package fakes

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"magpie/internal/transcode"
)

const (
	testPath  = "/media/Film.mkv"
	testSize  = 4 << 30
	testInode = 1234
)

func TestFileStatReturnsSetInfos(t *testing.T) {
	f := NewFileStat()
	want := transcode.FileInfo{Size: testSize, ModTime: time.Now(), Inode: testInode}
	f.Set(testPath, want)

	if got, err := f.Stat(testPath); err != nil || got != want {
		t.Fatalf("Stat = %+v, %v", got, err)
	}
	f.Remove(testPath)
	if _, err := f.Stat(testPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestFileStatScriptedError(t *testing.T) {
	f := NewFileStat()
	boom := errors.New("io error")
	f.FailNext(FileStatStat, boom)

	if _, err := f.Stat(testPath); !errors.Is(err, boom) {
		t.Errorf("err = %v, want scripted error", err)
	}
}
