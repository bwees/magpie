package identity

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"time"

	"magpie/internal/domain"
	"magpie/internal/identity/api"
	"magpie/internal/platform/db"
	"magpie/internal/platform/owner"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

const migrationsDir = "migrations"

// Migrations is Identity's goose directory.
func Migrations() db.MigrationSet {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return db.MigrationSet{Owner: owner.Identity, FS: sub}
}

type userRow struct {
	ID                 domain.UserID `gorm:"primaryKey"`
	Username           string
	DisplayName        string
	Role               api.Role
	JellyfinUserID     *string `gorm:"column:jellyfin_user_id"`
	IsLocalAdmin       bool
	PasswordHash       *string
	AutoApproveMovies  bool
	AutoApproveTV      bool `gorm:"column:auto_approve_tv"`
	ViewOthersRequests bool
	ReportIssues       bool
	Disabled           bool
	CreatedAt          time.Time
	LastLoginAt        *time.Time
}

func (userRow) TableName() string { return "identity_users" }

func (r userRow) user() api.User {
	u := api.User{
		ID:             r.ID,
		Username:       r.Username,
		DisplayName:    r.DisplayName,
		Role:           r.Role,
		JellyfinUserID: r.JellyfinUserID,
		LocalAdmin:     r.IsLocalAdmin,
		Disabled:       r.Disabled,
		Permissions: api.Permissions{
			AutoApproveMovies:  r.AutoApproveMovies,
			AutoApproveTV:      r.AutoApproveTV,
			ViewOthersRequests: r.ViewOthersRequests,
			ReportIssues:       r.ReportIssues,
		},
	}
	if u.IsAdmin() {
		u.Permissions = allPermissions
	}
	return u
}

// rowFromUser stores u's Permissions as its toggles.
func rowFromUser(u api.User) userRow {
	return userRow{
		ID:                 u.ID,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		JellyfinUserID:     u.JellyfinUserID,
		IsLocalAdmin:       u.LocalAdmin,
		AutoApproveMovies:  u.Permissions.AutoApproveMovies,
		AutoApproveTV:      u.Permissions.AutoApproveTV,
		ViewOthersRequests: u.Permissions.ViewOthersRequests,
		ReportIssues:       u.Permissions.ReportIssues,
		Disabled:           u.Disabled,
	}
}

var allPermissions = api.Permissions{AutoApproveMovies: true, AutoApproveTV: true, ViewOthersRequests: true, ReportIssues: true}

type sessionRow struct {
	TokenHash  string `gorm:"primaryKey"`
	UserID     domain.UserID
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	UserAgent  string
}

func (sessionRow) TableName() string { return "identity_sessions" }

const (
	byID             = "id = ?"
	byIDs            = "id IN ?"
	byUsername       = "username = ?"
	byRole           = "role = ?"
	byEnabled        = "NOT disabled"
	byLocalAdmin     = "is_local_admin"
	byTokenHash      = "token_hash = ?"
	byUserID         = "user_id = ?"
	byOtherTokenHash = "token_hash <> ?"
	orderByUsername  = "username"
)

// Repository stores users and sessions and is Identity's api.Reader.
type Repository struct {
	db *db.DB
}

var _ api.Reader = (*Repository)(nil)

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

func (r *Repository) User(ctx context.Context, id domain.UserID) (api.User, error) {
	var row userRow
	err := r.db.Read(ctx).Where(byID, id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.User{}, api.ErrNotFound
	}
	if err != nil {
		return api.User{}, err
	}
	return row.user(), nil
}

func (r *Repository) Users(ctx context.Context, ids []domain.UserID) ([]api.User, error) {
	var rows []userRow
	if err := r.db.Read(ctx).Where(byIDs, ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	found := make(map[domain.UserID]api.User, len(rows))
	for _, row := range rows {
		found[row.ID] = row.user()
	}
	users := make([]api.User, 0, len(rows))
	for _, id := range ids {
		if u, ok := found[id]; ok {
			users = append(users, u)
		}
	}
	return users, nil
}

func (r *Repository) Admins(ctx context.Context) ([]api.User, error) {
	return r.list(r.db.Read(ctx).Where(byRole, api.RoleAdmin).Where(byEnabled))
}

func (r *Repository) All(ctx context.Context) ([]api.User, error) {
	return r.list(r.db.Read(ctx))
}

func (r *Repository) list(query *gorm.DB) ([]api.User, error) {
	var rows []userRow
	if err := query.Order(orderByUsername).Find(&rows).Error; err != nil {
		return nil, err
	}
	users := make([]api.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, row.user())
	}
	return users, nil
}

func (r *Repository) adminExists(conn *gorm.DB) (bool, error) {
	var rows []userRow
	if err := conn.Where(byRole, api.RoleAdmin).Limit(1).Find(&rows).Error; err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// localAdminByUsername matches case-insensitively, as the username column does.
func (r *Repository) localAdminByUsername(conn *gorm.DB, username string) (userRow, error) {
	var row userRow
	err := conn.Where(byLocalAdmin).Where(byUsername, username).Take(&row).Error
	return row, err
}

func (r *Repository) userRow(conn *gorm.DB, id domain.UserID) (userRow, error) {
	var row userRow
	err := conn.Where(byID, id).Take(&row).Error
	return row, err
}

func (r *Repository) insertUser(tx *gorm.DB, row userRow) error {
	return tx.Create(&row).Error
}

func (r *Repository) setPasswordHash(tx *gorm.DB, id domain.UserID, hash string) error {
	return tx.Model(&userRow{}).Where(byID, id).Update("password_hash", hash).Error
}

func (r *Repository) setLastLogin(tx *gorm.DB, id domain.UserID, at time.Time) error {
	return tx.Model(&userRow{}).Where(byID, id).Update("last_login_at", at).Error
}

func (r *Repository) session(conn *gorm.DB, tokenHash string) (sessionRow, error) {
	var row sessionRow
	err := conn.Where(byTokenHash, tokenHash).Take(&row).Error
	return row, err
}

func (r *Repository) insertSession(tx *gorm.DB, row sessionRow) error {
	return tx.Create(&row).Error
}

func (r *Repository) touchSession(tx *gorm.DB, tokenHash string, seenAt, expiresAt time.Time) error {
	return tx.Model(&sessionRow{}).Where(byTokenHash, tokenHash).
		Updates(map[string]any{"last_seen_at": seenAt, "expires_at": expiresAt}).Error
}

func (r *Repository) deleteSession(tx *gorm.DB, tokenHash string) error {
	return tx.Where(byTokenHash, tokenHash).Delete(&sessionRow{}).Error
}

func (r *Repository) deleteOtherSessions(tx *gorm.DB, user domain.UserID, keepTokenHash string) error {
	return tx.Where(byUserID, user).Where(byOtherTokenHash, keepTokenHash).Delete(&sessionRow{}).Error
}
