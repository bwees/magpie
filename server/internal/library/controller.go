package library

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagLibrary = "library"

	pathFileID       = "id"
	pathRootFolderID = "id"
	pathDownloadID   = "downloadId"
	queryTitle       = "title"
	queryTitles      = "titles"
	queryTranscoded  = "transcoded"
	queryStatus      = "status"
	listSeparator    = ","

	// maxBatchTitles bounds one library-table lookup.
	maxBatchTitles = 500

	TypeFileNotFound        problem.Type = "urn:magpie:problem:file-not-found"
	TypeRootFolderNotFound  problem.Type = "urn:magpie:problem:root-folder-not-found"
	TypeRootFolderExists    problem.Type = "urn:magpie:problem:root-folder-exists"
	TypeRootFolderInUse     problem.Type = "urn:magpie:problem:root-folder-in-use"
	TypeInvalidRootFolder   problem.Type = "urn:magpie:problem:invalid-root-folder"
	TypeImportNotFound      problem.Type = "urn:magpie:problem:import-not-found"
	TypeImportNotStopped    problem.Type = "urn:magpie:problem:import-not-stopped"
	TypeInvalidFileMapping  problem.Type = "urn:magpie:problem:invalid-file-mapping"
	TypeTitleRequired       problem.Type = "urn:magpie:problem:title-required"
	TypeTooManyTitles       problem.Type = "urn:magpie:problem:too-many-titles"
	TypeUnknownImportStatus problem.Type = "urn:magpie:problem:unknown-import-status"
)

type StreamResponse struct {
	Index       int             `json:"index"`
	Kind        api.StreamKind  `json:"kind"`
	Codec       string          `json:"codec"`
	Language    domain.Language `json:"language"`
	Forced      bool            `json:"forced"`
	Default     bool            `json:"default"`
	Image       bool            `json:"image"`
	Channels    int             `json:"channels"`
	BitDepth    int             `json:"bitDepth"`
	BitrateKbps int             `json:"bitrateKbps"`
}

type MediaInfoResponse struct {
	Container       string           `json:"container"`
	DurationSeconds int64            `json:"durationSeconds"`
	BitrateKbps     int              `json:"bitrateKbps"`
	Width           int              `json:"width"`
	Height          int              `json:"height"`
	HDR             api.HDRFormat    `json:"hdr"`
	Streams         []StreamResponse `json:"streams" validate:"required"`
}

type EpisodeRefResponse struct {
	Episode domain.EpisodeID `json:"episode"`
	Season  int              `json:"season"`
	Number  int              `json:"number"`
}

// MediaFileResponse is one file of the title page's Files tab.
type MediaFileResponse struct {
	ID          domain.MediaFileID   `json:"id"`
	Title       domain.TitleID       `json:"title"`
	Episodes    []EpisodeRefResponse `json:"episodes" validate:"required"`
	RootFolder  string               `json:"rootFolder"`
	Path        string               `json:"path"`
	Size        int64                `json:"size"`
	Quality     domain.Quality       `json:"quality"`
	ReleaseName string               `json:"releaseName"`
	MediaInfo   MediaInfoResponse    `json:"mediaInfo"`
	Origin      domain.Origin        `json:"origin"`
	Download    *domain.DownloadID   `json:"download,omitempty"`
	Transcoded  bool                 `json:"transcoded"`
	ImportedAt  time.Time            `json:"importedAt"`
	// Available is true once Jellyfin has confirmed the file.
	Available bool `json:"available"`
	// JellyfinOverdue is true while Jellyfin still hasn't confirmed the file a day after its import.
	JellyfinOverdue bool `json:"jellyfinOverdue"`
	// RenameProblem says why the file kept its old name after an episode-group change.
	RenameProblem string `json:"renameProblem,omitempty"`
}

// TitleCoverageResponse is one series row's coverage on the TV page.
type TitleCoverageResponse struct {
	Title             domain.TitleID `json:"title" validate:"required"`
	EpisodesWithFile  int            `json:"episodesWithFile" validate:"required"`
	EpisodesAvailable int            `json:"episodesAvailable" validate:"required"`
	SizeBytes         int64          `json:"sizeBytes" validate:"required"`
}

// RootFolderResponse is a root folder with its hardlink check result.
type RootFolderResponse struct {
	ID   string           `json:"id"`
	Kind domain.MediaKind `json:"kind"`
	Path string           `json:"path"`
	// Link is HARDLINK (same filesystem as the downloads), COPY or UNKNOWN.
	Link LinkMode `json:"link"`
}

type AddRootFolderRequest struct {
	Kind domain.MediaKind `json:"kind"`
	Path string           `json:"path"`
}

type HardlinkCheckResponse struct {
	// DownloadPath is qBittorrent's download folder as Magpie sees it; "" when unknown.
	DownloadPath string               `json:"downloadPath"`
	RootFolders  []RootFolderResponse `json:"rootFolders" validate:"required"`
	// Problem says why the result is UNKNOWN.
	Problem string `json:"problem,omitempty"`
}

type ImportFileResponse struct {
	Path     string               `json:"path"`
	Size     int64                `json:"size"`
	Kind     FileKind             `json:"kind"`
	Sample   bool                 `json:"sample"`
	Episodes []EpisodeRefResponse `json:"episodes" validate:"required"`
}

type ImportEpisodeResponse struct {
	ID     domain.EpisodeID `json:"id"`
	Season int              `json:"season"`
	Number int              `json:"number"`
	Name   string           `json:"name"`
}

// ImportResponse is one row of Activity → Imports.
type ImportResponse struct {
	Download    domain.DownloadID `json:"download"`
	Title       domain.TitleID    `json:"title"`
	TitleName   string            `json:"titleName"`
	Kind        domain.MediaKind  `json:"kind"`
	ReleaseName string            `json:"releaseName"`
	// Status is PENDING, IMPORTED, FAILED, MANUAL or REMOVED.
	Status    ImportStatus         `json:"status"`
	Reason    string               `json:"reason"`
	Attempts  int                  `json:"attempts"`
	Files     []ImportFileResponse `json:"files" validate:"required"`
	UpdatedAt time.Time            `json:"updatedAt"`
	// Episodes are the series' episodes for the mapping picker; empty for a movie.
	Episodes []ImportEpisodeResponse `json:"episodes" validate:"required"`
}

type FileMappingRequest struct {
	Path string `json:"path"`
	// Episodes are the episodes the video holds; empty for a movie.
	Episodes []domain.EpisodeID `json:"episodes" validate:"required"`
}

// ManualImportRequest imports the listed videos as the given episodes; videos left out are skipped.
type ManualImportRequest struct {
	Files []FileMappingRequest `json:"files" validate:"required"`
}

type Controller struct {
	library *Service
}

func NewController(s *Service) *Controller {
	return &Controller{library: s}
}

func (c *Controller) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagLibrary)
	downloadPath := fuego.OptionPath(pathDownloadID, "Download ID")

	fuego.Get(s, "/library/files", c.files, tags,
		fuego.OptionOperationID("listMediaFiles"),
		fuego.OptionSummary("One title's media files, or several titles' files in input order; newest first per title (admin)"),
		fuego.OptionQuery(queryTitle, "One title's files"),
		fuego.OptionQueryArray(queryTitles, "Several titles' files, at most 500 titles", reflect.String),
		fuego.OptionQueryBool(queryTranscoded, "With titles: only transcoded (true) or not transcoded (false) files"))
	fuego.Get(s, "/library/coverage", c.coverage, tags,
		fuego.OptionOperationID("getCoverage"),
		fuego.OptionSummary("Episodes with a file, episodes Jellyfin confirmed and size per title, in input order (admin)"),
		fuego.OptionQueryArray(queryTitles, "Title IDs, at most 500", reflect.String))
	fuego.Get(s, "/library/files/{id}", c.file, tags,
		fuego.OptionPath(pathFileID, "Media file ID"),
		fuego.OptionOperationID("getMediaFile"),
		fuego.OptionSummary("One media file with its media info (admin)"))
	fuego.Delete(s, "/library/files/{id}", c.deleteFile, tags,
		fuego.OptionPath(pathFileID, "Media file ID"),
		fuego.OptionOperationID("deleteMediaFile"),
		fuego.OptionSummary("Delete a media file from disk; other contexts drop what they keep for it (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Get(s, "/library/root-folders", c.rootFolders, tags,
		fuego.OptionOperationID("listRootFolders"),
		fuego.OptionSummary("Root folders with their hardlink check (admin)"))
	fuego.Post(s, "/library/root-folders", c.addRootFolder, tags,
		fuego.OptionOperationID("addRootFolder"),
		fuego.OptionSummary("Add a root folder; it must exist and be writable (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Delete(s, "/library/root-folders/{id}", c.removeRootFolder, tags,
		fuego.OptionPath(pathRootFolderID, "Root folder ID"),
		fuego.OptionOperationID("removeRootFolder"),
		fuego.OptionSummary("Forget a root folder that holds no media files (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Get(s, "/library/hardlink-check", c.hardlinkCheck, tags,
		fuego.OptionOperationID("getHardlinkCheck"),
		fuego.OptionSummary("Whether qBittorrent's download folder shares a filesystem with each root folder (admin)"))
	fuego.Get(s, "/library/imports", c.imports, tags,
		fuego.OptionOperationID("listImports"),
		fuego.OptionSummary("Imports in the given statuses, most recently changed first (admin)"),
		fuego.OptionQuery(queryStatus, "Comma-separated statuses: PENDING, IMPORTED, FAILED, MANUAL, REMOVED", fuego.ParamRequired()))
	fuego.Post(s, "/library/imports/{downloadId}/manual", c.importManually, tags, downloadPath,
		fuego.OptionOperationID("importManually"),
		fuego.OptionSummary("Import a failed or manual import with a file → episodes mapping (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
	fuego.Post(s, "/library/imports/{downloadId}/retry", c.retryImport, tags, downloadPath,
		fuego.OptionOperationID("retryImport"),
		fuego.OptionSummary("Run a failed or manual import again (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
	fuego.Delete(s, "/library/imports/{downloadId}", c.removeImport, tags, downloadPath,
		fuego.OptionOperationID("removeImport"),
		fuego.OptionSummary("Remove & blocklist a failed or manual import; Acquisition drops its download and grabs another (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
}

func (c *Controller) files(ctx fuego.ContextNoBody) ([]MediaFileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	title := domain.TitleID(ctx.QueryParam(queryTitle))
	raw := ctx.QueryParamArr(queryTitles)
	if (title == "") == (len(raw) == 0) {
		return nil, problem.New(TypeTitleRequired, http.StatusBadRequest, "Pass either one title or a list of titles")
	}
	var files []api.MediaFile
	var err error
	if title != "" {
		files, err = c.library.FilesForTitle(ctx.Context(), title)
	} else {
		var titles []domain.TitleID
		if titles, err = batchTitles(raw); err != nil {
			return nil, err
		}
		var filter api.FileFilter
		if ctx.QueryParam(queryTranscoded) != "" {
			transcoded, parseErr := ctx.QueryParamBoolErr(queryTranscoded)
			if parseErr != nil {
				return nil, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "transcoded must be true or false")
			}
			filter.Transcoded = &transcoded
		}
		files, err = c.library.FilesForTitles(ctx.Context(), titles, filter)
	}
	if err != nil {
		return nil, err
	}
	problems, err := c.library.RenameProblems(ctx.Context(), title)
	if err != nil {
		return nil, err
	}
	out := make([]MediaFileResponse, 0, len(files))
	for _, f := range files {
		response := c.fileResponse(f)
		response.RenameProblem = problems[f.ID]
		out = append(out, response)
	}
	return out, nil
}

func (c *Controller) fileResponse(f api.MediaFile) MediaFileResponse {
	response := mediaFileResponse(f)
	response.JellyfinOverdue = c.library.JellyfinOverdue(f)
	return response
}

func (c *Controller) coverage(ctx fuego.ContextNoBody) ([]TitleCoverageResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	titles, err := batchTitles(ctx.QueryParamArr(queryTitles))
	if err != nil {
		return nil, err
	}
	coverage, err := c.library.Coverage(ctx.Context(), titles)
	if err != nil {
		return nil, err
	}
	out := make([]TitleCoverageResponse, 0, len(coverage))
	for _, cov := range coverage {
		out = append(out, TitleCoverageResponse(cov))
	}
	return out, nil
}

func batchTitles(raw []string) ([]domain.TitleID, error) {
	if len(raw) > maxBatchTitles {
		return nil, problem.New(TypeTooManyTitles, http.StatusBadRequest, fmt.Sprintf("Ask for at most %d titles at once", maxBatchTitles))
	}
	titles := make([]domain.TitleID, 0, len(raw))
	for _, id := range raw {
		titles = append(titles, domain.TitleID(id))
	}
	return titles, nil
}

func (c *Controller) file(ctx fuego.ContextNoBody) (MediaFileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MediaFileResponse{}, err
	}
	f, err := c.library.File(ctx.Context(), domain.MediaFileID(ctx.PathParam(pathFileID)))
	if err != nil {
		return MediaFileResponse{}, libraryProblem(err)
	}
	return c.fileResponse(f), nil
}

func (c *Controller) rootFolders(ctx fuego.ContextNoBody) ([]RootFolderResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	check, err := c.library.HardlinkCheck(ctx.Context())
	if err != nil {
		return nil, err
	}
	return rootFolderResponses(check.Folders), nil
}

func (c *Controller) addRootFolder(ctx fuego.ContextWithBody[AddRootFolderRequest]) (RootFolderResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return RootFolderResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return RootFolderResponse{}, err
	}
	folder, err := c.library.AddRootFolder(ctx.Context(), body.Kind, strings.TrimSpace(body.Path))
	if err != nil {
		return RootFolderResponse{}, libraryProblem(err)
	}
	return rootFolderResponse(folder), nil
}

func (c *Controller) deleteFile(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, libraryProblem(c.library.DeleteFile(ctx.Context(), domain.MediaFileID(ctx.PathParam(pathFileID))))
}

func (c *Controller) removeRootFolder(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, libraryProblem(c.library.RemoveRootFolder(ctx.Context(), ctx.PathParam(pathRootFolderID)))
}

func (c *Controller) hardlinkCheck(ctx fuego.ContextNoBody) (HardlinkCheckResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return HardlinkCheckResponse{}, err
	}
	check, err := c.library.HardlinkCheck(ctx.Context())
	if err != nil {
		return HardlinkCheckResponse{}, err
	}
	return HardlinkCheckResponse{DownloadPath: check.DownloadPath, RootFolders: rootFolderResponses(check.Folders), Problem: check.Problem}, nil
}

func (c *Controller) imports(ctx fuego.ContextNoBody) ([]ImportResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	var statuses []ImportStatus
	for _, raw := range strings.Split(ctx.QueryParam(queryStatus), listSeparator) {
		status := ImportStatus(strings.TrimSpace(raw))
		if !status.Valid() {
			return nil, problem.New(TypeUnknownImportStatus, http.StatusBadRequest, "Unknown import status "+string(status))
		}
		statuses = append(statuses, status)
	}
	views, err := c.library.Imports(ctx.Context(), statuses)
	if err != nil {
		return nil, err
	}
	out := make([]ImportResponse, 0, len(views))
	for _, v := range views {
		out = append(out, importResponse(v))
	}
	return out, nil
}

func (c *Controller) importManually(ctx fuego.ContextWithBody[ManualImportRequest]) (ImportResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ImportResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ImportResponse{}, err
	}
	mapping := make([]FileMapping, 0, len(body.Files))
	for _, f := range body.Files {
		mapping = append(mapping, FileMapping(f))
	}
	v, err := c.library.ImportManually(ctx.Context(), domain.DownloadID(ctx.PathParam(pathDownloadID)), mapping)
	if err != nil {
		return ImportResponse{}, libraryProblem(err)
	}
	return importResponse(v), nil
}

func (c *Controller) retryImport(ctx fuego.ContextNoBody) (ImportResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ImportResponse{}, err
	}
	v, err := c.library.RetryImport(ctx.Context(), domain.DownloadID(ctx.PathParam(pathDownloadID)))
	if err != nil {
		return ImportResponse{}, libraryProblem(err)
	}
	return importResponse(v), nil
}

func (c *Controller) removeImport(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, libraryProblem(c.library.RemoveImport(ctx.Context(), domain.DownloadID(ctx.PathParam(pathDownloadID))))
}

func libraryProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, api.ErrNotFound):
		return problem.New(TypeFileNotFound, http.StatusNotFound, "No such media file")
	case errors.Is(err, ErrRootFolderNotFound):
		return problem.New(TypeRootFolderNotFound, http.StatusNotFound, "No such root folder")
	case errors.Is(err, ErrRootFolderExists):
		return problem.New(TypeRootFolderExists, http.StatusConflict, "That folder is already a root folder")
	case errors.Is(err, ErrRootFolderInUse):
		return problem.New(TypeRootFolderInUse, http.StatusConflict, "The root folder still holds media files")
	case errors.Is(err, ErrInvalidRootFolder):
		return problem.New(TypeInvalidRootFolder, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrInvalidRootFolder.Error()+": "))
	case errors.Is(err, ErrImportNotFound):
		return problem.New(TypeImportNotFound, http.StatusNotFound, "No such import")
	case errors.Is(err, ErrImportNotStopped):
		return problem.New(TypeImportNotStopped, http.StatusConflict, "Only a failed or manual import can be retried or removed")
	case errors.Is(err, ErrInvalidMapping):
		return problem.New(TypeInvalidFileMapping, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrInvalidMapping.Error()+": "))
	}
	return err
}

func episodeRefs(list []domain.EpisodeRef) []EpisodeRefResponse {
	out := make([]EpisodeRefResponse, 0, len(list))
	for _, ref := range list {
		out = append(out, EpisodeRefResponse{Episode: ref.Episode, Season: ref.Season, Number: ref.Number})
	}
	return out
}

func mediaFileResponse(f api.MediaFile) MediaFileResponse {
	streams := make([]StreamResponse, 0, len(f.MediaInfo.Streams))
	for _, st := range f.MediaInfo.Streams {
		streams = append(streams, StreamResponse(st))
	}
	return MediaFileResponse{
		ID: f.ID, Title: f.Title, Episodes: episodeRefs(f.Episodes), RootFolder: f.RootFolder, Path: f.Path,
		Size: f.Size, Quality: f.Quality, ReleaseName: f.ReleaseName, Origin: f.Origin, Download: f.Download,
		Transcoded: f.Transcoded, ImportedAt: f.ImportedAt, Available: f.Available,
		MediaInfo: MediaInfoResponse{
			Container: f.MediaInfo.Container, DurationSeconds: int64(f.MediaInfo.Duration / time.Second),
			BitrateKbps: f.MediaInfo.BitrateKbps, Width: f.MediaInfo.Width, Height: f.MediaInfo.Height,
			HDR: f.MediaInfo.HDR, Streams: streams,
		},
	}
}

func rootFolderResponse(f FolderLink) RootFolderResponse {
	return RootFolderResponse{ID: f.ID, Kind: f.Kind, Path: f.Path, Link: f.Link}
}

func rootFolderResponses(folders []FolderLink) []RootFolderResponse {
	out := make([]RootFolderResponse, 0, len(folders))
	for _, f := range folders {
		out = append(out, rootFolderResponse(f))
	}
	return out
}

func importResponse(v ImportView) ImportResponse {
	files := make([]ImportFileResponse, 0, len(v.Files))
	for _, f := range v.Files {
		files = append(files, ImportFileResponse{Path: f.Path, Size: f.Size, Kind: f.Kind, Sample: f.Sample, Episodes: episodeRefs(f.Episodes)})
	}
	episodes := make([]ImportEpisodeResponse, 0, len(v.Episodes))
	for _, e := range v.Episodes {
		episodes = append(episodes, ImportEpisodeResponse{ID: e.ID, Season: e.Season, Number: e.Number, Name: e.Name})
	}
	return ImportResponse{
		Download: v.Download, Title: v.Title, TitleName: v.TitleName, Kind: v.Kind, ReleaseName: v.ReleaseName,
		Status: v.Status, Reason: v.LastError, Attempts: v.Attempts, Files: files, UpdatedAt: v.UpdatedAt,
		Episodes: episodes,
	}
}
