package backup

import (
	"errors"
	"net/http"
	"time"

	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagPlatform = "platform"
	pathName    = "name"

	TypeBackupNotFound problem.Type = "urn:magpie:problem:backup-not-found"
)

type BackupResponse struct {
	Name      string        `json:"name" validate:"required"`
	Kind      db.BackupKind `json:"kind" validate:"required"`
	CreatedAt time.Time     `json:"createdAt" validate:"required"`
	Size      int64         `json:"size" validate:"required"`
}

type RestoreResponse struct {
	Restarting bool `json:"restarting" validate:"required"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(api *fuego.Server) {
	fuego.Get(api, "/platform/backups", c.list,
		fuego.OptionOperationID("listBackups"),
		fuego.OptionSummary("List database backups, newest first (admin)"),
		fuego.OptionTags(tagPlatform),
	)
	fuego.Post(api, "/platform/backups/{name}/restore", c.restore,
		fuego.OptionOperationID("restoreBackup"),
		fuego.OptionSummary("Restore a backup and restart the server (admin)"),
		fuego.OptionTags(tagPlatform),
		fuego.OptionPath(pathName, "Backup file name from listBackups"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted),
	)
}

func (c *Controller) list(ctx fuego.ContextNoBody) ([]BackupResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	backups, err := c.service.List()
	if err != nil {
		return nil, err
	}
	response := make([]BackupResponse, 0, len(backups))
	for _, b := range backups {
		response = append(response, BackupResponse(b))
	}
	return response, nil
}

func (c *Controller) restore(ctx fuego.ContextNoBody) (RestoreResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return RestoreResponse{}, err
	}
	name := ctx.PathParam(pathName)
	err := c.service.Restore(name)
	if errors.Is(err, db.ErrBackupNotFound) || errors.Is(err, db.ErrInvalidBackupName) {
		return RestoreResponse{}, problem.New(TypeBackupNotFound, http.StatusNotFound, "no backup named "+name)
	}
	if err != nil {
		return RestoreResponse{}, err
	}
	return RestoreResponse{Restarting: true}, nil
}
