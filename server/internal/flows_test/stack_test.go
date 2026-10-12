package flows_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	acquisitionfakes "magpie/internal/acquisition/fakes"
	"magpie/internal/catalog"
	catalogapi "magpie/internal/catalog/api"
	catalogfakes "magpie/internal/catalog/fakes"
	"magpie/internal/domain"
	"magpie/internal/identity"
	identityfakes "magpie/internal/identity/fakes"
	"magpie/internal/library"
	libraryapi "magpie/internal/library/api"
	libraryfakes "magpie/internal/library/fakes"
	"magpie/internal/platform/jobs"
	"magpie/internal/requests"
	requestsapi "magpie/internal/requests/api"
	"magpie/internal/subtitles"
	subtitlesfakes "magpie/internal/subtitles/fakes"
	"magpie/internal/transcode"
	transcodeapi "magpie/internal/transcode/api"

	"go.uber.org/fx"
)

const (
	flowIndexerID   = 1
	flowIndexerName = "Flow Indexer"
	moviesCategory  = 2000
	releaseSeeders  = 10
	// topSeeders ranks a release above an otherwise equal one with releaseSeeders.
	topSeeders = 100

	// releaseSize fits the default profile's size limits for a movie of movieRuntime.
	releaseSize     int64 = 4 << 30
	movieRuntime          = 120
	movieLength           = movieRuntime * time.Minute
	movieKbps             = 12000
	movieWidth            = 1920
	movieHeight           = 1080
	hdWidth               = 1280
	hdHeight              = 720
	eightBit              = 8
	tenBit                = 10
	videoExt              = ".mkv"
	magnetPrefix          = "magnet:?xt=urn:btih:"
	magnetNameParam       = "&dn="

	dirMode  = 0o755
	fileMode = 0o644

	requestsPath      = "/api/requests"
	rootFoldersPath   = "/api/library/root-folders"
	libraryScanPath   = "/api/library/scan"
	libraryFilesPath  = "/api/library/files"
	indexerSyncPath   = "/api/acquisition/indexers/sync"
	downloadsPath     = "/api/acquisition/downloads"
	searchesPath      = "/api/acquisition/searches"
	cutoffPath        = "/api/acquisition/cutoff"
	pipelinePath      = "/api/activity/pipeline"
	transcodeJobsPath = "/api/transcode/jobs"
	scanReviewPath    = "/api/catalog/scan-review"
	profilesPath      = "/api/acquisition/profiles"
	qualitiesPath     = "/api/acquisition/qualities"
	titleQuery        = "?title="
)

// stack is the whole app with every external system faked: TMDB, Prowlarr, qBittorrent, ffprobe, Jellyfin
// and OpenSubtitles. Files live on the real disk under a temp dir, with one Movies root folder and the
// download folder next to it, so imports hardlink as they do on one volume.
type stack struct {
	*harness
	tmdb         *catalogfakes.TMDB
	jellyfinAuth *identityfakes.JellyfinAuth
	prowlarr     *acquisitionfakes.Prowlarr
	qbit         *acquisitionfakes.QBittorrent
	prober       *libraryfakes.MediaProber
	jellyfin     *libraryfakes.JellyfinLibrary
	downloads    *acquisition.DownloadService
	searches     *acquisition.SearchService
	enqueuer     jobs.Enqueuer
	movies       string
	downloadDir  string
}

// newStack boots the app on fakes, signs in the admin, adds the Movies root folder and syncs one indexer.
func newStack(t *testing.T, options ...fx.Option) *stack {
	t.Helper()
	disk := t.TempDir()
	s := &stack{
		tmdb: catalogfakes.NewTMDB(), jellyfinAuth: identityfakes.NewJellyfinAuth(),
		prowlarr: acquisitionfakes.NewProwlarr(), qbit: acquisitionfakes.NewQBittorrent(),
		prober: libraryfakes.NewMediaProber(), jellyfin: libraryfakes.NewJellyfinLibrary(),
		movies: filepath.Join(disk, "movies"), downloadDir: filepath.Join(disk, "downloads"),
	}
	for _, dir := range []string{s.movies, s.downloadDir} {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			t.Fatal(err)
		}
	}
	s.prowlarr.AddIndexer(acquisition.ProwlarrIndexer{
		ID: flowIndexerID, Name: flowIndexerName, Privacy: acquisition.PrivacyPublic,
		Caps: &acquisition.TorznabCaps{
			MovieSearch: true, MovieParams: []acquisition.TorznabParam{acquisition.ParamTMDB, acquisition.ParamQuery},
			Categories: []int{moviesCategory},
		},
	})
	location := libraryfakes.NewDownloadLocation()
	location.Set(s.downloadDir)

	all := append([]fx.Option{
		withAgentSecret(),
		fx.Decorate(func(identity.JellyfinAuth) identity.JellyfinAuth { return s.jellyfinAuth }),
		fx.Decorate(func(catalog.TMDB) catalog.TMDB { return s.tmdb }),
		fx.Decorate(func(acquisition.Prowlarr) acquisition.Prowlarr { return s.prowlarr }),
		fx.Decorate(func(acquisition.QBittorrent) acquisition.QBittorrent { return s.qbit }),
		fx.Decorate(func(library.DownloadLocation) library.DownloadLocation { return location }),
		fx.Decorate(func(library.MediaProber) library.MediaProber { return s.prober }),
		fx.Decorate(func(transcode.MediaProber) transcode.MediaProber { return s.prober }),
		fx.Decorate(func(library.JellyfinLibrary) library.JellyfinLibrary { return s.jellyfin }),
		fx.Decorate(func(subtitles.OpenSubtitles) subtitles.OpenSubtitles { return subtitlesfakes.NewOpenSubtitles() }),
		fx.Decorate(func(subtitles.Syncer) subtitles.Syncer { return subtitlesfakes.NewSyncer() }),
		fx.Populate(&s.downloads, &s.searches, &s.enqueuer),
	}, options...)
	s.harness = boot(t, all...)
	s.signInAsAdmin()

	rec := s.do(http.MethodPost, rootFoldersPath, library.AddRootFolderRequest{Kind: domain.MediaKindMovie, Path: s.movies})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add root folder = %d: %s", rec.Code, rec.Body)
	}
	if rec := s.do(http.MethodPost, indexerSyncPath, nil); rec.Code != http.StatusOK {
		t.Fatalf("sync indexers = %d: %s", rec.Code, rec.Body)
	}
	return s
}

// as sends a request with another user's session; later requests keep it.
func (s *stack) as(session *http.Cookie, method, path string, body any) *httptest.ResponseRecorder {
	s.t.Helper()
	s.session = session
	return s.do(method, path, body)
}

func (s *stack) request(session *http.Cookie, tmdb domain.TmdbID) requests.RequestResponse {
	s.t.Helper()
	rec := s.as(session, http.MethodPost, requestsPath, requests.CreateRequestRequest{Kind: domain.MediaKindMovie, TmdbID: tmdb, Scope: requestsapi.ScopeMovie})
	return decodeBody[requests.RequestResponse](s.t, rec.Code, rec.Body.Bytes(), http.StatusCreated)
}

// movieOnTMDB makes TMDB know a two-hour English movie released digitally in year.
func (s *stack) movieOnTMDB(id domain.TmdbID, name string, year int) {
	released := time.Date(year, time.March, 1, 0, 0, 0, 0, time.UTC)
	s.tmdb.PutMovie(catalog.TmdbMovie{
		ID: id, Title: name, OriginalLanguage: english, ReleaseDate: &released, RuntimeMinutes: movieRuntime,
		ReleaseDates: []catalog.TmdbReleaseDate{{Type: catalog.TmdbReleaseDigital, Date: released}},
	})
}

// titleAdded waits for Catalog to add the movie and returns the title's ID.
func (s *stack) titleAdded(tmdb domain.TmdbID) domain.TitleID {
	s.t.Helper()
	return awaitEvent(s.harness, catalogapi.TypeTitleAdded, func(e catalogapi.TitleAdded) bool { return e.External.Tmdb == tmdb }).Title
}

// release is a movie release offered as a magnet.
func release(name string, seeders int) acquisition.TorznabItem {
	hash := sha1.Sum([]byte(name))
	return acquisition.TorznabItem{
		GUID: name, Title: name, MagnetURI: magnetPrefix + hex.EncodeToString(hash[:]) + magnetNameParam + url.QueryEscape(name),
		SizeBytes: releaseSize, Seeders: seeders, PublishedAt: time.Now(), Categories: []int{moviesCategory},
		DownloadVolumeFactor: 1, UploadVolumeFactor: 1,
	}
}

// answer makes the indexer answer every search with items.
func (s *stack) answer(items ...acquisition.TorznabItem) {
	s.prowlarr.SetResults(flowIndexerID, items...)
}

// grabbed waits for Acquisition to grab the release.
func (s *stack) grabbed(item acquisition.TorznabItem) acquisitionapi.ReleaseGrabbed {
	s.t.Helper()
	return awaitEvent(s.harness, acquisitionapi.TypeReleaseGrabbed, func(e acquisitionapi.ReleaseGrabbed) bool { return e.Release == item.Title })
}

func hashOf(item acquisition.TorznabItem) string {
	return acquisitionfakes.HashOf(acquisition.AddRequest{Magnet: item.MagnetURI})
}

// pollUntil runs the download poll, as its ticker does every few seconds, until check holds. A poll queues a
// check only for a download no check is busy with, so one poll may come too early.
func (s *stack) pollUntil(what string, check func() bool) {
	s.t.Helper()
	eventually(s.t, what, func() bool {
		if err := s.downloads.Poll(context.Background(), s.enqueuer); err != nil {
			s.t.Fatal(err)
		}
		return check()
	})
}

// pollForEvent polls downloads until a matching durable event is published.
func pollForEvent[T any](s *stack, eventType string, match func(T) bool) T {
	s.t.Helper()
	var found []T
	s.pollUntil("a matching "+eventType, func() bool {
		found = published(s.harness, eventType, match)
		return len(found) > 0
	})
	return found[0]
}

func (s *stack) downloadState(id domain.DownloadID) (acquisitionapi.DownloadState, bool) {
	s.t.Helper()
	list := getJSON[[]acquisition.DownloadResponse](s.harness, downloadsPath)
	i := slices.IndexFunc(list, func(d acquisition.DownloadResponse) bool { return d.ID == id })
	if i < 0 {
		return "", false
	}
	return list[i].State, true
}

// pollToState polls downloads until the download reaches state.
func (s *stack) pollToState(id domain.DownloadID, state acquisitionapi.DownloadState) {
	s.t.Helper()
	s.pollUntil("download "+string(id)+" is "+string(state), func() bool {
		got, ok := s.downloadState(id)
		return ok && got == state
	})
}

// torrentFiles lists the files of a release's torrent, each under the release's folder.
func torrentFiles(item acquisition.TorznabItem, names ...string) []acquisition.TorrentFile {
	files := make([]acquisition.TorrentFile, 0, len(names))
	for i, name := range names {
		files = append(files, acquisition.TorrentFile{Index: i, Path: item.Title + "/" + name, Size: releaseSize, Priority: 1})
	}
	return files
}

// receiveMetadata gives the magnet its file list, as qBittorrent does once peers send it.
func (s *stack) receiveMetadata(item acquisition.TorznabItem, files ...acquisition.TorrentFile) {
	s.qbit.SetFiles(hashOf(item), files...)
}

// videoOf is the video file a release's torrent holds unless a test names another.
func videoOf(item acquisition.TorznabItem) string {
	return item.Title + videoExt
}

// finishTorrent writes the release's video and extra files to the download folder and marks the torrent done.
func (s *stack) finishTorrent(item acquisition.TorznabItem, info libraryapi.MediaInfo, video string, extras ...string) {
	s.t.Helper()
	content := filepath.Join(s.downloadDir, item.Title)
	s.writeVideo(filepath.Join(content, video), info)
	for _, extra := range extras {
		writeFile(s.t, filepath.Join(content, extra), []byte(extra))
	}
	s.qbit.Update(hashOf(item), func(t *acquisition.Torrent) {
		t.Progress, t.Status, t.ContentPath = 1, acquisition.TorrentUploading, content
	})
}

// startDownload passes the grabbed release's metadata check with clean files.
func (s *stack) startDownload(item acquisition.TorznabItem, files ...string) acquisitionapi.ReleaseGrabbed {
	s.t.Helper()
	grab := s.grabbed(item)
	s.receiveMetadata(item, torrentFiles(item, files...)...)
	s.pollToState(grab.Download, acquisitionapi.StateDownloading)
	return grab
}

// completeDownload finishes the torrent and polls until Acquisition hands it to Library.
func (s *stack) completeDownload(item acquisition.TorznabItem, grab acquisitionapi.ReleaseGrabbed, info libraryapi.MediaInfo, video string, extras ...string) {
	s.t.Helper()
	s.finishTorrent(item, info, video, extras...)
	pollForEvent(s, acquisitionapi.TypeDownloadCompleted, func(e acquisitionapi.DownloadCompleted) bool { return e.Download == grab.Download })
}

// download runs a grabbed release, its video and extra files, to its import.
func (s *stack) download(item acquisition.TorznabItem, info libraryapi.MediaInfo, extras ...string) libraryapi.FileImported {
	s.t.Helper()
	return s.downloadNamed(item, info, videoOf(item), extras...)
}

// downloadNamed is download for a release whose video has another name.
func (s *stack) downloadNamed(item acquisition.TorznabItem, info libraryapi.MediaInfo, video string, extras ...string) libraryapi.FileImported {
	s.t.Helper()
	grab := s.startDownload(item, append([]string{video}, extras...)...)
	s.completeDownload(item, grab, info, video, extras...)
	return s.imported(grab.Download)
}

// imported waits for Library to import the download.
func (s *stack) imported(download domain.DownloadID) libraryapi.FileImported {
	s.t.Helper()
	return awaitEvent(s.harness, libraryapi.TypeFileImported, func(e libraryapi.FileImported) bool {
		return e.Download != nil && *e.Download == download
	})
}

// writeVideo puts a video on disk and tells the fake ffprobe what it holds.
func (s *stack) writeVideo(path string, info libraryapi.MediaInfo) {
	s.t.Helper()
	writeFile(s.t, path, []byte("video "+path))
	s.prober.Set(path, info)
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, fileMode); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// h264Movie is a two-hour 1080p H.264 movie at a bitrate worth transcoding, with English audio.
func h264Movie() libraryapi.MediaInfo {
	return libraryapi.MediaInfo{
		Container: "matroska", Duration: movieLength, BitrateKbps: movieKbps, Width: movieWidth, Height: movieHeight,
		HDR: libraryapi.HDRNone,
		Streams: []libraryapi.Stream{
			{Index: 0, Kind: libraryapi.StreamVideo, Codec: "h264", BitDepth: eightBit},
			{Index: 1, Kind: libraryapi.StreamAudio, Codec: "aac", Language: english},
		},
	}
}

// hevcOf is how ffprobe reads a good HEVC 10-bit transcode of info.
func hevcOf(info libraryapi.MediaInfo) libraryapi.MediaInfo {
	info.Streams = slices.Clone(info.Streams)
	info.Streams[0] = libraryapi.Stream{Index: 0, Kind: libraryapi.StreamVideo, Codec: transcodeapi.VideoCodecHEVC, BitDepth: tenBit}
	return info
}

// scan asks Library to walk the root folders, as Settings → Library → Scan does.
func (s *stack) scan() {
	s.t.Helper()
	if rec := s.do(http.MethodPost, libraryScanPath, nil); rec.Code != http.StatusAccepted {
		s.t.Fatalf("scan = %d: %s", rec.Code, rec.Body)
	}
}

func (s *stack) libraryFile(id domain.MediaFileID) library.MediaFileResponse {
	s.t.Helper()
	return getJSON[library.MediaFileResponse](s.harness, libraryFilesPath+"/"+string(id))
}

func (s *stack) cutoff(title domain.TitleID) []acquisition.CutoffStatusResponse {
	s.t.Helper()
	return getJSON[[]acquisition.CutoffStatusResponse](s.harness, cutoffPath+titleQuery+string(title))
}

func (s *stack) transcodeJob(id domain.JobID) transcode.JobResponse {
	s.t.Helper()
	list := getJSON[transcode.JobListResponse](s.harness, transcodeJobsPath)
	i := slices.IndexFunc(list.Jobs, func(j transcode.JobResponse) bool { return j.ID == string(id) })
	if i < 0 {
		s.t.Fatalf("job %s is not listed: %+v", id, list.Jobs)
	}
	return list.Jobs[i]
}

func (s *stack) awaitJobState(id domain.JobID, state transcodeapi.JobState) {
	s.t.Helper()
	eventually(s.t, "job "+string(id)+" is "+string(state), func() bool { return s.transcodeJob(id).State == state })
}

// drain stops new offers to the agent, as the admin's Drain toggle does. The agent keeps its capabilities.
func (s *stack) drain(agent *fakeAgent) {
	s.t.Helper()
	eventually(s.t, "the agent is registered", func() bool {
		agents := getJSON[[]transcode.AgentResponse](s.harness, agentsPath)
		return slices.ContainsFunc(agents, func(a transcode.AgentResponse) bool { return a.ID == string(agent.id) })
	})
	body := transcode.UpdateAgentRequest{Mode: transcodeapi.TransferDownloadFirst, Draining: true}
	if rec := s.do(http.MethodPatch, agentsPath+"/"+string(agent.id), body); rec.Code != http.StatusOK {
		s.t.Fatalf("drain agent = %d: %s", rec.Code, rec.Body)
	}
}

// Size limits for a quality group added to a profile, in MB per minute of runtime.
const (
	groupMinMBPerMin       = 10
	groupPreferredMBPerMin = 1999
	groupMaxMBPerMin       = 2000
)

// editDefaultProfile changes the 1080p profile every new title gets, as Settings → Quality does.
func (s *stack) editDefaultProfile(change func(*acquisition.ProfileRequest)) {
	s.t.Helper()
	summaries := getJSON[[]acquisition.ProfileSummaryResponse](s.harness, profilesPath)
	i := slices.IndexFunc(summaries, func(p acquisition.ProfileSummaryResponse) bool { return p.Preset == acquisitionapi.Preset1080p })
	if i < 0 {
		s.t.Fatalf("profiles = %+v, want the 1080p preset", summaries)
	}
	path := profilesPath + "/" + string(summaries[i].ID)
	p := getJSON[acquisition.ProfileResponse](s.harness, path)
	req := acquisition.ProfileRequest{
		Name: p.Name, UpgradeAllowed: p.UpgradeAllowed, Cutoff: p.Cutoff, MinFormatScore: p.MinFormatScore,
		CutoffFormatScore: p.CutoffFormatScore, MinUpgradeFormatScore: p.MinUpgradeFormatScore, PropersMode: p.PropersMode,
		FormatScores: []acquisition.FormatScoreRequest{},
	}
	for _, score := range p.FormatScores {
		req.FormatScores = append(req.FormatScores, acquisition.FormatScoreRequest{FormatID: score.FormatID, Score: score.Score})
	}
	for _, item := range p.Items {
		out := acquisition.ProfileItemRequest{
			Allowed: item.Allowed, MinMBPerMin: item.MinMBPerMin, PreferredMBPerMin: item.PreferredMBPerMin, MaxMBPerMin: item.MaxMBPerMin,
		}
		if item.Group != nil {
			out.GroupName = item.Group.Name
		}
		for _, q := range item.Qualities {
			out.QualityIDs = append(out.QualityIDs, q.ID)
		}
		req.Items = append(req.Items, out)
	}
	change(&req)
	if rec := s.do(http.MethodPut, path, req); rec.Code != http.StatusOK {
		s.t.Fatalf("update profile = %d: %s", rec.Code, rec.Body)
	}
}

// qualityGroup is an allowed profile item holding every quality at one resolution.
func (s *stack) qualityGroup(name string, resolution int) acquisition.ProfileItemRequest {
	s.t.Helper()
	group := acquisition.ProfileItemRequest{
		GroupName: name, Allowed: true,
		MinMBPerMin: groupMinMBPerMin, PreferredMBPerMin: groupPreferredMBPerMin, MaxMBPerMin: groupMaxMBPerMin,
	}
	for _, q := range getJSON[[]acquisition.QualityResponse](s.harness, qualitiesPath) {
		if q.Resolution == resolution {
			group.QualityIDs = append(group.QualityIDs, q.ID)
		}
	}
	return group
}

// searchByHand runs the title page's interactive search and returns it once every indexer answered.
func (s *stack) searchByHand(title domain.TitleID) acquisition.SearchResponse {
	s.t.Helper()
	rec := s.do(http.MethodPost, searchesPath, acquisition.StartSearchRequest{Title: title, Interactive: true})
	started := decodeBody[acquisition.SearchResponse](s.t, rec.Code, rec.Body.Bytes(), http.StatusCreated)
	var search acquisition.SearchResponse
	eventually(s.t, "the interactive search completes", func() bool {
		search = getJSON[acquisition.SearchResponse](s.harness, searchesPath+"/"+string(started.SearchID))
		return search.Completed
	})
	return search
}
