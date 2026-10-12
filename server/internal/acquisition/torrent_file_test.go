package acquisition_test

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"

	"magpie/internal/acquisition"
)

func TestParseTorrentListsMultiFileTorrentsUnderTheirRoot(t *testing.T) {
	data := torrentFile("Movie.Title.2024", "Movie.mkv", "Extras/setup.exe")

	meta, err := acquisition.ParseTorrent(data)
	if err != nil {
		t.Fatal(err)
	}
	info := data[len("d4:info") : len(data)-1]
	sum := sha1.Sum(info)
	if meta.InfoHash != hex.EncodeToString(sum[:]) {
		t.Errorf("info hash = %s, want the SHA-1 of the info dict", meta.InfoHash)
	}
	if meta.Name != "Movie.Title.2024" || !slices.Equal(meta.Files, []string{"Movie.mkv", "Extras/setup.exe"}) {
		t.Errorf("meta = %+v", meta)
	}
}

func TestParseTorrentReadsSingleFileAndV2Torrents(t *testing.T) {
	single := []byte("d8:announce3:url4:infod6:lengthi5e4:name9:Movie.mkv12:piece lengthi16384e6:pieces20:" + strings.Repeat("x", pieceBytes) + "ee")
	meta, err := acquisition.ParseTorrent(single)
	if err != nil || !slices.Equal(meta.Files, []string{"Movie.mkv"}) {
		t.Errorf("single-file = %+v, %v", meta, err)
	}

	v2Info := "d9:file treed6:Extrasd9:setup.exed0:d6:lengthi1eeee9:Movie.mkvd0:d6:lengthi5eeee4:name5:Movie12:piece lengthi16384ee"
	v2, err := acquisition.ParseTorrent([]byte("d4:info" + v2Info + "e"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(v2Info))
	if v2.InfoHash != hex.EncodeToString(sum[:sha1.Size]) || !slices.Equal(v2.Files, []string{"Extras/setup.exe", "Movie.mkv"}) {
		t.Errorf("v2 = %+v", v2)
	}
}

func TestParseTorrentRejectsGarbage(t *testing.T) {
	for name, data := range map[string]string{
		"not bencode":       "<html>",
		"no info":           "d8:announce3:urle",
		"truncated":         "d4:infod5:filesl",
		"string too long":   "d4:info99:x",
		"info not a dict":   "d4:infoi1ee",
		"file without path": "d4:infod5:filesld6:lengthi1eeeee",
	} {
		if _, err := acquisition.ParseTorrent([]byte(data)); !errors.Is(err, acquisition.ErrInvalidTorrent) {
			t.Errorf("%s = %v, want ErrInvalidTorrent", name, err)
		}
	}
}

func TestMagnetInfoHash(t *testing.T) {
	const hexHash = "c12fe1c06bba254a9dc9f519b335aa7c1367a88a"
	for name, tc := range map[string]struct{ uri, want string }{
		"hex":    {"magnet:?xt=urn:btih:C12FE1C06BBA254A9DC9F519B335AA7C1367A88A&dn=Movie", hexHash},
		"base32": {"magnet:?xt=urn:btih:YEX6DQDLXISUVHOJ6UM3GNNKPQJWPKEK", hexHash},
		"v2":     {"magnet:?xt=urn:btmh:1220" + hexHash + "0123456789abcdef01234567", hexHash},
	} {
		if got, err := acquisition.MagnetInfoHash(tc.uri); err != nil || got != tc.want {
			t.Errorf("%s = %s, %v; want %s", name, got, err, tc.want)
		}
	}
	if _, err := acquisition.MagnetInfoHash("https://example.com"); !errors.Is(err, acquisition.ErrInvalidTorrent) {
		t.Errorf("not a magnet = %v", err)
	}
}
