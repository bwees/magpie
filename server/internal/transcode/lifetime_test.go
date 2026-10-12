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
)

var partialUpload = []byte("the first bytes of the output")

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

// stalledPut starts an upload whose body the agent stops sending partway.
func (f *fixture) stalledPut(agent domain.AgentID, job domain.JobID, body io.Reader) {
	req, err := http.NewRequest(http.MethodPut, f.server.URL+agentproto.OutputPath(job), body)
	if err != nil {
		f.t.Error(err)
		return
	}
	req.Header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+testSecret)
	req.Header.Set(agentproto.HeaderAgentID, string(agent))
	req.Header.Set(agentproto.HeaderUploadOffset, "0")
	req.Header.Set(agentproto.HeaderUploadLength, strconv.Itoa(stalledOutputSize))
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
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
			f.t.Fatalf("the %s transfer of job %s still runs after Stop", h.Direction, h.Job)
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
