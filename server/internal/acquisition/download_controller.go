package acquisition

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/access"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
	"magpie/internal/platform/settings"

	"github.com/go-fuego/fuego"
)

const (
	pathDownloadID       = "id"
	pathBlocklistEntryID = "id"
	queryDeleteData      = "deleteData"

	TypeDownloadNotFound       problem.Type = "urn:magpie:problem:download-not-found"
	TypeBlocklistEntryNotFound problem.Type = "urn:magpie:problem:blocklist-entry-not-found"
)

// DownloadResponse is one row of Activity → Downloads. The live fields hold the last poll.
type DownloadResponse struct {
	ID          domain.DownloadID  `json:"id"`
	Title       domain.TitleID     `json:"title"`
	TitleName   string             `json:"titleName"`
	Episodes    []domain.EpisodeID `json:"episodes" validate:"required"`
	ReleaseName string             `json:"releaseName"`
	Indexer     string             `json:"indexer"`
	InfoHash    string             `json:"infoHash"`
	Quality     domain.Quality     `json:"quality"`
	CFScore     int                `json:"cfScore"`
	State       api.DownloadState  `json:"state"`
	Progress    float64            `json:"progress"`
	SpeedBps    int64              `json:"speedBps"`
	ETASeconds  int64              `json:"etaSeconds"`
	Ratio       float64            `json:"ratio"`
	SizeBytes   int64              `json:"sizeBytes"`
	SeedingMode api.SeedingMode    `json:"seedingMode"`
	Trigger     api.SearchTrigger  `json:"trigger"`
	Override    bool               `json:"override"`
	GrabbedAt   time.Time          `json:"grabbedAt"`
	CompletedAt *time.Time         `json:"completedAt,omitempty"`
	Search      *domain.SearchID   `json:"search,omitempty"`
}

type BlocklistEntryResponse struct {
	ID          string              `json:"id"`
	Title       domain.TitleID      `json:"title"`
	TitleName   string              `json:"titleName"`
	InfoHash    string              `json:"infoHash"`
	ReleaseName string              `json:"releaseName"`
	Indexer     string              `json:"indexer"`
	Reason      api.BlocklistReason `json:"reason"`
	At          time.Time           `json:"at"`
	// ExpiresAt is null for a permanent entry.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type DownloadOptionsResponse struct {
	BlockedFilesEnabled    bool     `json:"blockedFilesEnabled"`
	BlockedPatterns        []string `json:"blockedPatterns" validate:"required"`
	MetadataTimeoutMinutes int      `json:"metadataTimeoutMinutes"`
	StalledLimitMinutes    int      `json:"stalledLimitMinutes"`
	// SeedingMode is the global "after download" mode: SEED_UNTIL_GOALS or DONT_SEED.
	SeedingMode api.SeedingMode `json:"seedingMode"`
	// SeedRatio and SeedDays are the global seeding goal, whichever comes first.
	SeedRatio float64 `json:"seedRatio"`
	SeedDays  int     `json:"seedDays"`
}

type DownloadOptionsRequest struct {
	BlockedFilesEnabled    bool            `json:"blockedFilesEnabled"`
	BlockedPatterns        []string        `json:"blockedPatterns" validate:"required"`
	MetadataTimeoutMinutes int             `json:"metadataTimeoutMinutes"`
	StalledLimitMinutes    int             `json:"stalledLimitMinutes"`
	SeedingMode            api.SeedingMode `json:"seedingMode"`
	SeedRatio              float64         `json:"seedRatio"`
	SeedDays               int             `json:"seedDays"`
}

// DownloadController serves Activity → Downloads and Blocklist, and Settings → Downloads.
type DownloadController struct {
	downloads *DownloadService
	gate      *access.FileGate
}

func NewDownloadController(d *DownloadService, g *access.FileGate) *DownloadController {
	return &DownloadController{downloads: d, gate: g}
}

func (c *DownloadController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagAcquisition)
	downloadPath := fuego.OptionPath(pathDownloadID, "Download ID")

	fuego.Get(s, "/acquisition/downloads", c.list, tags,
		fuego.OptionOperationID("listDownloads"),
		fuego.OptionSummary("Downloads still in qBittorrent with their last polled progress, newest first; file managers see their titles' only"))
	fuego.Delete(s, "/acquisition/downloads/{id}", c.remove, tags, downloadPath,
		fuego.OptionOperationID("removeDownload"),
		fuego.OptionSummary("Remove a download from qBittorrent, with its data when deleteData is set (file managers)"),
		fuego.OptionQueryBool(queryDeleteData, "Also delete the downloaded data"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Get(s, "/acquisition/blocklist", c.blocklist, tags,
		fuego.OptionOperationID("listBlocklist"),
		fuego.OptionSummary("Every blocklist entry, newest first, expired temporary ones included (admin)"))
	fuego.Delete(s, "/acquisition/blocklist/{id}", c.unblock, tags,
		fuego.OptionPath(pathBlocklistEntryID, "Blocklist entry ID"),
		fuego.OptionOperationID("removeBlocklistEntry"),
		fuego.OptionSummary("Remove a blocklist entry so its release can be grabbed again (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Get(s, "/acquisition/download-options", c.options, tags,
		fuego.OptionOperationID("getDownloadOptions"),
		fuego.OptionSummary("Blocked-file check, blocked patterns, metadata timeout and stalled limit (admin)"))
	fuego.Put(s, "/acquisition/download-options", c.setOptions, tags,
		fuego.OptionOperationID("setDownloadOptions"),
		fuego.OptionSummary("Change the blocked-file check and download timeouts (admin)"))
}

func (c *DownloadController) list(ctx fuego.ContextNoBody) ([]DownloadResponse, error) {
	views, err := c.downloads.Downloads(ctx.Context())
	if err != nil {
		return nil, err
	}
	views, err = access.Filter(ctx.Context(), c.gate, views, func(v DownloadView) domain.TitleID { return v.Title })
	if err != nil {
		return nil, err
	}
	out := make([]DownloadResponse, 0, len(views))
	for _, v := range views {
		out = append(out, downloadResponse(v))
	}
	return out, nil
}

func (c *DownloadController) remove(ctx fuego.ContextNoBody) (any, error) {
	if err := access.RequireManager(ctx.Context()); err != nil {
		return nil, err
	}
	id := domain.DownloadID(ctx.PathParam(pathDownloadID))
	title, err := c.downloads.DownloadTitle(ctx.Context(), id)
	if err != nil {
		return nil, downloadProblem(err)
	}
	if err := c.gate.Require(ctx.Context(), title); err != nil {
		return nil, err
	}
	deleteData, err := strconv.ParseBool(ctx.QueryParam(queryDeleteData))
	if ctx.QueryParam(queryDeleteData) != "" && err != nil {
		return nil, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "deleteData must be true or false")
	}
	_, err = c.downloads.Remove(ctx.Context(), id, deleteData)
	return nil, downloadProblem(err)
}

func (c *DownloadController) blocklist(ctx fuego.ContextNoBody) ([]BlocklistEntryResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	views, err := c.downloads.Blocklist(ctx.Context())
	if err != nil {
		return nil, err
	}
	out := make([]BlocklistEntryResponse, 0, len(views))
	for _, v := range views {
		out = append(out, blocklistResponse(v))
	}
	return out, nil
}

func (c *DownloadController) unblock(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	_, err := c.downloads.Unblock(ctx.Context(), ctx.PathParam(pathBlocklistEntryID))
	return nil, downloadProblem(err)
}

func (c *DownloadController) options(ctx fuego.ContextNoBody) (DownloadOptionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return DownloadOptionsResponse{}, err
	}
	o, err := c.downloads.Options(ctx.Context())
	if err != nil {
		return DownloadOptionsResponse{}, err
	}
	seeding, err := c.downloads.SeedingOptions(ctx.Context())
	if err != nil {
		return DownloadOptionsResponse{}, err
	}
	return downloadOptionsResponse(o, seeding), nil
}

func (c *DownloadController) setOptions(ctx fuego.ContextWithBody[DownloadOptionsRequest]) (DownloadOptionsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return DownloadOptionsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return DownloadOptionsResponse{}, err
	}
	o, seeding, err := c.downloads.SetDownloadSettings(ctx.Context(), DownloadOptions{
		BlockedFilesEnabled: body.BlockedFilesEnabled,
		BlockedPatterns:     nonNil(body.BlockedPatterns),
		MetadataTimeout:     settings.Duration(time.Duration(body.MetadataTimeoutMinutes) * time.Minute),
		StalledLimit:        settings.Duration(time.Duration(body.StalledLimitMinutes) * time.Minute),
	}, SeedingOptions{Mode: body.SeedingMode, Goal: SeedingGoal{Ratio: body.SeedRatio, Days: body.SeedDays}})
	if err != nil {
		return DownloadOptionsResponse{}, downloadProblem(err)
	}
	return downloadOptionsResponse(o, seeding), nil
}

func downloadProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrDownloadNotFound):
		return problem.New(TypeDownloadNotFound, http.StatusNotFound, "No such download")
	case errors.Is(err, ErrBlocklistEntryNotFound):
		return problem.New(TypeBlocklistEntryNotFound, http.StatusNotFound, "No such blocklist entry")
	case errors.Is(err, ErrInvalidDownloadOptions), errors.Is(err, ErrInvalidSeedingOptions):
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	return qbittorrentProblem(err)
}

func downloadResponse(v DownloadView) DownloadResponse {
	r := DownloadResponse{
		ID: v.ID, Title: v.Title, TitleName: v.TitleName, Episodes: nonNil(v.Episodes), ReleaseName: v.ReleaseName,
		Indexer: v.Indexer, InfoHash: v.InfoHash, Quality: v.Quality, CFScore: v.CFScore, State: v.State,
		Progress: v.Progress, SizeBytes: v.SizeBytes, SeedingMode: v.SeedingMode, Trigger: v.Trigger,
		Override: v.Override, GrabbedAt: v.GrabbedAt, CompletedAt: v.CompletedAt, Search: v.Search,
	}
	if v.Live != nil {
		r.State, r.Progress, r.SpeedBps, r.Ratio = v.Live.State, v.Live.Progress, v.Live.SpeedBps, v.Live.Ratio
		r.ETASeconds = int64(v.Live.ETA / time.Second)
	}
	return r
}

func blocklistResponse(v BlocklistView) BlocklistEntryResponse {
	return BlocklistEntryResponse{
		ID: v.ID, Title: v.Title, TitleName: v.TitleName, InfoHash: v.InfoHash, ReleaseName: v.ReleaseName,
		Indexer: v.Indexer, Reason: v.Reason, At: v.At, ExpiresAt: v.ExpiresAt,
	}
}

func downloadOptionsResponse(o DownloadOptions, seeding SeedingOptions) DownloadOptionsResponse {
	return DownloadOptionsResponse{
		BlockedFilesEnabled:    o.BlockedFilesEnabled,
		BlockedPatterns:        nonNil(o.BlockedPatterns),
		MetadataTimeoutMinutes: int(o.MetadataTimeout.Std() / time.Minute),
		StalledLimitMinutes:    int(o.StalledLimit.Std() / time.Minute),
		SeedingMode:            seeding.Mode,
		SeedRatio:              seeding.Goal.Ratio,
		SeedDays:               seeding.Goal.Days,
	}
}
