package identity

import (
	"context"
	"errors"
	"net"
	"net/http"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagIdentity     = "identity"
	userAgentHeader = "User-Agent"

	groupRoute         = "/identity"
	setupAdminRoute    = "/setup/admin"
	signInRoutePrefix  = "/login/"
	signInLocalRoute   = signInRoutePrefix + "local"
	signOutRoute       = "/logout"
	meRoute            = "/me"
	localPasswordRoute = "/local-admin/password"
)

const (
	TypeSetupRequired      problem.Type = "urn:magpie:problem:setup-required"
	TypeSetupComplete      problem.Type = "urn:magpie:problem:setup-complete"
	TypeWeakPassword       problem.Type = "urn:magpie:problem:weak-password"
	TypeInvalidCredentials problem.Type = "urn:magpie:problem:invalid-credentials"
	TypeTooManySignIns     problem.Type = "urn:magpie:problem:too-many-sign-ins"
	TypeNotLocalAdmin      problem.Type = "urn:magpie:problem:not-local-admin"
)

type CredentialsRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required"`
}

type MeResponse struct {
	ID          domain.UserID      `json:"id" validate:"required"`
	Username    string             `json:"username" validate:"required"`
	DisplayName string             `json:"displayName" validate:"required"`
	Role        api.Role           `json:"role" validate:"required"`
	Permissions domain.Permissions `json:"permissions" validate:"required"`
	LocalAdmin  bool               `json:"localAdmin" validate:"required"`
}

type SignedOutResponse struct {
	SignedOut bool `json:"signedOut" validate:"required"`
}

type PasswordChangedResponse struct {
	Changed bool `json:"changed" validate:"required"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(server *fuego.Server) {
	group := fuego.Group(server, groupRoute)
	fuego.Post(group, setupAdminRoute, c.setupAdmin,
		fuego.OptionOperationID("setupAdmin"),
		fuego.OptionSummary("Create the local admin while no admin exists, and sign them in"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Post(group, signInLocalRoute, c.signInLocal,
		fuego.OptionOperationID("signInLocal"),
		fuego.OptionSummary("Sign in as the local admin"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Post(group, signOutRoute, c.signOut,
		fuego.OptionOperationID("signOut"),
		fuego.OptionSummary("Delete the current session"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Get(group, meRoute, c.me,
		fuego.OptionOperationID("getMe"),
		fuego.OptionSummary("The signed-in user and their permissions"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Put(group, localPasswordRoute, c.changeLocalPassword,
		fuego.OptionOperationID("changeLocalAdminPassword"),
		fuego.OptionSummary("Change the local admin's password (local admin)"),
		fuego.OptionTags(tagIdentity),
	)
}

func (c *Controller) setupAdmin(ctx fuego.ContextWithBody[CredentialsRequest]) (MeResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return MeResponse{}, err
	}
	user, issued, err := c.service.SetupAdmin(ctx.Context(), body.Username, body.Password, ctx.Header(userAgentHeader))
	if err != nil {
		return MeResponse{}, toProblem(err)
	}
	setSessionCookie(ctx.Response(), ctx.Request(), issued.Token, issued.ExpiresAt)
	return meResponse(user), nil
}

func (c *Controller) signInLocal(ctx fuego.ContextWithBody[CredentialsRequest]) (MeResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return MeResponse{}, err
	}
	user, issued, err := c.service.SignInLocal(ctx.Context(), clientIP(ctx.Request()), body.Username, body.Password, ctx.Header(userAgentHeader))
	if err != nil {
		return MeResponse{}, toProblem(err)
	}
	setSessionCookie(ctx.Response(), ctx.Request(), issued.Token, issued.ExpiresAt)
	return meResponse(user), nil
}

func (c *Controller) signOut(ctx fuego.ContextNoBody) (SignedOutResponse, error) {
	if cookie, err := ctx.Cookie(SessionCookie); err == nil {
		if err := c.service.SignOut(ctx.Context(), cookie.Value); err != nil {
			return SignedOutResponse{}, err
		}
	}
	clearSessionCookie(ctx.Response(), ctx.Request())
	return SignedOutResponse{SignedOut: true}, nil
}

func (c *Controller) me(ctx fuego.ContextNoBody) (MeResponse, error) {
	principal, err := signedIn(ctx.Context())
	if err != nil {
		return MeResponse{}, err
	}
	user, err := c.service.Me(ctx.Context(), principal)
	if err != nil {
		return MeResponse{}, err
	}
	return meResponse(user), nil
}

func (c *Controller) changeLocalPassword(ctx fuego.ContextWithBody[ChangePasswordRequest]) (PasswordChangedResponse, error) {
	principal, err := signedIn(ctx.Context())
	if err != nil {
		return PasswordChangedResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return PasswordChangedResponse{}, err
	}
	cookie, err := ctx.Cookie(SessionCookie)
	if err != nil {
		return PasswordChangedResponse{}, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	if err := c.service.ChangeLocalAdminPassword(ctx.Context(), principal, cookie.Value, body.CurrentPassword, body.NewPassword); err != nil {
		return PasswordChangedResponse{}, toProblem(err)
	}
	return PasswordChangedResponse{Changed: true}, nil
}

func signedIn(ctx context.Context) (domain.Principal, error) {
	principal, ok := domain.PrincipalFrom(ctx)
	if !ok {
		return domain.Principal{}, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	return principal, nil
}

func meResponse(u api.User) MeResponse {
	return MeResponse{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Permissions: u.Permissions,
		LocalAdmin:  u.LocalAdmin,
	}
}

// clientIP is the connecting address. Behind a reverse proxy every client shares the proxy's IP.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func toProblem(err error) error {
	switch {
	case errors.Is(err, ErrSetupComplete):
		return problem.New(TypeSetupComplete, http.StatusConflict, "an admin already exists")
	case errors.Is(err, ErrWeakPassword):
		return problem.New(TypeWeakPassword, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUsernameRequired):
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, "username is required")
	case errors.Is(err, ErrWrongCredentials):
		return problem.New(TypeInvalidCredentials, http.StatusUnauthorized, "wrong username or password")
	case errors.Is(err, ErrTooManySignIns):
		return problem.New(TypeTooManySignIns, http.StatusTooManyRequests, "too many failed sign-ins; try again in a few minutes")
	case errors.Is(err, ErrNotLocalAdmin):
		return problem.New(TypeNotLocalAdmin, http.StatusForbidden, "only the local admin has a password in Magpie")
	default:
		return err
	}
}
