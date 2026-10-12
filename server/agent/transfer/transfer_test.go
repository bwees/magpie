package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/transcode/api/agentproto"
)

const (
	testETag  = `"7-100-1700000000"`
	testRetry = time.Millisecond
	testChunk = 4
)

func newClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := New(server.Client(), serverURL)
	c.RetryWait, c.ChunkSize = testRetry, testChunk
	return c
}

// sourceServer serves content with an ETag through http.ServeContent, as the server does.
type sourceServer struct {
	mu      sync.Mutex
	content []byte
	etag    string
	busy    int
	ranges  []string
}

func (s *sourceServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.ranges = append(s.ranges, r.Header.Get("Range"))
	busy := s.busy > 0
	if busy {
		s.busy--
	}
	content, etag := s.content, s.etag
	s.mu.Unlock()
	if busy {
		w.Header().Set(agentproto.HeaderRetryAfter, "0")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("ETag", etag)
	http.ServeContent(w, r, "source.mkv", time.Time{}, bytes.NewReader(content))
}

func source(content []byte) agentproto.SourceRef {
	return agentproto.SourceRef{URL: "/source", Size: int64(len(content)), ETag: testETag}
}

// Seed 13, the agent's part: a download resumes after the bytes the scratch dir kept.
func TestDownloadResumesAtTheOffsetItHas(t *testing.T) {
	content := []byte("0123456789abcdefghij")
	server := &sourceServer{content: content, etag: testETag, busy: 1}
	c := newClient(t, server)
	dest := filepath.Join(t.TempDir(), "source.mkv")
	if err := os.WriteFile(dest, content[:8], filePerm); err != nil {
		t.Fatal(err)
	}

	if err := c.Download(context.Background(), source(content), dest, nil); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != string(content) {
		t.Fatalf("downloaded %q, want %q", got, content)
	}
	if last := server.ranges[len(server.ranges)-1]; last != "bytes=8-" {
		t.Fatalf("Range = %q, want bytes=8- (the 503 is waited out, then the download resumes)", last)
	}
}

// Seed 14, the agent's part.
func TestDownloadFailsWhenTheSourceChanged(t *testing.T) {
	content := []byte("0123456789abcdefghij")
	c := newClient(t, &sourceServer{content: append(content, '!'), etag: `"8-101-1700000001"`})
	dest := filepath.Join(t.TempDir(), "source.mkv")
	if err := os.WriteFile(dest, content[:8], filePerm); err != nil {
		t.Fatal(err)
	}

	err := c.Download(context.Background(), source(content), dest, nil)

	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("err = %v, want ErrSourceChanged", err)
	}
}

func TestDownloadOfAJobNoLongerLeasedStops(t *testing.T) {
	c := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusConflict) }))

	err := c.Download(context.Background(), source([]byte("x")), filepath.Join(t.TempDir(), "s"), nil)

	if !errors.Is(err, ErrNotLeased) {
		t.Fatalf("err = %v, want ErrNotLeased", err)
	}
}

// outputServer keeps an upload the way the server does, with knobs to misbehave once.
type outputServer struct {
	mu        sync.Mutex
	committed []byte
	// corruptOnce flips a byte of the first chunk, so the digest of the whole upload mismatches.
	corruptOnce bool
	puts        []string
}

func (s *outputServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodHead {
		w.Header().Set(agentproto.HeaderUploadOffset, strconv.Itoa(len(s.committed)))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	offset, _ := strconv.Atoi(r.Header.Get(agentproto.HeaderUploadOffset))
	s.puts = append(s.puts, r.Header.Get(agentproto.HeaderUploadOffset))
	if offset != len(s.committed) {
		w.Header().Set(agentproto.HeaderUploadOffset, strconv.Itoa(len(s.committed)))
		w.WriteHeader(http.StatusConflict)
		return
	}
	body, _ := io.ReadAll(r.Body)
	if s.corruptOnce && len(body) > 0 {
		body[0] ^= 0xff
		s.corruptOnce = false
	}
	s.committed = append(s.committed, body...)
	if r.Header.Get(agentproto.HeaderUploadComplete) == agentproto.UploadCompleteTrue {
		sum := sha256.Sum256(s.committed)
		want := agentproto.DigestSHA256Prefix + base64.StdEncoding.EncodeToString(sum[:]) + agentproto.DigestSuffix
		if r.Header.Get(agentproto.HeaderContentDigest) != want {
			s.committed = nil
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeOutput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "job.mkv")
	if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUploadSendsChunksAndTheDigest(t *testing.T) {
	server := &outputServer{}
	c := newClient(t, server)
	const output = "hevc output bytes"

	sum, err := c.Upload(context.Background(), "/output", writeOutput(t, output), nil)

	if err != nil || string(server.committed) != output {
		t.Fatalf("upload = %q, %v; server has %q", sum, err, server.committed)
	}
	want, _ := Digest(writeOutput(t, output))
	if sum != want {
		t.Fatalf("sha256 = %s, want %s", sum, want)
	}
}

// Seed 17, the agent's part: an upload resumes from what the server committed.
func TestUploadResumesAtTheServersOffset(t *testing.T) {
	const output = "hevc output bytes"
	server := &outputServer{committed: []byte(output[:8])}
	c := newClient(t, server)

	if _, err := c.Upload(context.Background(), "/output", writeOutput(t, output), nil); err != nil {
		t.Fatal(err)
	}

	if string(server.committed) != output || server.puts[0] != "8" {
		t.Fatalf("server has %q after PUTs at %v, want the rest sent from 8", server.committed, server.puts)
	}
}

// Seed 16, the agent's part: after a digest mismatch the agent uploads again from offset 0.
func TestUploadStartsOverAfterADigestMismatch(t *testing.T) {
	const output = "hevc output bytes"
	server := &outputServer{corruptOnce: true}
	c := newClient(t, server)

	if _, err := c.Upload(context.Background(), "/output", writeOutput(t, output), nil); err != nil {
		t.Fatal(err)
	}

	if string(server.committed) != output || !strings.HasPrefix(strings.Join(server.puts, ","), "0,4,8,12,16,0") {
		t.Fatalf("server has %q after PUTs at %v, want a second upload from 0", server.committed, server.puts)
	}
}
