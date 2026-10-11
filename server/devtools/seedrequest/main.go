// Command seedrequest publishes a requests.RequestApproved into the dev database, so the add-title flow
// can be demoed before Requests exists. The running server picks the event up from the outbox.
//
//	mise run dev:seed-request -tmdb 974576 [-kind MOVIE] [-profile DEV] [-tmdb-key KEY]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"magpie/internal/domain"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/db"
	"magpie/internal/platform/settings"
	requestsapi "magpie/internal/requests/api"

	"gorm.io/gorm"
)

const defaultProfile = "DEV"

var errNoTmdbID = errors.New("-tmdb is required")

func main() {
	tmdbID := flag.Int64("tmdb", 0, "TMDB ID to request")
	kind := flag.String("kind", string(domain.MediaKindMovie), "MOVIE or SERIES")
	profile := flag.String("profile", defaultProfile, "quality profile ID stored on the Title")
	tmdbKey := flag.String("tmdb-key", "", "also store this TMDB key as the platform connection")
	flag.Parse()

	if err := run(*tmdbID, domain.MediaKind(*kind), domain.ProfileID(*profile), *tmdbKey); err != nil {
		log.Fatal(err)
	}
}

func run(tmdbID int64, kind domain.MediaKind, profile domain.ProfileID, tmdbKey string) error {
	if tmdbID == 0 {
		return errNoTmdbID
	}
	env, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(env.DataDir, db.FileName), db.BackupDir(env.DataDir), []db.MigrationSet{db.PlatformMigrations()})
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	if tmdbKey != "" {
		if err := settings.TMDB.Set(ctx, settings.NewStore(d), settings.TMDBConnection{APIKey: tmdbKey}); err != nil {
			return err
		}
	}

	user := domain.NewID[domain.UserID]()
	scope := requestsapi.ScopeMovie
	if kind == domain.MediaKindSeries {
		scope = requestsapi.ScopeAllSeasons
	}
	approved := requestsapi.RequestApproved{
		Request:    domain.NewID[domain.RequestID](),
		User:       user,
		Kind:       kind,
		Tmdb:       domain.TmdbID(tmdbID),
		Scope:      scope,
		Profile:    profile,
		Auto:       true,
		ApprovedBy: user,
	}
	b := bus.New()
	if err := d.Write(ctx, func(tx *gorm.DB) error { return b.Publish(tx, bus.EventOf(approved)) }); err != nil {
		return err
	}
	fmt.Printf("published %s for %s %d (request %s)\n", approved.Type(), kind, tmdbID, approved.Request)
	return nil
}
