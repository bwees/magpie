package disk

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"magpie/internal/library"
)

const (
	// osHashChunk is how much of each end of the file OpenSubtitles' hash reads.
	osHashChunk = 64 << 10
	wordBytes   = 8
	osHashShape = "%016x"
)

// Hasher reads files on disk.
type Hasher struct{}

var _ library.Hasher = Hasher{}

func NewHasher() Hasher { return Hasher{} }

// OSHash is OpenSubtitles' hash: the file size plus the little-endian uint64 sum of the first and last
// 64 KiB.
func (Hasher) OSHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := fi.Size()
	sum := uint64(size)
	for _, offset := range []int64{0, max(0, size-osHashChunk)} {
		chunk := make([]byte, min(osHashChunk, size))
		if _, err := f.ReadAt(chunk, offset); err != nil && err != io.EOF {
			return "", err
		}
		sum += wordSum(chunk)
	}
	return fmt.Sprintf(osHashShape, sum), nil
}

// wordSum adds up little-endian uint64 words, the last one zero-padded.
func wordSum(b []byte) uint64 {
	var sum uint64
	for len(b) > 0 {
		var word [wordBytes]byte
		n := copy(word[:], b)
		sum += binary.LittleEndian.Uint64(word[:])
		b = b[n:]
	}
	return sum
}

func (Hasher) SHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
