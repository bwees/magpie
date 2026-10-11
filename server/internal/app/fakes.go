package app

import (
	"magpie/internal/acquisition"
	acquisitionfakes "magpie/internal/acquisition/fakes"
	"magpie/internal/activity"
	activityfakes "magpie/internal/activity/fakes"
	"magpie/internal/adapters/prowlarr"
	"magpie/internal/adapters/tmdb"
	"magpie/internal/catalog"
	"magpie/internal/identity"
	identityfakes "magpie/internal/identity/fakes"
	"magpie/internal/library"
	libraryfakes "magpie/internal/library/fakes"
	requestsapi "magpie/internal/requests/api"
	requeststest "magpie/internal/requests/api/contracttest"
	"magpie/internal/subtitles"
	subtitlesfakes "magpie/internal/subtitles/fakes"
	"magpie/internal/transcode"
	transcodefakes "magpie/internal/transcode/fakes"

	"go.uber.org/fx"
)

// PortFakes provides every port declared in a context's ports.go with its in-memory fake.
// A slice that adds a real adapter replaces that port's line here with the adapter.
var PortFakes = fx.Provide(
	fx.Annotate(identityfakes.NewJellyfinAuth, fx.As(new(identity.JellyfinAuth))),
	fx.Annotate(tmdb.New, fx.As(new(catalog.TMDB))),
	fx.Annotate(prowlarr.NewFromSettings, fx.As(new(acquisition.Prowlarr))),
	fx.Annotate(acquisitionfakes.NewQBittorrent, fx.As(new(acquisition.QBittorrent))),
	fx.Annotate(acquisitionfakes.NewTrashGuides, fx.As(new(acquisition.TrashGuides))),
	fx.Annotate(libraryfakes.NewFileSystem, fx.As(fx.Self()), fx.As(new(library.FileSystem))),
	fx.Annotate(libraryfakes.NewMediaProber, fx.As(new(library.MediaProber))),
	fx.Annotate(libraryfakes.NewHasher, fx.As(new(library.Hasher))),
	fx.Annotate(libraryfakes.NewJellyfinLibrary, fx.As(new(library.JellyfinLibrary))),
	fx.Annotate(subtitlesfakes.NewOpenSubtitles, fx.As(new(subtitles.OpenSubtitles))),
	fx.Annotate(subtitlesfakes.NewSyncer, fx.As(new(subtitles.Syncer))),
	fx.Annotate(transcodefakes.NewFileStat, fx.As(new(transcode.FileStat))),
	fx.Annotate(activityfakes.NewNotificationSender, fx.As(new(activity.NotificationSender))),
)

// ReaderFakes provides the in-memory Reader of each context whose repository isn't built yet.
// A slice that implements a context's Reader removes that context's line here.
var ReaderFakes = fx.Provide(
	fx.Annotate(func() *requeststest.Fake { return requeststest.NewFake(requeststest.Fixture{}) }, fx.As(new(requestsapi.Reader))),
)
