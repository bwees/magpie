// Package arr implements the migration's Sonarr/Radarr port against their v3 APIs, read-only.
package arr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"magpie/internal/platform/migrate"
)

const (
	apiKeyHeader   = "X-Api-Key"
	acceptHeader   = "Accept"
	jsonContent    = "application/json"
	apiPrefix      = "/api/v3/"
	requestTimeout = 2 * time.Minute
	// maxBodyBytes bounds one answer; a large library's series list is a few MB.
	maxBodyBytes = 256 << 20
	// pageSize is how many blocklist or history rows one request asks for.
	pageSize = 1000

	paramSeriesID = "seriesId"
	paramPage     = "page"
	paramPageSize = "pageSize"

	appNameSonarr = "Sonarr"
	appNameRadarr = "Radarr"

	// History data's protocol is the DownloadProtocol enum's number.
	protocolUsenetNumber  = "1"
	protocolTorrentNumber = "2"
	dataProtocol          = "protocol"
)

var appNames = map[migrate.App]string{migrate.AppSonarr: appNameSonarr, migrate.AppRadarr: appNameRadarr}

// Client reads Sonarr and Radarr. It is safe for concurrent use.
type Client struct {
	http *http.Client
}

var _ migrate.Arr = (*Client)(nil)

func New() *Client {
	return &Client{http: &http.Client{Timeout: requestTimeout}}
}

func NewWithHTTP(h *http.Client) *Client {
	return &Client{http: h}
}

// get reads one endpoint into out. A 401 is ErrArrUnauthorized; a 404, found=false; anything else that
// isn't 200 is ErrArrUnreachable.
func (c *Client) get(ctx context.Context, conn migrate.Connection, path string, query url.Values, out any) (found bool, err error) {
	base, err := url.Parse(strings.TrimRight(conn.URL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return false, fmt.Errorf("%w: %q isn't a URL", migrate.ErrArrUnreachable, conn.URL)
	}
	endpoint := base.JoinPath(apiPrefix, path)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set(apiKeyHeader, conn.APIKey)
	req.Header.Set(acceptHeader, jsonContent)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, fmt.Errorf("%w: %w", migrate.ErrArrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, migrate.ErrArrUnauthorized
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("%w: %s answered HTTP %d", migrate.ErrArrUnreachable, path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(out); err != nil {
		return false, fmt.Errorf("%w: reading %s: %w", migrate.ErrArrUnreachable, path, err)
	}
	return true, nil
}

// must is get for an endpoint every supported version has.
func (c *Client) must(ctx context.Context, conn migrate.Connection, path string, query url.Values, out any) error {
	found, err := c.get(ctx, conn, path, query, out)
	if err == nil && !found {
		return fmt.Errorf("%w: %s isn't there; is this Sonarr 4 or Radarr?", migrate.ErrArrUnreachable, path)
	}
	return err
}

func (c *Client) Status(ctx context.Context, app migrate.App, conn migrate.Connection) (migrate.ArrStatus, error) {
	var status wireStatus
	if err := c.must(ctx, conn, "system/status", nil, &status); err != nil {
		return migrate.ArrStatus{}, err
	}
	if !strings.EqualFold(status.AppName, appNames[app]) {
		return migrate.ArrStatus{}, fmt.Errorf("%w: it says it is %s", migrate.ErrArrWrongApp, status.AppName)
	}
	return migrate.ArrStatus{AppName: status.AppName, Version: status.Version}, nil
}

func (c *Client) RootFolders(ctx context.Context, _ migrate.App, conn migrate.Connection) ([]migrate.ArrRootFolder, error) {
	var roots []wireRootFolder
	if err := c.must(ctx, conn, "rootfolder", nil, &roots); err != nil {
		return nil, err
	}
	out := make([]migrate.ArrRootFolder, 0, len(roots))
	for _, r := range roots {
		out = append(out, migrate.ArrRootFolder(r))
	}
	return out, nil
}

// Library reads everything the migration maps. Series are read with their episodes and files one by one,
// which is what progress counts.
func (c *Client) Library(ctx context.Context, app migrate.App, conn migrate.Connection, progress func(done, total int)) (migrate.ArrLibrary, error) {
	status, err := c.Status(ctx, app, conn)
	if err != nil {
		return migrate.ArrLibrary{}, err
	}
	lib := migrate.ArrLibrary{App: app, Version: status.Version}
	if lib.RootFolders, err = c.RootFolders(ctx, app, conn); err != nil {
		return migrate.ArrLibrary{}, err
	}
	steps := []func() error{
		func() error { return c.qualityModel(ctx, conn, &lib) },
		func() error { return c.named(ctx, conn, "tag", &lib.Tags) },
		func() error { return c.named(ctx, conn, "language", &lib.Languages) },
		func() error { return c.named(ctx, conn, "indexerflag", &lib.IndexerFlags) },
		func() error { return c.blocklist(ctx, conn, &lib) },
		func() error { return c.history(ctx, conn, &lib) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return migrate.ArrLibrary{}, err
		}
	}
	if app == migrate.AppSonarr {
		err = c.series(ctx, conn, &lib, progress)
	} else {
		err = c.movies(ctx, conn, &lib, progress)
	}
	return lib, err
}

// named reads a list of IDs with names; an endpoint this version lacks gives none.
func (c *Client) named(ctx context.Context, conn migrate.Connection, path string, out *[]migrate.ArrNamed) error {
	var items []wireNamed
	if _, err := c.get(ctx, conn, path, nil, &items); err != nil {
		return err
	}
	*out = make([]migrate.ArrNamed, 0, len(items))
	for _, item := range items {
		name := item.Name
		if name == "" {
			name = item.Label
		}
		*out = append(*out, migrate.ArrNamed{ID: item.ID, Name: name})
	}
	return nil
}

func (c *Client) series(ctx context.Context, conn migrate.Connection, lib *migrate.ArrLibrary, progress func(done, total int)) error {
	var series []wireSeries
	if err := c.must(ctx, conn, "series", nil, &series); err != nil {
		return err
	}
	for i, s := range series {
		query := url.Values{paramSeriesID: {strconv.Itoa(s.ID)}}
		var episodes []wireEpisode
		if err := c.must(ctx, conn, "episode", query, &episodes); err != nil {
			return err
		}
		var files []wireFile
		if err := c.must(ctx, conn, "episodefile", query, &files); err != nil {
			return err
		}
		out := migrate.ArrSeries{
			ID: s.ID, Title: s.Title, Year: s.Year, TvdbID: s.TvdbID, TmdbID: s.TmdbID, ImdbID: s.ImdbID, Path: s.Path,
			QualityProfileID: s.QualityProfileID, Monitored: s.Monitored, MonitorNewItems: s.MonitorNewItems,
			UseSceneNumbering: s.UseSceneNumbering, SeriesType: s.SeriesType, SeasonType: s.SeasonType, Tags: s.Tags,
			Added: s.Added, AlternateTitles: []string{},
		}
		for _, alt := range s.AlternateTitles {
			out.AlternateTitles = append(out.AlternateTitles, alt.Title)
		}
		for _, e := range episodes {
			out.Episodes = append(out.Episodes, migrate.ArrEpisode(e))
		}
		for _, f := range files {
			out.Files = append(out.Files, file(f))
		}
		lib.Series = append(lib.Series, out)
		if progress != nil {
			progress(i+1, len(series))
		}
	}
	return nil
}

func (c *Client) movies(ctx context.Context, conn migrate.Connection, lib *migrate.ArrLibrary, progress func(done, total int)) error {
	var movies []wireMovie
	if err := c.must(ctx, conn, "movie", nil, &movies); err != nil {
		return err
	}
	for _, m := range movies {
		out := migrate.ArrMovie{
			ID: m.ID, Title: m.Title, Year: m.Year, TmdbID: m.TmdbID, ImdbID: m.ImdbID, Path: m.Path,
			QualityProfileID: m.QualityProfileID, Monitored: m.Monitored, Tags: m.Tags, Added: m.Added,
		}
		if m.MovieFile != nil {
			f := file(*m.MovieFile)
			out.File = &f
		}
		lib.Movies = append(lib.Movies, out)
	}
	if progress != nil {
		progress(len(movies), len(movies))
	}
	return nil
}

func file(f wireFile) migrate.ArrFile {
	out := migrate.ArrFile{
		ID: f.ID, Path: f.Path, Size: f.Size, QualityID: f.Quality.Quality.ID, Revision: f.Quality.Revision.Version,
		SceneName: f.SceneName, ReleaseGroup: f.ReleaseGroup, DateAdded: f.DateAdded, Languages: []string{},
	}
	for _, l := range f.Languages {
		out.Languages = append(out.Languages, l.Name)
	}
	return out
}

func (c *Client) qualityModel(ctx context.Context, conn migrate.Connection, lib *migrate.ArrLibrary) error {
	var profiles []wireQualityProfile
	if err := c.must(ctx, conn, "qualityprofile", nil, &profiles); err != nil {
		return err
	}
	for _, p := range profiles {
		out := migrate.ArrQualityProfile{
			ID: p.ID, Name: p.Name, UpgradeAllowed: p.UpgradeAllowed, Cutoff: p.Cutoff, MinFormatScore: p.MinFormatScore,
			CutoffFormatScore: p.CutoffFormatScore, MinUpgradeFormatScore: p.MinUpgradeFormatScore,
		}
		for _, item := range p.Items {
			out.Items = append(out.Items, profileItem(item))
		}
		for _, fi := range p.FormatItems {
			out.FormatItems = append(out.FormatItems, migrate.ArrFormatScore(fi))
		}
		if p.Language != nil {
			out.Language = &migrate.ArrNamed{ID: p.Language.ID, Name: p.Language.Name}
		}
		lib.QualityProfiles = append(lib.QualityProfiles, out)
	}
	var definitions []wireQualityDefinition
	if err := c.must(ctx, conn, "qualitydefinition", nil, &definitions); err != nil {
		return err
	}
	for _, d := range definitions {
		lib.QualityDefinitions = append(lib.QualityDefinitions, migrate.ArrQualityDefinition{
			QualityID: d.Quality.ID, MinSize: d.MinSize, MaxSize: d.MaxSize, PreferredSize: d.PreferredSize,
		})
	}
	var formats []wireCustomFormat
	if err := c.must(ctx, conn, "customformat", nil, &formats); err != nil {
		return err
	}
	for _, f := range formats {
		out := migrate.ArrCustomFormat{ID: f.ID, Name: f.Name, IncludeWhenRenaming: f.IncludeCustomFormatWhenRenaming}
		for _, s := range f.Specifications {
			out.Specs = append(out.Specs, spec(s))
		}
		lib.CustomFormats = append(lib.CustomFormats, out)
	}
	var releaseProfiles []wireReleaseProfile
	if _, err := c.get(ctx, conn, "releaseprofile", nil, &releaseProfiles); err != nil {
		return err
	}
	for _, r := range releaseProfiles {
		lib.ReleaseProfiles = append(lib.ReleaseProfiles, migrate.ArrReleaseProfile{
			ID: r.ID, Name: r.Name, Enabled: r.Enabled, Required: r.Required, Ignored: r.Ignored, IndexerID: r.IndexerID, Tags: r.Tags,
		})
	}
	var delays []wireDelayProfile
	if err := c.must(ctx, conn, "delayprofile", nil, &delays); err != nil {
		return err
	}
	for _, d := range delays {
		lib.DelayProfiles = append(lib.DelayProfiles, migrate.ArrDelayProfile(d))
	}
	return nil
}

func profileItem(item wireProfileItem) migrate.ArrProfileItem {
	out := migrate.ArrProfileItem{Name: item.Name, Allowed: item.Allowed, MinSize: item.MinSize, MaxSize: item.MaxSize, PreferredSize: item.PreferredSize}
	if item.Quality != nil {
		out.QualityID = item.Quality.ID
		return out
	}
	out.GroupID = item.ID
	for _, member := range item.Items {
		out.Items = append(out.Items, profileItem(member))
	}
	return out
}

// Spec field names.
const (
	fieldValue = "value"
	fieldMin   = "min"
	fieldMax   = "max"
)

func spec(s wireSpec) migrate.ArrSpec {
	out := migrate.ArrSpec{Name: s.Name, Implementation: s.Implementation, Negate: s.Negate, Required: s.Required}
	for _, f := range s.Fields {
		switch f.Name {
		case fieldValue:
			out.Value = fieldText(f.Value)
		case fieldMin:
			out.Min = fieldNumber(f.Value)
		case fieldMax:
			out.Max = fieldNumber(f.Value)
		}
	}
	return out
}

// pages reads every page of a paged list.
func pages[T any](ctx context.Context, c *Client, conn migrate.Connection, path string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		var got wirePage[T]
		query := url.Values{paramPage: {strconv.Itoa(page)}, paramPageSize: {strconv.Itoa(pageSize)}}
		if err := c.must(ctx, conn, path, query, &got); err != nil {
			return nil, err
		}
		all = append(all, got.Records...)
		if len(got.Records) == 0 || len(all) >= got.TotalRecords {
			return all, nil
		}
	}
}

func (c *Client) blocklist(ctx context.Context, conn migrate.Connection, lib *migrate.ArrLibrary) error {
	rows, err := pages[wireBlocklistItem](ctx, c, conn, "blocklist")
	if err != nil {
		return err
	}
	for _, b := range rows {
		lib.Blocklist = append(lib.Blocklist, migrate.ArrBlocklistItem(b))
	}
	return nil
}

func (c *Client) history(ctx context.Context, conn migrate.Connection, lib *migrate.ArrLibrary) error {
	rows, err := pages[wireHistoryItem](ctx, c, conn, "history")
	if err != nil {
		return err
	}
	for _, h := range rows {
		data := map[string]string{}
		for key, raw := range h.Data {
			data[lowerFirst(key)] = dataText(raw)
		}
		switch data[dataProtocol] {
		case protocolUsenetNumber:
			data[dataProtocol] = migrate.ProtocolUsenet
		case protocolTorrentNumber:
			data[dataProtocol] = migrate.ProtocolTorrent
		}
		lib.History = append(lib.History, migrate.ArrHistoryItem{
			ID: h.ID, SeriesID: h.SeriesID, EpisodeID: h.EpisodeID, MovieID: h.MovieID, SourceTitle: h.SourceTitle,
			EventType: h.EventType, Date: h.Date, DownloadID: h.DownloadID, Data: data,
		})
	}
	return nil
}

// lowerFirst makes "TorrentInfoHash" and "torrentInfoHash" the same key.
func lowerFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToLower(r)) + s[size:]
}
