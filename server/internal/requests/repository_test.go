package requests

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"magpie/internal/domain"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/requests/api"
	"magpie/internal/requests/api/contracttest"

	"gorm.io/gorm"
)

func TestRepositoryMeetsTheRequestsContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
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
			for _, i := range f.Issues {
				if err := repo.saveIssue(tx, fixtureIssue(i)); err != nil {
					return err
				}
				for _, c := range fixtureComments(i) {
					if err := repo.addComment(tx, c); err != nil {
						return err
					}
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

func fixtureIssue(i api.Issue) issueRow {
	return issueRow{
		ID: i.ID, UserID: i.User, TitleID: i.Title, EpisodeID: i.Episode, Kind: i.Kind, Text: i.Text,
		Status: i.Status, CreatedAt: i.CreatedAt,
	}
}

// fixtureComments stores Comments replies by the reporter.
func fixtureComments(i api.Issue) []commentRow {
	rows := make([]commentRow, 0, i.Comments)
	for len(rows) < i.Comments {
		rows = append(rows, commentRow{ID: domain.NewID[CommentID](), IssueID: i.ID, UserID: i.User, Body: i.Text, CreatedAt: i.CreatedAt})
	}
	return rows
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
