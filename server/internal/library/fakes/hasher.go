package fakes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io/fs"
	"path/filepath"
	"sync"

	"magpie/internal/library"
	"magpie/internal/platform/scripted"
)

const (
	HasherOSHash scripted.Method = "Hasher.OSHash"
	HasherSHA256 scripted.Method = "Hasher.SHA256"

	opHash      = "hash"
	osHashShape = "%016x"
)

type hashes struct{ osHash, sha256 string }

// Hasher returns hashes set per path, else hashes of the file's content in the backing FileSystem.
// The content-derived OSHash is a stand-in that changes with the content, not the real algorithm.
type Hasher struct {
	scripted.Script

	files *FileSystem

	mu  sync.RWMutex
	set map[string]hashes
}

var _ library.Hasher = (*Hasher)(nil)

// NewHasher hashes content from files; files may be nil when every path is Set.
func NewHasher(files *FileSystem) *Hasher {
	return &Hasher{files: files, set: map[string]hashes{}}
}

func (h *Hasher) Set(path, osHash, sha256 string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.set[filepath.Clean(path)] = hashes{osHash: osHash, sha256: sha256}
}

func (h *Hasher) OSHash(path string) (string, error) {
	if err := h.Enter(context.Background(), HasherOSHash); err != nil {
		return "", err
	}
	got, err := h.hashes(path)
	return got.osHash, err
}

func (h *Hasher) SHA256(path string) (string, error) {
	if err := h.Enter(context.Background(), HasherSHA256); err != nil {
		return "", err
	}
	got, err := h.hashes(path)
	return got.sha256, err
}

func (h *Hasher) hashes(path string) (hashes, error) {
	path = filepath.Clean(path)
	h.mu.RLock()
	set, ok := h.set[path]
	h.mu.RUnlock()
	if ok {
		return set, nil
	}
	if h.files == nil {
		return hashes{}, &fs.PathError{Op: opHash, Path: path, Err: fs.ErrNotExist}
	}
	content, err := h.files.ReadFile(path)
	if err != nil {
		return hashes{}, err
	}
	sum := sha256.Sum256(content)
	short := fnv.New64a()
	_, _ = short.Write(content)
	return hashes{osHash: fmt.Sprintf(osHashShape, short.Sum64()), sha256: hex.EncodeToString(sum[:])}, nil
}
