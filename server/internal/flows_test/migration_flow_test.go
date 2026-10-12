package flows_test

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/migrate"
	migratefakes "magpie/internal/platform/migrate/fakes"
	"magpie/internal/transcode"
	transcodeapi "magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"go.uber.org/fx"
)

const (
	radarrVersion     = "6.4.4.10685"
	radarrMoviesRoot  = "/radarr/movies"
	radarrMovieID     = 40
	radarrFileID      = 41
	radarrProfileID   = 4
	radarrProfileName = "HD-1080p"
	radarrWebDL1080p  = 3
	radarrBluray1080p = 7
	radarrURL         = "http://radarr.test"
	radarrKey         = "radarr key"

	migratedTmdb   = domain.TmdbID(8008)
	migratedName   = "Migrated Flow"
	migratedYear   = 2022
	migratedFolder = "Migrated Flow (2022)"
	migratedScene  = "Migrated.Flow.2022.1080p.BluRay.x264-GRP"
)

// radarrWithOneMovie is a Radarr library holding one 1080p Blu-ray movie on its HD profile.
func radarrWithOneMovie() migrate.ArrLibrary {
	added := time.Date(migratedYear, time.June, 1, 0, 0, 0, 0, time.UTC)
	return migrate.ArrLibrary{
		App: migrate.AppRadarr, Version: radarrVersion,
		Movies: []migrate.ArrMovie{{
			ID: radarrMovieID, Title: migratedName, Year: migratedYear, TmdbID: int(migratedTmdb),
			Path: radarrMoviesRoot + "/" + migratedFolder, QualityProfileID: radarrProfileID, Monitored: true, Added: added,
			File: &migrate.ArrFile{
				ID: radarrFileID, Path: radarrMoviesRoot + "/" + migratedFolder + "/" + migratedScene + videoExt, Size: 1,
				QualityID: radarrBluray1080p, Revision: 1, SceneName: migratedScene, DateAdded: added,
			},
		}},
		RootFolders: []migrate.ArrRootFolder{{ID: 1, Path: radarrMoviesRoot}},
		QualityProfiles: []migrate.ArrQualityProfile{{
			ID: radarrProfileID, Name: radarrProfileName, UpgradeAllowed: true, Cutoff: radarrBluray1080p,
			Items: []migrate.ArrProfileItem{
				{QualityID: radarrWebDL1080p, Allowed: true}, {QualityID: radarrBluray1080p, Allowed: true},
			},
		}},
	}
}

func (s *stack) migration() migrate.MigrationResponse {
	s.t.Helper()
	return getJSON[migrate.MigrationResponse](s.harness, migrationPath)
}

// migrate walks the import wizard: Radarr connected, Sonarr skipped, Radarr's folder mapped to the server's,
// a dry run, every review settled by default, the cutover confirmed and the commit.
func (s *stack) migrate(radarrRoot string) {
	s.t.Helper()
	steps := []struct {
		method, path string
		body         any
		status       int
	}{
		{http.MethodPut, migrationPath + "/connections", migrate.MigrationConnectRequest{App: migrate.AppSonarr, Skip: true}, http.StatusOK},
		{http.MethodPut, migrationPath + "/connections", migrate.MigrationConnectRequest{App: migrate.AppRadarr, URL: radarrURL, APIKey: radarrKey}, http.StatusOK},
		{http.MethodPut, migrationPath + "/paths", migrate.MigrationPathsRequest{Paths: []migrate.MigrationPathRequest{
			{App: migrate.AppRadarr, From: radarrMoviesRoot, To: radarrRoot},
		}}, http.StatusOK},
		{http.MethodPost, migrationPath + "/dry-run", nil, http.StatusAccepted},
	}
	for _, step := range steps {
		if rec := s.do(step.method, step.path, step.body); rec.Code != step.status {
			s.t.Fatalf("%s %s = %d: %s", step.method, step.path, rec.Code, rec.Body)
		}
	}
	eventually(s.t, "the dry run reaches review", func() bool { return s.migration().Phase == migrate.PhaseReview })
	for _, queue := range migrate.ReviewQueues {
		if rec := s.do(http.MethodPost, migrationPath+"/reviews/skip", migrate.MigrationSkipQueueRequest{Queue: queue}); rec.Code != http.StatusOK {
			s.t.Fatalf("skip %s = %d: %s", queue, rec.Code, rec.Body)
		}
	}
	if rec := s.do(http.MethodPut, migrationPath+"/cutover", migrate.MigrationCutoverRequest{Confirmed: true}); rec.Code != http.StatusOK {
		s.t.Fatalf("cutover = %d: %s", rec.Code, rec.Body)
	}
	if rec := s.do(http.MethodPost, migrationPath+"/commit", nil); rec.Code != http.StatusAccepted {
		s.t.Fatalf("commit = %d: %s", rec.Code, rec.Body)
	}
	eventually(s.t, "the commit finishes", func() bool { return s.migration().Phase == migrate.PhaseDone })
}

// The migration: a Radarr movie is imported through the wizard. Catalog adds it and Library links its file
// as MIGRATION; nothing is searched, the file meets its cutoff and its transcode queues in the backlog.
func TestARadarrMovieIsMigratedLinkedAndQueuedInTheBacklog(t *testing.T) {
	arr := migratefakes.NewArr()
	arr.Put(radarrWithOneMovie())
	s := newStack(t, fx.Decorate(func(migrate.Arr) migrate.Arr { return arr }))
	agent := connectAgent(s.harness, agentproto.CapEncHEVC10)
	s.drain(agent)
	s.movieOnTMDB(migratedTmdb, migratedName, migratedYear)
	video := s.placeMovie(migratedFolder, migratedScene+videoExt)

	s.migrate(s.movies)

	migrated := func() []string {
		var problems []string
		added := published(s.harness, catalogapi.TypeTitleAdded, func(e catalogapi.TitleAdded) bool { return e.External.Tmdb == migratedTmdb })
		if len(added) != 1 || added[0].Origin != domain.OriginMigration {
			return append(problems, fmt.Sprintf("TitleAdded = %+v, want one MIGRATION", added))
		}
		imported := published(s.harness, libraryapi.TypeFileImported, func(e libraryapi.FileImported) bool { return e.Title == added[0].Title })
		if len(imported) != 1 || imported[0].Origin != domain.OriginMigration || imported[0].Path != video || imported[0].Download != nil {
			return append(problems, fmt.Sprintf("FileImported = %+v, want %s linked as MIGRATION", imported, filepath.Base(video)))
		}
		file := imported[0].MediaFile
		if searches := s.outbox(acquisitionapi.TypeSearchStarted); len(searches) != 0 {
			problems = append(problems, fmt.Sprintf("searches started: %v, want none", searches))
		}
		met := published(s.harness, acquisitionapi.TypeCutoffMet, func(e acquisitionapi.CutoffMet) bool { return e.File == file })
		if len(met) != 1 || met[0].Origin != domain.OriginMigration {
			problems = append(problems, fmt.Sprintf("CutoffMet = %+v, want one MIGRATION", met))
		}
		queued := published(s.harness, transcodeapi.TypeJobQueued, func(e transcodeapi.JobQueued) bool { return e.MediaFile == file })
		if len(queued) != 1 || queued[0].Origin != domain.OriginMigration {
			problems = append(problems, fmt.Sprintf("JobQueued = %+v, want one MIGRATION transcode", queued))
		}
		if backlog := getJSON[transcode.BacklogResponse](s.harness, transcodeBacklogPath); backlog.Backlog != 1 {
			problems = append(problems, fmt.Sprintf("backlog = %+v, want the migrated file queued", backlog))
		}
		return problems
	}
	settles(t, "the migrated movie", migrated)

	s.redeliverAll()
	settles(t, "the migrated movie after redelivery", migrated)
}
