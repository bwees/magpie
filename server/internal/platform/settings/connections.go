package settings

import "magpie/internal/platform/owner"

// Connection settings hold credentials for external services. Adapters read them; contexts never store credentials.

const connectionsPrefix = "connections."

const DefaultQBittorrentCategory = "magpie"

type TMDBConnection struct {
	APIKey string `json:"apiKey"`
}

type QBittorrentConnection struct {
	URL      string `json:"url"`
	APIKey   string `json:"apiKey"`
	Category string `json:"category"`
}

type ProwlarrConnection struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

type JellyfinConnection struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

type OpenSubtitlesConnection struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var (
	TMDB          = NewKey(owner.Platform, connectionsPrefix+"tmdb", TMDBConnection{})
	QBittorrent   = NewKey(owner.Platform, connectionsPrefix+"qbittorrent", QBittorrentConnection{Category: DefaultQBittorrentCategory})
	Prowlarr      = NewKey(owner.Platform, connectionsPrefix+"prowlarr", ProwlarrConnection{})
	Jellyfin      = NewKey(owner.Platform, connectionsPrefix+"jellyfin", JellyfinConnection{})
	OpenSubtitles = NewKey(owner.Platform, connectionsPrefix+"opensubtitles", OpenSubtitlesConnection{})
)
