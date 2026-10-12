package acquisition

import (
	"context"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/api/contracttest"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/parse"

	"gorm.io/gorm"
)

func TestReaderMeetsTheContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
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
			for _, dl := range f.Downloads {
				if err := insertDownload(tx, download{Download: dl}); err != nil {
					return err
				}
			}
			for _, e := range f.Blocklist {
				if _, err := insertBlocklistEntry(tx, e); err != nil {
					return err
				}
			}
			for _, s := range f.FileScores {
				score := FileScore{
					File: s.File, Title: s.Title, Quality: s.Quality, CFScore: s.CFScore, Profile: s.Profile,
					CutoffMet: s.CutoffMet, Transcoded: s.Transcoded, Origin: s.Origin,
				}
				if err := saveFileScore(tx, score, now); err != nil {
					return err
				}
			}
			if f.DefaultProfile == "" {
				return nil
			}
			return DefaultProfileKey.SetTx(tx, f.DefaultProfile)
		})
		if err != nil {
			t.Fatal(err)
		}
		return NewReader(NewQualityRepository(d), NewRepository(d))
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
