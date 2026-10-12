package app

import (
	"magpie/internal/acquisition"
	"magpie/internal/activity"
	"magpie/internal/adapters/disk"
	"magpie/internal/adapters/ffprobe"
	"magpie/internal/adapters/jellyfin"
	"magpie/internal/adapters/opensubtitles"
	"magpie/internal/adapters/prowlarr"
	"magpie/internal/adapters/qbittorrent"
	"magpie/internal/adapters/shoutrrr"
	"magpie/internal/adapters/subsync"
	"magpie/internal/adapters/tmdb"
	"magpie/internal/adapters/trash"
	"magpie/internal/catalog"
	"magpie/internal/identity"
	"magpie/internal/library"
	"magpie/internal/subtitles"
	"magpie/internal/transcode"
	transcodefakes "magpie/internal/transcode/fakes"

	"go.uber.org/fx"
)

// PortFakes provides every port declared in a context's ports.go with its in-memory fake.
// A slice that adds a real adapter replaces that port's line here with the adapter.
var PortFakes = fx.Provide(
	fx.Annotate(jellyfin.NewAuth, fx.As(new(identity.JellyfinAuth))),
	fx.Annotate(tmdb.New, fx.As(new(catalog.TMDB))),
	fx.Annotate(prowlarr.NewFromSettings, fx.As(new(acquisition.Prowlarr))),
	fx.Annotate(qbittorrent.NewFromSettings, fx.As(new(acquisition.QBittorrent)), fx.As(new(library.DownloadLocation))),
	fx.Annotate(trash.New, fx.As(new(acquisition.TrashGuides))),
	fx.Annotate(disk.NewFileSystem, fx.As(new(library.FileSystem))),
	fx.Annotate(ffprobe.New, fx.As(new(library.MediaProber))),
	fx.Annotate(disk.NewHasher, fx.As(new(library.Hasher))),
	fx.Annotate(jellyfin.NewLibrary, fx.As(new(library.JellyfinLibrary))),
	fx.Annotate(opensubtitles.New, fx.As(new(subtitles.OpenSubtitles))),
	fx.Annotate(func() *subsync.Syncer { return subsync.New(subsync.FFmpeg{}) }, fx.As(new(subtitles.Syncer))),
	fx.Annotate(disk.NewSidecarFiles, fx.As(new(subtitles.SidecarFiles))),
	fx.Annotate(transcodefakes.NewFileStat, fx.As(new(transcode.FileStat))),
	fx.Annotate(shoutrrr.New, fx.As(new(activity.NotificationSender))),
)

// ReaderFakes provides the in-memory Reader of each context whose repository isn't built yet.
// A slice that implements a context's Reader removes that context's line here.
var ReaderFakes = fx.Provide()
