package qbittorrent

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/platform/settings"
)

const (
	webAPIFixtureDir = "testdata/webapi"
	downloadingHash  = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b"
	otherAppHash     = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4"
	metadataHash     = "2c26b46b68ffc68ff99b453c1d30413413422d70"
	testMagnet       = "magnet:?xt=urn:btih:2c26b46b68ffc68ff99b453c1d30413413422d70&dn=Movie"
	maxFormBytes     = 1 << 20
)

// reply is a recorded answer: a fixture file served with a status.
type reply struct {
	status  int
	fixture string
}

// recordedRequest is what the client sent: its form fields and, for an upload, the .torrent part.
type recordedRequest struct {
	path    string
	form    url.Values
	torrent []byte
}

// webAPI replays recorded WebAPI 5.2 answers by path, and maindata by rid.
type webAPI struct {
	t        *testing.T
	replies  map[string]reply
	maindata map[string]string

	mu       sync.Mutex
	requests []recordedRequest
}

func newWebAPI(t *testing.T) *webAPI {
	return &webAPI{
		t: t,
		replies: map[string]reply{
			pathWebAPIVersion:  {http.StatusOK, "../health/" + fixtureWebAPI52},
			pathFiles:          {http.StatusOK, "files.json"},
			pathAdd:            {http.StatusAccepted, "add_pending.json"},
			pathStart:          {http.StatusOK, ""},
			pathDelete:         {http.StatusOK, ""},
			pathSetShareLimits: {http.StatusOK, ""},
		},
		maindata: map[string]string{"0": "maindata_full.json", "1": "maindata_delta.json"},
	}
}

func (w *webAPI) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Header.Get(authorizationHeader) != bearerPrefix+testAPIKey {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	recorded := recordedRequest{path: r.URL.Path, form: r.URL.Query()}
	if strings.HasPrefix(r.Header.Get(contentTypeHeader), "multipart/") {
		if err := r.ParseMultipartForm(maxFormBytes); err != nil {
			w.t.Error(err)
		}
		recorded.form = r.MultipartForm.Value
		if files := r.MultipartForm.File[formTorrents]; len(files) > 0 {
			f, _ := files[0].Open()
			recorded.torrent, _ = io.ReadAll(f)
		}
	} else if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			w.t.Error(err)
		}
		recorded.form = r.PostForm
	}
	w.mu.Lock()
	w.requests = append(w.requests, recorded)
	w.mu.Unlock()

	answer, ok := w.replies[r.URL.Path]
	if r.URL.Path == pathMainData {
		answer, ok = reply{http.StatusOK, w.maindata[r.URL.Query().Get(queryRID)]}, true
	}
	if !ok {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	rw.WriteHeader(answer.status)
	if answer.fixture == "" {
		return
	}
	body, err := os.ReadFile(filepath.Join(webAPIFixtureDir, answer.fixture))
	if err != nil {
		w.t.Error(err)
	}
	_, _ = rw.Write(body)
}

func (w *webAPI) last(path string) recordedRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := len(w.requests) - 1; i >= 0; i-- {
		if w.requests[i].path == path {
			return w.requests[i]
		}
	}
	w.t.Fatalf("no request to %s", path)
	return recordedRequest{}
}

func newClient(t *testing.T, w *webAPI, conn settings.QBittorrentConnection) *Client {
	t.Helper()
	server := httptest.NewServer(w)
	t.Cleanup(server.Close)
	conn.URL, conn.APIKey = server.URL, cmp.Or(conn.APIKey, testAPIKey)
	conn.Category = cmp.Or(conn.Category, settings.DefaultQBittorrentCategory)
	return New(func(context.Context) (settings.QBittorrentConnection, error) { return conn, nil })
}

func TestVersionAcceptsWebAPI215AndRejectsOlder(t *testing.T) {
	w := newWebAPI(t)
	client := newClient(t, w, settings.QBittorrentConnection{})
	v, err := client.Version(context.Background())
	if err != nil || v != (acquisition.WebAPIVersion{Major: 2, Minor: 15, Patch: 1}) {
		t.Errorf("version = %+v, %v", v, err)
	}

	w.replies[pathWebAPIVersion] = reply{http.StatusOK, "../health/" + fixtureWebAPI50}
	v, err = client.Version(context.Background())
	if !errors.Is(err, acquisition.ErrUnsupportedVersion) || v.Minor != 11 {
		t.Errorf("version 5.0 = %+v, %v; want ErrUnsupportedVersion", v, err)
	}
}

func TestAMagnetIsAddedToStopAtItsMetadata(t *testing.T) {
	w := newWebAPI(t)
	client := newClient(t, w, settings.QBittorrentConnection{})

	hash, err := client.AddTorrent(context.Background(), acquisition.AddRequest{Magnet: testMagnet, StopCondition: acquisition.StopMetadataReceived})

	if err != nil || hash != metadataHash {
		t.Fatalf("hash = %s, %v; want the magnet's hash while qBittorrent is still pending", hash, err)
	}
	form := w.last(pathAdd).form
	if form.Get(formURLs) != testMagnet || form.Get(formStopCondition) != stopMetadataReceived ||
		form.Get(formCategory) != settings.DefaultQBittorrentCategory || form.Get(formStopped) != "" {
		t.Errorf("add form = %v", form)
	}
}

func TestATorrentFileIsUploadedStopped(t *testing.T) {
	w := newWebAPI(t)
	w.replies[pathAdd] = reply{http.StatusOK, "add_success.json"}
	client := newClient(t, w, settings.QBittorrentConnection{})
	torrent := []byte("d4:infod6:lengthi1e4:name9:Movie.mkv12:piece lengthi16384e6:pieces20:xxxxxxxxxxxxxxxxxxxxee")

	hash, err := client.AddTorrent(context.Background(), acquisition.AddRequest{Torrent: torrent, Stopped: true, Category: "movies"})

	if err != nil || hash != downloadingHash {
		t.Fatalf("hash = %s, %v; want added_torrent_ids[0]", hash, err)
	}
	sent := w.last(pathAdd)
	if string(sent.torrent) != string(torrent) || sent.form.Get(formStopped) != formTrue || sent.form.Get(formCategory) != "movies" || sent.form.Get(formURLs) != "" {
		t.Errorf("add = %+v", sent.form)
	}
}

func TestARefusedAddIsAnError(t *testing.T) {
	w := newWebAPI(t)
	w.replies[pathAdd] = reply{http.StatusConflict, "add_conflict.json"}
	client := newClient(t, w, settings.QBittorrentConnection{})

	if _, err := client.AddTorrent(context.Background(), acquisition.AddRequest{Magnet: testMagnet}); !errors.Is(err, acquisition.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestStartDeleteAndFiles(t *testing.T) {
	w := newWebAPI(t)
	client := newClient(t, w, settings.QBittorrentConnection{})
	ctx := context.Background()

	if err := client.Start(ctx, downloadingHash, metadataHash); err != nil {
		t.Fatal(err)
	}
	if got := w.last(pathStart).form.Get(formHashes); got != downloadingHash+"|"+metadataHash {
		t.Errorf("start hashes = %s", got)
	}
	if err := client.Delete(ctx, true, metadataHash); err != nil {
		t.Fatal(err)
	}
	if form := w.last(pathDelete).form; form.Get(formHashes) != metadataHash || form.Get(formDeleteFiles) != "true" {
		t.Errorf("delete form = %v", form)
	}

	files, err := client.Files(ctx, downloadingHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[1].Index != 1 || files[1].Priority != 0 || !strings.HasSuffix(files[1].Path, "Sample/sample.mkv") || files[0].Size != 8589000000 {
		t.Errorf("files = %+v", files)
	}
	if got := w.last(pathFiles).form.Get(formHash); got != downloadingHash {
		t.Errorf("files hash = %s", got)
	}

	delete(w.replies, pathFiles)
	if _, err := client.Files(ctx, "gone"); !errors.Is(err, acquisition.ErrTorrentNotFound) {
		t.Errorf("files of a gone torrent = %v, want ErrTorrentNotFound", err)
	}
}

func TestSyncMergesUpdatesAndKeepsOnlyMagpiesCategory(t *testing.T) {
	w := newWebAPI(t)
	client := newClient(t, w, settings.QBittorrentConnection{RemotePath: "/data/torrents", LocalPath: "/mnt/downloads"})
	ctx := context.Background()

	full, err := client.Sync(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if full.RID != 1 || len(full.Torrents) != 2 {
		t.Fatalf("full = rid %d, %d torrents; want the two magpie torrents", full.RID, len(full.Torrents))
	}
	if _, found := full.Torrents[otherAppHash]; found {
		t.Error("a torrent from another category came back")
	}
	downloading := full.Torrents[downloadingHash]
	wantPath := "/mnt/downloads/magpie/Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	if downloading.Status != acquisition.TorrentDownloading || downloading.Progress != 0.5 || downloading.DownloadBps != 5242880 ||
		downloading.ETA != 819*time.Second || downloading.ContentPath != wantPath || !downloading.HasMetadata || downloading.CompletedAt != nil {
		t.Errorf("downloading = %+v", downloading)
	}
	if meta := full.Torrents[metadataHash]; meta.Status != acquisition.TorrentMetaDL || meta.HasMetadata || meta.ETA != 0 {
		t.Errorf("metadata torrent = %+v", meta)
	}

	delta, err := client.Sync(ctx, full.RID)
	if err != nil {
		t.Fatal(err)
	}
	finished := delta.Torrents[downloadingHash]
	if delta.RID != 2 || len(delta.Torrents) != 1 || finished.Status != acquisition.TorrentUploading || finished.Progress != 1 ||
		finished.CompletedAt == nil || finished.Name != downloading.Name || finished.ContentPath != wantPath {
		t.Errorf("delta = rid %d %+v; want the update merged over the full state and the removed torrent gone", delta.RID, delta.Torrents)
	}

	if _, err := client.Sync(ctx, 41); err != nil {
		t.Fatal(err)
	}
	if got := w.last(pathMainData).form.Get(queryRID); got != "0" {
		t.Errorf("a stale rid asked with rid=%s, want a full update", got)
	}
}

func TestSetShareLimitsAlwaysSendsTheFullSet(t *testing.T) {
	w := newWebAPI(t)
	client := newClient(t, w, settings.QBittorrentConnection{})
	ctx := context.Background()
	fields := []string{formRatioLimit, formSeedingTimeLimit, formInactiveSeedingTimeLimit, formShareLimitAction, formShareLimitsMode}

	for name, tc := range map[string]struct {
		limits acquisition.ShareLimits
		want   []string
	}{
		"custom": {acquisition.ShareLimits{Ratio: 2, SeedingTime: 14 * 24 * time.Hour, Action: acquisition.ShareRemoveWithContent, Mode: acquisition.ShareLimitsCustom},
			[]string{"2", "20160", "-1", "RemoveWithContent", "MatchAny"}},
		"global":    {acquisition.ShareLimits{Action: acquisition.ShareStop, Mode: acquisition.ShareLimitsGlobal}, []string{"-2", "-2", "-2", "Stop", "Default"}},
		"unlimited": {acquisition.ShareLimits{Action: acquisition.ShareStop, Mode: acquisition.ShareLimitsUnlimited}, []string{"-1", "-1", "-1", "Stop", "Default"}},
	} {
		if err := client.SetShareLimits(ctx, downloadingHash, tc.limits); err != nil {
			t.Fatal(err)
		}
		form := w.last(pathSetShareLimits).form
		got := []string{}
		for _, f := range fields {
			got = append(got, form.Get(f))
		}
		if !slices.Equal(got, tc.want) || form.Get(formHashes) != downloadingHash {
			t.Errorf("%s sent %v, want %v", name, got, tc.want)
		}
	}
}

func TestFreeSpaceReadsTheServerState(t *testing.T) {
	client := newClient(t, newWebAPI(t), settings.QBittorrentConnection{})
	free, err := client.FreeSpace(context.Background())
	if err != nil || free != 1846316417024 {
		t.Errorf("free = %d, %v", free, err)
	}
}

func TestErrorsMapToThePortErrors(t *testing.T) {
	ctx := context.Background()
	rejected := newClient(t, newWebAPI(t), settings.QBittorrentConnection{APIKey: "wrong"})
	if _, err := rejected.Version(ctx); !errors.Is(err, acquisition.ErrUnauthorized) {
		t.Errorf("bad key = %v, want ErrUnauthorized", err)
	}

	unconfigured := New(func(context.Context) (settings.QBittorrentConnection, error) {
		return settings.QBittorrentConnection{}, nil
	})
	if _, err := unconfigured.Sync(ctx, 0); !errors.Is(err, acquisition.ErrUnavailable) {
		t.Errorf("no connection = %v, want ErrUnavailable", err)
	}

	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	down := New(func(context.Context) (settings.QBittorrentConnection, error) {
		return settings.QBittorrentConnection{URL: server.URL, APIKey: testAPIKey, Category: settings.DefaultQBittorrentCategory}, nil
	})
	if _, err := down.Version(ctx); !errors.Is(err, acquisition.ErrUnavailable) {
		t.Errorf("unreachable = %v, want ErrUnavailable", err)
	}
}

func TestLocalPathMapsOnlyWholeFolders(t *testing.T) {
	conn := settings.QBittorrentConnection{RemotePath: "/data/torrents/", LocalPath: "/mnt/downloads"}
	for remote, want := range map[string]string{
		"/data/torrents/magpie/Movie": "/mnt/downloads/magpie/Movie",
		"/data/torrents":              "/mnt/downloads",
		"/data/torrents-old/Movie":    "/data/torrents-old/Movie",
		"/elsewhere/Movie":            "/elsewhere/Movie",
	} {
		if got := localPath(remote, conn); got != want {
			t.Errorf("%s = %s, want %s", remote, got, want)
		}
	}
	if got := localPath("/data/torrents/Movie", settings.QBittorrentConnection{}); got != "/data/torrents/Movie" {
		t.Errorf("without a mapping = %s", got)
	}
}
