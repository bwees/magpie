package transcode

import (
	"context"
	"io"
	"net/http"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/transcode/api/agentproto"
)

const (
	// stopRounds gives a Go that races Stop many chances to land after Stop's wait has begun.
	stopRounds = 200
	// stalledSourceSize is far more than the socket buffers hold, so the download blocks mid-response.
	stalledSourceSize = 256 << 20
	stalledOutputSize = 1 << 20

	// goroutineStacks asks the goroutine profile for every stack with its wait reason.
	goroutineProfile = "goroutine"
	goroutineStacks  = 2
	networkWait      = "[IO wait"
	downloadWrite    = "(*countingWriter).Write"
	uploadRead       = "contextReader.Read"

	// shortServerTimeout stands in for the server's read and write timeouts and shortIdle for
	// TransferIdleTimeout. Each agentPause outlasts the first but not the second, and slowPauses of them
	// outlast both.
	shortServerTimeout = 100 * time.Millisecond
	shortIdle          = 4 * shortServerTimeout
	agentPause         = 2 * shortServerTimeout
	slowPauses         = 3
	slowReadChunk      = 1 << 20
)

var (
	partialUpload = []byte("the first bytes of the output")
	uploadPiece   = []byte("a piece of the output")
)

func TestWorkHandedOverAfterStopNeverRuns(t *testing.T) {
	lifetime := NewLifetime()
	lifetime.Stop()

	ran := make(chan struct{})
	lifetime.Go(func(context.Context) { close(ran) })
	lifetime.Wait()

	select {
	case <-ran:
		t.Fatal("work handed over after Stop ran")
	default:
	}
}

// An agent request finishing as the server stops hands work to Go while Stop waits for earlier work.
func TestWorkHandedOverWhileStoppingNeverOutlivesStop(t *testing.T) {
	for range stopRounds {
		lifetime := NewLifetime()
		lifetime.Go(func(ctx context.Context) { <-ctx.Done() })
		stopped := make(chan struct{})
		handedOver := make(chan struct{})
		go func() {
			defer close(handedOver)
			<-lifetime.ctx.Done()
			lifetime.Go(func(context.Context) {
				select {
				case <-stopped:
					t.Error("work ran after Stop returned")
				default:
				}
			})
		}()

		lifetime.Stop()
		close(stopped)
		<-handedOver
		lifetime.Wait()
	}
}

func TestStopBreaksOffAStalledDownload(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	if err := os.Truncate(file.Path, stalledSourceSize); err != nil {
		t.Fatal(err)
	}

	f.request(a.id, http.MethodGet, spec.Source.URL, nil, nil)
	f.stalledIn(downloadWrite)
	a.answersClose()
	f.lifetime.Stop()

	f.transferEnds(TransferHolder{Job: spec.Job, Direction: DirectionSource})
}

func TestStopBreaksOffAStalledUpload(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)

	body, agent := io.Pipe()
	t.Cleanup(func() { _ = agent.Close() })
	go f.stalledPut(a.id, spec.Job, body)
	if _, err := agent.Write(partialUpload); err != nil {
		t.Fatal(err)
	}
	f.stalledIn(uploadRead)
	a.answersClose()
	f.lifetime.Stop()

	f.transferEnds(TransferHolder{Job: spec.Job, Direction: DirectionOutput})
}

func TestASlowDownloadThatKeepsMovingFinishes(t *testing.T) {
	f := newFixture(t)
	f.withServerTimeouts(shortServerTimeout)
	f.transfers.idle = shortIdle
	a, spec := f.leaseLargeSource()

	resp := f.request(a.id, http.MethodGet, spec.Source.URL, nil, nil)
	for range slowPauses {
		f.stalledIn(downloadWrite)
		time.Sleep(agentPause)
		if _, err := io.CopyN(io.Discard, resp.Body, slowReadChunk); err != nil {
			t.Fatalf("download stopped after a pause: %v", err)
		}
	}

	rest, err := io.Copy(io.Discard, resp.Body)
	if got := rest + slowPauses*slowReadChunk; err != nil || got != stalledSourceSize {
		t.Fatalf("downloaded %d of %d bytes: %v", got, stalledSourceSize, err)
	}
}

func TestADownloadThatStopsMovingIsCutAfterTheIdleTimeout(t *testing.T) {
	f := newFixture(t)
	f.transfers.idle = shortIdle
	a, spec := f.leaseLargeSource()

	f.request(a.id, http.MethodGet, spec.Source.URL, nil, nil)
	f.stalledIn(downloadWrite)

	f.transferEnds(TransferHolder{Job: spec.Job, Direction: DirectionSource})
}

func TestAnUploadThatStopsMovingIsCutAfterTheIdleTimeout(t *testing.T) {
	f := newFixture(t)
	f.transfers.idle = shortIdle
	a, spec := f.leaseLargeSource()

	body, agent := io.Pipe()
	t.Cleanup(func() { _ = agent.Close() })
	go f.stalledPut(a.id, spec.Job, body)
	if _, err := agent.Write(partialUpload); err != nil {
		t.Fatal(err)
	}
	f.stalledIn(uploadRead)

	f.transferEnds(TransferHolder{Job: spec.Job, Direction: DirectionOutput})
}

func TestASlowUploadThatKeepsMovingFinishes(t *testing.T) {
	f := newFixture(t)
	f.withServerTimeouts(shortServerTimeout)
	f.transfers.idle = shortIdle
	a, spec := f.leaseLargeSource()

	body, agent := io.Pipe()
	answered := make(chan *http.Response, 1)
	go func() {
		resp, err := f.putStream(a.id, spec.Job, body, slowPauses*len(uploadPiece))
		if err != nil {
			t.Error(err)
		}
		answered <- resp
	}()
	for range slowPauses {
		if _, err := agent.Write(uploadPiece); err != nil {
			t.Fatalf("upload stopped after a pause: %v", err)
		}
		f.stalledIn(uploadRead)
		time.Sleep(agentPause)
	}
	_ = agent.Close()

	resp := <-answered
	if resp == nil {
		t.FailNow()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("upload = %d: %s", resp.StatusCode, readAll(t, resp))
	}
}

// leaseLargeSource leases one agent the job of a source far larger than the socket buffers.
func (f *fixture) leaseLargeSource() (*testAgent, agentproto.JobSpec) {
	f.t.Helper()
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	if err := os.Truncate(file.Path, stalledSourceSize); err != nil {
		f.t.Fatal(err)
	}
	return a, spec
}

// stalledPut starts an upload whose body the agent stops sending partway.
func (f *fixture) stalledPut(agent domain.AgentID, job domain.JobID, body io.Reader) {
	if resp, err := f.putStream(agent, job, body, stalledOutputSize); err == nil {
		_ = resp.Body.Close()
	}
}

// putStream uploads body as the first chunk of an output of length bytes.
func (f *fixture) putStream(agent domain.AgentID, job domain.JobID, body io.Reader, length int) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, f.server.URL+agentproto.OutputPath(job), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+testSecret)
	req.Header.Set(agentproto.HeaderAgentID, string(agent))
	req.Header.Set(agentproto.HeaderUploadOffset, "0")
	req.Header.Set(agentproto.HeaderUploadLength, strconv.Itoa(length))
	return http.DefaultClient.Do(req)
}

// stalledIn waits until a goroutine is blocked on the network inside fn, so the test can stop the
// server while a transfer is stuck there.
func (f *fixture) stalledIn(fn string) {
	f.t.Helper()
	deadline := time.Now().Add(eventuallyWithin)
	for !blockedOnNetwork(f.t, fn) {
		if time.Now().After(deadline) {
			f.t.Fatalf("no goroutine is blocked on the network in %s", fn)
		}
		time.Sleep(eventuallyEvery)
	}
}

func blockedOnNetwork(t *testing.T, fn string) bool {
	t.Helper()
	var dump strings.Builder
	if err := pprof.Lookup(goroutineProfile).WriteTo(&dump, goroutineStacks); err != nil {
		t.Fatal(err)
	}
	for stack := range strings.SplitSeq(dump.String(), "\n\n") {
		if strings.Contains(stack, networkWait) && strings.Contains(stack, fn) {
			return true
		}
	}
	return false
}

// transferEnds waits for the transfer's handler to give back its slot.
func (f *fixture) transferEnds(h TransferHolder) {
	f.t.Helper()
	deadline := time.Now().Add(eventuallyWithin)
	for f.gate.holds(h) {
		if time.Now().After(deadline) {
			f.t.Fatalf("the %s transfer of job %s still runs after %v", h.Direction, h.Job, eventuallyWithin)
		}
		time.Sleep(eventuallyEvery)
	}
}

func (g *TransferGate) holds(h TransferHolder) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.holders[h]
	return ok
}
