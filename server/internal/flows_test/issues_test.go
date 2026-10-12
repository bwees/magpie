package flows_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity"
	activityapi "magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/requests"
	requestsapi "magpie/internal/requests/api"
	"magpie/internal/transcode/api/agentproto"
)

const (
	issuesPath       = "/api/requests/issues"
	blocklistPath    = "/api/acquisition/blocklist"
	attentionPath    = "/api/activity/attention"
	issueTmdb        = domain.TmdbID(6006)
	issueName        = "Issue Flow"
	reportedRelease  = "Issue.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-BAD"
	regrabRelease    = "Issue.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	issueDescription = "Green blocks all through"
)

func (s *stack) attentionFor(ref string) []activity.AttentionItemResponse {
	s.t.Helper()
	items := getJSON[[]activity.AttentionItemResponse](s.harness, attentionPath)
	return slices.DeleteFunc(items, func(a activity.AttentionItemResponse) bool { return a.Ref != ref })
}

// Flow 6: a downloaded movie was transcoded, then a user reports green blocks. The admin's REGRAB
// blocklists the reported release, searches and grabs another even though the file is transcoded, the
// import replaces the file, and the issue resolves and leaves Needs you.
func TestIssueRegrabReplacesATranscodedFileAndResolves(t *testing.T) {
	s := newStack(t)
	agent := connectAgent(s.harness, agentproto.CapEncHEVC10)
	s.movieOnTMDB(issueTmdb, issueName, movieYear)
	// The indexer's answer stays cached, so the REGRAB's search sees the same two releases; the reported
	// one has more seeders and was grabbed first.
	reportedItem, replacement := release(reportedRelease, topSeeders), release(regrabRelease, releaseSeeders)
	s.answer(reportedItem, replacement)
	s.request(s.session, issueTmdb)
	title := s.titleAdded(issueTmdb)
	reported := s.download(reportedItem, h264Movie())
	done := s.runTranscode(agent, reported)
	settles(t, "the transcoded file", func() []string { return s.swappedProblems(done) })
	s.drain(agent)

	rec := s.do(http.MethodPost, issuesPath, requests.ReportIssueRequest{TitleID: title, Kind: requestsapi.IssueKindVideo, Text: issueDescription})
	issue := decodeBody[requests.IssueResponse](t, rec.Code, rec.Body.Bytes(), http.StatusCreated)
	eventually(t, "the issue shows in Needs you", func() bool {
		return slices.ContainsFunc(s.attentionFor(string(issue.ID)), func(a activity.AttentionItemResponse) bool {
			return a.Kind == activityapi.AttentionIssueOpen
		})
	})

	rec = s.do(http.MethodPost, issuesPath+"/"+string(issue.ID)+"/actions", requests.IssueActionRequest{Action: requestsapi.IssueActionRegrab})
	if pending := decodeBody[requests.IssueResponse](t, rec.Code, rec.Body.Bytes(), http.StatusOK); pending.Status != requestsapi.IssueStatusActionPending {
		t.Fatalf("issue = %+v, want ACTION_PENDING", pending)
	}
	if grab := s.grabbed(replacement); grab.Trigger != acquisitionapi.TriggerRegrab {
		t.Fatalf("grab = %+v, want the REGRAB's", grab)
	}
	upgraded := s.download(replacement, h264Movie())

	resolved := func() []string {
		var problems []string
		entries := getJSON[[]acquisition.BlocklistEntryResponse](s.harness, blocklistPath)
		if !slices.ContainsFunc(entries, func(e acquisition.BlocklistEntryResponse) bool {
			return e.ReleaseName == reportedRelease && e.Reason == acquisitionapi.BlocklistIssue && e.ExpiresAt == nil
		}) {
			problems = append(problems, fmt.Sprintf("blocklist = %+v, want the reported release for the issue", entries))
		}
		started := published(s.harness, acquisitionapi.TypeSearchStarted, func(e acquisitionapi.SearchStarted) bool {
			return e.Trigger == acquisitionapi.TriggerRegrab && e.Title == title
		})
		if len(started) != 1 {
			problems = append(problems, fmt.Sprintf("REGRAB searches = %+v, want one", started))
		}
		if upgraded.Replaces == nil || *upgraded.Replaces != reported.MediaFile {
			problems = append(problems, fmt.Sprintf("FileImported.Replaces = %v, want the transcoded file", upgraded.Replaces))
		}
		deleted := published(s.harness, libraryapi.TypeFileDeleted, func(e libraryapi.FileDeleted) bool { return e.MediaFile == reported.MediaFile })
		if len(deleted) != 1 || deleted[0].Reason != libraryapi.DeleteUpgraded {
			problems = append(problems, fmt.Sprintf("FileDeleted = %+v, want the transcoded file UPGRADED", deleted))
		}
		files := getJSON[[]library.MediaFileResponse](s.harness, libraryFilesPath+titleQuery+string(title))
		if len(files) != 1 || files[0].ID != upgraded.MediaFile || files[0].Transcoded {
			problems = append(problems, fmt.Sprintf("library files = %+v, want only the replacement", files))
		}
		thread := getJSON[requests.IssueThreadResponse](s.harness, issuesPath+"/"+string(issue.ID))
		if thread.Issue.Status != requestsapi.IssueStatusResolved || thread.Issue.ResolvedBy != requestsapi.IssueResolvedByAction {
			problems = append(problems, fmt.Sprintf("issue = %+v, want RESOLVED by the action", thread.Issue))
		}
		byAction := published(s.harness, requestsapi.TypeIssueResolved, func(e requestsapi.IssueResolved) bool { return e.Issue == issue.ID })
		if len(byAction) != 1 || byAction[0].By != requestsapi.IssueResolvedByAction {
			problems = append(problems, fmt.Sprintf("IssueResolved = %+v, want one by ACTION", byAction))
		}
		if open := s.attentionFor(string(issue.ID)); len(open) != 0 {
			problems = append(problems, fmt.Sprintf("attention = %+v, want the issue cleared", open))
		}
		return problems
	}
	settles(t, "the regrabbed issue", resolved)

	s.redeliverAll()
	settles(t, "the regrabbed issue after redelivery", resolved)
}
