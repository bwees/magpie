// Package transfer moves a job's bytes between the agent and the server: a resumable source download
// with Range and If-Range, and a resumable offset-based upload of the output. Both wait out the server's
// transfer cap and retry network errors until their context ends.
package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"magpie/internal/transcode/api/agentproto"
)

var (
	// ErrSourceChanged means the server's source no longer has the ETag the job was leased with.
	ErrSourceChanged = errors.New("transfer: " + agentproto.ErrorSourceChanged)
	// ErrNotLeased means the server no longer leases the job to this agent.
	ErrNotLeased = errors.New("transfer: the job isn't leased to this agent")
)

const (
	// DefaultChunkSize is how much one upload PUT carries.
	DefaultChunkSize = 64 << 20
	// DefaultRetryWait is the wait after a network error, and after a 503 without Retry-After.
	DefaultRetryWait = 5 * time.Second
	// maxDigestRestarts bounds how often a whole upload starts over after a digest mismatch.
	maxDigestRestarts = 2

	filePerm   = 0o644
	copyBuffer = 1 << 20
)

// Progress reports bytes done out of the total.
type Progress func(done, total int64)

// Client talks to the server's job byte endpoints. HTTP must add the agent's auth headers.
type Client struct {
	HTTP      *http.Client
	ServerURL *url.URL
	ChunkSize int64
	RetryWait time.Duration
}

func New(httpClient *http.Client, serverURL *url.URL) *Client {
	return &Client{HTTP: httpClient, ServerURL: serverURL, ChunkSize: DefaultChunkSize, RetryWait: DefaultRetryWait}
}

func (c *Client) url(path string) string {
	return c.ServerURL.JoinPath(path).String()
}

// statusError is an answer the client can't act on.
type statusError struct {
	status int
	body   string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("transfer: server answered %d: %s", e.status, e.body)
}

func unexpected(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	return &statusError{status: resp.StatusCode, body: string(bytes.TrimSpace(body))}
}

// wait sleeps for the server's Retry-After, or the default wait, unless ctx ends first.
func (c *Client) wait(ctx context.Context, resp *http.Response) error {
	wait := c.RetryWait
	if resp != nil {
		if seconds, err := strconv.Atoi(resp.Header.Get(agentproto.HeaderRetryAfter)); err == nil && seconds > 0 {
			wait = time.Duration(seconds) * time.Second
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryable is a failure worth trying again: the network, or the server being busy or restarting.
func retryable(err error) bool {
	var status *statusError
	if errors.As(err, &status) {
		return status.status >= http.StatusInternalServerError
	}
	return !errors.Is(err, ErrSourceChanged) && !errors.Is(err, ErrNotLeased) && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded)
}

// Download fetches the source into dest, resuming after whatever dest already holds. It fails with
// ErrSourceChanged if the server's file no longer has etag.
func (c *Client) Download(ctx context.Context, source agentproto.SourceRef, dest string, progress Progress) error {
	for {
		resp, err := c.fetch(ctx, source, dest, progress)
		if err == nil || !retryable(err) {
			return err
		}
		if waitErr := c.wait(ctx, resp); waitErr != nil {
			return waitErr
		}
	}
}

func (c *Client) fetch(ctx context.Context, source agentproto.SourceRef, dest string, progress Progress) (*http.Response, error) {
	have, err := sizeOf(dest)
	if err != nil {
		return nil, err
	}
	if have == source.Size {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(source.URL), nil)
	if err != nil {
		return nil, err
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
		req.Header.Set("If-Range", source.ETag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	flags := os.O_WRONLY | os.O_CREATE
	switch {
	case resp.StatusCode == http.StatusServiceUnavailable:
		return resp, unexpected(resp)
	case resp.StatusCode == http.StatusConflict:
		return resp, ErrNotLeased
	case resp.Header.Get("ETag") != source.ETag:
		return resp, ErrSourceChanged
	case resp.StatusCode == http.StatusPartialContent:
		flags |= os.O_APPEND
	case resp.StatusCode == http.StatusOK:
		flags |= os.O_TRUNC
		have = 0
	default:
		return resp, unexpected(resp)
	}

	file, err := os.OpenFile(dest, flags, filePerm)
	if err != nil {
		return resp, err
	}
	defer func() { _ = file.Close() }()
	_, err = io.CopyBuffer(file, &progressReader{r: resp.Body, done: have, total: source.Size, report: progress}, make([]byte, copyBuffer))
	if syncErr := file.Sync(); err == nil {
		err = syncErr
	}
	if err != nil {
		return resp, err
	}
	if got, err := sizeOf(dest); err != nil || got != source.Size {
		return resp, errors.Join(err, fmt.Errorf("transfer: downloaded %d of %d bytes", got, source.Size))
	}
	return resp, nil
}

// Reserve takes the server's transfer slot for a direct-read job, which keeps it until the job moves on
// to uploading, so ffmpeg's own requests aren't refused mid-encode.
func (c *Client) Reserve(ctx context.Context, source agentproto.SourceRef) error {
	for {
		resp, err := c.peek(ctx, source)
		if err == nil || !retryable(err) {
			return err
		}
		if waitErr := c.wait(ctx, resp); waitErr != nil {
			return waitErr
		}
	}
}

func (c *Client) peek(ctx context.Context, source agentproto.SourceRef) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(source.URL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == http.StatusConflict:
		return resp, ErrNotLeased
	case resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK:
		return resp, unexpected(resp)
	case resp.Header.Get("ETag") != source.ETag:
		return resp, ErrSourceChanged
	}
	return resp, nil
}

// SourceURL is the absolute URL of a server path, for ffmpeg to read in direct-read mode.
func (c *Client) SourceURL(path string) string {
	return c.url(path)
}

func sizeOf(path string) (int64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

type progressReader struct {
	r           io.Reader
	done, total int64
	report      Progress
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.report != nil && n > 0 {
		p.report(p.done, p.total)
	}
	return n, err
}

// Upload sends the file at path to the job's output URL, resuming at the server's committed offset,
// and returns its SHA-256 in hex. A digest mismatch starts the upload over from offset 0.
func (c *Client) Upload(ctx context.Context, outputURL, path string, progress Progress) (string, error) {
	digest, size, err := digestOf(path)
	if err != nil {
		return "", err
	}
	restarts := maxDigestRestarts
	for {
		err := c.upload(ctx, outputURL, path, size, digest, progress)
		var status *statusError
		switch {
		case err == nil:
			return hex.EncodeToString(digest), nil
		case errors.As(err, &status) && status.status == http.StatusUnprocessableEntity && restarts > 0:
			restarts--
		case !retryable(err):
			return "", err
		default:
			if err := c.wait(ctx, nil); err != nil {
				return "", err
			}
		}
	}
}

func (c *Client) upload(ctx context.Context, outputURL, path string, size int64, digest []byte, progress Progress) error {
	offset, err := c.committed(ctx, outputURL)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	for {
		end := min(offset+c.ChunkSize, size)
		resp, err := c.put(ctx, outputURL, file, offset, end, size, digest)
		if err != nil {
			return err
		}
		next, err := c.answer(ctx, resp, offset, end)
		if err != nil {
			return err
		}
		if progress != nil {
			progress(next, size)
		}
		if next == size && end == size {
			return nil
		}
		offset = next
	}
}

// answer reads a PUT's response and returns where the next chunk starts.
func (c *Client) answer(ctx context.Context, resp *http.Response, offset, end int64) (int64, error) {
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return end, nil
	case http.StatusConflict:
		committed, err := strconv.ParseInt(resp.Header.Get(agentproto.HeaderUploadOffset), 10, 64)
		if err != nil {
			return 0, ErrNotLeased
		}
		return committed, nil
	case http.StatusServiceUnavailable:
		return offset, c.wait(ctx, resp)
	default:
		return 0, unexpected(resp)
	}
}

func (c *Client) put(ctx context.Context, outputURL string, file *os.File, offset, end, size int64, digest []byte) (*http.Response, error) {
	body := io.NewSectionReader(file, offset, end-offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.url(outputURL), body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = end - offset
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(agentproto.HeaderUploadOffset, strconv.FormatInt(offset, 10))
	req.Header.Set(agentproto.HeaderUploadLength, strconv.FormatInt(size, 10))
	if end == size {
		req.Header.Set(agentproto.HeaderUploadComplete, agentproto.UploadCompleteTrue)
		req.Header.Set(agentproto.HeaderContentDigest, agentproto.DigestSHA256Prefix+base64.StdEncoding.EncodeToString(digest)+agentproto.DigestSuffix)
	}
	return c.HTTP.Do(req)
}

// committed asks the server how much of the output it already has.
func (c *Client) committed(ctx context.Context, outputURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.url(outputURL), nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusConflict:
		return 0, ErrNotLeased
	case http.StatusOK, http.StatusNoContent:
		return strconv.ParseInt(resp.Header.Get(agentproto.HeaderUploadOffset), 10, 64)
	default:
		return 0, unexpected(resp)
	}
}

// Digest returns a file's SHA-256 in hex.
func Digest(path string) (string, error) {
	sum, _, err := digestOf(path)
	return hex.EncodeToString(sum), err
}

func digestOf(path string) ([]byte, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	n, err := io.CopyBuffer(hash, file, make([]byte, copyBuffer))
	return hash.Sum(nil), n, err
}
