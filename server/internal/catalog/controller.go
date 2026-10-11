package catalog

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagCatalog = "catalog"

	pathTitleID   = "id"
	pathImageID   = "titleId"
	pathImageKind = "kind"

	queryQ           = "q"
	queryKind        = "kind"
	queryMonitored   = "monitored"
	queryReleased    = "released"
	querySort        = "sort"
	queryDeleteFiles = "deleteFiles"
	queryWidth       = "w"

	imagesRoute       = httpserver.APIPrefix + "/catalog/images/"
	headerCache       = "Cache-Control"
	imageCacheControl = "public, max-age=86400"
	contentTypeJPEG   = "image/jpeg"
	contentTypePNG    = "image/png"

	attributionNotice = "This product uses the TMDB API but is not endorsed or certified by TMDB."
	attributionLink   = "https://www.themoviedb.org"
	svgDataURIPrefix  = "data:image/svg+xml;base64,"

	pathSeason    = "n"
	pathEpisodeID = "episodeId"

	TypeTitleNotFound       problem.Type = "urn:magpie:problem:title-not-found"
	TypeSeasonNotFound      problem.Type = "urn:magpie:problem:season-not-found"
	TypeEpisodeNotFound     problem.Type = "urn:magpie:problem:episode-not-found"
	TypeNotASeries          problem.Type = "urn:magpie:problem:not-a-series"
	TypeUnknownEpisodeGroup problem.Type = "urn:magpie:problem:unknown-episode-group"
	TypeInvalidLanguages    problem.Type = "urn:magpie:problem:invalid-wanted-languages"
	TypeImageNotFound       problem.Type = "urn:magpie:problem:image-not-found"
	TypeTmdbUnavailable     problem.Type = "urn:magpie:problem:tmdb-unavailable"
	TypeInvalidSettings     problem.Type = "urn:magpie:problem:invalid-catalog-settings"
)

// EpisodeGroupType names TMDB's episode group types.
type EpisodeGroupType string

const (
	EpisodeGroupTypeOriginalAirDate EpisodeGroupType = "ORIGINAL_AIR_DATE"
	EpisodeGroupTypeAbsolute        EpisodeGroupType = "ABSOLUTE"
	EpisodeGroupTypeDVD             EpisodeGroupType = "DVD"
	EpisodeGroupTypeDigital         EpisodeGroupType = "DIGITAL"
	EpisodeGroupTypeStoryArc        EpisodeGroupType = "STORY_ARC"
	EpisodeGroupTypeProduction      EpisodeGroupType = "PRODUCTION"
	EpisodeGroupTypeTV              EpisodeGroupType = "TV"
	EpisodeGroupTypeUnknown         EpisodeGroupType = "UNKNOWN"
)

var episodeGroupTypes = map[TmdbEpisodeGroupType]EpisodeGroupType{
	TmdbGroupOriginalAirDate: EpisodeGroupTypeOriginalAirDate,
	TmdbGroupAbsolute:        EpisodeGroupTypeAbsolute,
	TmdbGroupDVD:             EpisodeGroupTypeDVD,
	TmdbGroupDigital:         EpisodeGroupTypeDigital,
	TmdbGroupStoryArc:        EpisodeGroupTypeStoryArc,
	TmdbGroupProduction:      EpisodeGroupTypeProduction,
	TmdbGroupTV:              EpisodeGroupTypeTV,
}

//go:embed tmdb-logo.svg
var tmdbLogo []byte

var (
	mediaKinds = []domain.MediaKind{domain.MediaKindMovie, domain.MediaKindSeries}
	titleSorts = []TitleSort{TitleSortName, TitleSortAdded}
)

type ReleaseDatesResponse struct {
	Theatrical *time.Time `json:"theatrical"`
	Digital    *time.Time `json:"digital"`
	Physical   *time.Time `json:"physical"`
}

// TitleImagesResponse holds image URLs for cached images; add ?w= for a resized copy.
type TitleImagesResponse struct {
	Poster   *string `json:"poster"`
	Backdrop *string `json:"backdrop"`
	Logo     *string `json:"logo"`
}

type EpisodeResponse struct {
	ID             domain.EpisodeID `json:"id" validate:"required"`
	Season         int              `json:"season" validate:"required"`
	Number         int              `json:"number" validate:"required"`
	AiredSeason    int              `json:"airedSeason" validate:"required"`
	AiredNumber    int              `json:"airedNumber" validate:"required"`
	Name           string           `json:"name" validate:"required"`
	AirDate        *time.Time       `json:"airDate"`
	RuntimeMinutes int              `json:"runtimeMinutes" validate:"required"`
	Monitored      bool             `json:"monitored" validate:"required"`
	Released       bool             `json:"released" validate:"required"`
}

type SeasonResponse struct {
	Number    int               `json:"number" validate:"required"`
	Monitored bool              `json:"monitored" validate:"required"`
	Episodes  []EpisodeResponse `json:"episodes" validate:"required"`
}

type TitleResponse struct {
	ID                   domain.TitleID       `json:"id" validate:"required"`
	Kind                 domain.MediaKind     `json:"kind" validate:"required"`
	TmdbID               domain.TmdbID        `json:"tmdbId" validate:"required"`
	TvdbID               *string              `json:"tvdbId"`
	ImdbID               *string              `json:"imdbId"`
	Name                 string               `json:"name" validate:"required"`
	OriginalName         string               `json:"originalName" validate:"required"`
	Overview             string               `json:"overview" validate:"required"`
	Year                 int                  `json:"year" validate:"required"`
	RuntimeMinutes       int                  `json:"runtimeMinutes" validate:"required"`
	OriginalLanguage     domain.Language      `json:"originalLanguage" validate:"required"`
	SeriesStatus         string               `json:"seriesStatus" validate:"required"`
	ProfileID            domain.ProfileID     `json:"profileId" validate:"required"`
	Monitored            bool                 `json:"monitored" validate:"required"`
	MonitorFutureSeasons bool                 `json:"monitorFutureSeasons" validate:"required"`
	Released             bool                 `json:"released" validate:"required"`
	ReleasedAt           *time.Time           `json:"releasedAt"`
	ReleaseDates         ReleaseDatesResponse `json:"releaseDates" validate:"required"`
	Origin               domain.Origin        `json:"origin" validate:"required"`
	RequestedBy          *domain.UserID       `json:"requestedBy"`
	AddedAt              time.Time            `json:"addedAt" validate:"required"`
	Images               TitleImagesResponse  `json:"images" validate:"required"`
	// EpisodeGroupID is the TMDB episode group numbering the episodes; "" is aired order.
	EpisodeGroupID string `json:"episodeGroupId" validate:"required"`
	// LanguagesOverride is null when the Title uses the global wanted languages.
	LanguagesOverride []WantedLanguageEntry `json:"languagesOverride"`
	// Seasons is empty for movies and in lists.
	Seasons []SeasonResponse `json:"seasons" validate:"required"`
}

type WantedLanguageEntry struct {
	// Language is an ISO 639-1 code.
	Language domain.Language `json:"language" validate:"required"`
	// Forced also wants a forced track.
	Forced bool `json:"forced" validate:"required"`
	// HearingImpaired accepts the hearing-impaired variant.
	HearingImpaired bool `json:"hearingImpaired" validate:"required"`
}

type LanguageOptionResponse struct {
	Code domain.Language `json:"code" validate:"required"`
	Name string          `json:"name" validate:"required"`
}

type WantedLanguagesResponse struct {
	// Languages is the global list, most wanted first.
	Languages []WantedLanguageEntry `json:"languages" validate:"required"`
	// Available lists the ISO 639-1 languages a list may hold.
	Available []LanguageOptionResponse `json:"available" validate:"required"`
}

type UpdateWantedLanguagesRequest struct {
	Languages []WantedLanguageEntry `json:"languages" validate:"required"`
}

type UpdateTitleLanguagesRequest struct {
	// Languages overrides the global list for this Title; null goes back to the global list.
	Languages []WantedLanguageEntry `json:"languages"`
}

type UpdateMonitoredRequest struct {
	Monitored bool `json:"monitored"`
}

type EpisodeGroupResponse struct {
	ID           string           `json:"id" validate:"required"`
	Name         string           `json:"name" validate:"required"`
	Type         EpisodeGroupType `json:"type" validate:"required"`
	EpisodeCount int              `json:"episodeCount" validate:"required"`
	GroupCount   int              `json:"groupCount" validate:"required"`
}

type SelectEpisodeGroupRequest struct {
	// GroupID is a TMDB episode group of this series, or "" for aired order.
	GroupID string `json:"groupId"`
}

type UpdateTitleRequest struct {
	ProfileID            *domain.ProfileID `json:"profileId,omitempty"`
	Monitored            *bool             `json:"monitored,omitempty"`
	MonitorFutureSeasons *bool             `json:"monitorFutureSeasons,omitempty"`
}

type RefreshTitleResponse struct {
	Queued bool `json:"queued" validate:"required"`
}

type RemoveTitleResponse struct {
	ID          domain.TitleID `json:"id" validate:"required"`
	DeleteFiles bool           `json:"deleteFiles" validate:"required"`
}

type DiscoverItemResponse struct {
	TmdbID    domain.TmdbID    `json:"tmdbId" validate:"required"`
	Kind      domain.MediaKind `json:"kind" validate:"required"`
	Name      string           `json:"name" validate:"required"`
	Year      int              `json:"year" validate:"required"`
	Overview  string           `json:"overview" validate:"required"`
	PosterURL *string          `json:"posterUrl"`
	InLibrary bool             `json:"inLibrary" validate:"required"`
	// TitleID is set when InLibrary.
	TitleID *domain.TitleID `json:"titleId"`
}

type DiscoverResponse struct {
	// Trending is true when the query was empty and the items are TMDB's trending titles this week.
	Trending bool                   `json:"trending" validate:"required"`
	Items    []DiscoverItemResponse `json:"items" validate:"required"`
}

type CatalogSettingsResponse struct {
	Language      string `json:"language" validate:"required"`
	ReleaseRegion string `json:"releaseRegion" validate:"required"`
}

type UpdateCatalogSettingsRequest struct {
	Language      string `json:"language" validate:"required"`
	ReleaseRegion string `json:"releaseRegion" validate:"required"`
}

type AttributionResponse struct {
	Notice string `json:"notice" validate:"required"`
	// LogoURL is a data URI of TMDB's logo.
	LogoURL string `json:"logoUrl" validate:"required"`
	Link    string `json:"link" validate:"required"`
}

type Controller struct {
	service *Service
	repo    *Repository
	queue   jobs.Enqueuer
}

func NewController(s *Service, repo *Repository, queue jobs.Enqueuer) *Controller {
	return &Controller{service: s, repo: repo, queue: queue}
}

func (c *Controller) Register(server *fuego.Server) {
	api := fuego.Group(server, "/catalog")
	fuego.Get(api, "/titles", c.listTitles,
		fuego.OptionOperationID("listTitles"),
		fuego.OptionSummary("List Titles in the library"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionQuery(queryQ, "Match the name, original name or an alias"),
		fuego.OptionQuery(queryKind, "MOVIE or SERIES"),
		fuego.OptionQueryBool(queryMonitored, "Only monitored (true) or unmonitored (false) Titles"),
		fuego.OptionQueryBool(queryReleased, "Only released (true) or unreleased (false) Titles"),
		fuego.OptionQuery(querySort, "NAME (default) or ADDED, newest first"),
	)
	fuego.Get(api, "/titles/{id}", c.getTitle,
		fuego.OptionOperationID("getTitle"),
		fuego.OptionSummary("Get one Title with its seasons and episodes"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
	)
	fuego.Patch(api, "/titles/{id}", c.updateTitle,
		fuego.OptionOperationID("updateTitle"),
		fuego.OptionSummary("Change a Title's profile or monitoring (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
	)
	fuego.Post(api, "/titles/{id}/refresh", c.refreshTitle,
		fuego.OptionOperationID("refreshTitle"),
		fuego.OptionSummary("Refresh a Title's metadata and images from TMDB now (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted),
	)
	fuego.Patch(api, "/titles/{id}/seasons/{n}", c.updateSeason,
		fuego.OptionOperationID("updateSeason"),
		fuego.OptionSummary("Monitor or unmonitor a season and all its episodes (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
		fuego.OptionPath(pathSeason, "Season number in the current numbering"),
	)
	fuego.Patch(api, "/titles/{id}/episodes/{episodeId}", c.updateEpisode,
		fuego.OptionOperationID("updateEpisode"),
		fuego.OptionSummary("Monitor or unmonitor one episode (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
		fuego.OptionPath(pathEpisodeID, "Episode ID"),
	)
	fuego.Get(api, "/titles/{id}/episode-groups", c.listEpisodeGroups,
		fuego.OptionOperationID("listEpisodeGroups"),
		fuego.OptionSummary("List TMDB's episode groups for a series"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
	)
	fuego.Put(api, "/titles/{id}/episode-group", c.selectEpisodeGroup,
		fuego.OptionOperationID("selectEpisodeGroup"),
		fuego.OptionSummary("Number a series by a TMDB episode group, or by aired order for an empty ID (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
	)
	fuego.Put(api, "/titles/{id}/languages", c.updateTitleLanguages,
		fuego.OptionOperationID("updateTitleLanguages"),
		fuego.OptionSummary("Set or clear a Title's wanted-languages override (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
	)
	fuego.Get(api, "/languages", c.getLanguages,
		fuego.OptionOperationID("getWantedLanguages"),
		fuego.OptionSummary("Get the global wanted languages and the languages a list may hold"),
		fuego.OptionTags(tagCatalog),
	)
	fuego.Put(api, "/languages", c.updateLanguages,
		fuego.OptionOperationID("updateWantedLanguages"),
		fuego.OptionSummary("Replace the global wanted languages, most wanted first (admin)"),
		fuego.OptionTags(tagCatalog),
	)
	fuego.Delete(api, "/titles/{id}", c.removeTitle,
		fuego.OptionOperationID("removeTitle"),
		fuego.OptionSummary("Remove a Title from Magpie (admin)"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathTitleID, "Title ID"),
		fuego.OptionQueryBool(queryDeleteFiles, "Also delete its files from disk"),
	)
	fuego.GetStd(api, "/images/{titleId}/{kind}", c.serveImage,
		fuego.OptionOperationID("getTitleImage"),
		fuego.OptionSummary("Serve a cached poster, backdrop or logo, resized to the width asked for"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionPath(pathImageID, "Title ID"),
		fuego.OptionPath(pathImageKind, "POSTER, BACKDROP or LOGO"),
		fuego.OptionQueryInt(queryWidth, "Width in pixels; rounds up to a cached size"),
		fuego.OptionAddResponse(http.StatusOK, "The image", fuego.Response{Type: "", ContentTypes: []string{contentTypeJPEG, contentTypePNG}}),
	)
	fuego.Get(api, "/discover", c.discover,
		fuego.OptionOperationID("discoverTitles"),
		fuego.OptionSummary("Search TMDB, or list this week's trending titles for an empty query"),
		fuego.OptionTags(tagCatalog),
		fuego.OptionQuery(queryQ, "Search text; empty for trending"),
	)
	fuego.Get(api, "/settings", c.getSettings,
		fuego.OptionOperationID("getCatalogSettings"),
		fuego.OptionSummary("Get the metadata language and release region"),
		fuego.OptionTags(tagCatalog),
	)
	fuego.Put(api, "/settings", c.updateSettings,
		fuego.OptionOperationID("updateCatalogSettings"),
		fuego.OptionSummary("Set the metadata language and release region (admin)"),
		fuego.OptionTags(tagCatalog),
	)
	fuego.Get(api, "/attribution", c.attribution,
		fuego.OptionOperationID("getAttribution"),
		fuego.OptionSummary("TMDB logo and notice for the About page"),
		fuego.OptionTags(tagCatalog),
	)
}

func (c *Controller) listTitles(ctx fuego.ContextNoBody) ([]TitleResponse, error) {
	query, err := titleQuery(ctx)
	if err != nil {
		return nil, err
	}
	views, err := c.service.Titles(ctx.Context(), query)
	if err != nil {
		return nil, err
	}
	response := make([]TitleResponse, 0, len(views))
	for _, v := range views {
		response = append(response, titleResponse(v))
	}
	return response, nil
}

func titleQuery(ctx fuego.ContextNoBody) (TitleQuery, error) {
	q := TitleQuery{Search: ctx.QueryParam(queryQ), Sort: TitleSortName}
	if raw := ctx.QueryParam(queryKind); raw != "" {
		kind := domain.MediaKind(raw)
		if !slices.Contains(mediaKinds, kind) {
			return q, badRequest("kind must be MOVIE or SERIES")
		}
		q.Filter.Kind = &kind
	}
	for name, target := range map[string]**bool{queryMonitored: &q.Filter.Monitored, queryReleased: &q.Filter.Released} {
		if ctx.QueryParam(name) == "" {
			continue
		}
		value, err := ctx.QueryParamBoolErr(name)
		if err != nil {
			return q, badRequest(name + " must be true or false")
		}
		*target = &value
	}
	if raw := ctx.QueryParam(querySort); raw != "" {
		q.Sort = TitleSort(raw)
		if !slices.Contains(titleSorts, q.Sort) {
			return q, badRequest("sort must be NAME or ADDED")
		}
	}
	return q, nil
}

func (c *Controller) getTitle(ctx fuego.ContextNoBody) (TitleResponse, error) {
	view, err := c.service.Title(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)))
	if err != nil {
		return TitleResponse{}, titleError(err)
	}
	return titleResponse(view), nil
}

func (c *Controller) updateTitle(ctx fuego.ContextWithBody[UpdateTitleRequest]) (TitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TitleResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TitleResponse{}, err
	}
	update := TitleUpdate{Profile: body.ProfileID, Monitored: body.Monitored, MonitorFutureSeasons: body.MonitorFutureSeasons}
	view, err := c.service.UpdateTitle(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)), update)
	if err != nil {
		return TitleResponse{}, titleError(err)
	}
	return titleResponse(view), nil
}

func (c *Controller) refreshTitle(ctx fuego.ContextNoBody) (RefreshTitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return RefreshTitleResponse{}, err
	}
	view, err := c.service.Title(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)))
	if err != nil {
		return RefreshTitleResponse{}, titleError(err)
	}
	if err := c.queue.Enqueue(ctx.Context(), refreshTitleTask{Title: view.ID}); err != nil {
		return RefreshTitleResponse{}, err
	}
	return RefreshTitleResponse{Queued: true}, nil
}

func (c *Controller) updateSeason(ctx fuego.ContextWithBody[UpdateMonitoredRequest]) (TitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TitleResponse{}, err
	}
	number, err := strconv.Atoi(ctx.PathParam(pathSeason))
	if err != nil {
		return TitleResponse{}, badRequest("season must be a number")
	}
	body, err := ctx.Body()
	if err != nil {
		return TitleResponse{}, err
	}
	view, err := c.service.SetSeasonMonitored(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)), number, body.Monitored)
	return titleResult(view, err)
}

func (c *Controller) updateEpisode(ctx fuego.ContextWithBody[UpdateMonitoredRequest]) (TitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TitleResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TitleResponse{}, err
	}
	title, episode := domain.TitleID(ctx.PathParam(pathTitleID)), domain.EpisodeID(ctx.PathParam(pathEpisodeID))
	view, err := c.service.SetEpisodeMonitored(ctx.Context(), title, episode, body.Monitored)
	return titleResult(view, err)
}

func (c *Controller) listEpisodeGroups(ctx fuego.ContextNoBody) ([]EpisodeGroupResponse, error) {
	groups, err := c.service.EpisodeGroups(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)))
	if err != nil {
		return nil, titleError(err)
	}
	response := make([]EpisodeGroupResponse, 0, len(groups))
	for _, g := range groups {
		kind, ok := episodeGroupTypes[g.Type]
		if !ok {
			kind = EpisodeGroupTypeUnknown
		}
		response = append(response, EpisodeGroupResponse{ID: g.ID, Name: g.Name, Type: kind, EpisodeCount: g.EpisodeCount, GroupCount: g.GroupCount})
	}
	return response, nil
}

func (c *Controller) selectEpisodeGroup(ctx fuego.ContextWithBody[SelectEpisodeGroupRequest]) (TitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TitleResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TitleResponse{}, err
	}
	view, err := c.service.SelectEpisodeGroup(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)), body.GroupID)
	return titleResult(view, err)
}

func (c *Controller) updateTitleLanguages(ctx fuego.ContextWithBody[UpdateTitleLanguagesRequest]) (TitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TitleResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return TitleResponse{}, err
	}
	view, err := c.service.SetTitleLanguages(ctx.Context(), domain.TitleID(ctx.PathParam(pathTitleID)), wantedLanguages(body.Languages))
	return titleResult(view, err)
}

func (c *Controller) getLanguages(ctx fuego.ContextNoBody) (WantedLanguagesResponse, error) {
	langs, err := c.repo.GlobalLanguages(ctx.Context())
	return languagesResponse(langs), err
}

func (c *Controller) updateLanguages(ctx fuego.ContextWithBody[UpdateWantedLanguagesRequest]) (WantedLanguagesResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return WantedLanguagesResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return WantedLanguagesResponse{}, err
	}
	langs, err := c.service.SetGlobalLanguages(ctx.Context(), wantedLanguages(body.Languages))
	if err != nil {
		return WantedLanguagesResponse{}, titleError(err)
	}
	return languagesResponse(langs), nil
}

// wantedLanguages keeps nil as nil: for a Title it means "use the global list".
func wantedLanguages(entries []WantedLanguageEntry) []api.WantedLanguage {
	if entries == nil {
		return nil
	}
	langs := make([]api.WantedLanguage, 0, len(entries))
	for _, e := range entries {
		langs = append(langs, api.WantedLanguage(e))
	}
	return langs
}

func languageEntries(langs []api.WantedLanguage) []WantedLanguageEntry {
	if langs == nil {
		return nil
	}
	entries := make([]WantedLanguageEntry, 0, len(langs))
	for _, l := range langs {
		entries = append(entries, WantedLanguageEntry(l))
	}
	return entries
}

func languagesResponse(langs []api.WantedLanguage) WantedLanguagesResponse {
	response := WantedLanguagesResponse{Languages: languageEntries(langs), Available: make([]LanguageOptionResponse, 0, len(LanguageOptions))}
	for _, option := range LanguageOptions {
		response.Available = append(response.Available, LanguageOptionResponse(option))
	}
	return response
}

func titleResult(view TitleView, err error) (TitleResponse, error) {
	if err != nil {
		return TitleResponse{}, titleError(err)
	}
	return titleResponse(view), nil
}

func (c *Controller) removeTitle(ctx fuego.ContextNoBody) (RemoveTitleResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return RemoveTitleResponse{}, err
	}
	deleteFiles := false
	if ctx.QueryParam(queryDeleteFiles) != "" {
		value, err := ctx.QueryParamBoolErr(queryDeleteFiles)
		if err != nil {
			return RemoveTitleResponse{}, badRequest("deleteFiles must be true or false")
		}
		deleteFiles = value
	}
	id := domain.TitleID(ctx.PathParam(pathTitleID))
	if err := c.service.RemoveTitle(ctx.Context(), id, deleteFiles); err != nil {
		return RemoveTitleResponse{}, titleError(err)
	}
	return RemoveTitleResponse{ID: id, DeleteFiles: deleteFiles}, nil
}

func (c *Controller) serveImage(w http.ResponseWriter, r *http.Request) {
	title, kind := domain.TitleID(r.PathValue(pathImageID)), r.PathValue(pathImageKind)
	path, err := c.repo.ImagePath(r.Context(), title, kind)
	if errors.Is(err, api.ErrNotFound) {
		problem.Write(w, r, problem.New(TypeImageNotFound, http.StatusNotFound, "no cached "+kind+" for title "+string(title)))
		return
	}
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	width := 0
	if raw := r.URL.Query().Get(queryWidth); raw != "" {
		if width, err = strconv.Atoi(raw); err != nil || width <= 0 {
			problem.Write(w, r, badRequest("w must be a positive width in pixels"))
			return
		}
	}
	served, err := Variant(path, width)
	if err != nil {
		problem.Write(w, r, err)
		return
	}
	w.Header().Set(headerCache, imageCacheControl)
	http.ServeFile(w, r, served)
}

func (c *Controller) discover(ctx fuego.ContextNoBody) (DiscoverResponse, error) {
	query := ctx.QueryParam(queryQ)
	items, err := c.service.Discover(ctx.Context(), query)
	if err != nil {
		if isTmdbError(err) {
			return DiscoverResponse{}, problem.New(TypeTmdbUnavailable, http.StatusServiceUnavailable, "TMDB is unavailable: "+err.Error())
		}
		return DiscoverResponse{}, err
	}
	response := DiscoverResponse{Trending: strings.TrimSpace(query) == "", Items: make([]DiscoverItemResponse, 0, len(items))}
	for _, item := range items {
		entry := DiscoverItemResponse{
			TmdbID:    item.Item.ID,
			Kind:      item.Item.Kind,
			Name:      item.Item.Name,
			Year:      item.Item.Year,
			Overview:  item.Item.Overview,
			InLibrary: item.Title != nil,
			TitleID:   item.Title,
		}
		if item.PosterURL != "" {
			entry.PosterURL = &item.PosterURL
		}
		response.Items = append(response.Items, entry)
	}
	return response, nil
}

func (c *Controller) getSettings(ctx fuego.ContextNoBody) (CatalogSettingsResponse, error) {
	m, err := c.service.Settings(ctx.Context())
	return CatalogSettingsResponse(m), err
}

func (c *Controller) updateSettings(ctx fuego.ContextWithBody[UpdateCatalogSettingsRequest]) (CatalogSettingsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return CatalogSettingsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return CatalogSettingsResponse{}, err
	}
	m, err := c.service.UpdateSettings(ctx.Context(), MetadataSettings(body))
	if errors.Is(err, ErrInvalidSettings) {
		return CatalogSettingsResponse{}, problem.New(TypeInvalidSettings, http.StatusBadRequest, err.Error())
	}
	return CatalogSettingsResponse(m), err
}

func (c *Controller) attribution(fuego.ContextNoBody) (AttributionResponse, error) {
	return AttributionResponse{
		Notice:  attributionNotice,
		LogoURL: svgDataURIPrefix + base64.StdEncoding.EncodeToString(tmdbLogo),
		Link:    attributionLink,
	}, nil
}

func titleResponse(v TitleView) TitleResponse {
	r := TitleResponse{
		ID:                   v.ID,
		Kind:                 v.Kind,
		TmdbID:               v.External.Tmdb,
		TvdbID:               v.External.Tvdb,
		ImdbID:               v.External.Imdb,
		Name:                 v.Name,
		OriginalName:         v.OriginalName,
		Overview:             v.Overview,
		Year:                 v.Year,
		RuntimeMinutes:       v.RuntimeMinutes,
		OriginalLanguage:     v.OriginalLanguage,
		SeriesStatus:         v.SeriesStatus,
		ProfileID:            v.Profile,
		Monitored:            v.Monitored,
		MonitorFutureSeasons: v.MonitorFutureSeasons,
		Released:             v.Released,
		ReleasedAt:           v.ReleasedAt,
		ReleaseDates:         ReleaseDatesResponse(v.Dates),
		Origin:               v.Origin,
		RequestedBy:          v.RequestedBy,
		AddedAt:              v.AddedAt,
		EpisodeGroupID:       v.EpisodeGroupID,
		LanguagesOverride:    languageEntries(v.LanguagesOverride),
		Seasons:              make([]SeasonResponse, 0, len(v.Seasons)),
	}
	for _, kind := range v.Images {
		url := imagesRoute + string(v.ID) + "/" + kind
		switch kind {
		case api.ImageKindPoster:
			r.Images.Poster = &url
		case api.ImageKindBackdrop:
			r.Images.Backdrop = &url
		case api.ImageKindLogo:
			r.Images.Logo = &url
		}
	}
	for _, s := range v.Seasons {
		season := SeasonResponse{Number: s.Number, Monitored: s.Monitored, Episodes: make([]EpisodeResponse, 0, len(s.Episodes))}
		for _, e := range s.Episodes {
			season.Episodes = append(season.Episodes, EpisodeResponse{
				ID: e.ID, Season: e.Season, Number: e.Number, AiredSeason: e.AiredSeason, AiredNumber: e.AiredNumber,
				Name: e.Name, AirDate: e.AirDate, RuntimeMinutes: e.RuntimeMinutes, Monitored: e.Monitored, Released: e.Released,
			})
		}
		r.Seasons = append(r.Seasons, season)
	}
	return r
}

func titleError(err error) error {
	switch {
	case errors.Is(err, ErrSeasonNotFound):
		return problem.New(TypeSeasonNotFound, http.StatusNotFound, "no such season")
	case errors.Is(err, ErrEpisodeNotFound):
		return problem.New(TypeEpisodeNotFound, http.StatusNotFound, "no such episode")
	case errors.Is(err, api.ErrNotFound):
		return problem.New(TypeTitleNotFound, http.StatusNotFound, "no such title")
	case errors.Is(err, ErrNotASeries):
		return problem.New(TypeNotASeries, http.StatusConflict, "only series have seasons and episode groups")
	case errors.Is(err, ErrUnknownEpisodeGroup):
		return problem.New(TypeUnknownEpisodeGroup, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidLanguages):
		return problem.New(TypeInvalidLanguages, http.StatusBadRequest, err.Error())
	case isTmdbError(err):
		return problem.New(TypeTmdbUnavailable, http.StatusServiceUnavailable, "TMDB is unavailable: "+err.Error())
	}
	return err
}

func badRequest(detail string) error {
	return problem.New(problem.TypeBadRequest, http.StatusBadRequest, detail)
}
