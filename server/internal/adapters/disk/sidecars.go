package disk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"magpie/internal/subtitles"
)

const (
	sidecarPerm    = 0o644
	sidecarPattern = ".magpie-subtitle-*"
)

// SidecarFiles writes subtitle sidecars next to videos.
type SidecarFiles struct{}

var _ subtitles.SidecarFiles = SidecarFiles{}

func NewSidecarFiles() SidecarFiles { return SidecarFiles{} }

// Write goes through a temporary file in the same folder, so Jellyfin never reads half a subtitle.
func (SidecarFiles) Write(path string, content []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), sidecarPattern)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), sidecarPerm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Move renames within one filesystem, which a staged Whisper subtitle shares with its video.
func (SidecarFiles) Move(from, to string) error {
	if err := os.Chmod(from, sidecarPerm); err != nil {
		return err
	}
	return os.Rename(from, to)
}

func (SidecarFiles) Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
