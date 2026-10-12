package requests

import (
	"context"
	"errors"
	"time"

	"magpie/internal/domain"
	"magpie/internal/requests/api"

	"gorm.io/gorm"
)

const withCommentCount = "issues.*, " +
	"(SELECT COUNT(*) FROM issue_comments c WHERE c.issue_id = issues.id) AS comments"

// openIssueStatuses are the statuses an issue still needs someone in.
var openIssueStatuses = []string{api.IssueStatusOpen, api.IssueStatusActionPending}

type issueRow struct {
	ID            domain.IssueID `gorm:"primaryKey"`
	UserID        domain.UserID
	TitleID       domain.TitleID
	EpisodeID     *domain.EpisodeID
	Kind          string
	Text          string
	Status        string
	PendingAction *string
	CreatedAt     time.Time
	ResolvedAt    *time.Time
	ResolvedBy    *string
	// Comments is the thread's length, read by withCommentCount and never written.
	Comments int `gorm:"->"`
}

func (issueRow) TableName() string { return "issues" }

func (r issueRow) toAPI() api.Issue {
	return api.Issue{
		ID:        r.ID,
		User:      r.UserID,
		Title:     r.TitleID,
		Episode:   r.EpisodeID,
		Kind:      r.Kind,
		Text:      r.Text,
		Status:    r.Status,
		Comments:  r.Comments,
		CreatedAt: r.CreatedAt.UTC(),
	}
}

// pending reports whether the issue waits for the given action to finish.
func (r issueRow) pending(action string) bool {
	return r.Status == api.IssueStatusActionPending && r.PendingAction != nil && *r.PendingAction == action
}

// CommentID is a ULID. Comments never leave Requests, so the ID isn't in the shared kernel.
type CommentID string

type commentRow struct {
	ID        CommentID `gorm:"primaryKey"`
	IssueID   domain.IssueID
	UserID    domain.UserID
	Body      string
	CreatedAt time.Time
}

func (commentRow) TableName() string { return "issue_comments" }

// issueFilter fields that are nil or empty don't filter.
type issueFilter struct {
	Statuses []string
	User     *domain.UserID
	Title    *domain.TitleID
}

func (f issueFilter) apply(query *gorm.DB) *gorm.DB {
	if len(f.Statuses) > 0 {
		query = query.Where("status IN ?", f.Statuses)
	}
	if f.User != nil {
		query = query.Where("user_id = ?", *f.User)
	}
	if f.Title != nil {
		query = query.Where("title_id = ?", *f.Title)
	}
	return query
}

func (r *Repository) OpenIssues(ctx context.Context, title *domain.TitleID) ([]api.Issue, error) {
	rows, err := r.issues(r.db.Read(ctx), issueFilter{Statuses: openIssueStatuses, Title: title})
	if err != nil {
		return nil, err
	}
	issues := make([]api.Issue, 0, len(rows))
	for _, row := range rows {
		issues = append(issues, row.toAPI())
	}
	return issues, nil
}

func (r *Repository) Issue(ctx context.Context, id domain.IssueID) (api.Issue, error) {
	row, err := r.issue(r.db.Read(ctx), id)
	if err != nil {
		return api.Issue{}, err
	}
	return row.toAPI(), nil
}

func (r *Repository) issue(conn *gorm.DB, id domain.IssueID) (issueRow, error) {
	var row issueRow
	err := conn.Select(withCommentCount).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return issueRow{}, api.ErrNotFound
	}
	return row, err
}

func (r *Repository) issues(conn *gorm.DB, f issueFilter) ([]issueRow, error) {
	rows := []issueRow{}
	err := f.apply(conn.Select(withCommentCount)).Order(orderNewestFirst).Find(&rows).Error
	return rows, err
}

func (r *Repository) countIssues(conn *gorm.DB, f issueFilter) (int, error) {
	var total int64
	err := f.apply(conn.Model(&issueRow{})).Count(&total).Error
	return int(total), err
}

func (r *Repository) saveIssue(tx *gorm.DB, row issueRow) error {
	return tx.Save(&row).Error
}

func (r *Repository) comments(conn *gorm.DB, issue domain.IssueID) ([]commentRow, error) {
	rows := []commentRow{}
	err := conn.Where("issue_id = ?", issue).Order("created_at, id").Find(&rows).Error
	return rows, err
}

func (r *Repository) addComment(tx *gorm.DB, row commentRow) error {
	return tx.Create(&row).Error
}
