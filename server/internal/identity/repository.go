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

func (r userRow) details() UserDetails {
	return UserDetails{User: r.user(), CreatedAt: r.CreatedAt, LastLoginAt: r.LastLoginAt}
}

func (r userRow) enabledAdmin() bool {
	return r.Role == api.RoleAdmin && !r.Disabled
}

// with returns r with the set fields of change applied.
func (r userRow) with(change UserChange) userRow {
	for field, value := range map[*bool]*bool{
		&r.AutoApproveMovies:  change.AutoApproveMovies,
		&r.AutoApproveTV:      change.AutoApproveTV,
		&r.ViewOthersRequests: change.ViewOthersRequests,
		&r.ReportIssues:       change.ReportIssues,
		&r.Disabled:           change.Disabled,
	} {
		if value != nil {
			*field = *value
		}
	}
	if change.Role != nil {
		r.Role = *change.Role
	}
	return r
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
	byJellyfinID     = "jellyfin_user_id = ?"
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

func (r *Repository) userByJellyfinID(conn *gorm.DB, jellyfinID string) (userRow, error) {
	var row userRow
	err := conn.Where(byJellyfinID, jellyfinID).Take(&row).Error
	return row, err
}

// usernameTaken matches case-insensitively, as the username column does.
func (r *Repository) usernameTaken(conn *gorm.DB, username string) (bool, error) {
	var rows []userRow
	if err := conn.Where(byUsername, username).Limit(1).Find(&rows).Error; err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (r *Repository) allRows(conn *gorm.DB) ([]userRow, error) {
	var rows []userRow
	err := conn.Order(orderByUsername).Find(&rows).Error
	return rows, err
}

func (r *Repository) enabledAdminCount(conn *gorm.DB) (int64, error) {
	var n int64
	err := conn.Model(&userRow{}).Where(byRole, api.RoleAdmin).Where(byEnabled).Count(&n).Error
	return n, err
}

func (r *Repository) recordLogin(tx *gorm.DB, id domain.UserID, displayName string, at time.Time) error {
	return tx.Model(&userRow{}).Where(byID, id).
		Updates(map[string]any{"display_name": displayName, "last_login_at": at}).Error
}

// saveAccess writes the role, permission toggles and disabled flag of row.
func (r *Repository) saveAccess(tx *gorm.DB, row userRow) error {
	return tx.Model(&userRow{}).Where(byID, row.ID).Updates(map[string]any{
		"role":                 row.Role,
		"auto_approve_movies":  row.AutoApproveMovies,
		"auto_approve_tv":      row.AutoApproveTV,
		"view_others_requests": row.ViewOthersRequests,
		"report_issues":        row.ReportIssues,
		"disabled":             row.Disabled,
	}).Error
}

func (r *Repository) deleteUserSessions(tx *gorm.DB, user domain.UserID) error {
	return tx.Where(byUserID, user).Delete(&sessionRow{}).Error
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
