package api

import (
	"fmt"

	"magpie/internal/domain"
)

const (
	TypeTitleAdded          = "catalog.TitleAdded"
	TypeTitleAddFailed      = "catalog.TitleAddFailed"
	TypeMonitoringChanged   = "catalog.MonitoringChanged"
	TypeProfileChanged      = "catalog.ProfileChanged"
	TypeScanFolderMatched   = "catalog.ScanFolderMatched"
	TypeLanguagesChanged    = "catalog.LanguagesChanged"
	TypeEpisodesAnnounced   = "catalog.EpisodesAnnounced"
	TypeWantedReleased      = "catalog.WantedReleased"
	TypeEpisodesUnreleased  = "catalog.EpisodesUnreleased"
	TypeEpisodeGroupChanged = "catalog.EpisodeGroupChanged"
	TypeTitleRemoved        = "catalog.TitleRemoved"
)

// TitleAddFailed.Reason values.
const (
	TitleAddFailedTmdbNotFound     = "TMDB_NOT_FOUND"
	TitleAddFailedTmdbUnauthorized = "TMDB_UNAUTHORIZED"
	TitleAddFailedTmdbUnavailable  = "TMDB_UNAVAILABLE"
)

// GlobalLanguagesAggregate orders changes to the global wanted-language list.
const GlobalLanguagesAggregate = "GLOBAL"

const tmdbAggregateSeparator = ":"

// TitleAdded is durable: a new Title was created and its metadata stored.
type TitleAdded struct {
	Title       domain.TitleID     `json:"title"`
	Kind        domain.MediaKind   `json:"kind"`
	External    domain.ExternalIDs `json:"external"`
	Profile     domain.ProfileID   `json:"profile"`
	Monitored   []domain.EpisodeID `json:"monitored"` // empty for movies
	Released    bool               `json:"released"`
	RequestedBy *domain.UserID     `json:"requestedBy"`
	RequestID   *domain.RequestID  `json:"requestId"`  // correlation for Requests; nil unless Origin = REQUEST
	Origin      domain.Origin      `json:"origin"`     // REQUEST | MANUAL | LIBRARY_SCAN | MIGRATION
	ScanFolder  string             `json:"scanFolder"` // set when Origin = LIBRARY_SCAN: the folder Library reported
}

func (TitleAdded) Type() string          { return TypeTitleAdded }
func (e TitleAdded) AggregateID() string { return string(e.Title) }
func (TitleAdded) Durable() bool         { return true }

// TitleAddFailed is durable: a RequestApproved could not create the Title.
type TitleAddFailed struct {
	Tmdb      domain.TmdbID     `json:"tmdb"`
	Kind      domain.MediaKind  `json:"kind"`
	RequestID *domain.RequestID `json:"requestId"`
	Reason    string            `json:"reason"` // TitleAddFailedTmdb*
}

func (TitleAddFailed) Type() string { return TypeTitleAddFailed }

// AggregateID is the request, so Requests sees it in order with that request's other events.
func (e TitleAddFailed) AggregateID() string {
	if e.RequestID != nil {
		return string(*e.RequestID)
	}
	return string(e.Kind) + tmdbAggregateSeparator + fmt.Sprint(e.Tmdb)
}
func (TitleAddFailed) Durable() bool { return true }

// MonitoringChanged is durable: episodes or the movie were (un)monitored.
type MonitoringChanged struct {
	Title     domain.TitleID     `json:"title"`
	Added     []domain.EpisodeID `json:"added"`
	Removed   []domain.EpisodeID `json:"removed"`
	Movie     *bool              `json:"movie"`     // set for movies
	RequestID *domain.RequestID  `json:"requestId"` // set when caused by a request
}

func (MonitoringChanged) Type() string          { return TypeMonitoringChanged }
func (e MonitoringChanged) AggregateID() string { return string(e.Title) }
func (MonitoringChanged) Durable() bool         { return true }

// ProfileChanged is durable: the Title's quality profile assignment changed.
type ProfileChanged struct {
	Title domain.TitleID   `json:"title"`
	From  domain.ProfileID `json:"from"`
	To    domain.ProfileID `json:"to"`
}

func (ProfileChanged) Type() string          { return TypeProfileChanged }
func (e ProfileChanged) AggregateID() string { return string(e.Title) }
func (ProfileChanged) Durable() bool         { return true }

// ScanFolderMatched is durable: a folder reported by library.FolderFound now belongs to a Title
// (auto-matched or confirmed in the review queue). Library links the files.
type ScanFolderMatched struct {
	Folder string         `json:"folder"`
	Title  domain.TitleID `json:"title"`
}

func (ScanFolderMatched) Type() string          { return TypeScanFolderMatched }
func (e ScanFolderMatched) AggregateID() string { return string(e.Title) }
func (ScanFolderMatched) Durable() bool         { return true }

// LanguagesChanged is durable: the global wanted-language list or a Title's override changed.
type LanguagesChanged struct {
	Title     *domain.TitleID  `json:"title"`     // nil = global list changed
	Languages []WantedLanguage `json:"languages"` // new effective list (for Title) or new global list
}

func (LanguagesChanged) Type() string { return TypeLanguagesChanged }
func (e LanguagesChanged) AggregateID() string {
	if e.Title == nil {
		return GlobalLanguagesAggregate
	}
	return string(*e.Title)
}
func (LanguagesChanged) Durable() bool { return true }

// EpisodesAnnounced is durable: a metadata refresh found new episodes for a series.
type EpisodesAnnounced struct {
	Title    domain.TitleID      `json:"title"`
	Episodes []domain.EpisodeRef `json:"episodes"`
}

func (EpisodesAnnounced) Type() string          { return TypeEpisodesAnnounced }
func (e EpisodesAnnounced) AggregateID() string { return string(e.Title) }
func (EpisodesAnnounced) Durable() bool         { return true }

// WantedReleased is durable: monitored items crossed their release point.
type WantedReleased struct {
	Title    domain.TitleID     `json:"title"`
	Episodes []domain.EpisodeID `json:"episodes"` // empty for a movie
}

func (WantedReleased) Type() string          { return TypeWantedReleased }
func (e WantedReleased) AggregateID() string { return string(e.Title) }
func (WantedReleased) Durable() bool         { return true }

// EpisodesUnreleased is durable: a refresh moved released episodes' air dates into the future.
type EpisodesUnreleased struct {
	Title    domain.TitleID     `json:"title"`
	Episodes []domain.EpisodeID `json:"episodes"`
}

func (EpisodesUnreleased) Type() string          { return TypeEpisodesUnreleased }
func (e EpisodesUnreleased) AggregateID() string { return string(e.Title) }
func (EpisodesUnreleased) Durable() bool         { return true }

// EpisodeGroupChanged is durable: the numbering of a series changed.
type EpisodeGroupChanged struct {
	Title   domain.TitleID `json:"title"`
	GroupID string         `json:"groupId"` // "" = aired order
	Moves   []EpisodeMove  `json:"moves"`
}

type EpisodeMove struct {
	Episode    domain.EpisodeID `json:"episode"`
	FromSeason int              `json:"fromSeason"`
	FromNo     int              `json:"fromNo"`
	ToSeason   int              `json:"toSeason"`
	ToNo       int              `json:"toNo"`
}

func (EpisodeGroupChanged) Type() string          { return TypeEpisodeGroupChanged }
func (e EpisodeGroupChanged) AggregateID() string { return string(e.Title) }
func (EpisodeGroupChanged) Durable() bool         { return true }

// TitleRemoved is durable: the Title was deleted from Magpie.
type TitleRemoved struct {
	Title       domain.TitleID `json:"title"`
	DeleteFiles bool           `json:"deleteFiles"`
}

func (TitleRemoved) Type() string          { return TypeTitleRemoved }
func (e TitleRemoved) AggregateID() string { return string(e.Title) }
func (TitleRemoved) Durable() bool         { return true }

// Messages lists every event and command this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{
		TitleAdded{},
		TitleAddFailed{},
		MonitoringChanged{},
		ProfileChanged{},
		ScanFolderMatched{},
		LanguagesChanged{},
		EpisodesAnnounced{},
		WantedReleased{},
		EpisodesUnreleased{},
		EpisodeGroupChanged{},
		TitleRemoved{},
	}
}
