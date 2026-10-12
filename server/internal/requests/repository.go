package requests

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/requests/api"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	orderNewestFirst = "created_at DESC, id DESC"
	withItemCounts   = "requests_requests.*, " +
		"(SELECT COUNT(*) FROM requests_request_items i WHERE i.request_id = requests_requests.id) AS total_items, " +
		"(SELECT COUNT(*) FROM requests_request_items i WHERE i.request_id = requests_requests.id AND i.available_at IS NOT NULL) AS available_items"
)

type requestRow struct {
	ID            domain.RequestID `gorm:"primaryKey"`
	UserID        domain.UserID
	Kind          domain.MediaKind
	TmdbID        domain.TmdbID
	TitleID       *domain.TitleID
	Scope         string
	Seasons       []int `gorm:"serializer:json"`
	ProfileID     domain.ProfileID
	Status        api.Status
	CreatedAt     time.Time
	DecidedBy     *domain.UserID
	DecidedAt     *time.Time
	DeclineReason *string
	FailureReason *string
	AvailableAt   *time.Time
	// TotalItems and AvailableItems are the item progress, read by withItemCounts and never written.
	TotalItems     int `gorm:"->"`
	AvailableItems int `gorm:"->"`
}

func (requestRow) TableName() string { return "requests_requests" }

func (r requestRow) toAPI() api.Request {
	return api.Request{
		ID:             r.ID,
		User:           r.UserID,
		Kind:           r.Kind,
		Tmdb:           r.TmdbID,
		Title:          r.TitleID,
		Scope:          r.Scope,
		Seasons:        r.Seasons,
		Profile:        r.ProfileID,
		Status:         r.Status,
		CreatedAt:      r.CreatedAt.UTC(),
		AvailableItems: r.AvailableItems,
		TotalItems:     r.TotalItems,
	}
}

type itemRow struct {
	RequestID   domain.RequestID
	EpisodeID   *domain.EpisodeID
	AvailableAt *time.Time
}

func (itemRow) TableName() string { return "requests_request_items" }

// requestFilter fields that are nil or empty don't filter.
type requestFilter struct {
	Statuses []api.Status
	User     *domain.UserID
	Title    *domain.TitleID
	Kind     *domain.MediaKind
	Tmdb     *domain.TmdbID
}

func (f requestFilter) apply(query *gorm.DB) *gorm.DB {
	if len(f.Statuses) > 0 {
		query = query.Where("status IN ?", f.Statuses)
	}
	if f.User != nil {
		query = query.Where("user_id = ?", *f.User)
	}
	if f.Title != nil {
		query = query.Where("title_id = ?", *f.Title)
	}
	if f.Kind != nil {
		query = query.Where("kind = ?", *f.Kind)
	}
	if f.Tmdb != nil {
		query = query.Where("tmdb_id = ?", *f.Tmdb)
	}
	return query
}

// Repository stores requests and their items and implements api.Reader. Internal reads take a connection
// so the service can read inside its own write transaction.
type Repository struct {
	db *db.DB
}

var _ api.Reader = (*Repository)(nil)

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

func (r *Repository) Request(ctx context.Context, id domain.RequestID) (api.Request, error) {
	row, err := r.request(r.db.Read(ctx), id)
	if err != nil {
		return api.Request{}, err
	}
	return row.toAPI(), nil
}

func (r *Repository) RequestsForTitle(ctx context.Context, title domain.TitleID) ([]api.Request, error) {
	rows, err := r.requests(r.db.Read(ctx), requestFilter{Title: &title})
	if err != nil {
		return nil, err
	}
	requests := make([]api.Request, 0, len(rows))
	for _, row := range rows {
		requests = append(requests, row.toAPI())
	}
	return requests, nil
}

func (r *Repository) Requesters(ctx context.Context, title domain.TitleID) ([]domain.UserID, error) {
	users := []domain.UserID{}
	err := r.db.Read(ctx).Model(&requestRow{}).Distinct("user_id").
		Where("title_id = ? AND status <> ?", title, api.StatusDeclined).Pluck("user_id", &users).Error
	return users, err
}

func (r *Repository) request(conn *gorm.DB, id domain.RequestID) (requestRow, error) {
	var row requestRow
	err := conn.Select(withItemCounts).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return requestRow{}, api.ErrNotFound
	}
	return row, err
}

func (r *Repository) requests(conn *gorm.DB, f requestFilter) ([]requestRow, error) {
	rows := []requestRow{}
	err := f.apply(conn.Select(withItemCounts)).Order(orderNewestFirst).Find(&rows).Error
	return rows, err
}

// countByStatus counts the requests matching f in each status.
func (r *Repository) countByStatus(conn *gorm.DB, f requestFilter) (map[api.Status]int, error) {
	var counts []struct {
		Status api.Status
		Count  int
	}
	err := f.apply(conn.Model(&requestRow{})).Select("status, COUNT(*) AS count").Group("status").Find(&counts).Error
	byStatus := map[api.Status]int{}
	for _, c := range counts {
		byStatus[c.Status] = c.Count
	}
	return byStatus, err
}

func (r *Repository) create(tx *gorm.DB, row requestRow) error {
	return tx.Create(row.stored()).Error
}

func (r *Repository) save(tx *gorm.DB, row requestRow) error {
	return tx.Save(row.stored()).Error
}

// stored stores a movie's missing seasons as an empty list.
func (r requestRow) stored() *requestRow {
	if r.Seasons == nil {
		r.Seasons = []int{}
	}
	return &r
}

func (r *Repository) delete(tx *gorm.DB, id domain.RequestID) error {
	return tx.Where("id = ?", id).Delete(&requestRow{}).Error
}

// markItemsAvailable sets availableAt on a request's items that don't have it yet: its movie item when
// episodes is nil, else the items for those episodes.
func (r *Repository) markItemsAvailable(tx *gorm.DB, id domain.RequestID, episodes []domain.EpisodeID, at time.Time) error {
	query := tx.Model(&itemRow{}).Where("request_id = ? AND available_at IS NULL", id)
	if episodes == nil {
		query = query.Where("episode_id IS NULL")
	} else {
		query = query.Where("episode_id IN ?", episodes)
	}
	return query.Update("available_at", at).Error
}

// addItems stores the items a request doesn't have yet and reports whether any were new.
func (r *Repository) addItems(tx *gorm.DB, items []itemRow) (bool, error) {
	if len(items) == 0 {
		return false, nil
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&items)
	return result.RowsAffected > 0, result.Error
}
