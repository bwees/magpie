package activity

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	queryUnread = "unread"
	pathID      = "id"
)

const (
	TypeInvalidNotificationURL     problem.Type = "urn:magpie:problem:invalid-notification-url"
	TypeNotificationURLLimit       problem.Type = "urn:magpie:problem:notification-url-limit"
	TypeNotificationKindNotAllowed problem.Type = "urn:magpie:problem:notification-kind-not-allowed"
	TypeNotificationRejected       problem.Type = "urn:magpie:problem:notification-rejected"
	TypeNotificationUnavailable    problem.Type = "urn:magpie:problem:notification-unavailable"
)

type NotificationResponse struct {
	ID        string    `json:"id" validate:"required"`
	Kind      string    `json:"kind" validate:"required"`
	Title     string    `json:"title" validate:"required"`
	Body      string    `json:"body" validate:"required"`
	Link      string    `json:"link" validate:"required"`
	CreatedAt time.Time `json:"createdAt" validate:"required"`
	Read      bool      `json:"read" validate:"required"`
}

type NotificationListResponse struct {
	Notifications []NotificationResponse `json:"notifications" validate:"required"`
	UnreadCount   int                    `json:"unreadCount" validate:"required"`
}

type NotificationKindResponse struct {
	Kind  string `json:"kind" validate:"required"`
	Label string `json:"label" validate:"required"`
	Scope string `json:"scope" validate:"required"`
	OptIn bool   `json:"optIn" validate:"required"`
}

type NotificationURLResponse struct {
	ID         string     `json:"id" validate:"required"`
	Label      string     `json:"label" validate:"required"`
	MaskedURL  string     `json:"maskedUrl" validate:"required"`
	Kinds      []string   `json:"kinds" validate:"required"`
	CreatedAt  time.Time  `json:"createdAt" validate:"required"`
	LastSentAt *time.Time `json:"lastSentAt"`
	LastError  string     `json:"lastError" validate:"required"`
}

type NotificationURLRequest struct {
	Label string   `json:"label"`
	URL   string   `json:"url" validate:"required"`
	Kinds []string `json:"kinds"`
}

// NotificationURLChangeRequest edits a URL's label or kinds; omitted or null fields stay as they are.
type NotificationURLChangeRequest struct {
	Label *string  `json:"label,omitempty"`
	Kinds []string `json:"kinds"`
}

func (c *Controller) registerNotifications(server *fuego.Server) {
	tags := fuego.OptionTags(tagActivity)
	idPath := fuego.OptionPath(pathID, "ID")
	fuego.Get(server, "/activity/notifications", c.notifications, tags,
		fuego.OptionOperationID("listNotifications"),
		fuego.OptionSummary("Your in-app notifications, newest first, and how many are unread"),
		fuego.OptionQueryBool(queryUnread, "Only unread notifications"),
		fuego.OptionQuery(queryBefore, "Only entries strictly older than this RFC 3339 time"),
		fuego.OptionQueryInt(queryLimit, "At most this many entries (default 50, max 200)"),
	)
	fuego.Post(server, "/activity/notifications/{id}/read", c.markRead, tags, idPath,
		fuego.OptionOperationID("markNotificationRead"), fuego.OptionSummary("Mark one of your notifications read"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Post(server, "/activity/notifications/read-all", c.markAllRead, tags,
		fuego.OptionOperationID("markAllNotificationsRead"), fuego.OptionSummary("Mark all your notifications read"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Get(server, "/activity/notification-kinds", c.notificationKinds, tags,
		fuego.OptionOperationID("listNotificationKinds"),
		fuego.OptionSummary("The notification kinds your role may subscribe a URL to"))
	fuego.Get(server, "/activity/notification-urls", c.notificationURLs, tags,
		fuego.OptionOperationID("listNotificationUrls"), fuego.OptionSummary("Your notification URLs, masked"))
	fuego.Post(server, "/activity/notification-urls", c.addNotificationURL, tags,
		fuego.OptionOperationID("addNotificationUrl"),
		fuego.OptionSummary(fmt.Sprintf("Add a shoutrrr URL (at most %d per user)", MaxNotificationURLs)),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Patch(server, "/activity/notification-urls/{id}", c.updateNotificationURL, tags, idPath,
		fuego.OptionOperationID("updateNotificationUrl"), fuego.OptionSummary("Change a URL's label or event kinds"))
	fuego.Delete(server, "/activity/notification-urls/{id}", c.deleteNotificationURL, tags, idPath,
		fuego.OptionOperationID("deleteNotificationUrl"), fuego.OptionSummary("Delete a notification URL"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Post(server, "/activity/notification-urls/{id}/test", c.testNotificationURL, tags, idPath,
		fuego.OptionOperationID("testNotificationUrl"), fuego.OptionSummary("Send a test message to a URL now"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
}

func notificationProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotificationNotFound):
		return problem.New(problem.TypeNotFound, http.StatusNotFound, "no such notification")
	case errors.Is(err, ErrURLNotFound):
		return problem.New(problem.TypeNotFound, http.StatusNotFound, "no such notification URL")
	case errors.Is(err, ErrInvalidURL):
		return problem.New(TypeInvalidNotificationURL, http.StatusUnprocessableEntity,
			"That isn't a notification URL Magpie can use. Check the scheme and token.")
	case errors.Is(err, ErrTooManyURLs):
		return problem.New(TypeNotificationURLLimit, http.StatusUnprocessableEntity,
			fmt.Sprintf("You can add at most %d notification URLs.", MaxNotificationURLs))
	case errors.Is(err, ErrKindNotAllowed):
		return problem.New(TypeNotificationKindNotAllowed, http.StatusUnprocessableEntity,
			"One of the events isn't available to your account.")
	case errors.Is(err, ErrRejected):
		return problem.New(TypeNotificationRejected, http.StatusUnprocessableEntity, "The service refused the message: "+err.Error())
	case errors.Is(err, ErrUnavailable):
		return problem.New(TypeNotificationUnavailable, http.StatusBadGateway, "The service couldn't be reached: "+err.Error())
	}
	return err
}

func (c *Controller) notifications(ctx fuego.ContextNoBody) (NotificationListResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return NotificationListResponse{}, err
	}
	p, err := page(ctx)
	if err != nil {
		return NotificationListResponse{}, err
	}
	notifications, unread, err := c.service.Notifications(ctx.Context(), principal.User, ctx.QueryParamBool(queryUnread), p)
	if err != nil {
		return NotificationListResponse{}, err
	}
	return NotificationListResponse{Notifications: mapAll(notifications, notificationResponse), UnreadCount: unread}, nil
}

func (c *Controller) markRead(ctx fuego.ContextNoBody) (any, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	return nil, notificationProblem(c.service.MarkRead(ctx.Context(), principal.User, ctx.PathParam(pathID)))
}

func (c *Controller) markAllRead(ctx fuego.ContextNoBody) (any, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	return nil, c.service.MarkAllRead(ctx.Context(), principal.User)
}

func (c *Controller) notificationKinds(ctx fuego.ContextNoBody) ([]NotificationKindResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	return mapAll(KindsFor(principal.Admin), func(k KindSpec) NotificationKindResponse {
		return NotificationKindResponse{Kind: k.Kind, Label: k.Label, Scope: string(k.Scope), OptIn: k.OptIn}
	}), nil
}

func (c *Controller) notificationURLs(ctx fuego.ContextNoBody) ([]NotificationURLResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	urls, err := c.service.NotificationURLs(ctx.Context(), principal)
	if err != nil {
		return nil, err
	}
	return mapAll(urls, notificationURLResponse), nil
}

func (c *Controller) addNotificationURL(ctx fuego.ContextWithBody[NotificationURLRequest]) (NotificationURLResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return NotificationURLResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return NotificationURLResponse{}, err
	}
	added, err := c.service.AddNotificationURL(ctx.Context(), principal, NewNotificationURL(body))
	if err != nil {
		return NotificationURLResponse{}, notificationProblem(err)
	}
	return notificationURLResponse(added), nil
}

func (c *Controller) updateNotificationURL(ctx fuego.ContextWithBody[NotificationURLChangeRequest]) (NotificationURLResponse, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return NotificationURLResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return NotificationURLResponse{}, err
	}
	updated, err := c.service.UpdateNotificationURL(ctx.Context(), principal, ctx.PathParam(pathID), NotificationURLChange(body))
	if err != nil {
		return NotificationURLResponse{}, notificationProblem(err)
	}
	return notificationURLResponse(updated), nil
}

func (c *Controller) deleteNotificationURL(ctx fuego.ContextNoBody) (any, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	return nil, notificationProblem(c.service.DeleteNotificationURL(ctx.Context(), principal, ctx.PathParam(pathID)))
}

func (c *Controller) testNotificationURL(ctx fuego.ContextNoBody) (any, error) {
	principal, err := viewer(ctx.Context())
	if err != nil {
		return nil, err
	}
	return nil, notificationProblem(c.service.TestNotificationURL(ctx.Context(), principal, ctx.PathParam(pathID)))
}

func notificationResponse(n api.Notification) NotificationResponse {
	return NotificationResponse(n)
}

func notificationURLResponse(u NotificationURL) NotificationURLResponse {
	return NotificationURLResponse{
		ID: u.ID, Label: u.Label, MaskedURL: u.Masked, Kinds: u.Kinds,
		CreatedAt: u.CreatedAt, LastSentAt: u.LastSentAt, LastError: u.LastError,
	}
}
