package arr

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// The v3 API's JSON, as Sonarr 4 and Radarr 5/6 send it. Only the fields the migration maps are read.

type wireStatus struct {
	AppName string `json:"appName"`
	Version string `json:"version"`
}

type wireNamed struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

type wireRootFolder struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

type wireAlternateTitle struct {
	Title string `json:"title"`
}

type wireSeries struct {
	ID                int                  `json:"id"`
	Title             string               `json:"title"`
	Year              int                  `json:"year"`
	TvdbID            int                  `json:"tvdbId"`
	TmdbID            int                  `json:"tmdbId"`
	ImdbID            string               `json:"imdbId"`
	Path              string               `json:"path"`
	QualityProfileID  int                  `json:"qualityProfileId"`
	Monitored         bool                 `json:"monitored"`
	MonitorNewItems   string               `json:"monitorNewItems"`
	UseSceneNumbering bool                 `json:"useSceneNumbering"`
	SeriesType        string               `json:"seriesType"`
	SeasonType        string               `json:"seasonType"`
	Tags              []int                `json:"tags"`
	Added             time.Time            `json:"added"`
	AlternateTitles   []wireAlternateTitle `json:"alternateTitles"`
}

type wireEpisode struct {
	ID            int    `json:"id"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	TvdbID        int    `json:"tvdbId"`
	Title         string `json:"title"`
	AirDate       string `json:"airDate"`
	Monitored     bool   `json:"monitored"`
	EpisodeFileID int    `json:"episodeFileId"`
}

type wireQualityModel struct {
	Quality  struct{ ID int } `json:"quality"`
	Revision struct {
		Version int `json:"version"`
	} `json:"revision"`
}

type wireFile struct {
	ID           int              `json:"id"`
	Path         string           `json:"path"`
	Size         int64            `json:"size"`
	Quality      wireQualityModel `json:"quality"`
	SceneName    string           `json:"sceneName"`
	ReleaseGroup string           `json:"releaseGroup"`
	Languages    []wireNamed      `json:"languages"`
	DateAdded    time.Time        `json:"dateAdded"`
}

type wireMovie struct {
	ID               int       `json:"id"`
	Title            string    `json:"title"`
	Year             int       `json:"year"`
	TmdbID           int       `json:"tmdbId"`
	ImdbID           string    `json:"imdbId"`
	Path             string    `json:"path"`
	QualityProfileID int       `json:"qualityProfileId"`
	Monitored        bool      `json:"monitored"`
	Tags             []int     `json:"tags"`
	Added            time.Time `json:"added"`
	MovieFile        *wireFile `json:"movieFile"`
}

type wireProfileItem struct {
	ID            int               `json:"id"`
	Name          string            `json:"name"`
	Quality       *struct{ ID int } `json:"quality"`
	Items         []wireProfileItem `json:"items"`
	Allowed       bool              `json:"allowed"`
	MinSize       *float64          `json:"minSize"`
	MaxSize       *float64          `json:"maxSize"`
	PreferredSize *float64          `json:"preferredSize"`
}

type wireFormatItem struct {
	Format int `json:"format"`
	Score  int `json:"score"`
}

type wireQualityProfile struct {
	ID                    int               `json:"id"`
	Name                  string            `json:"name"`
	UpgradeAllowed        bool              `json:"upgradeAllowed"`
	Cutoff                int               `json:"cutoff"`
	Items                 []wireProfileItem `json:"items"`
	MinFormatScore        int               `json:"minFormatScore"`
	CutoffFormatScore     int               `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int               `json:"minUpgradeFormatScore"`
	FormatItems           []wireFormatItem  `json:"formatItems"`
	Language              *wireNamed        `json:"language"`
}

type wireQualityDefinition struct {
	Quality       struct{ ID int } `json:"quality"`
	MinSize       float64          `json:"minSize"`
	MaxSize       *float64         `json:"maxSize"`
	PreferredSize *float64         `json:"preferredSize"`
}

type wireField struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type wireSpec struct {
	Name           string      `json:"name"`
	Implementation string      `json:"implementation"`
	Negate         bool        `json:"negate"`
	Required       bool        `json:"required"`
	Fields         []wireField `json:"fields"`
}

type wireCustomFormat struct {
	ID                              int        `json:"id"`
	Name                            string     `json:"name"`
	IncludeCustomFormatWhenRenaming bool       `json:"includeCustomFormatWhenRenaming"`
	Specifications                  []wireSpec `json:"specifications"`
}

// wireTerms reads release-profile terms, which are a list in Sonarr 4 and a comma-separated string before.
type wireTerms []string

const termSeparator = ","

func (t *wireTerms) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*t = list
		return nil
	}
	var joined string
	if err := json.Unmarshal(data, &joined); err != nil {
		return err
	}
	*t = nil
	for _, term := range strings.Split(joined, termSeparator) {
		if term = strings.TrimSpace(term); term != "" {
			*t = append(*t, term)
		}
	}
	return nil
}

type wireReleaseProfile struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Required  wireTerms `json:"required"`
	Ignored   wireTerms `json:"ignored"`
	IndexerID int       `json:"indexerId"`
	Tags      []int     `json:"tags"`
}

type wireDelayProfile struct {
	ID                             int   `json:"id"`
	EnableTorrent                  bool  `json:"enableTorrent"`
	TorrentDelay                   int   `json:"torrentDelay"`
	BypassIfHighestQuality         bool  `json:"bypassIfHighestQuality"`
	BypassIfAboveCustomFormatScore bool  `json:"bypassIfAboveCustomFormatScore"`
	MinimumCustomFormatScore       int   `json:"minimumCustomFormatScore"`
	Tags                           []int `json:"tags"`
}

type wirePage[T any] struct {
	Page         int `json:"page"`
	PageSize     int `json:"pageSize"`
	TotalRecords int `json:"totalRecords"`
	Records      []T `json:"records"`
}

type wireBlocklistItem struct {
	ID          int       `json:"id"`
	SeriesID    int       `json:"seriesId"`
	EpisodeIDs  []int     `json:"episodeIds"`
	MovieID     int       `json:"movieId"`
	SourceTitle string    `json:"sourceTitle"`
	Protocol    string    `json:"protocol"`
	Indexer     string    `json:"indexer"`
	Message     string    `json:"message"`
	Date        time.Time `json:"date"`
}

type wireHistoryItem struct {
	ID          int                        `json:"id"`
	SeriesID    int                        `json:"seriesId"`
	EpisodeID   int                        `json:"episodeId"`
	MovieID     int                        `json:"movieId"`
	SourceTitle string                     `json:"sourceTitle"`
	EventType   string                     `json:"eventType"`
	Date        time.Time                  `json:"date"`
	DownloadID  string                     `json:"downloadId"`
	Data        map[string]json.RawMessage `json:"data"`
}

// fieldText renders a spec field's value as text: a string as is, a number or bool in its JSON form.
func fieldText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return strings.TrimSpace(string(raw))
}

func fieldNumber(raw json.RawMessage) float64 {
	n, err := strconv.ParseFloat(fieldText(raw), 64)
	if err != nil {
		return 0
	}
	return n
}

// dataText renders a history data value as text; the API sends strings, but older rows hold numbers.
func dataText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	return fieldText(raw)
}
