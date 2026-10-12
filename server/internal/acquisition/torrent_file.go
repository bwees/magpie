package acquisition

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

var ErrInvalidTorrent = errors.New("acquisition: not a valid .torrent")

// TorrentMetainfo is what a .torrent's info dict says about its content.
type TorrentMetainfo struct {
	// InfoHash is qBittorrent's ID for the torrent: the v1 hash, or the truncated v2 hash of a v2-only torrent.
	InfoHash string
	Name     string
	// Files are paths relative to the content root, with "/" separators.
	Files []string
}

const (
	infoHashBytes   = sha1.Size
	maxBencodeDepth = 64
	keyInfo         = "info"
	keyName         = "name"
	keyFiles        = "files"
	keyPath         = "path"
	keyLength       = "length"
	keyPieces       = "pieces"
	keyFileTree     = "file tree"
	fileTreeLeaf    = ""
)

// ParseTorrent reads a bencoded .torrent and lists its files without contacting anyone.
func ParseTorrent(data []byte) (TorrentMetainfo, error) {
	d := &bdecoder{data: data}
	start, end, err := d.infoSpan()
	if err != nil {
		return TorrentMetainfo{}, fmt.Errorf("%w: %w", ErrInvalidTorrent, err)
	}
	raw := data[start:end]
	value, err := (&bdecoder{data: raw}).value(0)
	if err != nil {
		return TorrentMetainfo{}, fmt.Errorf("%w: %w", ErrInvalidTorrent, err)
	}
	info, ok := value.(map[string]any)
	if !ok {
		return TorrentMetainfo{}, fmt.Errorf("%w: the info entry isn't a dictionary", ErrInvalidTorrent)
	}
	name, _ := info[keyName].(string)
	meta := TorrentMetainfo{InfoHash: infoHash(info, raw), Name: name}
	switch {
	case info[keyFiles] != nil:
		meta.Files, err = v1Files(info[keyFiles])
	case info[keyFileTree] != nil:
		meta.Files, err = v2Files(info[keyFileTree], "")
	case name != "":
		meta.Files = []string{name}
	default:
		err = errors.New("no files")
	}
	if err != nil {
		return TorrentMetainfo{}, fmt.Errorf("%w: %w", ErrInvalidTorrent, err)
	}
	return meta, nil
}

func infoHash(info map[string]any, raw []byte) string {
	if _, v1 := info[keyPieces]; v1 {
		sum := sha1.Sum(raw)
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:infoHashBytes])
}

func v1Files(value any) ([]string, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("files isn't a list")
	}
	files := make([]string, 0, len(list))
	for _, entry := range list {
		file, ok := entry.(map[string]any)
		if !ok {
			return nil, errors.New("a file entry isn't a dictionary")
		}
		parts, ok := file[keyPath].([]any)
		if !ok || len(parts) == 0 {
			return nil, errors.New("a file has no path")
		}
		segments := make([]string, 0, len(parts))
		for _, part := range parts {
			segment, ok := part.(string)
			if !ok {
				return nil, errors.New("a path segment isn't a string")
			}
			segments = append(segments, segment)
		}
		files = append(files, path.Join(segments...))
	}
	return files, nil
}

// v2Files walks a BEP 52 file tree: nested dictionaries whose leaves sit under an empty key.
func v2Files(value any, prefix string) ([]string, error) {
	tree, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("file tree isn't a dictionary")
	}
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	files := []string{}
	for _, name := range names {
		if name == fileTreeLeaf {
			files = append(files, prefix)
			continue
		}
		nested, err := v2Files(tree[name], path.Join(prefix, name))
		if err != nil {
			return nil, err
		}
		files = append(files, nested...)
	}
	return files, nil
}

// bdecoder reads bencode: integers, byte strings, lists and dictionaries.
type bdecoder struct {
	data []byte
	pos  int
}

// infoSpan finds the raw bytes of the top-level "info" value, which the info hash is computed over.
func (d *bdecoder) infoSpan() (int, int, error) {
	if !d.consume('d') {
		return 0, 0, errors.New("not a dictionary")
	}
	for !d.consume('e') {
		key, err := d.str()
		if err != nil {
			return 0, 0, err
		}
		start := d.pos
		if _, err := d.value(1); err != nil {
			return 0, 0, err
		}
		if key == keyInfo {
			return start, d.pos, nil
		}
	}
	return 0, 0, errors.New("no info dictionary")
}

func (d *bdecoder) consume(b byte) bool {
	if d.pos < len(d.data) && d.data[d.pos] == b {
		d.pos++
		return true
	}
	return false
}

func (d *bdecoder) value(depth int) (any, error) {
	if depth > maxBencodeDepth {
		return nil, errors.New("nested too deeply")
	}
	if d.pos >= len(d.data) {
		return nil, errors.New("unexpected end")
	}
	switch d.data[d.pos] {
	case 'i':
		return d.integer()
	case 'l':
		d.pos++
		list := []any{}
		for !d.consume('e') {
			item, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		return list, nil
	case 'd':
		d.pos++
		dict := map[string]any{}
		for !d.consume('e') {
			key, err := d.str()
			if err != nil {
				return nil, err
			}
			if dict[key], err = d.value(depth + 1); err != nil {
				return nil, err
			}
		}
		return dict, nil
	}
	return d.str()
}

func (d *bdecoder) integer() (int64, error) {
	d.pos++
	end := bytes.IndexByte(d.data[d.pos:], 'e')
	if end < 0 {
		return 0, errors.New("unterminated integer")
	}
	n, err := strconv.ParseInt(string(d.data[d.pos:d.pos+end]), 10, 64)
	d.pos += end + 1
	return n, err
}

func (d *bdecoder) str() (string, error) {
	colon := bytes.IndexByte(d.data[d.pos:], ':')
	if colon < 0 {
		return "", errors.New("malformed string")
	}
	length, err := strconv.Atoi(string(d.data[d.pos : d.pos+colon]))
	if err != nil || length < 0 {
		return "", errors.New("malformed string length")
	}
	start := d.pos + colon + 1
	if start+length > len(d.data) {
		return "", errors.New("string runs past the end")
	}
	d.pos = start + length
	return string(d.data[start:d.pos]), nil
}

const (
	magnetScheme  = "magnet"
	magnetTopic   = "xt"
	btihPrefix    = "urn:btih:"
	btmhPrefix    = "urn:btmh:"
	sha256Multi   = "1220"
	base32HashLen = 32
)

// MagnetInfoHash returns the info hash a magnet names, lowercase hex, as qBittorrent reports it.
func MagnetInfoHash(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != magnetScheme {
		return "", fmt.Errorf("%w: not a magnet link", ErrInvalidTorrent)
	}
	for _, topic := range parsed.Query()[magnetTopic] {
		lower := strings.ToLower(topic)
		switch {
		case strings.HasPrefix(lower, btihPrefix):
			return btih(topic[len(btihPrefix):])
		case strings.HasPrefix(lower, btmhPrefix+sha256Multi):
			digest := lower[len(btmhPrefix+sha256Multi):]
			if len(digest) >= 2*infoHashBytes {
				return digest[:2*infoHashBytes], nil
			}
		}
	}
	return "", fmt.Errorf("%w: the magnet names no info hash", ErrInvalidTorrent)
}

func btih(hash string) (string, error) {
	if len(hash) == base32HashLen {
		decoded, err := base32.StdEncoding.DecodeString(strings.ToUpper(hash))
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrInvalidTorrent, err)
		}
		return hex.EncodeToString(decoded), nil
	}
	if _, err := hex.DecodeString(hash); err != nil || len(hash) != 2*infoHashBytes {
		return "", fmt.Errorf("%w: bad btih %q", ErrInvalidTorrent, hash)
	}
	return strings.ToLower(hash), nil
}
