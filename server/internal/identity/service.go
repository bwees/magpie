package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/httpserver"

	"gorm.io/gorm"
)

const (
	// SessionLifetime slides: every touch pushes expiry this far past the request.
	SessionLifetime = 30 * 24 * time.Hour
	// SessionTouchInterval bounds how often a session's last_seen_at is written.
	SessionTouchInterval = 5 * time.Minute

	sessionTokenBytes = 32
)

var (
	ErrSetupComplete    = errors.New("identity: an admin already exists")
	ErrWrongCredentials = errors.New("identity: wrong username or password")
	ErrTooManySignIns   = errors.New("identity: too many failed sign-ins")
	ErrNoSession        = errors.New("identity: session missing, expired or revoked")
	ErrNotLocalAdmin    = errors.New("identity: only the local admin has a password here")
	ErrUsernameRequired = errors.New("identity: username is required")
)

var tokenEncoding = base64.RawURLEncoding

// issuedSession is what the client keeps: the raw token, never stored server-side.
type issuedSession struct {
	Token     string
	ExpiresAt time.Time
}

// activeSession is a valid session resolved from its token. Renewed is set when this request slid its expiry.
type activeSession struct {
	Principal domain.Principal
	TokenHash string
	ExpiresAt time.Time
	Renewed   bool
}

type Service struct {
	db      *db.DB
	bus     *bus.Bus
	repo    *Repository
	limiter *signInLimiter
	now     func() time.Time
	// adminExists only ever turns true: the last admin can't be removed.
	adminExists atomic.Bool
	// selfAuthenticated paths check their own credentials, so the middleware leaves them alone.
	selfAuthenticated []httpserver.SelfAuthenticated
}

func NewService(d *db.DB, b *bus.Bus, repo *Repository, selfAuthenticated []httpserver.SelfAuthenticated) *Service {
	return &Service{db: d, bus: b, repo: repo, limiter: newSignInLimiter(), now: time.Now, selfAuthenticated: selfAuthenticated}
}

// SetupRequired reports whether no admin exists yet, in which case only setup may run.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	if s.adminExists.Load() {
		return false, nil
	}
	exists, err := s.repo.adminExists(s.db.Read(ctx))
	if err != nil {
		return false, err
	}
	if exists {
		s.adminExists.Store(true)
	}
	return !exists, nil
}

// SetupAdmin creates the local admin and signs them in. It fails with ErrSetupComplete once any admin exists.
func (s *Service) SetupAdmin(ctx context.Context, username, password, userAgent string) (api.User, issuedSession, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return api.User{}, issuedSession{}, ErrUsernameRequired
	}
	if err := checkPasswordStrength(password); err != nil {
		return api.User{}, issuedSession{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return api.User{}, issuedSession{}, err
	}

	now := s.now()
	row := userRow{
		ID:           domain.NewID[domain.UserID](),
		Username:     username,
		DisplayName:  username,
		Role:         api.RoleAdmin,
		IsLocalAdmin: true,
		PasswordHash: &hash,
		ReportIssues: true,
		CreatedAt:    now,
		LastLoginAt:  &now,
	}
	var issued issuedSession
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		exists, err := s.repo.adminExists(tx)
		if err != nil {
			return err
		}
		if exists {
			return ErrSetupComplete
		}
		if err := s.repo.insertUser(tx, row); err != nil {
			return err
		}
		created := api.UserCreated{User: row.ID, Role: row.Role, Source: api.UserSourceLocal}
		if err := s.bus.Publish(tx, bus.EventOf(created)); err != nil {
			return err
		}
		issued, err = s.startSession(tx, row.ID, userAgent)
		return err
	})
	if err != nil {
		return api.User{}, issuedSession{}, err
	}
	s.adminExists.Store(true)
	return row.user(), issued, nil
}

// SignInLocal checks the local admin's password. Ten failures from one IP inside the window block that IP.
func (s *Service) SignInLocal(ctx context.Context, clientIP, username, password, userAgent string) (api.User, issuedSession, error) {
	if !s.limiter.Allowed(clientIP) {
		return api.User{}, issuedSession{}, ErrTooManySignIns
	}
	row, err := s.repo.localAdminByUsername(s.db.Read(ctx), strings.TrimSpace(username))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.limiter.Failed(clientIP)
		return api.User{}, issuedSession{}, ErrWrongCredentials
	}
	if err != nil {
		return api.User{}, issuedSession{}, err
	}
	if err := s.checkPassword(row, password); err != nil {
		if errors.Is(err, ErrWrongCredentials) {
			s.limiter.Failed(clientIP)
		}
		return api.User{}, issuedSession{}, err
	}

	var issued issuedSession
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := s.repo.setLastLogin(tx, row.ID, s.now()); err != nil {
			return err
		}
		issued, err = s.startSession(tx, row.ID, userAgent)
		return err
	})
	return row.user(), issued, err
}

// SignOut deletes the session behind token. Unknown tokens are ignored.
func (s *Service) SignOut(ctx context.Context, token string) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		return s.repo.deleteSession(tx, hashToken(token))
	})
}

// Authenticate resolves a session token. Expired sessions are deleted; live ones slide their expiry,
// writing at most once per SessionTouchInterval.
func (s *Service) Authenticate(ctx context.Context, token string) (activeSession, error) {
	tokenHash := hashToken(token)
	session, err := s.repo.session(s.db.Read(ctx), tokenHash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return activeSession{}, ErrNoSession
	}
	if err != nil {
		return activeSession{}, err
	}

	now := s.now()
	if !now.Before(session.ExpiresAt) {
		return activeSession{}, errors.Join(ErrNoSession, s.SignOut(ctx, token))
	}
	user, err := s.repo.User(ctx, session.UserID)
	if errors.Is(err, api.ErrNotFound) {
		return activeSession{}, ErrNoSession
	}
	if err != nil {
		return activeSession{}, err
	}
	if user.Disabled {
		return activeSession{}, ErrNoSession
	}

	active := activeSession{
		Principal: domain.Principal{User: user.ID, Admin: user.IsAdmin(), Permissions: user.Permissions},
		TokenHash: tokenHash,
		ExpiresAt: session.ExpiresAt,
	}
	if now.Sub(session.LastSeenAt) < SessionTouchInterval {
		return active, nil
	}
	active.ExpiresAt = now.Add(SessionLifetime)
	active.Renewed = true
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		return s.repo.touchSession(tx, tokenHash, now, active.ExpiresAt)
	})
	return active, err
}

// Me returns the signed-in user with effective permissions.
func (s *Service) Me(ctx context.Context, principal domain.Principal) (api.User, error) {
	return s.repo.User(ctx, principal.User)
}

// ChangeLocalAdminPassword replaces the local admin's password and signs out their other sessions.
func (s *Service) ChangeLocalAdminPassword(ctx context.Context, principal domain.Principal, token, current, next string) error {
	row, err := s.repo.userRow(s.db.Read(ctx), principal.User)
	if err != nil {
		return err
	}
	if !row.IsLocalAdmin {
		return ErrNotLocalAdmin
	}
	if err := s.checkPassword(row, current); err != nil {
		return err
	}
	if err := checkPasswordStrength(next); err != nil {
		return err
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := s.repo.setPasswordHash(tx, row.ID, hash); err != nil {
			return err
		}
		return s.repo.deleteOtherSessions(tx, row.ID, hashToken(token))
	})
}

func (s *Service) checkPassword(row userRow, password string) error {
	if row.PasswordHash == nil {
		return ErrWrongCredentials
	}
	ok, err := passwordMatches(password, *row.PasswordHash)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWrongCredentials
	}
	return nil
}

// startSession stores a new session for user in tx and returns the token for the cookie.
func (s *Service) startSession(tx *gorm.DB, user domain.UserID, userAgent string) (issuedSession, error) {
	token, err := newSessionToken()
	if err != nil {
		return issuedSession{}, err
	}
	now := s.now()
	row := sessionRow{
		TokenHash:  hashToken(token),
		UserID:     user,
		CreatedAt:  now,
		ExpiresAt:  now.Add(SessionLifetime),
		LastSeenAt: now,
		UserAgent:  userAgent,
	}
	if err := s.repo.insertSession(tx, row); err != nil {
		return issuedSession{}, err
	}
	return issuedSession{Token: token, ExpiresAt: row.ExpiresAt}, nil
}

func newSessionToken() (string, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return tokenEncoding.EncodeToString(raw), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
