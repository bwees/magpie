package flows_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/subtitles"
	subtitlesapi "magpie/internal/subtitles/api"
	subtitlesfakes "magpie/internal/subtitles/fakes"
	"magpie/internal/transcode/api/agentproto"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	// whisperFlowWithin covers the scheduler's tick and the job queue's polling.
	whisperFlowWithin = 30 * time.Second
	whisperDevice     = "cuda:0"
	english           = domain.Language("en")
	whisperSRT        = "1\n00:00:04,000 --> 00:00:05,000\nHello\n\n"
)

// whisperFlow is a movie on disk that OpenSubtitles has nothing for.
type whisperFlow struct {
	*harness
	file libraryapi.MediaFile
}

func newWhisperFlow(t *testing.T) *whisperFlow {
	t.Helper()
	root := t.TempDir()
	title := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: "Whisper Flow", Year: 2024,
		External: domain.ExternalIDs{Tmdb: 101}, OriginalLanguage: english,
	}
	video := filepath.Join(root, "Whisper Flow (2024)", "Whisper Flow (2024).mkv")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("a movie with speech")
	if err := os.WriteFile(video, content, 0o644); err != nil {
		t.Fatal(err)
	}
	file := libraryapi.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: title.ID, RootFolder: root, Path: video, Size: int64(len(content)),
		Origin: domain.OriginDownload, ImportedAt: time.Now(),
		MediaInfo: libraryapi.MediaInfo{Container: "matroska", Streams: []libraryapi.Stream{
			{Index: 0, Kind: libraryapi.StreamVideo, Codec: "h264"},
			{Index: 1, Kind: libraryapi.StreamAudio, Codec: "aac", Language: english},
		}},
	}
	catalog := catalogtest.NewFake(catalogtest.Fixture{Titles: []catalogapi.Title{title}, GlobalLanguages: []catalogapi.WantedLanguage{{Language: english}}})
	library := librarytest.NewFake(librarytest.Fixture{Files: []librarytest.StoredFile{{File: file, ETag: "flow-etag"}}})
	h := boot(t, withAgentSecret(),
		fx.Decorate(func(catalogapi.Reader) catalogapi.Reader { return catalog }),
		fx.Decorate(func(libraryapi.Reader) libraryapi.Reader { return library }),
		fx.Decorate(func(subtitles.OpenSubtitles) subtitles.OpenSubtitles { return subtitlesfakes.NewOpenSubtitles() }),
	)
	h.signInAsAdmin()
	return &whisperFlow{harness: h, file: file}
}

func (f *whisperFlow) imported() {
	f.t.Helper()
	imported := libraryapi.FileImported{
		MediaFile: f.file.ID, Title: f.file.Title, Path: f.file.Path, MediaInfo: f.file.MediaInfo, Origin: domain.OriginDownload,
	}
	err := f.DB.Write(context.Background(), func(tx *gorm.DB) error { return f.Bus.Publish(tx, bus.EventOf(imported)) })
	if err != nil {
		f.t.Fatal(err)
	}
}

// fakeAgent is a Whisper agent that answers its one job with a ready subtitle.
type fakeAgent struct {
	t      *testing.T
	id     domain.AgentID
	server string
	conn   *websocket.Conn
}

func (f *whisperFlow) connectAgent() *fakeAgent {
	f.t.Helper()
	server := httptest.NewServer(f.server.Mux)
	f.t.Cleanup(server.Close)
	a := &fakeAgent{t: f.t, id: domain.NewID[domain.AgentID](), server: server.URL}
	conn, _, err := websocket.Dial(context.Background(), server.URL+agentproto.ControlPath, &websocket.DialOptions{HTTPHeader: a.headers()})
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = conn.CloseNow() })
	a.conn = conn
	a.send(agentproto.TypeHello, agentproto.Hello{
		AgentID: a.id, Hostname: "whisper-agent", Mode: agentproto.ModeDownloadFirst, ScratchFree: 1 << 40,
		Devices: []agentproto.DeviceReport{{
			DeviceID: whisperDevice, Vendor: agentproto.VendorNVIDIA,
			Capabilities: []agentproto.CapabilityResult{{Capability: agentproto.CapWhisperVulkan, OK: true}},
		}},
	})
	return a
}

func (a *fakeAgent) headers() http.Header {
	header := http.Header{}
	header.Set(agentproto.HeaderAuthorization, agentproto.BearerPrefix+flowAgentSecret)
	header.Set(agentproto.HeaderAgentID, string(a.id))
	return header
}

func (a *fakeAgent) send(t agentproto.MessageType, body any) {
	a.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		a.t.Fatal(err)
	}
	if err := wsjson.Write(context.Background(), a.conn, agentproto.Envelope{Type: t, ID: string(domain.NewID[domain.JobID]()), Body: raw}); err != nil {
		a.t.Fatal(err)
	}
}

// offer reads frames until the server offers a job.
func (a *fakeAgent) offer() agentproto.JobOffer {
	a.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), whisperFlowWithin)
	defer cancel()
	for {
		var env agentproto.Envelope
		if err := wsjson.Read(ctx, a.conn, &env); err != nil {
			a.t.Fatalf("waiting for a job offer: %v", err)
		}
		if env.Type != agentproto.TypeJobOffer {
			continue
		}
		var offer agentproto.JobOffer
		if err := json.Unmarshal(env.Body, &offer); err != nil {
			a.t.Fatal(err)
		}
		return offer
	}
}

func (a *fakeAgent) request(method, path string, header http.Header, body []byte) *http.Response {
	a.t.Helper()
	req, err := http.NewRequest(method, a.server+path, bytes.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	for name, values := range a.headers() {
		header[name] = values
	}
	req.Header = header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

// run accepts the job, fetches its source and uploads the subtitle, as the agent's job run does.
func (a *fakeAgent) run(spec agentproto.JobSpec) {
	a.t.Helper()
	a.send(agentproto.TypeJobAccept, agentproto.JobAccept{Job: spec.Job})
	if resp := a.request(http.MethodGet, spec.Source.URL, http.Header{}, nil); resp.StatusCode != http.StatusOK {
		a.t.Fatalf("source = %d", resp.StatusCode)
	}
	output := []byte(whisperSRT)
	sum := sha256.Sum256(output)
	header := http.Header{}
	header.Set(agentproto.HeaderUploadOffset, "0")
	header.Set(agentproto.HeaderUploadLength, strconv.Itoa(len(output)))
	header.Set(agentproto.HeaderUploadComplete, agentproto.UploadCompleteTrue)
	header.Set(agentproto.HeaderContentDigest, agentproto.DigestSHA256Prefix+base64.StdEncoding.EncodeToString(sum[:])+agentproto.DigestSuffix)
	if resp := a.request(http.MethodPut, spec.OutputURL, header, output); resp.StatusCode != http.StatusNoContent {
		a.t.Fatalf("upload = %d", resp.StatusCode)
	}
	a.send(agentproto.TypeJobResult, agentproto.JobResult{
		Job: spec.Job, Outcome: agentproto.OutcomeUploaded,
		Verify: agentproto.VerifyReport{OutputSize: int64(len(output)), SHA256: hex.EncodeToString(sum[:])},
	})
}

// Flow 5: Subtitles finds no provider match, asks Transcode for Whisper, an agent writes the subtitle,
// and Subtitles stores it next to the video.
func TestWhisperWritesTheOriginalLanguageSubtitle(t *testing.T) {
	f := newWhisperFlow(t)
	agent := f.connectAgent()
	f.imported()

	offer := agent.offer()
	if offer.Spec.Kind != agentproto.KindWhisper || offer.Spec.Whisper == nil || offer.Spec.Whisper.Language != english {
		t.Fatalf("offer = %+v, want a Whisper job for English", offer.Spec)
	}
	agent.run(offer.Spec)

	sidecar := filepath.Join(filepath.Dir(f.file.Path), "Whisper Flow (2024).en.srt")
	deadline := time.Now().Add(whisperFlowWithin)
	for time.Now().Before(deadline) {
		var got subtitles.FileSubtitlesResponse
		rec := f.get("/api/subtitles/files/" + string(f.file.ID))
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("subtitles = %d: %s", rec.Code, rec.Body)
		}
		if len(got.Subtitles) == 1 && got.Subtitles[0].Source == subtitlesapi.SourceWhisper &&
			len(got.Coverage) == 1 && got.Coverage[0].Status == subtitlesapi.CoverageSatisfied {
			content, err := os.ReadFile(sidecar)
			if err != nil || string(content) != whisperSRT || got.Subtitles[0].Synced {
				t.Fatalf("sidecar %s = %q, %v; subtitle = %+v", sidecar, content, err, got.Subtitles[0])
			}
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatal("the Whisper subtitle never reached the Files tab")
}
