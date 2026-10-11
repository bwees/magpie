package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

// takenUsernameShape keeps a Jellyfin user apart from a Magpie user who already has their name.
const takenUsernameShape = "%s-%s"

var (
	ErrJellyfinSignInOff    = errors.New("identity: Jellyfin sign-in is turned off")
	ErrJellyfinUserDisabled = errors.New("identity: the Jellyfin account is disabled")
	ErrUserDisabled         = errors.New("identity: the Magpie account is disabled")
	ErrQuickConnectOff      = errors.New("identity: Quick Connect is turned off in Jellyfin")
	ErrQuickConnectPending  = errors.New("identity: Quick Connect is waiting for approval")
)

// SignInMethods says what the sign-in page offers besides the local admin.
type SignInMethods struct {
	Jellyfin          bool
	QuickConnect      bool
	JellyfinReachable bool
}

type QuickConnectStart struct {
	Code      string
	Handle    string
	ExpiresAt time.Time
}

func (s *Service) SignInMethods(ctx context.Context) (SignInMethods, error) {
	allowed, err := s.jellyfinAllowed(ctx)
	if err != nil || !allowed {
		return SignInMethods{}, err
	}
	enabled, err := s.jellyfin.QuickConnectEnabled(ctx)
	return SignInMethods{Jellyfin: true, QuickConnect: err == nil && enabled, JellyfinReachable: err == nil}, nil
}

// SignInJellyfin checks a username and password with Jellyfin. Wrong passwords count towards the rate limit.
func (s *Service) SignInJellyfin(ctx context.Context, clientIP, username, password, userAgent string) (api.User, issuedSession, error) {
	if !s.limiter.Allowed(clientIP) {
		return api.User{}, issuedSession{}, ErrTooManySignIns
	}
	if err := s.requireJellyfinAllowed(ctx); err != nil {
		return api.User{}, issuedSession{}, err
	}
	account, err := s.jellyfin.AuthenticateByName(ctx, strings.TrimSpace(username), password)
	if errors.Is(err, ErrInvalidCredentials) {
		s.limiter.Failed(clientIP)
	}
	if err != nil {
		return api.User{}, issuedSession{}, err
	}
	return s.signInJellyfinAccount(ctx, account, userAgent)
}

// StartQuickConnect asks Jellyfin for a code. The returned handle stands in for Jellyfin's secret.
func (s *Service) StartQuickConnect(ctx context.Context) (QuickConnectStart, error) {
	if err := s.requireJellyfinAllowed(ctx); err != nil {
		return QuickConnectStart{}, err
	}
	enabled, err := s.jellyfin.QuickConnectEnabled(ctx)
	if err != nil {
		return QuickConnectStart{}, err
	}
	if !enabled {
		return QuickConnectStart{}, ErrQuickConnectOff
	}
	qc, err := s.jellyfin.QuickConnectInitiate(ctx)
	if err != nil {
		return QuickConnectStart{}, err
	}
	handle, err := s.quickConnects.add(qc.Secret, qc.ExpiresAt)
	if err != nil {
		return QuickConnectStart{}, err
	}
	return QuickConnectStart{Code: qc.Code, Handle: handle, ExpiresAt: qc.ExpiresAt}, nil
}

// PollQuickConnect signs the user in once they approved the code in Jellyfin. Until then it returns
// ErrQuickConnectPending; once Jellyfin or Magpie forgot the handle, ErrExpired.
func (s *Service) PollQuickConnect(ctx context.Context, handle, userAgent string) (api.User, issuedSession, error) {
	if err := s.requireJellyfinAllowed(ctx); err != nil {
		return api.User{}, issuedSession{}, err
	}
	secret, ok := s.quickConnects.secret(handle)
	if !ok {
		return api.User{}, issuedSession{}, ErrExpired
	}
	approved, err := s.jellyfin.QuickConnectPoll(ctx, secret)
	if errors.Is(err, ErrExpired) {
		s.quickConnects.remove(handle)
	}
	if err != nil {
		return api.User{}, issuedSession{}, err
	}
	if !approved {
		return api.User{}, issuedSession{}, ErrQuickConnectPending
	}
	account, err := s.jellyfin.AuthenticateWithQuickConnect(ctx, secret)
	if err != nil {
		return api.User{}, issuedSession{}, err
	}
	s.quickConnects.remove(handle)
	return s.signInJellyfinAccount(ctx, account, userAgent)
}

// signInJellyfinAccount creates the Magpie user on first sign-in and issues a session.
// Jellyfin's IsAdministrator is ignored: only a Magpie admin can promote a user.
func (s *Service) signInJellyfinAccount(ctx context.Context, account JellyfinUser, userAgent string) (api.User, issuedSession, error) {
	if account.IsDisabled {
		return api.User{}, issuedSession{}, ErrJellyfinUserDisabled
	}
	var (
		row    userRow
		issued issuedSession
	)
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		var err error
		row, err = s.repo.userByJellyfinID(tx, account.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row, err = s.createJellyfinUser(tx, account)
		}
		if err != nil {
			return err
		}
		if row.Disabled {
			return ErrUserDisabled
		}
		now := s.now()
		row.DisplayName = account.Name
		row.LastLoginAt = &now
		if err := s.repo.recordLogin(tx, row.ID, row.DisplayName, now); err != nil {
			return err
		}
		issued, err = s.startSession(tx, row.ID, userAgent)
		return err
	})
	if err != nil {
		return api.User{}, issuedSession{}, err
	}
	return row.user(), issued, nil
}

func (s *Service) createJellyfinUser(tx *gorm.DB, account JellyfinUser) (userRow, error) {
	defaults, err := defaultsKey.GetTx(tx)
	if err != nil {
		return userRow{}, err
	}
	username := account.Name
	taken, err := s.repo.usernameTaken(tx, username)
	if err != nil {
		return userRow{}, err
	}
	if taken {
		username = fmt.Sprintf(takenUsernameShape, account.Name, account.ID)
	}
	jellyfinID := account.ID
	row := rowFromUser(api.User{
		ID:             domain.NewID[domain.UserID](),
		Username:       username,
		DisplayName:    account.Name,
		Role:           api.RoleRequester,
		Permissions:    defaults,
		JellyfinUserID: &jellyfinID,
	})
	row.CreatedAt = s.now()
	if err := s.repo.insertUser(tx, row); err != nil {
		return userRow{}, err
	}
	created := api.UserCreated{User: row.ID, Role: row.Role, Source: api.UserSourceJellyfin}
	return row, s.bus.Publish(tx, bus.EventOf(created))
}

func (s *Service) jellyfinAllowed(ctx context.Context) (bool, error) {
	signIn, err := signInSettingsKey.Get(ctx, s.store)
	return signIn.AllowJellyfin, err
}

func (s *Service) requireJellyfinAllowed(ctx context.Context) error {
	allowed, err := s.jellyfinAllowed(ctx)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrJellyfinSignInOff
	}
	return nil
}
