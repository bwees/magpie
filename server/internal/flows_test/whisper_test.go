package flows_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/subtitles"
	subtitlesapi "magpie/internal/subtitles/api"
	subtitlesfakes "magpie/internal/subtitles/fakes"
	"magpie/internal/transcode/api/agentproto"

	"go.uber.org/fx"
)

const (
	english    = domain.Language("en")
	whisperSRT = "1\n00:00:04,000 --> 00:00:05,000\nHello\n\n"

	subtitleSettingsPath = "/api/subtitles/settings"
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
	if getJSON[subtitles.SubtitleSettingsBody](h, subtitleSettingsPath).WhisperEnabled {
		t.Fatal("Whisper is on in a fresh install")
	}
	h.enableWhisper()
	return &whisperFlow{harness: h, file: file}
}

// enableWhisper turns Whisper on, as the admin does once an agent can run it; it is off on a fresh install.
func (h *harness) enableWhisper() {
	h.t.Helper()
	cfg := getJSON[subtitles.SubtitleSettingsBody](h, subtitleSettingsPath)
	cfg.WhisperEnabled = true
	if rec := h.do(http.MethodPut, subtitleSettingsPath, cfg); rec.Code != http.StatusOK {
		h.t.Fatalf("turning Whisper on = %d: %s", rec.Code, rec.Body)
	}
}

func (f *whisperFlow) imported() {
	f.t.Helper()
	f.publish(libraryapi.FileImported{
		MediaFile: f.file.ID, Title: f.file.Title, Path: f.file.Path, MediaInfo: f.file.MediaInfo, Origin: domain.OriginDownload,
	})
}

func (f *whisperFlow) subtitles() subtitles.FileSubtitlesResponse {
	f.t.Helper()
	return getJSON[subtitles.FileSubtitlesResponse](f.harness, "/api/subtitles/files/"+string(f.file.ID))
}

// Flow 5: Subtitles finds no provider match, asks Transcode for Whisper, an agent writes the subtitle,
// and Subtitles stores it next to the video.
func TestWhisperWritesTheOriginalLanguageSubtitle(t *testing.T) {
	f := newWhisperFlow(t)
	agent := connectAgent(f.harness, agentproto.CapWhisperVulkan)
	f.imported()

	offer := agent.offer()
	if offer.Spec.Kind != agentproto.KindWhisper || offer.Spec.Whisper == nil || offer.Spec.Whisper.Language != english {
		t.Fatalf("offer = %+v, want a Whisper job for English", offer.Spec)
	}
	agent.accept(offer.Spec)
	agent.fetchSource(offer.Spec)
	agent.deliver(offer.Spec, []byte(whisperSRT))

	sidecar := filepath.Join(filepath.Dir(f.file.Path), "Whisper Flow (2024).en.srt")
	satisfied := func() bool {
		got := f.subtitles()
		return len(got.Subtitles) == 1 && got.Subtitles[0].Source == subtitlesapi.SourceWhisper &&
			len(got.Coverage) == 1 && got.Coverage[0].Status == subtitlesapi.CoverageSatisfied
	}
	assertSidecar := func() {
		t.Helper()
		got := f.subtitles()
		content, err := os.ReadFile(sidecar)
		if !satisfied() || err != nil || string(content) != whisperSRT || got.Subtitles[0].Synced {
			t.Fatalf("sidecar %s = %q, %v; subtitles = %+v", sidecar, content, err, got)
		}
	}
	eventually(t, "the Whisper subtitle reaches the Files tab", satisfied)
	awaitEvent(f.harness, subtitlesapi.TypeCoverageSettled, func(e subtitlesapi.CoverageSettled) bool { return e.MediaFile == f.file.ID })
	assertSidecar()

	f.redeliverAll()
	assertSidecar()
}
