package transcode

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
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/problem"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

const (
	pathJobID = "id"

	stagingDirPerm  = 0o755
	stagingFilePerm = 0o644

	TypeJobNotLeased    problem.Type = "urn:magpie:problem:job-not-leased"
	TypeTransferBusy    problem.Type = "urn:magpie:problem:transfer-cap-full"
	TypeUploadOffset    problem.Type = "urn:magpie:problem:upload-offset-mismatch"
	TypeDigestMismatch  problem.Type = "urn:magpie:problem:upload-digest-mismatch"
	TypeUploadMalformed problem.Type = "urn:magpie:problem:upload-malformed"
)

var (
	errJobNotLeased = problem.New(TypeJobNotLeased, http.StatusConflict, "the job isn't leased to this agent")
	errTransferBusy = problem.New(TypeTransferBusy, http.StatusServiceUnavailable, "the server is at its transfer cap; retry later")
)

// Transfers serves the agent byte endpoints: a job's source, and the resumable upload of its output.
// Every request must come from the agent the job is leased to.
type Transfers struct {
	service  *JobService
	gate     *TransferGate
	secret   AgentSecret
	lifetime *Lifetime
	now      func() time.Time
}

func NewTransfers(service *JobService, gate *TransferGate, secret AgentSecret, lifetime *Lifetime) *Transfers {
	return &Transfers{service: service, gate: gate, secret: secret, lifetime: lifetime, now: time.Now}
}

// leased authenticates the agent and returns the job leased to it.
func (t *Transfers) leased(r *http.Request) (domain.AgentID, jobRow, error) {
	agent, err := authenticateAgent(r, t.secret)
	if err != nil {
		return "", jobRow{}, err
	}
	row, err := t.service.LeasedJob(r.Context(), agent, domain.JobID(r.PathValue(pathJobID)))
	if errors.Is(err, ErrNotLeased) {
		return "", jobRow{}, errJobNotLeased
	}
	return agent, row, err
}

// acquire takes a transfer slot, or answers 503 with Retry-After.
func (t *Transfers) acquire(w http.ResponseWriter, r *http.Request, h TransferHolder, rank TransferRank) bool {
	capacity, err := t.service.TransferCap(r.Context())
	if err != nil {
		problem.Write(w, r, err)
		return false
	}
	if t.gate.Acquire(h, rank, capacity) {
		return true
	}
	w.Header().Set(agentproto.HeaderRetryAfter, strconv.Itoa(int(RetryAfter.Seconds())))
	problem.Write(w, r, errTransferBusy)
	return false
}

// ServeSource sends the job's source read-only, with a strong ETag so a resumed download fails instead
// of splicing two different files. A direct-read agent keeps its slot until it moves on to uploading.
func (t *Transfers) ServeSource(w http.ResponseWriter, r *http.Request) {
	agent, row, err := t.leased(r)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	source, err := t.service.SourceOf(r.Context(), row)
	if errors.Is(err, libraryapi.ErrNotFound) {
		problem.Write(w, r, errJobNotLeased)
		return
	}
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	file, err := os.Open(source.Path)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := t.service.deps.Files.Stat(source.Path)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	mode, err := t.service.TransferMode(r.Context(), agent)
	if err != nil {
		problem.Write(w, r, err)
		return
	}

	holder := TransferHolder{Job: row.ID, Direction: DirectionSource}
	rank := RankDownload
	if row.Prefetch {
		rank = RankPrefetch
	}
	if !t.acquire(w, r, holder, rank) {
		return
	}
	if mode != api.TransferDirectRead {
		defer t.gate.Release(holder)
	}

	ctx, cancel := t.lifetime.Bind(r.Context())
	defer cancel()
	w.Header().Set("ETag", info.ETag())
	counted := &countingWriter{ResponseWriter: w, ctx: ctx}
	started := t.now()
	http.ServeContent(counted, r.WithContext(ctx), filepath.Base(source.Path), info.ModTime, file)
	t.observe(ctx, agent, counted.written, started)
}

func (t *Transfers) observe(ctx context.Context, agent domain.AgentID, written int64, started time.Time) {
	err := t.service.ObserveTransfer(context.WithoutCancel(ctx), agent, written, t.now().Sub(started))
	if err != nil {
		slog.WarnContext(ctx, "transcode: recording transfer speed", "agent", agent, "error", err)
	}
}

// OutputOffset answers HEAD with the bytes of the output the server has committed.
func (t *Transfers) OutputOffset(w http.ResponseWriter, r *http.Request) {
	_, row, err := t.leased(r)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	committed, err := committedBytes(row.StagingPath)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	w.Header().Set(agentproto.HeaderUploadOffset, strconv.FormatInt(committed, 10))
	w.WriteHeader(http.StatusNoContent)
}

func committedBytes(path string) (int64, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// upload is a PUT's position in the whole output.
type upload struct {
	offset, length int64
	complete       bool
	digest         []byte
}

func parseUpload(h http.Header) (upload, error) {
	offset, err := strconv.ParseInt(h.Get(agentproto.HeaderUploadOffset), 10, 64)
	if err != nil || offset < 0 {
		return upload{}, fmt.Errorf("%s must be a byte offset", agentproto.HeaderUploadOffset)
	}
	length, err := strconv.ParseInt(h.Get(agentproto.HeaderUploadLength), 10, 64)
	if err != nil || length < offset {
		return upload{}, fmt.Errorf("%s must be the output size", agentproto.HeaderUploadLength)
	}
	u := upload{offset: offset, length: length, complete: h.Get(agentproto.HeaderUploadComplete) == agentproto.UploadCompleteTrue}
	if !u.complete {
		return u, nil
	}
	encoded, ok := strings.CutPrefix(h.Get(agentproto.HeaderContentDigest), agentproto.DigestSHA256Prefix)
	encoded, suffixed := strings.CutSuffix(encoded, agentproto.DigestSuffix)
	digest, err := base64.StdEncoding.DecodeString(encoded)
	if !ok || !suffixed || err != nil || len(digest) != sha256.Size {
		return upload{}, fmt.Errorf("the last chunk needs %s: %s<base64>%s", agentproto.HeaderContentDigest, agentproto.DigestSHA256Prefix, agentproto.DigestSuffix)
	}
	u.digest = digest
	return u, nil
}

// PutOutput writes a chunk at Upload-Offset into the staging dir. The last chunk carries the digest of
// the whole output: a match records the upload, a mismatch empties the staged file so the agent starts
// over.
func (t *Transfers) PutOutput(w http.ResponseWriter, r *http.Request) {
	agent, row, err := t.leased(r)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	chunk, err := parseUpload(r.Header)
	if err != nil {
		problem.Write(w, r, problem.New(TypeUploadMalformed, http.StatusBadRequest, err.Error()))
		return
	}
	if err := os.MkdirAll(filepath.Dir(row.StagingPath), stagingDirPerm); err != nil {
		problem.Write(w, r, err)
		return
	}
	file, err := os.OpenFile(row.StagingPath, os.O_WRONLY|os.O_CREATE, stagingFilePerm)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	if committed := info.Size(); committed != chunk.offset {
		w.Header().Set(agentproto.HeaderUploadOffset, strconv.FormatInt(committed, 10))
		problem.Write(w, r, problem.New(TypeUploadOffset, http.StatusConflict, fmt.Sprintf("the server has %d bytes", committed)))
		return
	}

	holder := TransferHolder{Job: row.ID, Direction: DirectionOutput}
	if !t.acquire(w, r, holder, RankUpload) {
		return
	}
	defer t.gate.Release(holder)

	ctx, cancel := t.lifetime.Bind(r.Context())
	defer cancel()
	started := t.now()
	written, err := t.write(ctx, file, r.Body, chunk)
	t.observe(ctx, agent, written, started)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	if chunk.complete {
		if err := t.complete(ctx, agent, row, file, chunk); err != nil {
			problem.Write(w, r, err)
			return
		}
	}
	w.Header().Set(agentproto.HeaderUploadOffset, strconv.FormatInt(chunk.offset+written, 10))
	w.WriteHeader(http.StatusNoContent)
}

// write appends the body at the chunk's offset and flushes it, keeping what arrived if the body breaks
// off so the agent can resume from there.
func (t *Transfers) write(ctx context.Context, file *os.File, body io.Reader, chunk upload) (int64, error) {
	if _, err := file.Seek(chunk.offset, io.SeekStart); err != nil {
		return 0, err
	}
	limit := chunk.length - chunk.offset
	written, err := io.Copy(file, io.LimitReader(contextReader{ctx: ctx, r: body}, limit+1))
	if syncErr := file.Sync(); syncErr != nil {
		err = errors.Join(err, syncErr)
	}
	if err != nil {
		return written, err
	}
	if written > limit {
		if err := file.Truncate(chunk.length); err != nil {
			return limit, err
		}
		return limit, problem.New(TypeUploadMalformed, http.StatusBadRequest, "the body runs past "+agentproto.HeaderUploadLength)
	}
	if chunk.complete && chunk.offset+written != chunk.length {
		return written, problem.New(TypeUploadMalformed, http.StatusBadRequest, "the last chunk ends before "+agentproto.HeaderUploadLength)
	}
	return written, nil
}

func (t *Transfers) complete(ctx context.Context, agent domain.AgentID, row jobRow, file *os.File, chunk upload) error {
	sum, err := digestOf(row.StagingPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(sum, chunk.digest) {
		if err := file.Truncate(0); err != nil {
			return err
		}
		return problem.New(TypeDigestMismatch, http.StatusUnprocessableEntity, "the output doesn't match "+agentproto.HeaderContentDigest+"; upload it again from offset 0")
	}
	err = t.service.RecordUpload(context.WithoutCancel(ctx), agent, row.ID, hex.EncodeToString(sum), chunk.length)
	if errors.Is(err, ErrNotLeased) {
		return errJobNotLeased
	}
	return err
}

func digestOf(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// countingWriter stops a download when its context ends and counts what it sent.
type countingWriter struct {
	http.ResponseWriter
	ctx     context.Context
	written int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.ResponseWriter.Write(p)
	w.written += int64(n)
	return n, err
}

func (w *countingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// contextReader stops an upload when its context ends.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
