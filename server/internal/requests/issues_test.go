package requests

import (
	"net/http"
	"slices"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/requests/api"
	subtitlesapi "magpie/internal/subtitles/api"
)

const (
	issuesPath     = requestsPath + "/issues"
	waitingRelease = "Show.S01E01.1080p.WEB-DL-GRP"
	issueText      = "The audio is out of sync"
)

func (h *harness) issuePath(id domain.IssueID, action string) string {
	return issuesPath + "/" + string(id) + action
}

// reportingRequester lets the requester report issues.
func (h *harness) reportingRequester() {
	h.grant(&h.requester, requesterName, identityapi.Permissions{ReportIssues: true})
}

// report files an issue as p and fails the test on an error status.
func (h *harness) report(p domain.Principal, title domain.TitleID, episode *domain.EpisodeID) IssueResponse {
	h.t.Helper()
	body := ReportIssueRequest{TitleID: title, EpisodeID: episode, Kind: api.IssueKindAudio, Text: issueText}
	rec := h.do(http.MethodPost, issuesPath, body, p)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("report = %d: %s", rec.Code, rec.Body)
	}
	return decode[IssueResponse](h.t, rec)
}

func (h *harness) act(id domain.IssueID, action string, waitingRemoved bool) int {
	h.t.Helper()
	return h.do(http.MethodPost, h.issuePath(id, "/actions"), IssueActionRequest{Action: action, WaitingRemoved: waitingRemoved}, h.admin).Code
}

func (h *harness) issue(id domain.IssueID) api.Issue {
	h.t.Helper()
	i, err := h.repo.Issue(h.t.Context(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return i
}

// episodeIssue is an open issue by the requester on the first episode of a two-episode series.
func (h *harness) episodeIssue() (IssueResponse, domain.TitleID, []domain.EpisodeID) {
	h.t.Helper()
	h.reportingRequester()
	title, episodes := h.series(severanceTmdb, [2]int{1, 1}, [2]int{1, 2})
	return h.report(h.requester, title, &episodes[0]), title, episodes
}

func (h *harness) waitingDownload(title domain.TitleID, episodes ...domain.EpisodeID) acquisitionapi.Download {
	d := acquisitionapi.Download{
		ID: domain.NewID[domain.DownloadID](), Title: title, Episodes: episodes, ReleaseName: waitingRelease,
		State: acquisitionapi.StateImportPendingUser,
	}
	h.acquisition.PutDownload(d)
	return d
}

// Seed requests #13.
func TestReportingNeedsTheReportIssuesToggle(t *testing.T) {
	h := newHarness(t)
	title := h.movieTitle(duneTmdb, true)

	rec := h.do(http.MethodPost, issuesPath, ReportIssueRequest{TitleID: title, Kind: api.IssueKindVideo, Text: issueText}, h.requester)

	if rec.Code != http.StatusForbidden {
		t.Errorf("report without the toggle = %d, want 403", rec.Code)
	}
	if reported := events[api.IssueReported](h); len(reported) != 0 {
		t.Errorf("IssueReported = %+v, want none", reported)
	}
}

func TestReportingStoresAnOpenIssueAndTellsAdmins(t *testing.T) {
	h := newHarness(t)
	got, title, episodes := h.episodeIssue()

	if got.Status != api.IssueStatusOpen || got.Reporter.DisplayName != requesterName || got.Kind != api.IssueKindAudio || got.Text != issueText {
		t.Errorf("issue = %+v", got)
	}
	if got.Episode == nil || got.Episode.Season != 1 || got.Episode.Number != 1 {
		t.Errorf("episode = %+v, want S01E01", got.Episode)
	}
	reported := events[api.IssueReported](h)
	want := api.IssueReported{Issue: got.ID, Title: title, Episode: &episodes[0], Kind: api.IssueKindAudio, User: h.requester.User}
	if len(reported) != 1 || reported[0].Issue != want.Issue || *reported[0].Episode != *want.Episode || reported[0].User != want.User || reported[0].Title != title {
		t.Errorf("IssueReported = %+v, want %+v", reported, want)
	}
}

func TestReportingRejectsAnEpisodeOfAnotherTitle(t *testing.T) {
	h := newHarness(t)
	h.reportingRequester()
	movie := h.movieTitle(duneTmdb, true)
	_, episodes := h.series(severanceTmdb, [2]int{1, 1})

	for name, body := range map[string]ReportIssueRequest{
		"foreign episode": {TitleID: movie, EpisodeID: &episodes[0], Kind: api.IssueKindVideo, Text: issueText},
		"unknown title":   {TitleID: domain.NewID[domain.TitleID](), Kind: api.IssueKindVideo, Text: issueText},
		"unknown kind":    {TitleID: movie, Kind: "SMELL", Text: issueText},
		"blank text":      {TitleID: movie, Kind: api.IssueKindVideo, Text: "  "},
	} {
		if rec := h.do(http.MethodPost, issuesPath, body, h.requester); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
}

// Seed requests #14.
func TestAnActionMakesTheIssuePendingAndASecondIsRejected(t *testing.T) {
	h := newHarness(t)
	reported, title, episodes := h.episodeIssue()

	if code := h.act(reported.ID, api.IssueActionRegrab, false); code != http.StatusOK {
		t.Fatalf("REGRAB = %d", code)
	}

	if got := h.issue(reported.ID); got.Status != api.IssueStatusActionPending {
		t.Errorf("status = %s, want ACTION_PENDING", got.Status)
	}
	requested := events[api.IssueActionRequested](h)
	if len(requested) != 1 || requested[0].Action != api.IssueActionRegrab || requested[0].Title != title ||
		requested[0].Episode == nil || *requested[0].Episode != episodes[0] || len(requested[0].RemovedDownloads) != 0 {
		t.Errorf("IssueActionRequested = %+v, want one REGRAB for the episode", requested)
	}
	if code := h.act(reported.ID, api.IssueActionRerunSubtitles, false); code != http.StatusConflict {
		t.Errorf("second action = %d, want 409", code)
	}
	if len(events[api.IssueActionRequested](h)) != 1 {
		t.Error("the second action published an event")
	}
}

func TestOnlyAdminsRequestActions(t *testing.T) {
	h := newHarness(t)
	reported, _, _ := h.episodeIssue()

	rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/actions"), IssueActionRequest{Action: api.IssueActionRegrab}, h.requester)

	if rec.Code != http.StatusForbidden {
		t.Errorf("requester action = %d, want 403", rec.Code)
	}
	if code := h.act(reported.ID, "REBOOT", false); code != http.StatusBadRequest {
		t.Errorf("unknown action = %d, want 400", code)
	}
}

// Seed requests #17.
func TestARegrabWaitsForADownloadWaitingForMapping(t *testing.T) {
	h := newHarness(t)
	reported, title, episodes := h.episodeIssue()
	h.waitingDownload(title, episodes[1])
	waiting := h.waitingDownload(title, episodes[0])

	rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/actions"), IssueActionRequest{Action: api.IssueActionRegrab}, h.admin)

	if rec.Code != http.StatusConflict {
		t.Fatalf("REGRAB = %d: %s, want 409", rec.Code, rec.Body)
	}
	p := decodeProblem(t, rec)
	if p.Type != string(TypeImportWaiting) || p.Instance != "/api/library/imports/"+string(waiting.ID) {
		t.Errorf("problem = %+v, want import-waiting naming %s", p, waiting.ID)
	}
	if got := h.issue(reported.ID); got.Status != api.IssueStatusOpen || len(events[api.IssueActionRequested](h)) != 0 {
		t.Errorf("status = %s, want OPEN and no event", got.Status)
	}

	if code := h.act(reported.ID, api.IssueActionRegrab, true); code != http.StatusOK {
		t.Fatalf("REGRAB after removing the waiting download = %d", code)
	}
	requested := events[api.IssueActionRequested](h)
	if len(requested) != 1 || !slices.Equal(requested[0].RemovedDownloads, []domain.DownloadID{waiting.ID}) {
		t.Errorf("IssueActionRequested = %+v, want it to name %s only", requested, waiting.ID)
	}
}

func TestRerunningSubtitlesIgnoresWaitingDownloads(t *testing.T) {
	h := newHarness(t)
	reported, title, episodes := h.episodeIssue()
	h.waitingDownload(title, episodes[0])

	if code := h.act(reported.ID, api.IssueActionRerunSubtitles, false); code != http.StatusOK {
		t.Errorf("RERUN_SUBTITLES = %d, want 200", code)
	}
}

// Seed requests #15.
func TestAnImportForTheEpisodeResolvesARegrabOnce(t *testing.T) {
	h := newHarness(t)
	reported, title, episodes := h.episodeIssue()
	h.act(reported.ID, api.IssueActionRegrab, false)

	h.deliver(libraryapi.FileImported{Title: title, Episodes: []domain.EpisodeRef{{Episode: episodes[1]}}, Origin: domain.OriginDownload})
	h.deliver(libraryapi.FileImported{Title: title, Episodes: []domain.EpisodeRef{{Episode: episodes[0]}}, Origin: domain.OriginLibraryScan})
	if got := h.issue(reported.ID); got.Status != api.IssueStatusActionPending {
		t.Fatalf("status = %s after another episode and a scan, want ACTION_PENDING", got.Status)
	}

	imported := h.deliver(libraryapi.FileImported{Title: title, Episodes: []domain.EpisodeRef{{Episode: episodes[0]}}, Origin: domain.OriginDownload})
	h.deliverEvent(imported)

	if got := h.issue(reported.ID); got.Status != api.IssueStatusResolved {
		t.Errorf("status = %s, want RESOLVED", got.Status)
	}
	resolved := events[api.IssueResolved](h)
	if len(resolved) != 1 || resolved[0].By != api.IssueResolvedByAction || resolved[0].Reporter != h.requester.User {
		t.Errorf("IssueResolved = %+v, want once by ACTION", resolved)
	}
}

func TestASubtitleForTheEpisodeResolvesARerun(t *testing.T) {
	h := newHarness(t)
	reported, title, episodes := h.episodeIssue()
	regrab := h.report(h.requester, title, &episodes[0])
	h.act(reported.ID, api.IssueActionRerunSubtitles, false)
	h.act(regrab.ID, api.IssueActionRegrab, false)

	h.deliver(subtitlesapi.SubtitleAdded{Title: title, Episodes: []domain.EpisodeID{episodes[1]}})
	if got := h.issue(reported.ID); got.Status != api.IssueStatusActionPending {
		t.Fatalf("status = %s after another episode's subtitle, want ACTION_PENDING", got.Status)
	}
	h.deliver(subtitlesapi.SubtitleAdded{Title: title, Episodes: []domain.EpisodeID{episodes[0]}})

	if got := h.issue(reported.ID); got.Status != api.IssueStatusResolved {
		t.Errorf("rerun status = %s, want RESOLVED", got.Status)
	}
	if got := h.issue(regrab.ID); got.Status != api.IssueStatusActionPending {
		t.Errorf("regrab status = %s, want a subtitle not to resolve it", got.Status)
	}
}

func TestAMovieImportResolvesAWholeTitleIssue(t *testing.T) {
	h := newHarness(t)
	h.reportingRequester()
	title := h.movieTitle(duneTmdb, true)
	reported := h.report(h.requester, title, nil)
	h.act(reported.ID, api.IssueActionRegrab, false)

	h.deliver(libraryapi.FileImported{Title: title, Episodes: []domain.EpisodeRef{}, Origin: domain.OriginDownload})

	if got := h.issue(reported.ID); got.Status != api.IssueStatusResolved {
		t.Errorf("status = %s, want RESOLVED", got.Status)
	}
}

// Seed requests #16.
func TestAnAdminCommentTellsTheReporter(t *testing.T) {
	h := newHarness(t)
	reported, _, _ := h.episodeIssue()

	rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/comments"), CommentIssueRequest{Body: "Looking into it"}, h.admin)

	if rec.Code != http.StatusCreated {
		t.Fatalf("comment = %d: %s", rec.Code, rec.Body)
	}
	commented := events[api.IssueCommented](h)
	if len(commented) != 1 || commented[0].Reporter != h.requester.User || commented[0].Author != h.admin.User {
		t.Errorf("IssueCommented = %+v, want the reporter named", commented)
	}
	thread := decode[IssueThreadResponse](t, h.do(http.MethodGet, h.issuePath(reported.ID, ""), nil, h.requester))
	if thread.Issue.Comments != 1 || len(thread.Comments) != 1 || thread.Comments[0].Author.DisplayName != adminName {
		t.Errorf("thread = %+v, want the admin's reply", thread)
	}
}

func TestOnlyTheReporterOrAnAdminComments(t *testing.T) {
	h := newHarness(t)
	reported, _, _ := h.episodeIssue()

	if rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/comments"), CommentIssueRequest{Body: "me too"}, h.other); rec.Code != http.StatusNotFound {
		t.Errorf("hidden issue comment = %d, want 404", rec.Code)
	}
	h.grant(&h.other, otherName, identityapi.Permissions{ViewOthersRequests: true})
	if rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/comments"), CommentIssueRequest{Body: "me too"}, h.other); rec.Code != http.StatusForbidden {
		t.Errorf("visible issue comment = %d, want 403", rec.Code)
	}
	if rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/comments"), CommentIssueRequest{Body: "Still broken"}, h.requester); rec.Code != http.StatusCreated {
		t.Errorf("reporter comment = %d, want 201", rec.Code)
	}
}

func TestRequestersSeeOnlyTheirOwnIssues(t *testing.T) {
	h := newHarness(t)
	mine, title, _ := h.episodeIssue()
	theirs := h.report(h.admin, title, nil)

	listed := decode[IssueListResponse](t, h.do(http.MethodGet, issuesPath, nil, h.requester))
	if len(listed.Items) != 1 || listed.Items[0].ID != mine.ID || listed.Total != 1 {
		t.Errorf("requester's list = %+v, want only their issue", listed)
	}
	if rec := h.do(http.MethodGet, h.issuePath(theirs.ID, ""), nil, h.requester); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's issue = %d, want 404", rec.Code)
	}
	all := decode[IssueListResponse](t, h.do(http.MethodGet, issuesPath+"?status=OPEN&count=true", nil, h.admin))
	if all.Total != 2 || len(all.Items) != 0 {
		t.Errorf("admin count = %+v, want 2 and no items", all)
	}
}

func TestTheReporterResolvesTheirIssueOnce(t *testing.T) {
	h := newHarness(t)
	reported, _, _ := h.episodeIssue()

	if rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/resolve"), nil, h.other); rec.Code != http.StatusNotFound {
		t.Errorf("another requester resolving = %d, want 404", rec.Code)
	}
	got := decode[IssueResponse](t, h.do(http.MethodPost, h.issuePath(reported.ID, "/resolve"), nil, h.requester))
	if got.Status != api.IssueStatusResolved || got.ResolvedBy != string(h.requester.User) || got.ResolvedAt == nil {
		t.Errorf("resolved = %+v", got)
	}
	if rec := h.do(http.MethodPost, h.issuePath(reported.ID, "/resolve"), nil, h.admin); rec.Code != http.StatusConflict {
		t.Errorf("second resolve = %d, want 409", rec.Code)
	}
	if code := h.act(reported.ID, api.IssueActionRegrab, false); code != http.StatusConflict {
		t.Errorf("action on a resolved issue = %d, want 409", code)
	}
	if resolved := events[api.IssueResolved](h); len(resolved) != 1 || resolved[0].By != string(h.requester.User) {
		t.Errorf("IssueResolved = %+v", resolved)
	}
}

func TestRemovingTheTitleClosesItsOpenIssues(t *testing.T) {
	h := newHarness(t)
	reported, title, _ := h.episodeIssue()

	h.deliver(catalogapi.TitleRemoved{Title: title})

	if got := h.issue(reported.ID); got.Status != api.IssueStatusResolved {
		t.Errorf("status = %s, want RESOLVED", got.Status)
	}
	if resolved := events[api.IssueResolved](h); len(resolved) != 1 || resolved[0].By != api.IssueResolvedByTitleRemoved {
		t.Errorf("IssueResolved = %+v, want once by TITLE_REMOVED", resolved)
	}
}
