package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/settings"
	requestsapi "magpie/internal/requests/api"

	"gorm.io/gorm"
)

const (
	// fullRefreshAge forces a full metadata refresh even when TMDB reports no change.
	fullRefreshAge = 30 * 24 * time.Hour
	// changesWindow is the longest span TMDB's /changes accepts.
	changesWindow = 14 * 24 * time.Hour
	// imageMaxAgeMonths keeps cached images inside TMDB's 6-month cache limit.
	imageMaxAgeMonths = 5

	discoverPosterSize = "w342"
)

var ErrSeriesUnsupported = errors.New("catalog: series are not supported yet")

// imageKinds is the order images are cached in.
var imageKinds = []string{api.ImageKindPoster, api.ImageKindBackdrop, api.ImageKindLogo}

// Service owns every change to Catalog's tables and the events that go with it.
type Service struct {
	repo     *Repository
	db       *db.DB
	bus      *bus.Bus
	tmdb     TMDB
	settings *settings.Store
	images   *ImageCache
	now      func() time.Time
}

func NewService(repo *Repository, d *db.DB, b *bus.Bus, tmdb TMDB, store *settings.Store, images *ImageCache) *Service {
	return &Service{repo: repo, db: d, bus: b, tmdb: tmdb, settings: store, images: images, now: time.Now}
}

// TitleView is a Title as the HTTP layer shows it.
type TitleView struct {
	api.Title
	Overview string
	AddedAt  time.Time
	// Images lists the cached image kinds.
	Images  []string
	Seasons []SeasonView
}

type SeasonView struct {
	Number    int
	Monitored bool
	Episodes  []api.Episode
}

// TitleUpdate fields that are nil stay as they are.
type TitleUpdate struct {
	Profile              *domain.ProfileID
	Monitored            *bool
	MonitorFutureSeasons *bool
}

// DiscoverItem is one TMDB result, marked with its Title when it's already in the library.
type DiscoverItem struct {
	Item TmdbSearchItem
	// Title is set when the item is already in the library.
	Title     *domain.TitleID
	PosterURL string
}

func (s *Service) Title(ctx context.Context, id domain.TitleID) (TitleView, error) {
	return s.view(s.db.Read(ctx), id)
}

func (s *Service) Titles(ctx context.Context, q TitleQuery) ([]TitleView, error) {
	conn := s.db.Read(ctx)
	rows, err := s.repo.titleRows(conn, q)
	if err != nil {
		return nil, err
	}
	views := make([]TitleView, 0, len(rows))
	for _, row := range rows {
		view, err := s.summary(conn, row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) summary(conn *gorm.DB, row titleRow) (TitleView, error) {
	title, err := row.toAPI()
	if err != nil {
		return TitleView{}, err
	}
	images, err := s.repo.imageRows(conn, title.ID)
	if err != nil {
		return TitleView{}, err
	}
	view := TitleView{Title: title, Overview: row.Overview, AddedAt: row.AddedAt.UTC(), Images: []string{}, Seasons: []SeasonView{}}
	for _, img := range images {
		view.Images = append(view.Images, img.Kind)
	}
	return view, nil
}

func (s *Service) view(conn *gorm.DB, id domain.TitleID) (TitleView, error) {
	row, err := s.repo.title(conn, id)
	if err != nil {
		return TitleView{}, err
	}
	view, err := s.summary(conn, row)
	if err != nil {
		return TitleView{}, err
	}
	seasons, err := s.repo.seasonRows(conn, id)
	if err != nil {
		return TitleView{}, err
	}
	episodes, err := s.repo.episodeRows(conn.Where("title_id = ?", id))
	if err != nil {
		return TitleView{}, err
	}
	for _, season := range seasons {
		sv := SeasonView{Number: season.Number, Monitored: season.Monitored, Episodes: []api.Episode{}}
		for _, e := range episodes {
			if e.SeasonNumber == season.Number {
				sv.Episodes = append(sv.Episodes, e.toAPI())
			}
		}
		view.Seasons = append(view.Seasons, sv)
	}
	return view, nil
}

// MonitorTracked handles a RequestApproved for a Title Catalog already tracks: it monitors it and reports
// handled. When the Title isn't tracked, handled is false and the caller fetches it from TMDB.
func (s *Service) MonitorTracked(tx *gorm.DB, req requestsapi.RequestApproved) (handled bool, err error) {
	if req.Kind != domain.MediaKindMovie {
		return false, fmt.Errorf("%w: request %s", ErrSeriesUnsupported, req.Request)
	}
	row, err := s.repo.titleByTmdb(tx, req.Kind, req.Tmdb)
	if errors.Is(err, api.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, s.monitorForRequest(tx, row, req)
}

// monitorForRequest is a no-op for the request that created the Title, so a redelivered request adds nothing.
func (s *Service) monitorForRequest(tx *gorm.DB, row titleRow, req requestsapi.RequestApproved) error {
	if row.RequestID != nil && *row.RequestID == string(req.Request) {
		return nil
	}
	row.Monitored = true
	if err := s.repo.saveTitle(tx, row); err != nil {
		return err
	}
	return s.publish(tx, api.MonitoringChanged{
		Title:     domain.TitleID(row.ID),
		Added:     []domain.EpisodeID{},
		Removed:   []domain.EpisodeID{},
		Movie:     ptr(true),
		RequestID: ptr(req.Request),
	})
}

// AddRequestedMovie fetches a requested movie from TMDB, stores it monitored with the request's profile and
// publishes TitleAdded. Images are cached after the commit; a failed download is retried by the refresh job.
func (s *Service) AddRequestedMovie(ctx context.Context, req requestsapi.RequestApproved) error {
	meta, err := metadataSetting.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	movie, err := s.tmdb.Movie(ctx, req.Tmdb, meta.Language)
	if err != nil {
		return err
	}

	var added *domain.TitleID
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		existing, err := s.repo.titleByTmdb(tx, domain.MediaKindMovie, req.Tmdb)
		if err == nil {
			return s.monitorForRequest(tx, existing, req)
		}
		if !errors.Is(err, api.ErrNotFound) {
			return err
		}

		row, err := s.movieRow(movie, meta.ReleaseRegion)
		if err != nil {
			return err
		}
		row.ProfileID = string(req.Profile)
		row.Monitored = true
		row.Origin = string(domain.OriginRequest)
		row.RequestedBy = ptr(string(req.User))
		row.RequestID = ptr(string(req.Request))
		if err := s.repo.createTitle(tx, row); err != nil {
			return err
		}
		added = ptr(domain.TitleID(row.ID))
		return s.publish(tx, api.TitleAdded{
			Title:       domain.TitleID(row.ID),
			Kind:        domain.MediaKindMovie,
			External:    domain.ExternalIDs{Tmdb: req.Tmdb, Tvdb: row.TvdbID, Imdb: row.ImdbID},
			Profile:     req.Profile,
			Monitored:   []domain.EpisodeID{},
			Released:    row.Released,
			RequestedBy: ptr(req.User),
			RequestID:   ptr(req.Request),
			Origin:      domain.OriginRequest,
		})
	})
	if err != nil || added == nil {
		return err
	}
	if err := s.cacheImages(ctx, *added, movie.Images); err != nil {
		slog.WarnContext(ctx, "caching title images failed; the refresh job retries", "title", *added, "error", err)
	}
	return nil
}

// FailRequestedTitle tells Requests the Title could not be added.
func (s *Service) FailRequestedTitle(ctx context.Context, req requestsapi.RequestApproved, cause error) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		return s.publish(tx, api.TitleAddFailed{
			Tmdb:      req.Tmdb,
			Kind:      req.Kind,
			RequestID: ptr(req.Request),
			Reason:    failureReason(cause),
		})
	})
}

func failureReason(err error) string {
	switch {
	case errors.Is(err, ErrTmdbNotFound):
		return api.TitleAddFailedTmdbNotFound
	case errors.Is(err, ErrTmdbUnauthorized):
		return api.TitleAddFailedTmdbUnauthorized
	default:
		return api.TitleAddFailedTmdbUnavailable
	}
}

func isTmdbError(err error) bool {
	return errors.Is(err, ErrTmdbNotFound) || errors.Is(err, ErrTmdbUnauthorized) || errors.Is(err, ErrTmdbUnavailable)
}

func (s *Service) movieRow(movie TmdbMovie, region string) (titleRow, error) {
	now := s.now().UTC()
	aliases, err := encodeAliases(movie.AlternativeTitles)
	if err != nil {
		return titleRow{}, err
	}
	row := titleRow{
		ID:               string(domain.NewID[domain.TitleID]()),
		Kind:             string(domain.MediaKindMovie),
		TmdbID:           int64(movie.ID),
		ExternalIDSource: ExternalIDSourceTMDB,
		Aliases:          aliases,
		AddedAt:          now,
	}
	applyMovie(&row, movie, region, now)
	if releasedBy(row.releaseDates(), now) {
		row.Released, row.ReleasedAt = true, &now
	}
	return row, nil
}

// applyMovie copies TMDB metadata onto a row. A released movie stays released when dates move.
func applyMovie(row *titleRow, movie TmdbMovie, region string, now time.Time) {
	row.Name = movie.Title
	row.OriginalName = movie.OriginalTitle
	row.Overview = movie.Overview
	row.OriginalLanguage = string(movie.OriginalLanguage)
	row.RuntimeMinutes = movie.RuntimeMinutes
	if movie.ReleaseDate != nil {
		row.Year = movie.ReleaseDate.Year()
	}
	if movie.External.Imdb != nil || movie.External.Tvdb != nil {
		row.ImdbID, row.TvdbID, row.ExternalIDSource = movie.External.Imdb, movie.External.Tvdb, ExternalIDSourceTMDB
	}
	dates := resolveReleaseDates(movie.ReleaseDates, region)
	row.ReleaseTheatricalAt, row.ReleaseDigitalAt, row.ReleasePhysicalAt = dates.Theatrical, dates.Digital, dates.Physical
	row.MetadataRefreshedAt = now
}

// RunReleaseClock marks movies whose release point has passed as released and publishes one
// WantedReleased per monitored Title. Unmonitored movies are marked without an event.
func (s *Service) RunReleaseClock(ctx context.Context) error {
	now := s.now().UTC()
	movie, unreleased := domain.MediaKindMovie, false
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		rows, err := s.repo.titleRows(tx, TitleQuery{Filter: api.TitleFilter{Kind: &movie, Released: &unreleased}})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if !releasedBy(row.releaseDates(), now) {
				continue
			}
			row.Released, row.ReleasedAt = true, &now
			if err := s.repo.saveTitle(tx, row); err != nil {
				return err
			}
			if !row.Monitored {
				continue
			}
			if err := s.publish(tx, api.WantedReleased{Title: domain.TitleID(row.ID), Episodes: []domain.EpisodeID{}}); err != nil {
				return err
			}
		}
		return nil
	})
}

// RefreshMovies refreshes movies TMDB reports as changed and any not fully refreshed for 30 days, and
// re-fetches stale images for the rest. An invalid key pauses refreshing until TMDB is healthy again.
func (s *Service) RefreshMovies(ctx context.Context) error {
	if paused, err := refreshPaused.Get(ctx, s.settings); err != nil || paused {
		return err
	}
	interval, err := movieRefreshInterval.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	changedIDs, err := s.tmdb.Changes(ctx, domain.MediaKindMovie, now.Add(-min(interval.Std(), changesWindow)))
	if err != nil {
		return s.pauseOnUnauthorized(ctx, err)
	}
	changed := map[domain.TmdbID]bool{}
	for _, id := range changedIDs {
		changed[id] = true
	}

	movie := domain.MediaKindMovie
	rows, err := s.repo.titleRows(s.db.Read(ctx), TitleQuery{Filter: api.TitleFilter{Kind: &movie}})
	if err != nil {
		return err
	}
	var errs []error
	for _, row := range rows {
		id := domain.TitleID(row.ID)
		if changed[domain.TmdbID(row.TmdbID)] || row.MetadataRefreshedAt.Before(now.Add(-fullRefreshAge)) {
			err = s.RefreshTitle(ctx, id)
		} else {
			err = s.refreshStaleImages(ctx, id)
		}
		if errors.Is(err, ErrTmdbUnauthorized) {
			return s.pauseOnUnauthorized(ctx, err)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("title %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) pauseOnUnauthorized(ctx context.Context, err error) error {
	if !errors.Is(err, ErrTmdbUnauthorized) {
		return err
	}
	slog.WarnContext(ctx, "TMDB rejected the key; metadata refresh paused until the connection is healthy", "error", err)
	return refreshPaused.Set(ctx, s.settings, true)
}

// ResumeRefresh clears the pause set by an invalid TMDB key.
func (s *Service) ResumeRefresh(tx *gorm.DB) error {
	return refreshPaused.SetTx(tx, false)
}

// RefreshPaused reports whether an invalid key paused metadata refresh.
func (s *Service) RefreshPaused(ctx context.Context) (bool, error) {
	return refreshPaused.Get(ctx, s.settings)
}

// RefreshTitle re-reads a Title's metadata and images from TMDB.
func (s *Service) RefreshTitle(ctx context.Context, id domain.TitleID) error {
	row, err := s.repo.title(s.db.Read(ctx), id)
	if err != nil {
		return err
	}
	if domain.MediaKind(row.Kind) != domain.MediaKindMovie {
		return fmt.Errorf("%w: title %s", ErrSeriesUnsupported, id)
	}
	meta, err := metadataSetting.Get(ctx, s.settings)
	if err != nil {
		return err
	}
	movie, err := s.tmdb.Movie(ctx, domain.TmdbID(row.TmdbID), meta.Language)
	if err != nil {
		return err
	}
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		current, err := s.repo.title(tx, id)
		if err != nil {
			return err
		}
		aliases, err := encodeAliases(movie.AlternativeTitles)
		if err != nil {
			return err
		}
		current.Aliases = aliases
		applyMovie(&current, movie, meta.ReleaseRegion, s.now().UTC())
		return s.repo.saveTitle(tx, current)
	})
	if err != nil {
		return err
	}
	return s.cacheImages(ctx, id, movie.Images)
}

// refreshStaleImages re-downloads cached images older than five months from their stored TMDB paths.
func (s *Service) refreshStaleImages(ctx context.Context, id domain.TitleID) error {
	rows, err := s.repo.imageRows(s.db.Read(ctx), id)
	if err != nil {
		return err
	}
	stored := TmdbImages{}
	for _, row := range rows {
		setImagePath(&stored, row.Kind, row.TmdbPath)
	}
	return s.cacheImages(ctx, id, stored)
}

// cacheImages downloads images that are new, changed or stale, and drops ones TMDB no longer has.
func (s *Service) cacheImages(ctx context.Context, id domain.TitleID, wanted TmdbImages) error {
	rows, err := s.repo.imageRows(s.db.Read(ctx), id)
	if err != nil {
		return err
	}
	cached := map[string]imageRow{}
	for _, row := range rows {
		cached[row.Kind] = row
	}
	staleBefore := s.now().UTC().AddDate(0, -imageMaxAgeMonths, 0)

	var errs []error
	for _, kind := range imageKinds {
		path := imagePath(wanted, kind)
		row, isCached := cached[kind]
		switch {
		case path == "" && isCached:
			errs = append(errs, s.dropImage(ctx, id, kind))
		case path == "":
		case isCached && row.TmdbPath == path && !row.FetchedAt.Before(staleBefore):
		default:
			errs = append(errs, s.downloadImage(ctx, id, kind, path))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) downloadImage(ctx context.Context, id domain.TitleID, kind, path string) error {
	local, err := s.images.Download(ctx, string(id), kind, path)
	if err != nil {
		return fmt.Errorf("%s image: %w", kind, err)
	}
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		if _, err := s.repo.title(tx, id); err != nil {
			return err
		}
		return s.repo.saveImage(tx, imageRow{TitleID: string(id), Kind: kind, TmdbPath: path, LocalPath: local, FetchedAt: s.now().UTC()})
	})
}

func (s *Service) dropImage(ctx context.Context, id domain.TitleID, kind string) error {
	err := s.db.Write(ctx, func(tx *gorm.DB) error { return s.repo.deleteImage(tx, id, kind) })
	return errors.Join(err, s.images.RemoveKind(string(id), kind))
}

func imagePath(images TmdbImages, kind string) string {
	switch kind {
	case api.ImageKindPoster:
		return images.Poster
	case api.ImageKindBackdrop:
		return images.Backdrop
	default:
		return images.Logo
	}
}

func setImagePath(images *TmdbImages, kind, path string) {
	switch kind {
	case api.ImageKindPoster:
		images.Poster = path
	case api.ImageKindBackdrop:
		images.Backdrop = path
	default:
		images.Logo = path
	}
}

// UpdateTitle applies an admin's change and publishes ProfileChanged and, for movies, MonitoringChanged.
func (s *Service) UpdateTitle(ctx context.Context, id domain.TitleID, update TitleUpdate) (TitleView, error) {
	var view TitleView
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.title(tx, id)
		if err != nil {
			return err
		}
		var events []domain.Message
		if update.Profile != nil && string(*update.Profile) != row.ProfileID {
			events = append(events, api.ProfileChanged{Title: id, From: domain.ProfileID(row.ProfileID), To: *update.Profile})
			row.ProfileID = string(*update.Profile)
		}
		if update.Monitored != nil && *update.Monitored != row.Monitored {
			row.Monitored = *update.Monitored
			if domain.MediaKind(row.Kind) == domain.MediaKindMovie {
				events = append(events, api.MonitoringChanged{Title: id, Added: []domain.EpisodeID{}, Removed: []domain.EpisodeID{}, Movie: ptr(row.Monitored)})
			}
		}
		if update.MonitorFutureSeasons != nil {
			row.MonitorFutureSeasons = *update.MonitorFutureSeasons
		}
		if err := s.repo.saveTitle(tx, row); err != nil {
			return err
		}
		for _, ev := range events {
			if err := s.publish(tx, ev); err != nil {
				return err
			}
		}
		view, err = s.view(tx, id)
		return err
	})
	return view, err
}

// RemoveTitle deletes the Title's rows and cached images and publishes TitleRemoved. Other contexts
// clean up their own data, including files when deleteFiles is set.
func (s *Service) RemoveTitle(ctx context.Context, id domain.TitleID, deleteFiles bool) error {
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		if _, err := s.repo.title(tx, id); err != nil {
			return err
		}
		if err := s.repo.deleteTitle(tx, id); err != nil {
			return err
		}
		return s.publish(tx, api.TitleRemoved{Title: id, DeleteFiles: deleteFiles})
	})
	if err != nil {
		return err
	}
	return s.images.Remove(string(id))
}

// Discover searches TMDB, or lists this week's trending titles for an empty query.
func (s *Service) Discover(ctx context.Context, query string) ([]DiscoverItem, error) {
	var items []TmdbSearchItem
	if query = strings.TrimSpace(query); query == "" {
		trending, err := s.tmdb.Trending(ctx, TmdbTrendingWeek)
		if err != nil {
			return nil, err
		}
		items = trending
	} else {
		page, err := s.tmdb.Search(ctx, query, 1)
		if err != nil {
			return nil, err
		}
		items = page.Items
	}

	conn := s.db.Read(ctx)
	idsByKind := map[domain.MediaKind][]domain.TmdbID{}
	for _, item := range items {
		idsByKind[item.Kind] = append(idsByKind[item.Kind], item.ID)
	}
	trackedByKind := map[domain.MediaKind]map[domain.TmdbID]domain.TitleID{}
	for kind, ids := range idsByKind {
		tracked, err := s.repo.tracked(conn, kind, ids)
		if err != nil {
			return nil, err
		}
		trackedByKind[kind] = tracked
	}

	results := make([]DiscoverItem, 0, len(items))
	for _, item := range items {
		result := DiscoverItem{Item: item}
		if item.PosterPath != "" {
			result.PosterURL = s.tmdb.ImageURL(item.PosterPath, discoverPosterSize)
		}
		if id, ok := trackedByKind[item.Kind][item.ID]; ok {
			result.Title = &id
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) Settings(ctx context.Context) (MetadataSettings, error) {
	return metadataSetting.Get(ctx, s.settings)
}

func (s *Service) UpdateSettings(ctx context.Context, m MetadataSettings) (MetadataSettings, error) {
	if err := m.Validate(); err != nil {
		return MetadataSettings{}, err
	}
	return m, metadataSetting.Set(ctx, s.settings, m)
}

func (s *Service) publish(tx *gorm.DB, m domain.Message) error {
	return s.bus.Publish(tx, bus.EventOf(m))
}
