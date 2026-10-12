package requests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/requests/api"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var (
	ErrInvalidRequest   = errors.New("requests: invalid request")
	ErrAlreadyMonitored = errors.New("requests: already monitored")
	ErrWrongStatus      = errors.New("requests: not allowed in this status")
	ErrNotYours         = errors.New("requests: not your request")
	ErrHiddenUser       = errors.New("requests: other users' requests are hidden")
	ErrUnknownProfile   = errors.New("requests: unknown profile")
)

// DuplicateError rejects a request another open request already covers.
type DuplicateError struct {
	Existing domain.RequestID
}

func (e DuplicateError) Error() string {
	return fmt.Sprintf("requests: already requested in %s", e.Existing)
}

// specialsSeason is season 0, which a whole-series request leaves unmonitored.
const specialsSeason = 0

// openStatuses are the statuses a request still covers its scope in: everything but DECLINED and FAILED.
var openStatuses = []api.Status{api.StatusPending, api.StatusApproved, api.StatusProcessing, api.StatusPartiallyAvailable, api.StatusAvailable}

// trackingStatuses are the approved statuses whose items follow the Title.
var trackingStatuses = []api.Status{api.StatusApproved, api.StatusProcessing, api.StatusPartiallyAvailable, api.StatusAvailable}

// AllStatuses lists every status in the order the UI counts them.
var AllStatuses = []api.Status{api.StatusPending, api.StatusApproved, api.StatusProcessing, api.StatusPartiallyAvailable, api.StatusAvailable, api.StatusDeclined, api.StatusFailed}

// NewRequest is what a user asks for.
type NewRequest struct {
	Kind    domain.MediaKind
	Tmdb    domain.TmdbID
	Scope   string
	Seasons []int
}

// RequestView is a request as the HTTP layer shows it.
type RequestView struct {
	api.Request
	RequesterName string
	DecidedBy     *domain.UserID
	DecidedAt     *time.Time
	DeclineReason string
	FailureReason string
}

// ListQuery fields that are nil or empty don't filter.
type ListQuery struct {
	Statuses  []api.Status
	User      *domain.UserID
	Title     *domain.TitleID
	CountOnly bool
}

// ListResult holds the matching requests, their number, and how many of the viewer's requests are in each
// status regardless of the status filter.
type ListResult struct {
	Items  []RequestView
	Total  int
	Counts map[api.Status]int
}

type ServiceParams struct {
	fx.In

	Repository  *Repository
	DB          *db.DB
	Bus         *bus.Bus
	Identity    identityapi.Reader
	Catalog     catalogapi.Reader
	Acquisition acquisitionapi.Reader
}

// Service owns every change to requests and the events that go with them.
type Service struct {
	repo        *Repository
	db          *db.DB
	bus         *bus.Bus
	identity    identityapi.Reader
	catalog     catalogapi.Reader
	acquisition acquisitionapi.Reader
	now         func() time.Time
}

func NewService(p ServiceParams) *Service {
	return &Service{
		repo:        p.Repository,
		db:          p.DB,
		bus:         p.Bus,
		identity:    p.Identity,
		catalog:     p.Catalog,
		acquisition: p.Acquisition,
		now:         time.Now,
	}
}

// Create stores a request. Admins and users with the kind's auto-approve toggle are approved at once;
// everyone else waits for an admin.
func (s *Service) Create(ctx context.Context, user domain.UserID, in NewRequest) (RequestView, error) {
	in, err := in.normalized()
	if err != nil {
		return RequestView{}, err
	}
	requester, err := s.identity.User(ctx, user)
	if err != nil {
		return RequestView{}, err
	}
	if err := s.checkNotMonitored(ctx, in); err != nil {
		return RequestView{}, err
	}
	profile, err := s.acquisition.DefaultProfile(ctx)
	if err != nil {
		return RequestView{}, err
	}

	now := s.now().UTC()
	row := requestRow{
		ID:        domain.NewID[domain.RequestID](),
		UserID:    user,
		Kind:      in.Kind,
		TmdbID:    in.Tmdb,
		Scope:     in.Scope,
		Seasons:   in.Seasons,
		ProfileID: profile,
		Status:    api.StatusPending,
		CreatedAt: now,
	}
	auto := requester.IsAdmin() || autoApproves(requester.Permissions, in.Kind)
	if auto {
		row.Status, row.DecidedBy, row.DecidedAt = api.StatusApproved, &user, &now
	}
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := s.checkNotRequested(ctx, tx, in); err != nil {
			return err
		}
		if err := s.repo.create(tx, row); err != nil {
			return err
		}
		if auto {
			return s.publish(tx, approved(row, user, true))
		}
		return s.publish(tx, api.RequestCreated{Request: row.ID, User: user, Kind: in.Kind, Tmdb: in.Tmdb, Scope: in.Scope, Seasons: in.Seasons})
	})
	if err != nil {
		return RequestView{}, err
	}
	return s.view(ctx, row.ID)
}

func autoApproves(p identityapi.Permissions, kind domain.MediaKind) bool {
	if kind == domain.MediaKindMovie {
		return p.AutoApproveMovies
	}
	return p.AutoApproveTV
}

func (in NewRequest) normalized() (NewRequest, error) {
	if in.Tmdb <= 0 {
		return in, fmt.Errorf("%w: tmdbId must be positive", ErrInvalidRequest)
	}
	switch {
	case in.Kind == domain.MediaKindMovie && in.Scope == api.ScopeMovie && len(in.Seasons) == 0:
	case in.Kind == domain.MediaKindSeries && in.Scope == api.ScopeAllSeasons && len(in.Seasons) == 0:
	case in.Kind == domain.MediaKindSeries && in.Scope == api.ScopeSeasons && len(in.Seasons) > 0:
		if slices.ContainsFunc(in.Seasons, func(season int) bool { return season < 0 }) {
			return in, fmt.Errorf("%w: season numbers can't be negative", ErrInvalidRequest)
		}
		in.Seasons = slices.Compact(slices.Sorted(slices.Values(in.Seasons)))
		return in, nil
	default:
		return in, fmt.Errorf("%w: a movie needs scope MOVIE; a series needs ALL_SEASONS, or SEASONS with seasons", ErrInvalidRequest)
	}
	in.Seasons = []int{}
	return in, nil
}

// checkNotMonitored rejects a request whose whole scope Catalog already monitors.
func (s *Service) checkNotMonitored(ctx context.Context, in NewRequest) error {
	title, err := s.catalog.TitleByTmdb(ctx, in.Kind, in.Tmdb)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if in.Kind == domain.MediaKindMovie {
		if title.Monitored {
			return ErrAlreadyMonitored
		}
		return nil
	}
	episodes, err := s.catalog.Episodes(ctx, title.ID)
	if err != nil {
		return err
	}
	if in.Scope == api.ScopeAllSeasons {
		regular := slices.DeleteFunc(episodes, func(e catalogapi.Episode) bool { return e.Season == specialsSeason })
		if title.MonitorFutureSeasons && allMonitored(regular) {
			return ErrAlreadyMonitored
		}
		return nil
	}
	for _, season := range in.Seasons {
		inSeason := slices.DeleteFunc(slices.Clone(episodes), func(e catalogapi.Episode) bool { return e.Season != season })
		if !allMonitored(inSeason) {
			return nil
		}
	}
	return ErrAlreadyMonitored
}

func allMonitored(episodes []catalogapi.Episode) bool {
	return len(episodes) > 0 && !slices.ContainsFunc(episodes, func(e catalogapi.Episode) bool { return !e.Monitored })
}

// checkNotRequested rejects a request an open one already covers. A request whose Title was removed from
// Catalog is history and covers nothing.
func (s *Service) checkNotRequested(ctx context.Context, tx *gorm.DB, in NewRequest) error {
	candidates, err := s.repo.requests(tx, requestFilter{Statuses: openStatuses, Kind: &in.Kind, Tmdb: &in.Tmdb})
	if err != nil {
		return err
	}
	for _, c := range candidates {
		if !covers(c, in) {
			continue
		}
		gone, err := s.titleGone(ctx, c.TitleID)
		if err != nil {
			return err
		}
		if !gone {
			return DuplicateError{Existing: c.ID}
		}
	}
	return nil
}

func covers(existing requestRow, in NewRequest) bool {
	switch existing.Scope {
	case api.ScopeSeasons:
		return in.Scope == api.ScopeSeasons && !slices.ContainsFunc(in.Seasons, func(season int) bool {
			return !slices.Contains(existing.Seasons, season)
		})
	default:
		return true
	}
}

func (s *Service) titleGone(ctx context.Context, title *domain.TitleID) (bool, error) {
	if title == nil {
		return false, nil
	}
	_, err := s.catalog.Title(ctx, *title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return true, nil
	}
	return false, err
}

// Approve approves a pending request with the given profile, or the default profile when nil.
func (s *Service) Approve(ctx context.Context, admin domain.UserID, id domain.RequestID, profile *domain.ProfileID) (RequestView, error) {
	chosen, err := s.profileFor(ctx, profile)
	if err != nil {
		return RequestView{}, err
	}
	return s.decide(ctx, id, api.StatusPending, func(tx *gorm.DB, row *requestRow) error {
		now := s.now().UTC()
		row.Status, row.ProfileID, row.DecidedBy, row.DecidedAt = api.StatusApproved, chosen, &admin, &now
		return s.publish(tx, approved(*row, admin, false))
	})
}

func (s *Service) profileFor(ctx context.Context, profile *domain.ProfileID) (domain.ProfileID, error) {
	if profile == nil {
		return s.acquisition.DefaultProfile(ctx)
	}
	_, err := s.acquisition.Profile(ctx, *profile)
	if errors.Is(err, acquisitionapi.ErrNotFound) {
		return "", fmt.Errorf("%w: %s", ErrUnknownProfile, *profile)
	}
	return *profile, err
}

// Decline declines a pending request.
func (s *Service) Decline(ctx context.Context, admin domain.UserID, id domain.RequestID, reason string) (RequestView, error) {
	return s.decide(ctx, id, api.StatusPending, func(tx *gorm.DB, row *requestRow) error {
		now := s.now().UTC()
		row.Status, row.DecidedBy, row.DecidedAt = api.StatusDeclined, &admin, &now
		if reason != "" {
			row.DeclineReason = &reason
		}
		return s.publish(tx, api.RequestDeclined{Request: row.ID, User: row.UserID, Reason: reason})
	})
}

// Retry approves a failed request again, so Catalog tries to add its Title once more.
func (s *Service) Retry(ctx context.Context, admin domain.UserID, id domain.RequestID) (RequestView, error) {
	return s.decide(ctx, id, api.StatusFailed, func(tx *gorm.DB, row *requestRow) error {
		now := s.now().UTC()
		row.Status, row.DecidedBy, row.DecidedAt, row.FailureReason = api.StatusApproved, &admin, &now, nil
		return s.publish(tx, approved(*row, admin, false))
	})
}

// decide changes a request that must be in status from, then saves it.
func (s *Service) decide(ctx context.Context, id domain.RequestID, from api.Status, change func(tx *gorm.DB, row *requestRow) error) (RequestView, error) {
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.request(tx, id)
		if err != nil {
			return err
		}
		if row.Status != from {
			return fmt.Errorf("%w: request is %s", ErrWrongStatus, row.Status)
		}
		if err := change(tx, &row); err != nil {
			return err
		}
		return s.repo.save(tx, row)
	})
	if err != nil {
		return RequestView{}, err
	}
	return s.view(ctx, id)
}

// Cancel deletes the user's own pending request.
func (s *Service) Cancel(ctx context.Context, user domain.UserID, id domain.RequestID) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		current, err := s.repo.request(tx, id)
		if err != nil {
			return err
		}
		if current.UserID != user {
			return ErrNotYours
		}
		if current.Status != api.StatusPending {
			return fmt.Errorf("%w: request is %s", ErrWrongStatus, current.Status)
		}
		if err := s.repo.delete(tx, id); err != nil {
			return err
		}
		return s.publish(tx, api.RequestCancelled{Request: id, User: user})
	})
}

func approved(row requestRow, by domain.UserID, auto bool) api.RequestApproved {
	return api.RequestApproved{
		Request:    row.ID,
		User:       row.UserID,
		Kind:       row.Kind,
		Tmdb:       row.TmdbID,
		Scope:      row.Scope,
		Seasons:    row.Seasons,
		Profile:    row.ProfileID,
		Auto:       auto,
		ApprovedBy: by,
	}
}

// Get returns one request the viewer may see. Others' requests are ErrNotFound without ViewOthersRequests.
func (s *Service) Get(ctx context.Context, viewer domain.Principal, id domain.RequestID) (RequestView, error) {
	view, err := s.view(ctx, id)
	if err != nil {
		return RequestView{}, err
	}
	if !seesEveryone(viewer) && view.User != viewer.User {
		return RequestView{}, api.ErrNotFound
	}
	return view, nil
}

// List returns the requests the viewer may see, newest first. Without ViewOthersRequests that is their own.
func (s *Service) List(ctx context.Context, viewer domain.Principal, q ListQuery) (ListResult, error) {
	if !seesEveryone(viewer) {
		if q.User != nil && *q.User != viewer.User {
			return ListResult{}, ErrHiddenUser
		}
		q.User = &viewer.User
	}
	conn := s.db.Read(ctx)
	visible := requestFilter{User: q.User, Title: q.Title}
	counts, err := s.repo.countByStatus(conn, visible)
	if err != nil {
		return ListResult{}, err
	}
	result := ListResult{Items: []RequestView{}, Counts: counts}
	for _, status := range statusesOrAll(q.Statuses) {
		result.Total += counts[status]
	}
	if q.CountOnly {
		return result, nil
	}
	visible.Statuses = q.Statuses
	rows, err := s.repo.requests(conn, visible)
	if err != nil {
		return ListResult{}, err
	}
	result.Items, err = s.views(ctx, rows)
	return result, err
}

func statusesOrAll(statuses []api.Status) []api.Status {
	if len(statuses) == 0 {
		return AllStatuses
	}
	return slices.Compact(slices.Sorted(slices.Values(statuses)))
}

func seesEveryone(viewer domain.Principal) bool {
	return viewer.Admin || viewer.Permissions.ViewOthersRequests
}

func (s *Service) view(ctx context.Context, id domain.RequestID) (RequestView, error) {
	row, err := s.repo.request(s.db.Read(ctx), id)
	if err != nil {
		return RequestView{}, err
	}
	views, err := s.views(ctx, []requestRow{row})
	if err != nil {
		return RequestView{}, err
	}
	return views[0], nil
}

// views adds each requester's display name. A user Identity no longer knows shows without one.
func (s *Service) views(ctx context.Context, rows []requestRow) ([]RequestView, error) {
	ids := make([]domain.UserID, 0, len(rows))
	for _, row := range rows {
		if !slices.Contains(ids, row.UserID) {
			ids = append(ids, row.UserID)
		}
	}
	users, err := s.identity.Users(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := map[domain.UserID]string{}
	for _, u := range users {
		names[u.ID] = u.DisplayName
	}
	views := make([]RequestView, 0, len(rows))
	for _, row := range rows {
		views = append(views, RequestView{
			Request:       row.toAPI(),
			RequesterName: names[row.UserID],
			DecidedBy:     row.DecidedBy,
			DecidedAt:     row.DecidedAt,
			DeclineReason: deref(row.DeclineReason),
			FailureReason: deref(row.FailureReason),
		})
	}
	return views, nil
}

// LinkTitle links a request to the Title Catalog added or monitored for it, and builds its items.
// Unknown requests and requests no longer approved are ignored.
func (s *Service) LinkTitle(ctx context.Context, tx *gorm.DB, id domain.RequestID, title domain.TitleID) error {
	row, err := s.repo.request(tx, id)
	if errors.Is(err, api.ErrNotFound) {
		return nil
	}
	if err != nil || !slices.Contains(trackingStatuses, row.Status) {
		return err
	}
	row.TitleID = &title
	if row.Status == api.StatusApproved {
		row.Status = api.StatusProcessing
	}
	return s.track(ctx, tx, row)
}

// ExtendItems adds newly wanted episodes to the approved requests on a Title.
func (s *Service) ExtendItems(ctx context.Context, tx *gorm.DB, title domain.TitleID) error {
	rows, err := s.repo.requests(tx, requestFilter{Title: &title, Statuses: trackingStatuses})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.track(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

// track stores the request's items in scope and saves it. A new item makes an AVAILABLE request partial again.
func (s *Service) track(ctx context.Context, tx *gorm.DB, row requestRow) error {
	items, err := s.itemsInScope(ctx, row)
	if err != nil {
		return err
	}
	added, err := s.repo.addItems(tx, items)
	if err != nil {
		return err
	}
	if added && row.Status == api.StatusAvailable {
		row.Status = api.StatusPartiallyAvailable
	}
	return s.repo.save(tx, row)
}

// MarkAvailable records that Jellyfin confirmed a Title's movie or episodes, then moves each approved
// request on it: some items available is PARTIALLY_AVAILABLE, all of them AVAILABLE. A request publishes
// RequestAvailable the first time it becomes AVAILABLE, and never again.
func (s *Service) MarkAvailable(tx *gorm.DB, title domain.TitleID, episodes []domain.EpisodeID) error {
	rows, err := s.repo.requests(tx, requestFilter{Title: &title, Statuses: trackingStatuses})
	if err != nil {
		return err
	}
	now := s.now().UTC()
	for _, row := range rows {
		var confirmed []domain.EpisodeID
		if row.Kind == domain.MediaKindSeries {
			confirmed = append([]domain.EpisodeID{}, episodes...)
		}
		if err := s.repo.markItemsAvailable(tx, row.ID, confirmed, now); err != nil {
			return err
		}
		counted, err := s.repo.request(tx, row.ID)
		if err != nil {
			return err
		}
		if err := s.settle(tx, counted, now); err != nil {
			return err
		}
	}
	return nil
}

// settle derives a linked request's status from its item progress.
func (s *Service) settle(tx *gorm.DB, row requestRow, now time.Time) error {
	switch {
	case row.AvailableItems == 0:
		return nil
	case row.AvailableItems < row.TotalItems:
		row.Status = api.StatusPartiallyAvailable
	default:
		row.Status = api.StatusAvailable
	}
	first := row.Status == api.StatusAvailable && row.AvailableAt == nil
	if first {
		row.AvailableAt = &now
	}
	if err := s.repo.save(tx, row); err != nil {
		return err
	}
	if !first {
		return nil
	}
	return s.publish(tx, api.RequestAvailable{Request: row.ID, User: row.UserID, Title: *row.TitleID})
}

// itemsInScope is the movie, or every wanted (monitored and released) episode in the request's seasons.
func (s *Service) itemsInScope(ctx context.Context, row requestRow) ([]itemRow, error) {
	if row.Kind == domain.MediaKindMovie {
		return []itemRow{{RequestID: row.ID}}, nil
	}
	wanted, _, err := s.catalog.Wanted(ctx, *row.TitleID)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := []itemRow{}
	for _, e := range wanted {
		if row.Scope == api.ScopeAllSeasons || slices.Contains(row.Seasons, e.Season) {
			items = append(items, itemRow{RequestID: row.ID, EpisodeID: &e.ID})
		}
	}
	return items, nil
}

// Fail marks an approved request FAILED when Catalog could not add its Title.
func (s *Service) Fail(tx *gorm.DB, id domain.RequestID, reason string) error {
	row, err := s.repo.request(tx, id)
	if errors.Is(err, api.ErrNotFound) {
		return nil
	}
	if err != nil || row.Status != api.StatusApproved {
		return err
	}
	row.Status, row.FailureReason = api.StatusFailed, &reason
	return s.repo.save(tx, row)
}

func (s *Service) publish(tx *gorm.DB, m domain.Message) error {
	return s.bus.Publish(tx, bus.EventOf(m))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
