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
	// RemotePath is where qBittorrent's downloads sit as qBittorrent sees them, and LocalPath the same
	// folder as Magpie sees it. Both empty when the paths match.
	RemotePath string `json:"remotePath,omitempty"`
	LocalPath  string `json:"localPath,omitempty"`
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
