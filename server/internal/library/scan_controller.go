package library

import (
	"net/http"
	"time"

	"magpie/internal/platform/httpserver"

	"github.com/go-fuego/fuego"
)

// LibraryScanResponse is the last library scan and the folders scans have found.
type LibraryScanResponse struct {
	Running    bool       `json:"running" validate:"required"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	// Error says why the last scan stopped early; empty when it finished.
	Error string `json:"error" validate:"required"`
	// Reported is how many folders the last scan reported or relinked.
	Reported int `json:"reported" validate:"required"`
	// Found counts every folder scans have reported; Linked those whose files are in the library, Ignored
	// those matched but without a playable video.
	Found         int  `json:"found" validate:"required"`
	Linked        int  `json:"linked" validate:"required"`
	Ignored       int  `json:"ignored" validate:"required"`
	RenameMatched bool `json:"renameMatched" validate:"required"`
}

type ScanOptionsRequest struct {
	// RenameMatched moves a matched folder's videos and sidecars to the fixed naming.
	RenameMatched bool `json:"renameMatched"`
}

// ScanController serves the library scan in Settings → Library.
type ScanController struct {
	library *Service
}

func NewScanController(s *Service) *ScanController {
	return &ScanController{library: s}
}

func (c *ScanController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagLibrary)
	fuego.Get(s, "/library/scan", c.status, tags,
		fuego.OptionOperationID("getLibraryScan"),
		fuego.OptionSummary("The last library scan and the folders scans have found (admin)"))
	fuego.Post(s, "/library/scan", c.start, tags,
		fuego.OptionOperationID("startLibraryScan"),
		fuego.OptionSummary("Scan the root folders for folders Magpie doesn't know yet; nothing happens while one runs (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
	fuego.Put(s, "/library/scan/options", c.setOptions, tags,
		fuego.OptionOperationID("setLibraryScanOptions"),
		fuego.OptionSummary("Choose whether matched folders are renamed to the fixed naming (admin)"))
}

func (c *ScanController) status(ctx fuego.ContextNoBody) (LibraryScanResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return LibraryScanResponse{}, err
	}
	return scanResponse(c.library.ScanStatus(ctx.Context()))
}

func (c *ScanController) start(ctx fuego.ContextNoBody) (LibraryScanResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return LibraryScanResponse{}, err
	}
	return scanResponse(c.library.StartScan(ctx.Context()))
}

func (c *ScanController) setOptions(ctx fuego.ContextWithBody[ScanOptionsRequest]) (LibraryScanResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return LibraryScanResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return LibraryScanResponse{}, err
	}
	if err := c.library.SetScanOptions(ctx.Context(), ScanOptions(body)); err != nil {
		return LibraryScanResponse{}, err
	}
	return scanResponse(c.library.ScanStatus(ctx.Context()))
}

func scanResponse(s ScanStatus, err error) (LibraryScanResponse, error) {
	return LibraryScanResponse{
		Running: s.Running, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, Error: s.Error, Reported: s.Reported,
		Found: s.Found, Linked: s.Linked, Ignored: s.Ignored, RenameMatched: s.Options.RenameMatched,
	}, err
}
