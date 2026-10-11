package requests

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/requests/api"
	"magpie/internal/requests/api/contracttest"

	"gorm.io/gorm"
)

func TestRepositoryMeetsTheRequestsContract(t *testing.T) {
	contracttest.RunRequests(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, Migrations())
		repo := NewRepository(d)
		err := d.Write(context.Background(), func(tx *gorm.DB) error {
			for _, r := range f.Requests {
				if err := repo.create(tx, fixtureRow(r)); err != nil {
					return err
				}
				if _, err := repo.addItems(tx, fixtureItems(r)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return repo
	})
}

func TestRepositoryHasNoIssuesYet(t *testing.T) {
	repo := NewRepository(dbtest.Open(t, Migrations()))
	if issues, err := repo.OpenIssues(context.Background(), nil); err != nil || issues == nil || len(issues) != 0 {
		t.Errorf("OpenIssues = %#v, %v; want an empty slice", issues, err)
	}
	if _, err := repo.Issue(context.Background(), domain.NewID[domain.IssueID]()); err != api.ErrNotFound {
		t.Errorf("Issue err = %v, want ErrNotFound", err)
	}
}

func fixtureRow(r api.Request) requestRow {
	return requestRow{
		ID:        r.ID,
		UserID:    r.User,
		Kind:      r.Kind,
		TmdbID:    r.Tmdb,
		TitleID:   r.Title,
		Scope:     r.Scope,
		Seasons:   r.Seasons,
		ProfileID: r.Profile,
		Status:    r.Status,
		CreatedAt: r.CreatedAt,
	}
}

// fixtureItems stores TotalItems episodes, the first AvailableItems of them available.
func fixtureItems(r api.Request) []itemRow {
	items := make([]itemRow, 0, r.TotalItems)
	for len(items) < r.TotalItems {
		item := itemRow{RequestID: r.ID, EpisodeID: ptr(domain.NewID[domain.EpisodeID]())}
		if len(items) < r.AvailableItems {
			item.AvailableAt = ptr(r.CreatedAt)
		}
		items = append(items, item)
	}
	return items
}

// markAvailable stands in for library.ItemAvailable until Requests consumes it.
func (h *harness) markAvailable(id domain.RequestID) {
	h.t.Helper()
	now := time.Now().UTC()
	err := h.db.Write(context.Background(), func(tx *gorm.DB) error {
		if err := tx.Model(&itemRow{}).Where("request_id = ?", id).Update("available_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&requestRow{}).Where("id = ?", id).Updates(map[string]any{"status": api.StatusAvailable, "available_at": now}).Error
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

type problemBody struct {
	Type     string `json:"type"`
	Instance string `json:"instance"`
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) problemBody {
	t.Helper()
	var p problemBody
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body, err)
	}
	return p
}

func equalApproved(a, b api.RequestApproved) bool {
	return reflect.DeepEqual(a, b)
}
