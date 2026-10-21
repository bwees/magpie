package identity

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagIdentity     = "identity"
	userAgentHeader = "User-Agent"

	groupRoute            = "/identity"
	setupAdminRoute       = "/setup/admin"
	signInRoutePrefix     = "/login/"
	signInLocalRoute      = signInRoutePrefix + "local"
	signInJellyfinRoute   = signInRoutePrefix + "jellyfin"
	signInMethodsRoute    = signInRoutePrefix + "methods"
	quickConnectRoute     = signInRoutePrefix + "quick-connect"
	quickConnectPollRoute = quickConnectRoute + "/{" + pathHandle + "}"
	signOutRoute          = "/logout"
	meRoute               = "/me"
	localPasswordRoute    = "/local-admin/password"
	usersRoute            = "/users"
	userRoute             = usersRoute + "/{" + pathUserID + "}"
	defaultsRoute         = "/defaults"
	signInSettingsRoute   = "/sign-in-settings"

	pathHandle = "handle"
	pathUserID = "id"
)

const (
	TypeSetupRequired        problem.Type = "urn:magpie:problem:setup-required"
	TypeSetupComplete        problem.Type = "urn:magpie:problem:setup-complete"
	TypeWeakPassword         problem.Type = "urn:magpie:problem:weak-password"
	TypeInvalidCredentials   problem.Type = "urn:magpie:problem:invalid-credentials"
	TypeTooManySignIns       problem.Type = "urn:magpie:problem:too-many-sign-ins"
	TypeNotLocalAdmin        problem.Type = "urn:magpie:problem:not-local-admin"
	TypeJellyfinUnavailable  problem.Type = "urn:magpie:problem:jellyfin-unavailable"
	TypeJellyfinUserDisabled problem.Type = "urn:magpie:problem:jellyfin-user-disabled"
	TypeUserDisabled         problem.Type = "urn:magpie:problem:user-disabled"
	TypeJellyfinSignInOff    problem.Type = "urn:magpie:problem:jellyfin-sign-in-off"
	TypeQuickConnectOff      problem.Type = "urn:magpie:problem:quick-connect-off"
	TypeQuickConnectExpired  problem.Type = "urn:magpie:problem:quick-connect-expired"
	TypeLastAdmin            problem.Type = "urn:magpie:problem:last-admin"
	TypeLocalAdminLocked     problem.Type = "urn:magpie:problem:local-admin-locked"
	TypeOwnAccount           problem.Type = "urn:magpie:problem:own-account"
)

// QuickConnectStatus is the state of a Quick Connect sign-in as the browser polls it.
type QuickConnectStatus string

const (
	QuickConnectPending  QuickConnectStatus = "PENDING"
	QuickConnectApproved QuickConnectStatus = "APPROVED"
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

type SignInMethodsResponse struct {
	Jellyfin          bool `json:"jellyfin" validate:"required"`
	QuickConnect      bool `json:"quickConnect" validate:"required"`
	JellyfinReachable bool `json:"jellyfinReachable" validate:"required"`
}

type QuickConnectStartResponse struct {
	Code      string    `json:"code" validate:"required"`
	Handle    string    `json:"handle" validate:"required"`
	ExpiresAt time.Time `json:"expiresAt" validate:"required"`
}

// QuickConnectPollResponse carries the user once Status is APPROVED.
type QuickConnectPollResponse struct {
	Status QuickConnectStatus `json:"status" validate:"required"`
	User   *MeResponse        `json:"user,omitempty"`
}

type UserResponse struct {
	ID          domain.UserID      `json:"id" validate:"required"`
	Username    string             `json:"username" validate:"required"`
	DisplayName string             `json:"displayName" validate:"required"`
	Role        api.Role           `json:"role" validate:"required"`
	Permissions domain.Permissions `json:"permissions" validate:"required"`
	Source      string             `json:"source" validate:"required"`
	LocalAdmin  bool               `json:"localAdmin" validate:"required"`
	Disabled    bool               `json:"disabled" validate:"required"`
	CreatedAt   time.Time          `json:"createdAt" validate:"required"`
	LastLoginAt *time.Time         `json:"lastLoginAt,omitempty"`
}

// UpdateUserRequest changes only the fields that are set.
type UpdateUserRequest struct {
	Role               *api.Role `json:"role,omitempty"`
	AutoApproveMovies  *bool     `json:"autoApproveMovies,omitempty"`
	AutoApproveTV      *bool     `json:"autoApproveTv,omitempty"`
	ViewOthersRequests *bool     `json:"viewOthersRequests,omitempty"`
	ReportIssues       *bool     `json:"reportIssues,omitempty"`
	ManageFiles        *bool     `json:"manageFiles,omitempty"`
	Disabled           *bool     `json:"disabled,omitempty"`
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
	fuego.Get(group, signInMethodsRoute, c.signInMethods,
		fuego.OptionOperationID("getSignInMethods"),
		fuego.OptionSummary("What the sign-in page offers besides the local admin"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Post(group, signInJellyfinRoute, c.signInJellyfin,
		fuego.OptionOperationID("signInJellyfin"),
		fuego.OptionSummary("Sign in with a Jellyfin username and password"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Post(group, quickConnectRoute, c.startQuickConnect,
		fuego.OptionOperationID("startQuickConnect"),
		fuego.OptionSummary("Start a Jellyfin Quick Connect sign-in; returns the code to approve and a handle to poll"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Get(group, quickConnectPollRoute, c.pollQuickConnect,
		fuego.OptionOperationID("pollQuickConnect"),
		fuego.OptionSummary("Poll a Quick Connect sign-in: 202 while pending, a session once approved, 410 once expired"),
		fuego.OptionTags(tagIdentity),
		fuego.OptionPath(pathHandle, "Handle from startQuickConnect"),
		fuego.OptionAddResponse(http.StatusAccepted, "Waiting for approval in Jellyfin", fuego.Response{Type: QuickConnectPollResponse{}}),
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
	fuego.Get(group, usersRoute, c.listUsers,
		fuego.OptionOperationID("listUsers"),
		fuego.OptionSummary("Every user ordered by username (admin)"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Patch(group, userRoute, c.updateUser,
		fuego.OptionOperationID("updateUser"),
		fuego.OptionSummary("Change a user's role, permissions or disabled flag (admin)"),
		fuego.OptionTags(tagIdentity),
		fuego.OptionPath(pathUserID, "User ID"),
	)
	fuego.Get(group, defaultsRoute, c.defaults,
		fuego.OptionOperationID("getUserDefaults"),
		fuego.OptionSummary("Permissions new Jellyfin users start with (admin)"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Put(group, defaultsRoute, c.setDefaults,
		fuego.OptionOperationID("setUserDefaults"),
		fuego.OptionSummary("Set the permissions new Jellyfin users start with (admin)"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Get(group, signInSettingsRoute, c.signInSettings,
		fuego.OptionOperationID("getSignInSettings"),
		fuego.OptionSummary("Whether Jellyfin users may sign in (admin)"),
		fuego.OptionTags(tagIdentity),
	)
	fuego.Put(group, signInSettingsRoute, c.setSignInSettings,
		fuego.OptionOperationID("setSignInSettings"),
		fuego.OptionSummary("Allow or stop Jellyfin sign-in; the local admin always stays available (admin)"),
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

func (c *Controller) signInMethods(ctx fuego.ContextNoBody) (SignInMethodsResponse, error) {
	methods, err := c.service.SignInMethods(ctx.Context())
	if err != nil {
		return SignInMethodsResponse{}, err
	}
	return SignInMethodsResponse(methods), nil
}

func (c *Controller) signInJellyfin(ctx fuego.ContextWithBody[CredentialsRequest]) (MeResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return MeResponse{}, err
	}
	user, issued, err := c.service.SignInJellyfin(ctx.Context(), clientIP(ctx.Request()), body.Username, body.Password, ctx.Header(userAgentHeader))
	if err != nil {
		return MeResponse{}, toProblem(err)
	}
	setSessionCookie(ctx.Response(), ctx.Request(), issued.Token, issued.ExpiresAt)
	return meResponse(user), nil
}

func (c *Controller) startQuickConnect(ctx fuego.ContextNoBody) (QuickConnectStartResponse, error) {
	start, err := c.service.StartQuickConnect(ctx.Context())
	if err != nil {
		return QuickConnectStartResponse{}, toProblem(err)
	}
	return QuickConnectStartResponse(start), nil
}

func (c *Controller) pollQuickConnect(ctx fuego.ContextNoBody) (QuickConnectPollResponse, error) {
	user, issued, err := c.service.PollQuickConnect(ctx.Context(), ctx.PathParam(pathHandle), ctx.Header(userAgentHeader))
	if errors.Is(err, ErrQuickConnectPending) {
		ctx.SetStatus(http.StatusAccepted)
		return QuickConnectPollResponse{Status: QuickConnectPending}, nil
	}
	if err != nil {
		return QuickConnectPollResponse{}, toProblem(err)
	}
	setSessionCookie(ctx.Response(), ctx.Request(), issued.Token, issued.ExpiresAt)
	me := meResponse(user)
	return QuickConnectPollResponse{Status: QuickConnectApproved, User: &me}, nil
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

func (c *Controller) listUsers(ctx fuego.ContextNoBody) ([]UserResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	users, err := c.service.Users(ctx.Context())
	if err != nil {
		return nil, err
	}
	responses := make([]UserResponse, 0, len(users))
	for _, u := range users {
		responses = append(responses, userResponse(u))
	}
	return responses, nil
}

func (c *Controller) updateUser(ctx fuego.ContextWithBody[UpdateUserRequest]) (UserResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return UserResponse{}, err
	}
	principal, err := signedIn(ctx.Context())
	if err != nil {
		return UserResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return UserResponse{}, err
	}
	updated, err := c.service.UpdateUser(ctx.Context(), principal, domain.UserID(ctx.PathParam(pathUserID)), UserChange(body))
	if err != nil {
		return UserResponse{}, toProblem(err)
	}
	return userResponse(updated), nil
}

func (c *Controller) defaults(ctx fuego.ContextNoBody) (domain.Permissions, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return domain.Permissions{}, err
	}
	return c.service.Defaults(ctx.Context())
}

func (c *Controller) setDefaults(ctx fuego.ContextWithBody[domain.Permissions]) (domain.Permissions, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return domain.Permissions{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return domain.Permissions{}, err
	}
	return body, c.service.SetDefaults(ctx.Context(), body)
}

func (c *Controller) signInSettings(ctx fuego.ContextNoBody) (SignInSettings, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SignInSettings{}, err
	}
	return c.service.SignInSettings(ctx.Context())
}

func (c *Controller) setSignInSettings(ctx fuego.ContextWithBody[SignInSettings]) (SignInSettings, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SignInSettings{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return SignInSettings{}, err
	}
	return body, c.service.SetSignInSettings(ctx.Context(), body)
}

func userResponse(u UserDetails) UserResponse {
	source := api.UserSourceJellyfin
	if u.LocalAdmin {
		source = api.UserSourceLocal
	}
	return UserResponse{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Permissions: u.Permissions,
		Source:      source,
		LocalAdmin:  u.LocalAdmin,
		Disabled:    u.Disabled,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
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
	case errors.Is(err, ErrInvalidCredentials):
		return problem.New(TypeInvalidCredentials, http.StatusUnauthorized, "wrong Jellyfin username or password")
	case errors.Is(err, ErrUnavailable):
		return problem.New(TypeJellyfinUnavailable, http.StatusServiceUnavailable, "Jellyfin is not reachable; the local admin can still sign in")
	case errors.Is(err, ErrJellyfinUserDisabled):
		return problem.New(TypeJellyfinUserDisabled, http.StatusForbidden, "this Jellyfin account is disabled")
	case errors.Is(err, ErrUserDisabled):
		return problem.New(TypeUserDisabled, http.StatusForbidden, "this account is disabled in Magpie; ask an admin")
	case errors.Is(err, ErrJellyfinSignInOff):
		return problem.New(TypeJellyfinSignInOff, http.StatusForbidden, "Jellyfin sign-in is turned off; the local admin can still sign in")
	case errors.Is(err, ErrQuickConnectOff):
		return problem.New(TypeQuickConnectOff, http.StatusForbidden, "Quick Connect is turned off in Jellyfin")
	case errors.Is(err, ErrExpired):
		return problem.New(TypeQuickConnectExpired, http.StatusGone, "the Quick Connect code expired; start again")
	case errors.Is(err, ErrLastAdmin):
		return problem.New(TypeLastAdmin, http.StatusConflict, "at least one enabled admin must remain")
	case errors.Is(err, ErrLocalAdminLocked):
		return problem.New(TypeLocalAdminLocked, http.StatusConflict, "the local admin can't be demoted or disabled")
	case errors.Is(err, ErrOwnAccount):
		return problem.New(TypeOwnAccount, http.StatusForbidden, "you can't change your own role or permissions")
	case errors.Is(err, ErrUnknownRole):
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, "role must be ADMIN or REQUESTER")
	case errors.Is(err, api.ErrNotFound):
		return problem.New(problem.TypeNotFound, http.StatusNotFound, "no such user")
	default:
		return err
	}
}
