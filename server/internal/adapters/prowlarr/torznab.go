package prowlarr

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/domain"
)

// Torznab request vocabulary (Torznab 1.3 / Newznab API).
const (
	paramFunction   = "t"
	paramQuery      = "q"
	paramIMDb       = "imdbid"
	paramTMDB       = "tmdbid"
	paramTVDB       = "tvdbid"
	paramSeason     = "season"
	paramEpisode    = "ep"
	paramYear       = "year"
	paramCategories = "cat"
	paramExtended   = "extended"

	functionCaps        = "caps"
	functionSearch      = "search"
	functionMovieSearch = "movie"
	functionTVSearch    = "tvsearch"

	extendedAttributes = "1"
	listSeparator      = ","
	modeAvailable      = "yes"
	errorElement       = "error"
	magnetPrefix       = "magnet:"

	attrSeeders              = "seeders"
	attrPeers                = "peers"
	attrInfoHash             = "infohash"
	attrMagnetURL            = "magneturl"
	attrDownloadVolumeFactor = "downloadvolumefactor"
	attrUploadVolumeFactor   = "uploadvolumefactor"

	// fullVolumeFactor counts all traffic against the ratio: Torznab's value when an item has no factor attrs.
	fullVolumeFactor = 1.0
)

// Caps params Magpie uses; the rest (rid, tvmazeid, doubanid, genre …) are ignored.
var capsParams = map[string]acquisition.TorznabParam{
	paramQuery:   acquisition.ParamQuery,
	paramIMDb:    acquisition.ParamIMDb,
	paramTMDB:    acquisition.ParamTMDB,
	paramTVDB:    acquisition.ParamTVDB,
	paramSeason:  acquisition.ParamSeason,
	paramEpisode: acquisition.ParamEpisode,
	paramYear:    acquisition.ParamYear,
}

var searchFunctions = map[domain.MediaKind]string{
	domain.MediaKindMovie:  functionMovieSearch,
	domain.MediaKindSeries: functionTVSearch,
}

const torznabEndpoint = "api"

// indexerPath is one indexer's own Torznab endpoint, /{id}/api.
func indexerPath(indexerID int) string {
	return path.Join(strconv.Itoa(indexerID), torznabEndpoint)
}

type errorDoc struct {
	XMLName     xml.Name
	Code        int    `xml:"code,attr"`
	Description string `xml:"description,attr"`
}

// getTorznab calls one indexer's Torznab endpoint and decodes its answer into out.
// A Torznab <error> becomes *acquisition.ErrIndexerFailed whatever the HTTP status.
func (c *Client) getTorznab(ctx context.Context, indexerID int, query url.Values, out any) error {
	query.Set(paramExtended, extendedAttributes)
	a, err := c.get(ctx, indexerPath(indexerID), query)
	if err != nil {
		return err
	}
	var root errorDoc
	if xml.Unmarshal(a.body, &root) == nil && root.XMLName.Local == errorElement {
		return &acquisition.ErrIndexerFailed{Code: root.Code, Message: root.Description}
	}
	if a.statusErr != nil {
		return a.statusErr
	}
	if err := xml.Unmarshal(a.body, out); err != nil {
		return fmt.Errorf("%w: decoding Torznab answer of indexer %d: %w", acquisition.ErrUnavailable, indexerID, err)
	}
	return nil
}

type capsDoc struct {
	Searching struct {
		MovieSearch searchMode `xml:"movie-search"`
		TVSearch    searchMode `xml:"tv-search"`
	} `xml:"searching"`
	Categories []capsCategory `xml:"categories>category"`
}

type searchMode struct {
	Available       string `xml:"available,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

type capsCategory struct {
	ID      int `xml:"id,attr"`
	Subcats []struct {
		ID int `xml:"id,attr"`
	} `xml:"subcat"`
}

func (c *Client) caps(ctx context.Context, indexerID int) (acquisition.TorznabCaps, error) {
	var doc capsDoc
	if err := c.getTorznab(ctx, indexerID, url.Values{paramFunction: {functionCaps}}, &doc); err != nil {
		return acquisition.TorznabCaps{}, err
	}
	caps := acquisition.TorznabCaps{
		MovieSearch: doc.Searching.MovieSearch.available(),
		MovieParams: doc.Searching.MovieSearch.params(),
		TVSearch:    doc.Searching.TVSearch.available(),
		TVParams:    doc.Searching.TVSearch.params(),
	}
	for _, category := range doc.Categories {
		caps.Categories = append(caps.Categories, category.ID)
		for _, sub := range category.Subcats {
			caps.Categories = append(caps.Categories, sub.ID)
		}
	}
	return caps, nil
}

func (m searchMode) available() bool {
	return m.Available == modeAvailable
}

// params is empty for a mode the indexer doesn't offer.
func (m searchMode) params() []acquisition.TorznabParam {
	if !m.available() {
		return nil
	}
	var params []acquisition.TorznabParam
	for name := range strings.SplitSeq(m.SupportedParams, listSeparator) {
		if param, ok := capsParams[strings.ToLower(strings.TrimSpace(name))]; ok {
			params = append(params, param)
		}
	}
	return params
}

// Search queries one indexer. The query's kind picks t=movie or t=tvsearch; no kind is a plain t=search.
func (c *Client) Search(ctx context.Context, indexerID int, q acquisition.TorznabQuery) ([]acquisition.TorznabItem, error) {
	return c.feed(ctx, indexerID, searchQuery(q))
}

// RSS reads an indexer's recent releases: a plain search with no query.
func (c *Client) RSS(ctx context.Context, indexerID int, categories []int) ([]acquisition.TorznabItem, error) {
	query := url.Values{paramFunction: {functionSearch}}
	setCategories(query, categories)
	return c.feed(ctx, indexerID, query)
}

func searchQuery(q acquisition.TorznabQuery) url.Values {
	function, ok := searchFunctions[q.Kind]
	if !ok {
		function = functionSearch
	}
	query := url.Values{paramFunction: {function}}
	setIf(query, paramQuery, q.Query)
	setIf(query, paramIMDb, q.Imdb)
	setIf(query, paramTVDB, q.Tvdb)
	if q.Tmdb != 0 {
		query.Set(paramTMDB, strconv.FormatInt(int64(q.Tmdb), 10))
	}
	setNumber(query, paramSeason, q.Season)
	setNumber(query, paramEpisode, q.Episode)
	setNumber(query, paramYear, q.Year)
	setCategories(query, q.Categories)
	return query
}

func setIf(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setNumber(query url.Values, key string, value *int) {
	if value != nil {
		query.Set(key, strconv.Itoa(*value))
	}
}

func setCategories(query url.Values, categories []int) {
	if len(categories) == 0 {
		return
	}
	ids := make([]string, 0, len(categories))
	for _, id := range categories {
		ids = append(ids, strconv.Itoa(id))
	}
	query.Set(paramCategories, strings.Join(ids, listSeparator))
}

type feedDoc struct {
	Items []feedItem `xml:"channel>item"`
}

type feedItem struct {
	Title      string `xml:"title"`
	GUID       string `xml:"guid"`
	Link       string `xml:"link"`
	PubDate    string `xml:"pubDate"`
	Size       int64  `xml:"size"`
	Categories []int  `xml:"category"`
	Enclosure  struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"http://torznab.com/schemas/2015/feed attr"`
}

func (c *Client) feed(ctx context.Context, indexerID int, query url.Values) ([]acquisition.TorznabItem, error) {
	var doc feedDoc
	if err := c.getTorznab(ctx, indexerID, query, &doc); err != nil {
		return nil, err
	}
	items := make([]acquisition.TorznabItem, 0, len(doc.Items))
	for _, raw := range doc.Items {
		items = append(items, raw.item())
	}
	return items, nil
}

// pubDateLayouts: Prowlarr writes .NET's "r" format, which names the zone GMT; other feeds use an offset.
var pubDateLayouts = []string{time.RFC1123Z, time.RFC1123}

// parsePubDate returns the zero time for a date in no known layout.
func parsePubDate(value string) time.Time {
	for _, layout := range pubDateLayouts {
		if published, err := time.Parse(layout, value); err == nil {
			return published.UTC()
		}
	}
	return time.Time{}
}

func (raw feedItem) item() acquisition.TorznabItem {
	item := acquisition.TorznabItem{
		GUID:                 raw.GUID,
		Title:                raw.Title,
		SizeBytes:            raw.Size,
		Categories:           raw.Categories,
		DownloadVolumeFactor: fullVolumeFactor,
		UploadVolumeFactor:   fullVolumeFactor,
	}
	item.PublishedAt = parsePubDate(raw.PubDate)
	link := raw.Link
	if link == "" {
		link = raw.Enclosure.URL
	}
	if strings.HasPrefix(link, magnetPrefix) {
		item.MagnetURI = link
	} else {
		item.DownloadURL = link
	}
	for _, attr := range raw.Attrs {
		switch attr.Name {
		case attrSeeders:
			item.Seeders, _ = strconv.Atoi(attr.Value)
		case attrPeers:
			item.Peers, _ = strconv.Atoi(attr.Value)
		case attrInfoHash:
			item.InfoHash = strings.ToLower(attr.Value)
		case attrMagnetURL:
			item.MagnetURI = attr.Value
		case attrDownloadVolumeFactor:
			item.DownloadVolumeFactor, _ = strconv.ParseFloat(attr.Value, 64)
		case attrUploadVolumeFactor:
			item.UploadVolumeFactor, _ = strconv.ParseFloat(attr.Value, 64)
		}
	}
	return item
}
