package acquisition

import (
	"context"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/api/contracttest"
	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/parse"

	"gorm.io/gorm"
)

// profileReader serves the profile part of the contract from the real repository; downloads,
// blocklist and cutoff status come later and stay with the fake until then.
type profileReader struct {
	api.Reader
	repo *QualityRepository
}

func (r profileReader) Profile(ctx context.Context, id domain.ProfileID) (api.Profile, error) {
	return r.repo.Profile(ctx, id)
}

func (r profileReader) Profiles(ctx context.Context) ([]api.ProfileSummary, error) {
	return r.repo.Profiles(ctx)
}

func TestRepositoryMeetsTheProfileContract(t *testing.T) {
	contracttest.RunProfiles(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, Migrations())
		now := time.Now()
		err := d.Write(context.Background(), func(tx *gorm.DB) error {
			for _, p := range f.Profiles {
				for _, s := range p.FormatScores {
					if err := saveFormat(tx, CustomFormat{ID: s.Format, Name: s.Name}, now); err != nil {
						return err
					}
				}
				if err := saveProfile(tx, p, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return profileReader{Reader: contracttest.NewFake(contracttest.Fixture{}), repo: NewQualityRepository(d)}
	})
}

type qualityTableRow struct {
	ID         int
	Name       string
	Source     string
	Resolution int
	Modifier   string
}

func TestQualitiesTableHoldsRadarrsThirtyIDs(t *testing.T) {
	d := dbtest.Open(t, Migrations())
	var rows []qualityTableRow
	if err := d.Read(context.Background()).Table("acquisition_qualities").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	catalogue := parse.Qualities()
	if len(rows) != len(catalogue) {
		t.Fatalf("%d qualities, want %d", len(rows), len(catalogue))
	}
	for i, q := range catalogue {
		want := qualityTableRow{int(q.ID), q.Name, string(q.Source), int(q.Resolution), string(q.Modifier)}
		if rows[i] != want {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want)
		}
	}
}

func openQualityDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.Open(t, Migrations())
}
