package identity

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity/api"

	"gorm.io/gorm"
)

var (
	ErrLastAdmin        = errors.New("identity: at least one enabled admin must remain")
	ErrLocalAdminLocked = errors.New("identity: the local admin can't be changed")
	ErrOwnAccount       = errors.New("identity: users can't change their own role or permissions")
	ErrUnknownRole      = errors.New("identity: role must be ADMIN or REQUESTER")
)

var knownRoles = map[api.Role]bool{api.RoleAdmin: true, api.RoleRequester: true}

// UserChange holds the fields an admin changes; nil fields stay as they are.
type UserChange struct {
	Role               *api.Role
	AutoApproveMovies  *bool
	AutoApproveTV      *bool
	ViewOthersRequests *bool
	ReportIssues       *bool
	ManageFiles        *bool
	Disabled           *bool
}

// UserDetails is a user as the admin Users screen shows them.
type UserDetails struct {
	api.User
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

func (s *Service) Users(ctx context.Context) ([]UserDetails, error) {
	rows, err := s.repo.allRows(s.db.Read(ctx))
	if err != nil {
		return nil, err
	}
	users := make([]UserDetails, 0, len(rows))
	for _, row := range rows {
		users = append(users, row.details())
	}
	return users, nil
}

// UpdateUser applies an admin's change. Disabling a user signs them out everywhere.
func (s *Service) UpdateUser(ctx context.Context, principal domain.Principal, id domain.UserID, change UserChange) (UserDetails, error) {
	if change.Role != nil && !knownRoles[*change.Role] {
		return UserDetails{}, ErrUnknownRole
	}
	var updated userRow
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		current, err := s.repo.userRow(tx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return api.ErrNotFound
		}
		if err != nil {
			return err
		}
		updated = current.with(change)
		if err := s.checkChange(tx, principal, current, updated); err != nil {
			return err
		}
		if err := s.repo.saveAccess(tx, updated); err != nil {
			return err
		}
		if updated.Disabled && !current.Disabled {
			return s.repo.deleteUserSessions(tx, id)
		}
		return nil
	})
	return updated.details(), err
}

// checkChange enforces the invariants in order: the last admin stays, the local admin is fixed,
// and nobody edits their own account.
func (s *Service) checkChange(tx *gorm.DB, principal domain.Principal, current, updated userRow) error {
	if current.enabledAdmin() && !updated.enabledAdmin() {
		admins, err := s.repo.enabledAdminCount(tx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	if current.IsLocalAdmin {
		return ErrLocalAdminLocked
	}
	if current.ID == principal.User {
		return ErrOwnAccount
	}
	return nil
}

func (s *Service) Defaults(ctx context.Context) (api.Permissions, error) {
	return defaultsKey.Get(ctx, s.store)
}

func (s *Service) SetDefaults(ctx context.Context, defaults api.Permissions) error {
	return defaultsKey.Set(ctx, s.store, defaults)
}

func (s *Service) SignInSettings(ctx context.Context) (SignInSettings, error) {
	return signInSettingsKey.Get(ctx, s.store)
}

func (s *Service) SetSignInSettings(ctx context.Context, signIn SignInSettings) error {
	return signInSettingsKey.Set(ctx, s.store, signIn)
}
