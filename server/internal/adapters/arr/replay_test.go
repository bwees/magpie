package arr_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"magpie/internal/adapters/arr"
	"magpie/internal/platform/migrate"
)

const (
	testKey         = "0123456789abcdef"
	fixtureSuffix   = ".json"
	nameSeparator   = "_"
	apiPathPrefix   = "/api/v3/"
	testdataDir     = "testdata"
	sonarrFixtures  = testdataDir + "/sonarr"
	radarrFixtures  = testdataDir + "/radarr"
	apiKeyHeaderKey = "X-Api-Key"
)

// replay serves recorded v3 answers from testdata/<app>: the path after /api/v3/ with "/" as "_", then
// "_" and the query when there is one. An endpoint without a recording is a 404, as in an older version.
func replay(t *testing.T, dir string) migrate.Connection {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiKeyHeaderKey) != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		name := strings.ReplaceAll(strings.TrimPrefix(r.URL.Path, apiPathPrefix), "/", nameSeparator)
		if r.URL.RawQuery != "" {
			name += nameSeparator + r.URL.RawQuery
		}
		body, err := os.ReadFile(filepath.Join(dir, name+fixtureSuffix))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return migrate.Connection{URL: server.URL + "/", APIKey: testKey}
}

func TestStatusReadsTheVersionAndRefusesTheOtherApp(t *testing.T) {
	ctx := context.Background()
	client := arr.New()
	sonarr := replay(t, sonarrFixtures)

	status, err := client.Status(ctx, migrate.AppSonarr, sonarr)
	if err != nil || status.Version != "4.0.20.3014" {
		t.Fatalf("status = %+v, %v; want Sonarr 4.0.20.3014", status, err)
	}
	if _, err := client.Status(ctx, migrate.AppRadarr, sonarr); !errors.Is(err, migrate.ErrArrWrongApp) {
		t.Fatalf("Sonarr as Radarr = %v, want ErrArrWrongApp", err)
	}
	sonarr.APIKey = "wrong"
	if _, err := client.Status(ctx, migrate.AppSonarr, sonarr); !errors.Is(err, migrate.ErrArrUnauthorized) {
		t.Fatalf("wrong key = %v, want ErrArrUnauthorized", err)
	}
	if _, err := client.Status(ctx, migrate.AppSonarr, migrate.Connection{URL: "not a url"}); !errors.Is(err, migrate.ErrArrUnreachable) {
		t.Fatalf("bad URL = %v, want ErrArrUnreachable", err)
	}
}

func TestSonarrLibraryReadsSeriesEpisodesFilesAndTheQualityModel(t *testing.T) {
	var progress [][2]int
	lib, err := arr.New().Library(context.Background(), migrate.AppSonarr, replay(t, sonarrFixtures), func(done, total int) {
		progress = append(progress, [2]int{done, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Series) != 3 || len(progress) != 3 || progress[2] != [2]int{3, 3} {
		t.Fatalf("series = %d, progress %v; want 3 series read one by one", len(lib.Series), progress)
	}
	bluey := lib.Series[0]
	if bluey.TmdbID != 0 || bluey.TvdbID != 371980 || bluey.ImdbID != "tt7678620" || bluey.MonitorNewItems != migrate.MonitorNewItemsAll || len(bluey.AlternateTitles) != 1 {
		t.Fatalf("Bluey = %+v", bluey)
	}
	if len(bluey.Episodes) != 2 || bluey.Episodes[0].TvdbID != 6789001 || bluey.Episodes[0].EpisodeFileID != 11 || bluey.Episodes[0].AirDate != "2018-10-01" {
		t.Fatalf("Bluey episodes = %+v", bluey.Episodes)
	}
	file := bluey.Files[0]
	if file.QualityID != 20 || file.Revision != 1 || file.SceneName != "Bluey.2018.S01E01.1080p.BluRay.REMUX.AVC.DD5.1-GRP" || file.Languages[0] != "English" {
		t.Fatalf("Bluey file = %+v", file)
	}
	if daily := lib.Series[2]; daily.SeriesType != migrate.SeriesTypeDaily {
		t.Fatalf("The Daily Show type = %q", daily.SeriesType)
	}

	hd := lib.QualityProfiles[0]
	if hd.Cutoff != 1001 || len(hd.Items) != 5 || hd.Items[2].GroupID != 1001 || len(hd.Items[2].Items) != 2 || hd.Items[2].Items[1].QualityID != 3 {
		t.Fatalf("HD-1080p = %+v", hd)
	}
	remux := lib.CustomFormats[0].Specs[0]
	if remux.Implementation != migrate.ImplSource || remux.Value != "7" || !remux.Required {
		t.Fatalf("Remux spec = %+v", remux)
	}
	size := lib.CustomFormats[3].Specs[1]
	if size.Min != 0 || size.Max != 1.5 {
		t.Fatalf("size spec = %+v", size)
	}
	if rp := lib.ReleaseProfiles[0]; len(rp.Ignored) != 2 || rp.Ignored[1] != `/\bRARBG\b/i` || rp.Tags[0] != 1 {
		t.Fatalf("release profile = %+v", rp)
	}
	if len(lib.QualityDefinitions) != 6 || lib.QualityDefinitions[5].MaxSize != nil {
		t.Fatalf("definitions = %+v", lib.QualityDefinitions)
	}
	if len(lib.Tags) != 1 || lib.Tags[0].Name != "kids" || len(lib.Languages) != 8 || len(lib.IndexerFlags) != 6 {
		t.Fatalf("tags %+v, languages %d, flags %d", lib.Tags, len(lib.Languages), len(lib.IndexerFlags))
	}
	if len(lib.Blocklist) != 2 || lib.Blocklist[0].EpisodeIDs[0] != 102 || lib.Blocklist[1].Protocol != migrate.ProtocolUsenet {
		t.Fatalf("blocklist = %+v", lib.Blocklist)
	}
	grab := lib.History[0]
	if grab.EventType != migrate.HistoryGrabbed || grab.Data[migrate.HistoryDataTorrentInfoHash] != "0A1B2C3D4E5F60718293A4B5C6D7E8F901234567" || grab.Data[migrate.HistoryDataProtocol] != migrate.ProtocolTorrent {
		t.Fatalf("grab = %+v", grab)
	}
	if lib.History[4].Data[migrate.HistoryDataProtocol] != migrate.ProtocolUsenet {
		t.Fatalf("usenet grab = %+v", lib.History[4])
	}
}

func TestRadarrLibraryReadsMoviesWithTheirFilesAndProfileLanguages(t *testing.T) {
	lib, err := arr.New().Library(context.Background(), migrate.AppRadarr, replay(t, radarrFixtures), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Movies) != 3 || len(lib.Series) != 0 {
		t.Fatalf("movies = %d, series = %d", len(lib.Movies), len(lib.Series))
	}
	conclave := lib.Movies[0]
	if conclave.TmdbID != 974576 || conclave.File == nil || conclave.File.QualityID != 7 || conclave.File.ReleaseGroup != "SPARKS" {
		t.Fatalf("Conclave = %+v", conclave)
	}
	if lib.Movies[2].File != nil || lib.Movies[2].TmdbID != 0 {
		t.Fatalf("Home Movies = %+v", lib.Movies[2])
	}
	if lang := lib.QualityProfiles[0].Language; lang == nil || lang.Name != "English" {
		t.Fatalf("HD-1080p language = %+v", lang)
	}
	if modifier := lib.CustomFormats[1].Specs[1]; modifier.Implementation != migrate.ImplQualityModifier || modifier.Value != "5" {
		t.Fatalf("modifier spec = %+v", modifier)
	}
	if len(lib.ReleaseProfiles) != 0 || len(lib.DelayProfiles) != 1 || lib.DelayProfiles[0].TorrentDelay != 30 {
		t.Fatalf("release %+v delay %+v", lib.ReleaseProfiles, lib.DelayProfiles)
	}
}

func TestAMissingOptionalEndpointReadsAsEmpty(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"system_status", "rootfolder", "qualityprofile", "qualitydefinition", "customformat", "delayprofile", "movie"} {
		body, err := os.ReadFile(filepath.Join(radarrFixtures, name+fixtureSuffix))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+fixtureSuffix), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"blocklist_page=1&pageSize=1000", "history_page=1&pageSize=1000"} {
		if err := os.WriteFile(filepath.Join(dir, name+fixtureSuffix), []byte(`{"page":1,"totalRecords":0,"records":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := arr.New().Library(context.Background(), migrate.AppRadarr, replay(t, dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.IndexerFlags) != 0 || len(lib.Languages) != 0 || len(lib.ReleaseProfiles) != 0 {
		t.Fatalf("library = %+v, want the missing endpoints empty", lib)
	}
}
