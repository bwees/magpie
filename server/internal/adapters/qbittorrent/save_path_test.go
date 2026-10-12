package qbittorrent

import (
	"context"
	"net/http"
	"testing"

	"magpie/internal/platform/settings"
)

func TestSavePathUsesTheCategoryUnderTheDefaultAndTheMapping(t *testing.T) {
	w := newWebAPI(t)
	w.replies[pathDefaultSavePath] = reply{http.StatusOK, "default_save_path.txt"}
	w.replies[pathCategories] = reply{http.StatusOK, "categories_magpie.json"}
	client := newClient(t, w, settings.QBittorrentConnection{RemotePath: "/data/torrents", LocalPath: "/mnt/downloads"})

	got, err := client.SavePath(context.Background())
	if want := "/mnt/downloads/magpie"; err != nil || got != want {
		t.Errorf("SavePath = %q, %v; want %q", got, err, want)
	}
}

func TestSavePathFallsBackToTheDefault(t *testing.T) {
	w := newWebAPI(t)
	w.replies[pathDefaultSavePath] = reply{http.StatusOK, "default_save_path.txt"}
	w.replies[pathCategories] = reply{http.StatusOK, "../health/categories.json"}
	client := newClient(t, w, settings.QBittorrentConnection{})

	got, err := client.SavePath(context.Background())
	if want := "/data/torrents"; err != nil || got != want {
		t.Errorf("SavePath = %q, %v; want %q", got, err, want)
	}
}

func TestLocalPathUsesTheMappingSetNow(t *testing.T) {
	w := newWebAPI(t)
	client := newClient(t, w, settings.QBittorrentConnection{RemotePath: "/data/torrents", LocalPath: "/mnt/downloads"})

	got, err := client.LocalPath(context.Background(), "/data/torrents/magpie/Movie")
	if want := "/mnt/downloads/magpie/Movie"; err != nil || got != want {
		t.Errorf("LocalPath = %q, %v; want %q", got, err, want)
	}
}
