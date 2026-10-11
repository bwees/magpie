package bus

import (
	"context"
	"errors"
	"net/http"
	"time"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagPlatform                     = "platform"
	TypeEventNotParked problem.Type = "urn:magpie:problem:event-not-parked"
)

type ParkedEventResponse struct {
	Handler     string    `json:"handler" validate:"required"`
	EventID     string    `json:"eventId" validate:"required"`
	EventType   string    `json:"eventType" validate:"required"`
	AggregateID string    `json:"aggregateId" validate:"required"`
	Attempts    int       `json:"attempts" validate:"required"`
	ParkedAt    time.Time `json:"parkedAt" validate:"required"`
	LastError   string    `json:"lastError" validate:"required"`
}

type RetryParkedEventRequest struct {
	Handler string `json:"handler" validate:"required"`
	EventID string `json:"eventId" validate:"required"`
}

type Controller struct {
	dispatcher *Dispatcher
}

func NewController(d *Dispatcher) *Controller {
	return &Controller{dispatcher: d}
}

func (c *Controller) Register(api *fuego.Server) {
	fuego.Get(api, "/platform/parked-events", c.listParked,
		fuego.OptionOperationID("listParkedEvents"),
		fuego.OptionSummary("List events a handler gave up on (admin)"),
		fuego.OptionTags(tagPlatform),
	)
	fuego.Post(api, "/platform/parked-events/retry", c.retryParked,
		fuego.OptionOperationID("retryParkedEvent"),
		fuego.OptionSummary("Re-queue a parked event for its handler (admin)"),
		fuego.OptionTags(tagPlatform),
	)
}

func (c *Controller) listParked(ctx fuego.ContextNoBody) ([]ParkedEventResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return c.parked(ctx.Context())
}

func (c *Controller) retryParked(ctx fuego.ContextWithBody[RetryParkedEventRequest]) ([]ParkedEventResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	body, err := ctx.Body()
	if err != nil {
		return nil, err
	}
	err = c.dispatcher.Retry(ctx.Context(), body.Handler, body.EventID)
	if errors.Is(err, ErrNotParked) {
		return nil, problem.New(TypeEventNotParked, http.StatusNotFound, "no parked event "+body.EventID+" for handler "+body.Handler)
	}
	if err != nil {
		return nil, err
	}
	return c.parked(ctx.Context())
}

func (c *Controller) parked(ctx context.Context) ([]ParkedEventResponse, error) {
	parked, err := c.dispatcher.Parked(ctx)
	if err != nil {
		return nil, err
	}
	response := make([]ParkedEventResponse, 0, len(parked))
	for _, p := range parked {
		response = append(response, ParkedEventResponse(p))
	}
	return response, nil
}
