package flows_test

import (
	"testing"

	"magpie/internal/acquisition"
	"magpie/internal/activity"
	"magpie/internal/catalog"
	"magpie/internal/identity"
	"magpie/internal/library"
	"magpie/internal/subtitles"
	"magpie/internal/transcode"

	"go.uber.org/fx"
)

type ports struct {
	fx.In

	JellyfinAuth       identity.JellyfinAuth
	TMDB               catalog.TMDB
	Prowlarr           acquisition.Prowlarr
	QBittorrent        acquisition.QBittorrent
	TrashGuides        acquisition.TrashGuides
	FileSystem         library.FileSystem
	MediaProber        library.MediaProber
	Hasher             library.Hasher
	JellyfinLibrary    library.JellyfinLibrary
	OpenSubtitles      subtitles.OpenSubtitles
	Syncer             subtitles.Syncer
	FileStat           transcode.FileStat
	NotificationSender activity.NotificationSender
}

func TestBootProvidesEveryPort(t *testing.T) {
	var got ports
	boot(t, fx.Populate(&got))

	for name, port := range map[string]any{
		"JellyfinAuth": got.JellyfinAuth, "TMDB": got.TMDB, "Prowlarr": got.Prowlarr,
		"QBittorrent": got.QBittorrent, "TrashGuides": got.TrashGuides, "FileSystem": got.FileSystem,
		"MediaProber": got.MediaProber, "Hasher": got.Hasher, "JellyfinLibrary": got.JellyfinLibrary,
		"OpenSubtitles": got.OpenSubtitles, "Syncer": got.Syncer, "FileStat": got.FileStat,
		"NotificationSender": got.NotificationSender,
	} {
		if port == nil {
			t.Errorf("%s is not provided", name)
		}
	}
}
