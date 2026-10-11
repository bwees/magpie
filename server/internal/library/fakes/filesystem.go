// Package fakes holds in-memory fakes of Library's ports, for tests and for booting without real adapters.
package fakes

import (
	"context"
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const (
	FileSystemStat       scripted.Method = "FileSystem.Stat"
	FileSystemSameDevice scripted.Method = "FileSystem.SameDevice"
	FileSystemHardlink   scripted.Method = "FileSystem.Hardlink"
	FileSystemCopy       scripted.Method = "FileSystem.Copy"
	FileSystemMove       scripted.Method = "FileSystem.Move"
	FileSystemRename     scripted.Method = "FileSystem.Rename"
	FileSystemRemove     scripted.Method = "FileSystem.Remove"
	FileSystemMkdirAll   scripted.Method = "FileSystem.MkdirAll"
	FileSystemWalk       scripted.Method = "FileSystem.Walk"

	// RootDevice is the device of every path outside a Mount.
	RootDevice uint64 = 1

	rootPath = "/"
	dirMode  = fs.ModeDir | 0o755
	fileMode = fs.FileMode(0o644)

	opStat     = "stat"
	opLink     = "link"
	opCopy     = "copy"
	opRename   = "rename"
	opRemove   = "remove"
	opMkdir    = "mkdir"
	opWalk     = "walk"
	opReadFile = "read"
)

var errDirectoryNotEmpty = errors.New("directory not empty")

type inode struct {
	device  uint64
	mode    fs.FileMode
	content []byte
	modTime time.Time
}

// FileSystem is an in-memory tree with devices and inodes. Paths are absolute and slash-separated.
// Hardlinks share an inode, Rename works only within a device, and Move across devices copies then deletes.
// Like the OS, creating a path needs its parent directory to exist.
type FileSystem struct {
	scripted.Script

	mu      sync.RWMutex
	now     func() time.Time
	mounts  map[string]uint64
	entries map[string]uint64
	inodes  map[uint64]*inode
}

var _ library.FileSystem = (*FileSystem)(nil)

// NewFileSystem starts with only "/" on RootDevice.
func NewFileSystem() *FileSystem {
	f := &FileSystem{
		now:     time.Now,
		mounts:  map[string]uint64{rootPath: RootDevice},
		entries: map[string]uint64{},
		inodes:  map[uint64]*inode{},
	}
	f.entries[rootPath] = f.newInode(RootDevice, dirMode, nil)
	return f
}

// Mount puts every path under root on device and creates root.
func (f *FileSystem) Mount(root string, device uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	root = filepath.Clean(root)
	f.mounts[root] = device
	f.mkdirAll(root)
}

// WriteFile creates or replaces a file's content with a fresh inode, as a download or upgrade would.
func (f *FileSystem) WriteFile(path string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	if err := f.parentExists(opCopy, path); err != nil {
		return err
	}
	f.entries[path] = f.newInode(f.deviceOf(path), fileMode, slices.Clone(content))
	return nil
}

func (f *FileSystem) ReadFile(path string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	node, err := f.node(opReadFile, filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	return slices.Clone(node.content), nil
}

// Exists reports whether path is present.
func (f *FileSystem) Exists(path string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.entries[filepath.Clean(path)]
	return ok
}

func (f *FileSystem) Stat(path string) (library.FileInfo, error) {
	if err := f.Enter(context.Background(), FileSystemStat); err != nil {
		return library.FileInfo{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.stat(opStat, filepath.Clean(path))
}

func (f *FileSystem) SameDevice(a, b string) (bool, error) {
	if err := f.Enter(context.Background(), FileSystemSameDevice); err != nil {
		return false, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.deviceOf(filepath.Clean(a)) == f.deviceOf(filepath.Clean(b)), nil
}

func (f *FileSystem) Hardlink(src, dst string) error {
	if err := f.Enter(context.Background(), FileSystemHardlink); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	src, dst = filepath.Clean(src), filepath.Clean(dst)
	id, err := f.inodeOf(opLink, src)
	if err != nil {
		return err
	}
	if err := f.creatable(opLink, dst); err != nil {
		return err
	}
	if f.inodes[id].device != f.deviceOf(dst) {
		return &os.LinkError{Op: opLink, Old: src, New: dst, Err: library.ErrCrossDevice}
	}
	f.entries[dst] = id
	return nil
}

func (f *FileSystem) Copy(ctx context.Context, src, dst string) error {
	if err := f.Enter(ctx, FileSystemCopy); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.copyFile(filepath.Clean(src), filepath.Clean(dst))
}

func (f *FileSystem) Move(src, dst string) error {
	if err := f.Enter(context.Background(), FileSystemMove); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	src, dst = filepath.Clean(src), filepath.Clean(dst)
	if f.deviceOf(src) == f.deviceOf(dst) {
		return f.rename(src, dst)
	}
	if err := f.copyFile(src, dst); err != nil {
		return err
	}
	delete(f.entries, src)
	return nil
}

func (f *FileSystem) Rename(src, dst string) error {
	if err := f.Enter(context.Background(), FileSystemRename); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rename(filepath.Clean(src), filepath.Clean(dst))
}

func (f *FileSystem) Remove(path string) error {
	if err := f.Enter(context.Background(), FileSystemRemove); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	node, err := f.node(opRemove, path)
	if err != nil {
		return err
	}
	if node.mode.IsDir() && len(f.under(path)) > 0 {
		return &fs.PathError{Op: opRemove, Path: path, Err: errDirectoryNotEmpty}
	}
	delete(f.entries, path)
	return nil
}

func (f *FileSystem) MkdirAll(path string) error {
	if err := f.Enter(context.Background(), FileSystemMkdirAll); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path = filepath.Clean(path)
	if node, ok := f.lookup(path); ok && !node.mode.IsDir() {
		return &fs.PathError{Op: opMkdir, Path: path, Err: fs.ErrExist}
	}
	f.mkdirAll(path)
	return nil
}

func (f *FileSystem) Walk(root string, fn func(path string, info library.FileInfo) error) error {
	if err := f.Enter(context.Background(), FileSystemWalk); err != nil {
		return err
	}
	root = filepath.Clean(root)
	f.mu.RLock()
	if _, ok := f.entries[root]; !ok {
		f.mu.RUnlock()
		return &fs.PathError{Op: opWalk, Path: root, Err: fs.ErrNotExist}
	}
	paths := append([]string{root}, f.under(root)...)
	infos := make([]library.FileInfo, 0, len(paths))
	for _, p := range paths {
		info, _ := f.stat(opWalk, p)
		infos = append(infos, info)
	}
	f.mu.RUnlock()

	var skipped []string
	for _, info := range infos {
		if slices.ContainsFunc(skipped, func(dir string) bool { return isUnder(dir, info.Path) }) {
			continue
		}
		err := fn(info.Path, info)
		switch {
		case errors.Is(err, fs.SkipAll):
			return nil
		case errors.Is(err, fs.SkipDir) && info.IsDir():
			skipped = append(skipped, info.Path)
		case errors.Is(err, fs.SkipDir):
			return nil
		case err != nil:
			return err
		}
	}
	return nil
}

func (f *FileSystem) rename(src, dst string) error {
	if _, err := f.node(opRename, src); err != nil {
		return err
	}
	if err := f.parentExists(opRename, dst); err != nil {
		return err
	}
	if f.deviceOf(src) != f.deviceOf(dst) {
		return &os.LinkError{Op: opRename, Old: src, New: dst, Err: library.ErrCrossDevice}
	}
	moved := append([]string{src}, f.under(src)...)
	for _, p := range moved {
		f.entries[dst+strings.TrimPrefix(p, src)] = f.entries[p]
		delete(f.entries, p)
	}
	return nil
}

func (f *FileSystem) copyFile(src, dst string) error {
	node, err := f.node(opCopy, src)
	if err != nil {
		return err
	}
	if err := f.parentExists(opCopy, dst); err != nil {
		return err
	}
	f.entries[dst] = f.newInode(f.deviceOf(dst), node.mode, slices.Clone(node.content))
	return nil
}

func (f *FileSystem) mkdirAll(path string) {
	for dir := path; ; dir = filepath.Dir(dir) {
		if _, ok := f.entries[dir]; !ok {
			f.entries[dir] = f.newInode(f.deviceOf(dir), dirMode, nil)
		}
		if dir == rootPath {
			return
		}
	}
}

func (f *FileSystem) newInode(device uint64, mode fs.FileMode, content []byte) uint64 {
	id := rand.Uint64()
	for _, taken := f.inodes[id]; taken; _, taken = f.inodes[id] {
		id = rand.Uint64()
	}
	f.inodes[id] = &inode{device: device, mode: mode, content: content, modTime: f.now()}
	return id
}

func (f *FileSystem) deviceOf(path string) uint64 {
	for dir := path; ; dir = filepath.Dir(dir) {
		if device, ok := f.mounts[dir]; ok {
			return device
		}
		if dir == rootPath {
			return RootDevice
		}
	}
}

func (f *FileSystem) lookup(path string) (*inode, bool) {
	id, ok := f.entries[path]
	if !ok {
		return nil, false
	}
	return f.inodes[id], true
}

func (f *FileSystem) inodeOf(op, path string) (uint64, error) {
	id, ok := f.entries[path]
	if !ok {
		return 0, &fs.PathError{Op: op, Path: path, Err: fs.ErrNotExist}
	}
	return id, nil
}

func (f *FileSystem) node(op, path string) (*inode, error) {
	id, err := f.inodeOf(op, path)
	if err != nil {
		return nil, err
	}
	return f.inodes[id], nil
}

func (f *FileSystem) stat(op, path string) (library.FileInfo, error) {
	id, err := f.inodeOf(op, path)
	if err != nil {
		return library.FileInfo{}, err
	}
	node := f.inodes[id]
	return library.FileInfo{
		Path:    path,
		Size:    int64(len(node.content)),
		Mode:    node.mode,
		ModTime: node.modTime,
		Device:  node.device,
		Inode:   id,
	}, nil
}

func (f *FileSystem) parentExists(op, path string) error {
	parent, ok := f.lookup(filepath.Dir(path))
	if !ok || !parent.mode.IsDir() {
		return &fs.PathError{Op: op, Path: path, Err: fs.ErrNotExist}
	}
	return nil
}

func (f *FileSystem) creatable(op, path string) error {
	if _, ok := f.entries[path]; ok {
		return &fs.PathError{Op: op, Path: path, Err: fs.ErrExist}
	}
	return f.parentExists(op, path)
}

// under lists every path below dir in walk order: lexical within each directory, parents first.
func (f *FileSystem) under(dir string) []string {
	paths := []string{}
	for p := range f.entries {
		if isUnder(dir, p) {
			paths = append(paths, p)
		}
	}
	slices.SortFunc(paths, func(a, b string) int {
		return slices.Compare(strings.Split(a, string(filepath.Separator)), strings.Split(b, string(filepath.Separator)))
	})
	return paths
}

func isUnder(dir, path string) bool {
	if dir == rootPath {
		return path != rootPath
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}
