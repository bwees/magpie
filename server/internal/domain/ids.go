// Package domain holds the IDs and value types shared by every context.
package domain

import (
	"github.com/oklog/ulid/v2"
)

type (
	TitleID     string
	EpisodeID   string
	MediaFileID string
	RequestID   string
	IssueID     string
	UserID      string
	DownloadID  string
	ReleaseID   string
	SearchID    string
	AgentID     string
	JobID       string
	ProfileID   string
)

// ID is any of the ULID-backed identifiers.
type ID interface {
	~string
}

// TmdbID is TMDB's numeric ID for a movie or series.
type TmdbID int64

// NewID returns a fresh ULID. IDs made by one process sort in creation order.
func NewID[T ID]() T {
	return T(ulid.Make().String())
}

// ValidID reports whether id is a well-formed ULID.
func ValidID[T ID](id T) bool {
	_, err := ulid.ParseStrict(string(id))
	return err == nil
}
