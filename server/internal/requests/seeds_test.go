package requests

import (
	"net/http"
	"slices"
	"testing"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	"magpie/internal/requests/api"
)

// Seed requests #1.
func TestRequesterWithoutAutoApproveWaitsForApproval(t *testing.T) {
	h := newHarness(t)

	got := h.create(h.requester, movie(duneTmdb))

	if got.Status != api.StatusPending || got.ProfileID != defaultProfile || got.Requester.DisplayName != requesterName {
		t.Errorf("request = %+v, want PENDING with the default profile", got)
	}
	created := events[api.RequestCreated](h)
	if len(created) != 1 || created[0].Request != got.ID || created[0].User != h.requester.User || created[0].Tmdb != duneTmdb {
		t.Errorf("RequestCreated = %+v", created)
	}
	if approved := events[api.RequestApproved](h); len(approved) != 0 {
		t.Errorf("RequestApproved = %+v, want none", approved)
	}
}

// Seed requests #2.
func TestAutoApproveMoviesApprovesAMovieAtOnce(t *testing.T) {
	h := newHarness(t)
	h.grant(&h.requester, requesterName, identityapi.Permissions{AutoApproveMovies: true})

	got := h.create(h.requester, movie(duneTmdb))

	if got.Status != api.StatusApproved {
		t.Errorf("status = %s, want APPROVED", got.Status)
	}
	approved := events[api.RequestApproved](h)
	want := api.RequestApproved{
		Request: got.ID, User: h.requester.User, Kind: domain.MediaKindMovie, Tmdb: duneTmdb, Scope: api.ScopeMovie,
		Seasons: []int{}, Profile: defaultProfile, Auto: true, ApprovedBy: h.requester.User,
	}
	if len(approved) != 1 || !equalApproved(approved[0], want) {
		t.Errorf("RequestApproved = %+v, want %+v", approved, want)
	}
	if created := events[api.RequestCreated](h); len(created) != 0 {
		t.Errorf("RequestCreated = %+v, want none for an auto-approved request", created)
	}
}

func TestAdminRequestsAreApprovedAtOnce(t *testing.T) {
	h := newHarness(t)

	got := h.create(h.admin, wholeSeries(severanceTmdb))

	approved := events[api.RequestApproved](h)
	if got.Status != api.StatusApproved || len(approved) != 1 || !approved[0].Auto || approved[0].ApprovedBy != h.admin.User {
		t.Errorf("request = %+v, RequestApproved = %+v", got, approved)
	}
}

// Seed requests #3.
func TestAutoApproveMoviesDoesNotApproveSeries(t *testing.T) {
	h := newHarness(t)
	h.grant(&h.requester, requesterName, identityapi.Permissions{AutoApproveMovies: true})

	got := h.create(h.requester, seasons(severanceTmdb, 2, 1, 2))

	if got.Status != api.StatusPending {
		t.Errorf("status = %s, want PENDING", got.Status)
	}
	if !slices.Equal(got.Seasons, []int{1, 2}) {
		t.Errorf("seasons = %v, want them sorted without repeats", got.Seasons)
	}
}

// Seed requests #4.
func TestAdminApprovesWithAnotherProfile(t *testing.T) {
	h := newHarness(t)
	pending := h.create(h.requester, movie(duneTmdb))

	rec := h.do(http.MethodPost, requestsPath+"/"+string(pending.ID)+"/approve", ApproveRequestRequest{ProfileID: ptr(otherProfile)}, h.admin)
	got := decode[RequestResponse](t, rec)

	if got.Status != api.StatusApproved || got.ProfileID != otherProfile || got.DecidedBy == nil || *got.DecidedBy != h.admin.User {
		t.Errorf("request = %+v", got)
	}
	approved := events[api.RequestApproved](h)
	if len(approved) != 1 || approved[0].Profile != otherProfile || approved[0].ApprovedBy != h.admin.User || approved[0].Auto {
		t.Errorf("RequestApproved = %+v, want profile %s approved by the admin", approved, otherProfile)
	}
}

func TestApproveWithoutAProfileUsesTheDefaultAndRefusesUnknownProfiles(t *testing.T) {
	h := newHarness(t)
	pending := h.create(h.requester, movie(duneTmdb))
	h.acquisition.SetDefaultProfile(otherProfile)
	approvePath := requestsPath + "/" + string(pending.ID) + "/approve"

	if rec := h.do(http.MethodPost, approvePath, ApproveRequestRequest{ProfileID: ptr(domain.ProfileID("missing"))}, h.admin); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown profile = %d, want 400", rec.Code)
	}
	got := decode[RequestResponse](t, h.do(http.MethodPost, approvePath, ApproveRequestRequest{}, h.admin))
	if got.ProfileID != otherProfile {
		t.Errorf("profile = %s, want the current default %s", got.ProfileID, otherProfile)
	}
	if rec := h.do(http.MethodPost, approvePath, ApproveRequestRequest{}, h.admin); rec.Code != http.StatusConflict {
		t.Errorf("approving twice = %d, want 409", rec.Code)
	}
}

func TestOnlyAdminsDecide(t *testing.T) {
	h := newHarness(t)
	pending := h.create(h.requester, movie(duneTmdb))
	path := requestsPath + "/" + string(pending.ID)

	for _, action := range []string{"/approve", "/decline", "/retry"} {
		if rec := h.do(http.MethodPost, path+action, struct{}{}, h.requester); rec.Code != http.StatusForbidden {
			t.Errorf("%s as a requester = %d, want 403", action, rec.Code)
		}
	}
}

func TestDeclineKeepsTheReasonAndPublishesRequestDeclined(t *testing.T) {
	h := newHarness(t)
	pending := h.create(h.requester, movie(duneTmdb))

	got := decode[RequestResponse](t, h.do(http.MethodPost, requestsPath+"/"+string(pending.ID)+"/decline", DeclineRequestRequest{Reason: "Not in 4K"}, h.admin))

	if got.Status != api.StatusDeclined || got.DeclineReason != "Not in 4K" {
		t.Errorf("request = %+v", got)
	}
	declined := events[api.RequestDeclined](h)
	if len(declined) != 1 || declined[0] != (api.RequestDeclined{Request: pending.ID, User: h.requester.User, Reason: "Not in 4K"}) {
		t.Errorf("RequestDeclined = %+v", declined)
	}
	again := h.create(h.other, movie(duneTmdb))
	if again.Status != api.StatusPending {
		t.Errorf("a declined request still blocks new ones: %+v", again)
	}
}

// Seed requests #5.
func TestDuplicateRequestIsAConflictNamingTheExistingRequest(t *testing.T) {
	h := newHarness(t)
	existing := h.create(h.requester, movie(duneTmdb))

	rec := h.do(http.MethodPost, requestsPath, movie(duneTmdb), h.other)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	problem := decodeProblem(t, rec)
	if problem.Type != string(TypeAlreadyRequested) || problem.Instance != requestsPath+"/"+string(existing.ID) {
		t.Errorf("problem = %+v, want the existing request %s", problem, existing.ID)
	}
}

func TestSeasonRequestsOnlyConflictWhenCovered(t *testing.T) {
	h := newHarness(t)
	h.create(h.requester, seasons(severanceTmdb, 1, 2))

	if rec := h.do(http.MethodPost, requestsPath, seasons(severanceTmdb, 2), h.other); rec.Code != http.StatusConflict {
		t.Errorf("season 2 again = %d, want 409", rec.Code)
	}
	if got := h.create(h.other, seasons(severanceTmdb, 2, 3)); got.Status != api.StatusPending {
		t.Errorf("seasons 2 and 3 = %+v, want a new request", got)
	}
	if got := h.create(h.other, wholeSeries(severanceTmdb)); got.Status != api.StatusPending {
		t.Errorf("whole series = %+v, want a new request", got)
	}
	if rec := h.do(http.MethodPost, requestsPath, seasons(severanceTmdb, 7), h.requester); rec.Code != http.StatusConflict {
		t.Errorf("any season after a whole-series request = %d, want 409", rec.Code)
	}
}

func TestMonitoredScopeIsAConflict(t *testing.T) {
	h := newHarness(t)
	h.movieTitle(duneTmdb, true)
	h.series(severanceTmdb, [2]int{1, 1}, [2]int{1, 2})

	if rec := h.do(http.MethodPost, requestsPath, movie(duneTmdb), h.requester); rec.Code != http.StatusConflict || decodeProblem(t, rec).Type != string(TypeAlreadyMonitored) {
		t.Errorf("monitored movie = %d: %s", rec.Code, rec.Body)
	}
	if rec := h.do(http.MethodPost, requestsPath, seasons(severanceTmdb, 1), h.requester); rec.Code != http.StatusConflict {
		t.Errorf("monitored season = %d, want 409", rec.Code)
	}
	if got := h.create(h.requester, seasons(severanceTmdb, 2)); got.Status != api.StatusPending {
		t.Errorf("a season Catalog doesn't monitor yet = %+v", got)
	}
}

func TestARemovedTitlesRequestNoLongerBlocksANewOne(t *testing.T) {
	h := newHarness(t)
	first := h.create(h.admin, movie(duneTmdb))
	title := h.movieTitle(duneTmdb, true)
	h.deliver(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindMovie, RequestID: &first.ID, Origin: domain.OriginRequest})
	h.catalog.RemoveTitle(title)

	if got := h.create(h.requester, movie(duneTmdb)); got.Status != api.StatusPending {
		t.Errorf("request after the Title was removed = %+v", got)
	}
}

func TestInvalidScopesAreRejected(t *testing.T) {
	h := newHarness(t)
	for name, body := range map[string]CreateRequestRequest{
		"movie with seasons":     {Kind: domain.MediaKindMovie, TmdbID: duneTmdb, Scope: api.ScopeSeasons, Seasons: []int{1}},
		"series as a movie":      {Kind: domain.MediaKindSeries, TmdbID: severanceTmdb, Scope: api.ScopeMovie},
		"seasons without any":    seasons(severanceTmdb),
		"negative season":        seasons(severanceTmdb, -1),
		"whole series + seasons": {Kind: domain.MediaKindSeries, TmdbID: severanceTmdb, Scope: api.ScopeAllSeasons, Seasons: []int{1}},
		"no TMDB ID":             movie(0),
	} {
		if rec := h.do(http.MethodPost, requestsPath, body, h.requester); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
}

// Seed requests #6.
func TestTitleAddedLinksTheRequestAndBuildsItemsInScope(t *testing.T) {
	h := newHarness(t)
	got := h.create(h.admin, seasons(severanceTmdb, 2))
	title, episodes := h.series(severanceTmdb, [2]int{1, 1}, [2]int{2, 1}, [2]int{2, 2}, [2]int{2, 3})
	h.catalog.PutEpisode(catalogapi.Episode{ID: domain.NewID[domain.EpisodeID](), Title: title, Season: 2, Number: 4, Monitored: true})

	h.deliver(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindSeries, RequestID: &got.ID, Origin: domain.OriginRequest})

	request := h.request(got.ID)
	if request.Status != api.StatusProcessing || request.Title == nil || *request.Title != title {
		t.Errorf("request = %+v, want PROCESSING linked to the Title", request)
	}
	if request.TotalItems != 3 || request.AvailableItems != 0 {
		t.Errorf("items = %d/%d, want the 3 released season-2 episodes", request.AvailableItems, request.TotalItems)
	}
	if items := h.items(got.ID); slices.ContainsFunc(items, func(i itemRow) bool { return *i.EpisodeID == episodes[0] }) {
		t.Errorf("items %+v include season 1", items)
	}
}

func TestMovieTitleAddedBuildsOneItem(t *testing.T) {
	h := newHarness(t)
	got := h.create(h.admin, movie(duneTmdb))
	title := h.movieTitle(duneTmdb, true)

	h.deliver(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindMovie, RequestID: &got.ID, Origin: domain.OriginRequest})

	items := h.items(got.ID)
	if len(items) != 1 || items[0].EpisodeID != nil {
		t.Errorf("items = %+v, want one item without an episode", items)
	}
}

// Seed requests #7.
func TestTitleAddedTwiceDoesNotDuplicateItems(t *testing.T) {
	h := newHarness(t)
	got := h.create(h.admin, movie(duneTmdb))
	title := h.movieTitle(duneTmdb, true)
	added := catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindMovie, RequestID: &got.ID, Origin: domain.OriginRequest}

	h.deliver(added)
	h.deliver(added)

	if items := h.items(got.ID); len(items) != 1 {
		t.Errorf("items = %+v, want one", items)
	}
}

func TestMonitoringChangedLinksARequestForATrackedTitle(t *testing.T) {
	h := newHarness(t)
	title, _ := h.series(severanceTmdb, [2]int{1, 1}, [2]int{2, 1})
	got := h.create(h.admin, wholeSeries(severanceTmdb))

	h.deliver(catalogapi.MonitoringChanged{Title: title, RequestID: &got.ID})

	if request := h.request(got.ID); request.Status != api.StatusProcessing || request.TotalItems != 2 {
		t.Errorf("request = %+v, want PROCESSING with both episodes", request)
	}
}

func TestNewlyWantedEpisodesExtendOpenRequests(t *testing.T) {
	h := newHarness(t)
	title, _ := h.series(severanceTmdb, [2]int{1, 1})
	whole := h.create(h.admin, wholeSeries(severanceTmdb))
	h.deliver(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindSeries, RequestID: &whole.ID, Origin: domain.OriginRequest})

	released := domain.NewID[domain.EpisodeID]()
	h.catalog.PutEpisode(catalogapi.Episode{ID: released, Title: title, Season: 1, Number: 2, Monitored: true, Released: true})
	ev := h.deliver(catalogapi.WantedReleased{Title: title, Episodes: []domain.EpisodeID{released}})
	h.deliverEvent(ev)

	if request := h.request(whole.ID); request.TotalItems != 2 {
		t.Errorf("items = %d, want the released episode added once", request.TotalItems)
	}
}

func TestAddedItemMakesAnAvailableRequestPartialAgain(t *testing.T) {
	h := newHarness(t)
	title, _ := h.series(severanceTmdb, [2]int{1, 1})
	whole := h.create(h.admin, wholeSeries(severanceTmdb))
	h.deliver(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindSeries, RequestID: &whole.ID, Origin: domain.OriginRequest})
	h.markAvailable(whole.ID)

	h.catalog.PutEpisode(catalogapi.Episode{ID: domain.NewID[domain.EpisodeID](), Title: title, Season: 1, Number: 2, Monitored: true, Released: true})
	h.deliver(catalogapi.MonitoringChanged{Title: title})

	if request := h.request(whole.ID); request.Status != api.StatusPartiallyAvailable {
		t.Errorf("status = %s, want PARTIALLY_AVAILABLE", request.Status)
	}
}

func TestEventsForUnknownRequestsAreIgnored(t *testing.T) {
	h := newHarness(t)
	unknown := domain.NewID[domain.RequestID]()

	h.deliver(catalogapi.TitleAdded{Title: domain.NewID[domain.TitleID](), RequestID: &unknown})
	h.deliver(catalogapi.TitleAddFailed{RequestID: &unknown, Reason: catalogapi.TitleAddFailedTmdbNotFound})
}

// Seed requests #11.
func TestFailedTitleAddCanBeRetried(t *testing.T) {
	h := newHarness(t)
	got := h.create(h.admin, movie(duneTmdb))

	h.deliver(catalogapi.TitleAddFailed{Tmdb: duneTmdb, Kind: domain.MediaKindMovie, RequestID: &got.ID, Reason: catalogapi.TitleAddFailedTmdbNotFound})

	failed := decode[RequestResponse](t, h.do(http.MethodGet, requestsPath+"/"+string(got.ID), nil, h.admin))
	if failed.Status != api.StatusFailed || failed.FailureReason != catalogapi.TitleAddFailedTmdbNotFound {
		t.Fatalf("request = %+v, want FAILED with the reason", failed)
	}

	retried := decode[RequestResponse](t, h.do(http.MethodPost, requestsPath+"/"+string(got.ID)+"/retry", nil, h.admin))

	if retried.Status != api.StatusApproved || retried.FailureReason != "" {
		t.Errorf("request = %+v, want APPROVED", retried)
	}
	if approved := events[api.RequestApproved](h); len(approved) != 2 || approved[1].Request != got.ID {
		t.Errorf("RequestApproved = %+v, want it published again", approved)
	}
	if rec := h.do(http.MethodPost, requestsPath+"/"+string(got.ID)+"/retry", nil, h.admin); rec.Code != http.StatusConflict {
		t.Errorf("retrying an approved request = %d, want 409", rec.Code)
	}
}

// Seed requests #12.
func TestRequestersListOnlyTheirOwnRequests(t *testing.T) {
	h := newHarness(t)
	mine := h.create(h.requester, movie(duneTmdb))
	theirs := h.create(h.other, wholeSeries(severanceTmdb))

	got := h.list("", h.requester)

	if len(got.Items) != 1 || got.Items[0].ID != mine.ID || got.Total != 1 {
		t.Errorf("list = %+v, want only their own request", got)
	}
	if rec := h.do(http.MethodGet, requestsPath+"?userId="+string(h.other.User), nil, h.requester); rec.Code != http.StatusForbidden {
		t.Errorf("listing another user's requests = %d, want 403", rec.Code)
	}
	if rec := h.do(http.MethodGet, requestsPath+"/"+string(theirs.ID), nil, h.requester); rec.Code != http.StatusNotFound {
		t.Errorf("reading another user's request = %d, want 404", rec.Code)
	}

	h.grant(&h.requester, requesterName, identityapi.Permissions{ViewOthersRequests: true})
	if all := h.list("", h.requester); len(all.Items) != 2 {
		t.Errorf("with view others' requests = %+v, want both", all)
	}
}

func TestListFiltersByStatusAndCountsEveryStatus(t *testing.T) {
	h := newHarness(t)
	h.create(h.requester, movie(duneTmdb))
	h.create(h.other, seasons(severanceTmdb, 1))
	h.create(h.admin, seasons(severanceTmdb, 2))

	got := h.list("?status=PENDING&count=true", h.admin)

	if got.Total != 2 || len(got.Items) != 0 {
		t.Errorf("pending count = %+v, want total 2 without items", got)
	}
	counts := map[api.Status]int{}
	for _, c := range got.Counts {
		counts[c.Status] = c.Count
	}
	if len(got.Counts) != len(AllStatuses) || counts[api.StatusPending] != 2 || counts[api.StatusApproved] != 1 {
		t.Errorf("counts = %+v", got.Counts)
	}
	approved := h.list("?status=APPROVED&status=PROCESSING", h.admin)
	if len(approved.Items) != 1 || approved.Items[0].Requester.DisplayName != adminName {
		t.Errorf("approved = %+v", approved.Items)
	}
	if rec := h.do(http.MethodGet, requestsPath+"?status=SOMETHING", nil, h.admin); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown status = %d, want 400", rec.Code)
	}
}

func TestListByTitle(t *testing.T) {
	h := newHarness(t)
	got := h.create(h.admin, movie(duneTmdb))
	title := h.movieTitle(duneTmdb, true)
	h.deliver(catalogapi.TitleAdded{Title: title, Kind: domain.MediaKindMovie, RequestID: &got.ID, Origin: domain.OriginRequest})
	h.create(h.requester, wholeSeries(severanceTmdb))

	list := h.list("?titleId="+string(title), h.admin)

	if len(list.Items) != 1 || list.Items[0].ID != got.ID {
		t.Errorf("list = %+v, want the request linked to the Title", list.Items)
	}
}

func TestRequesterCancelsTheirOwnPendingRequest(t *testing.T) {
	h := newHarness(t)
	mine := h.create(h.requester, movie(duneTmdb))
	path := requestsPath + "/" + string(mine.ID)

	if rec := h.do(http.MethodDelete, path, nil, h.other); rec.Code != http.StatusForbidden {
		t.Errorf("cancel by another user = %d, want 403", rec.Code)
	}
	if rec := h.do(http.MethodDelete, path, nil, h.requester); rec.Code != http.StatusNoContent {
		t.Fatalf("cancel = %d: %s", rec.Code, rec.Body)
	}
	if rec := h.do(http.MethodGet, path, nil, h.requester); rec.Code != http.StatusNotFound {
		t.Errorf("cancelled request = %d, want 404", rec.Code)
	}
	cancelled := events[api.RequestCancelled](h)
	if len(cancelled) != 1 || cancelled[0] != (api.RequestCancelled{Request: mine.ID, User: h.requester.User}) {
		t.Errorf("RequestCancelled = %+v", cancelled)
	}

	approved := h.create(h.admin, movie(duneTmdb))
	if rec := h.do(http.MethodDelete, requestsPath+"/"+string(approved.ID), nil, h.admin); rec.Code != http.StatusConflict {
		t.Errorf("cancelling an approved request = %d, want 409", rec.Code)
	}
}
