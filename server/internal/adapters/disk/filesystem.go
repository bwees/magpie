// Package disk implements Library's FileSystem and Hasher ports on the server's own disks.
package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"magpie/internal/library"
)

const (
	dirPerm = 0o755
	// copyChunk is how much Copy writes between checks for cancellation.
	copyChunk = 4 << 20
)

// FileSystem is the operating system's filesystem.
type FileSystem struct{}

var _ library.FileSystem = FileSystem{}

func NewFileSystem() FileSystem { return FileSystem{} }

func infoOf(path string, fi fs.FileInfo) library.FileInfo {
	info := library.FileInfo{Path: path, Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		info.Device, info.Inode = uint64(st.Dev), st.Ino
	}
	return info
}

func (FileSystem) Stat(path string) (library.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return library.FileInfo{}, err
	}
	return infoOf(path, fi), nil
}

func (f FileSystem) SameDevice(a, b string) (bool, error) {
	first, err := f.Stat(a)
	if err != nil {
		return false, err
	}
	second, err := f.Stat(b)
	if err != nil {
		return false, err
	}
	return first.Device == second.Device, nil
}

// crossDevice marks an EXDEV error with library.ErrCrossDevice.
func crossDevice(err error) error {
	if errors.Is(err, syscall.EXDEV) {
		return fmt.Errorf("%w: %w", library.ErrCrossDevice, err)
	}
	return err
}

func (FileSystem) Hardlink(src, dst string) error {
	return crossDevice(os.Link(src, dst))
}

// Copy writes src's content to dst with src's permissions, flushed to disk. A failed or cancelled copy
// leaves no dst behind.
func (FileSystem) Copy(ctx context.Context, src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(dst)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := io.CopyN(out, in, copyChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

// Move renames on one device, or copies then removes the source across devices.
func (f FileSystem) Move(src, dst string) error {
	err := os.Rename(src, dst)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := f.Copy(context.Background(), src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func (FileSystem) Rename(src, dst string) error {
	return crossDevice(os.Rename(src, dst))
}

func (FileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (FileSystem) MkdirAll(path string) error {
	return os.MkdirAll(path, dirPerm)
}

func (FileSystem) Walk(root string, fn func(path string, info library.FileInfo) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		return fn(path, infoOf(path, fi))
	})
}
