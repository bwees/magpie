package tmdb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/domain"
)

const (
	paramLanguage        = "language"
	paramAppend          = "append_to_response"
	paramImageLanguage   = "include_image_language"
	paramQuery           = "query"
	paramPage            = "page"
	paramIncludeAdult    = "include_adult"
	paramExternalSource  = "external_source"
	paramStartDate       = "start_date"
	paramEndDate         = "end_date"
	appendDetails        = "release_dates,external_ids,images,alternative_titles"
	appendSeriesDetails  = "external_ids,images,alternative_titles"
	imageLanguageNeutral = "null"
	listSeparator        = ","
	excludeAdult         = "false"
	firstPage            = 1
	changesForMovies     = "/movie/changes"
	changesForSeries     = "/tv/changes"
)

func (c *Client) Movie(ctx context.Context, id domain.TmdbID, lang string) (catalog.TmdbMovie, error) {
	var wire wireMovie
	err := c.getJSON(ctx, "/movie/"+formatID(id), detailsQuery(lang, appendDetails), &wire)
	if err != nil {
		return catalog.TmdbMovie{}, err
	}
	return wire.toPort(lang), nil
}

func (c *Client) Series(ctx context.Context, id domain.TmdbID, lang string) (catalog.TmdbSeries, error) {
	var wire wireSeries
	err := c.getJSON(ctx, "/tv/"+formatID(id), detailsQuery(lang, appendSeriesDetails), &wire)
	if err != nil {
		return catalog.TmdbSeries{}, err
	}
	return wire.toPort(lang), nil
}

func (c *Client) Season(ctx context.Context, id domain.TmdbID, season int, lang string) (catalog.TmdbSeason, error) {
	var wire wireSeason
	err := c.getJSON(ctx, "/tv/"+formatID(id)+"/season/"+strconv.Itoa(season), url.Values{paramLanguage: {lang}}, &wire)
	if err != nil {
		return catalog.TmdbSeason{}, err
	}
	return wire.toPort(id), nil
}

func (c *Client) EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]catalog.TmdbEpisodeGroupSummary, error) {
	var wire struct {
		Results []wireEpisodeGroupSummary `json:"results"`
	}
	if err := c.getJSON(ctx, "/tv/"+formatID(id)+"/episode_groups", nil, &wire); err != nil {
		return nil, err
	}
	groups := make([]catalog.TmdbEpisodeGroupSummary, 0, len(wire.Results))
	for _, g := range wire.Results {
		groups = append(groups, catalog.TmdbEpisodeGroupSummary{
			ID:           g.ID,
			Name:         g.Name,
			Type:         catalog.TmdbEpisodeGroupType(g.Type),
			EpisodeCount: g.EpisodeCount,
			GroupCount:   g.GroupCount,
		})
	}
	return groups, nil
}

func (c *Client) EpisodeGroup(ctx context.Context, groupID string) (catalog.TmdbEpisodeGroup, error) {
	var wire wireEpisodeGroup
	if err := c.getJSON(ctx, "/tv/episode_group/"+url.PathEscape(groupID), nil, &wire); err != nil {
		return catalog.TmdbEpisodeGroup{}, err
	}
	return wire.toPort(), nil
}

func (c *Client) Find(ctx context.Context, source string, externalID string) (catalog.TmdbFindResult, error) {
	var wire wireFind
	err := c.getJSON(ctx, "/find/"+url.PathEscape(externalID), url.Values{paramExternalSource: {source}}, &wire)
	if err != nil {
		return catalog.TmdbFindResult{}, err
	}
	result := catalog.TmdbFindResult{
		Movies: itemsOfKind(wire.Movies, domain.MediaKindMovie),
		Series: itemsOfKind(wire.Series, domain.MediaKindSeries),
	}
	for _, e := range wire.Episodes {
		result.Episodes = append(result.Episodes, catalog.TmdbFoundEpisode{
			ID:     catalog.TmdbEpisodeID(e.ID),
			Series: domain.TmdbID(e.ShowID),
			Season: e.SeasonNumber,
			Number: e.EpisodeNumber,
		})
	}
	return result, nil
}

func (c *Client) Search(ctx context.Context, query string, page int) (catalog.TmdbSearchPage, error) {
	var wire wirePage
	params := url.Values{
		paramQuery:        {query},
		paramPage:         {strconv.Itoa(max(page, firstPage))},
		paramIncludeAdult: {excludeAdult},
	}
	if err := c.getJSON(ctx, "/search/multi", params, &wire); err != nil {
		return catalog.TmdbSearchPage{}, err
	}
	return catalog.TmdbSearchPage{
		Page:         wire.Page,
		TotalPages:   wire.TotalPages,
		TotalResults: wire.TotalResults,
		Items:        mixedItems(wire.Results),
	}, nil
}

func (c *Client) Trending(ctx context.Context, window string) ([]catalog.TmdbSearchItem, error) {
	var wire wirePage
	if err := c.getJSON(ctx, "/trending/all/"+url.PathEscape(window), nil, &wire); err != nil {
		return nil, err
	}
	return mixedItems(wire.Results), nil
}

// Changes reads every page of TMDB's change list from since until today.
func (c *Client) Changes(ctx context.Context, kind domain.MediaKind, since time.Time) ([]domain.TmdbID, error) {
	path := changesForMovies
	if kind == domain.MediaKindSeries {
		path = changesForSeries
	}
	ids := []domain.TmdbID{}
	for page := firstPage; ; page++ {
		var wire wireChanges
		params := url.Values{
			paramStartDate: {since.UTC().Format(time.DateOnly)},
			paramEndDate:   {c.now().UTC().Format(time.DateOnly)},
			paramPage:      {strconv.Itoa(page)},
		}
		if err := c.getJSON(ctx, path, params, &wire); err != nil {
			return nil, err
		}
		for _, r := range wire.Results {
			ids = append(ids, domain.TmdbID(r.ID))
		}
		if page >= wire.TotalPages {
			return ids, nil
		}
	}
}

// Image needs no key: image.tmdb.org is public.
func (c *Client) Image(ctx context.Context, path string, size string) (io.ReadCloser, error) {
	if path == "" {
		return nil, errNoImagePath
	}
	body, err := c.send(ctx, c.ImageURL(path, size), http.Header{})
	if err != nil {
		return nil, fmt.Errorf("tmdb image %s: %w", path, err)
	}
	return body, nil
}

func detailsQuery(lang, appendTo string) url.Values {
	imageLanguages := []string{}
	for _, l := range logoLanguages(lang) {
		if l == neutralLanguage {
			l = imageLanguageNeutral
		}
		imageLanguages = append(imageLanguages, l)
	}
	return url.Values{
		paramLanguage:      {lang},
		paramAppend:        {appendTo},
		paramImageLanguage: {strings.Join(imageLanguages, listSeparator)},
	}
}

func formatID(id domain.TmdbID) string {
	return strconv.FormatInt(int64(id), 10)
}
