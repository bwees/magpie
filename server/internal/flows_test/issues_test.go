package flows_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity"
	activityapi "magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/requests"
	requestsapi "magpie/internal/requests/api"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	issuesPath       = "/api/requests/issues"
	blocklistPath    = "/api/acquisition/blocklist"
	attentionPath    = "/api/activity/attention"
	reportedFileHash = "c12fe1c06bba254a9dc9f519b335aa7c1367a88a"
	reportedRelease  = "Issue.Flow.2024.1080p.WEB-DL-GRP"
)

// issueFlow is a transcoded movie on disk that a REGRAB must replace anyway.
type issueFlow struct {
	*harness
	title catalogapi.Title
}

func newIssueFlow(t *testing.T) *issueFlow {
	t.Helper()
	title := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: "Issue Flow", Year: 2024,
		External: domain.ExternalIDs{Tmdb: 202}, Monitored: true, Released: true, RuntimeMinutes: 120,
	}
	file := libraryapi.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: title.ID, Path: "/movies/Issue Flow (2024)/Issue Flow (2024).mkv",
		ReleaseName: reportedRelease, InfoHash: reportedFileHash, Transcoded: true, Origin: domain.OriginDownload, ImportedAt: time.Now(),
	}
	catalog := catalogtest.NewFake(catalogtest.Fixture{Titles: []catalogapi.Title{title}})
	library := librarytest.NewFake(librarytest.Fixture{Files: []librarytest.StoredFile{{File: file}}})
	h := boot(t,
		fx.Decorate(func(catalogapi.Reader) catalogapi.Reader { return catalog }),
		fx.Decorate(func(libraryapi.Reader) libraryapi.Reader { return library }),
	)
	h.signInAsAdmin()
	return &issueFlow{harness: h, title: title}
}

func decodeBody[T any](t *testing.T, code int, raw []byte, want int) T {
	t.Helper()
	if code != want {
		t.Fatalf("status %d, want %d: %s", code, want, raw)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// eventually polls check until it holds or the handlers had their time.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitForHandler)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatalf("never: %s", what)
}

func (f *issueFlow) outbox(eventType string) []string {
	f.t.Helper()
	var payloads []string
	err := f.DB.Read(context.Background()).Table("platform_outbox").Where("type = ?", eventType).Pluck("payload", &payloads).Error
	if err != nil {
		f.t.Fatal(err)
	}
	return payloads
}

// Flow 6: an admin's REGRAB blocklists the reported file's release, even a transcoded one, and searches
// again; the import that replaces it resolves the issue and clears the attention item.
func TestIssueRegrabBlocklistsSearchesAndResolvesOnImport(t *testing.T) {
	f := newIssueFlow(t)

	rec := f.do(http.MethodPost, issuesPath, requests.ReportIssueRequest{TitleID: f.title.ID, Kind: requestsapi.IssueKindVideo, Text: "Green blocks all through"})
	issue := decodeBody[requests.IssueResponse](t, rec.Code, rec.Body.Bytes(), http.StatusCreated)
	eventually(t, "the issue shows in Needs you", func() bool {
		items := decodeBody[[]activity.AttentionItemResponse](t, http.StatusOK, f.get(attentionPath).Body.Bytes(), http.StatusOK)
		return slices.ContainsFunc(items, func(a activity.AttentionItemResponse) bool {
			return a.Ref == string(issue.ID) && a.Kind == activityapi.AttentionIssueOpen
		})
	})

	rec = f.do(http.MethodPost, issuesPath+"/"+string(issue.ID)+"/actions", requests.IssueActionRequest{Action: requestsapi.IssueActionRegrab})
	if pending := decodeBody[requests.IssueResponse](t, rec.Code, rec.Body.Bytes(), http.StatusOK); pending.Status != requestsapi.IssueStatusActionPending {
		t.Fatalf("issue = %+v, want ACTION_PENDING", pending)
	}

	eventually(t, "the reported release is blocklisted for the issue", func() bool {
		entries := decodeBody[[]acquisition.BlocklistEntryResponse](t, http.StatusOK, f.get(blocklistPath).Body.Bytes(), http.StatusOK)
		return slices.ContainsFunc(entries, func(e acquisition.BlocklistEntryResponse) bool {
			return e.InfoHash == reportedFileHash && e.Reason == acquisitionapi.BlocklistIssue && e.ExpiresAt == nil
		})
	})
	eventually(t, "a REGRAB search starts", func() bool {
		return slices.ContainsFunc(f.outbox(acquisitionapi.TypeSearchStarted), func(raw string) bool {
			var started acquisitionapi.SearchStarted
			return json.Unmarshal([]byte(raw), &started) == nil && started.Trigger == acquisitionapi.TriggerRegrab && started.Title == f.title.ID
		})
	})

	imported := libraryapi.FileImported{
		MediaFile: domain.NewID[domain.MediaFileID](), Title: f.title.ID, Episodes: []domain.EpisodeRef{},
		ReleaseName: "Issue.Flow.2024.2160p.WEB-DL-GRP", Origin: domain.OriginDownload,
	}
	err := f.DB.Write(context.Background(), func(tx *gorm.DB) error { return f.Bus.Publish(tx, bus.EventOf(imported)) })
	if err != nil {
		t.Fatal(err)
	}

	eventually(t, "the import resolves the issue", func() bool {
		rec := f.get(issuesPath + "/" + string(issue.ID))
		thread := decodeBody[requests.IssueThreadResponse](t, rec.Code, rec.Body.Bytes(), http.StatusOK)
		return thread.Issue.Status == requestsapi.IssueStatusResolved && thread.Issue.ResolvedBy == requestsapi.IssueResolvedByAction
	})
	eventually(t, "the attention item clears", func() bool {
		items := decodeBody[[]activity.AttentionItemResponse](t, http.StatusOK, f.get(attentionPath).Body.Bytes(), http.StatusOK)
		return !slices.ContainsFunc(items, func(a activity.AttentionItemResponse) bool { return a.Ref == string(issue.ID) })
	})
}
