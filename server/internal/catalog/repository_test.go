package catalog

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/catalog/api/contracttest"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"

	"gorm.io/gorm"
)

// Seed catalog #13 is in the suite: unknown TitleIDs give ErrNotFound from Title and Episodes.
func TestRepositoryPassesTheReaderContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, Migrations())
		loadFixture(t, d, f)
		return NewRepository(d)
	})
}

// loadFixture stores a contract fixture the way the service would, so the real Reader reads it back.
func loadFixture(t *testing.T, d *db.DB, f contracttest.Fixture) {
	t.Helper()
	err := d.Write(context.Background(), func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&languageRow{}).Error; err != nil {
			return err
		}
		for position, lang := range f.GlobalLanguages {
			row := languageRow{Position: position, Language: string(lang.Language), Forced: lang.Forced, HearingImpaired: lang.HearingImpaired}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for _, title := range f.Titles {
			row := fixtureTitleRow(t, title)
			if override, ok := f.LanguageOverrides[title.ID]; ok {
				encoded, err := json.Marshal(override)
				if err != nil {
					return err
				}
				value := string(encoded)
				row.LanguagesOverride = &value
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for position, e := range f.Episodes {
			row := episodeRow{
				ID: string(e.ID), TitleID: string(e.Title), TmdbEpisodeID: int64(position),
				SeasonNumber: e.Season, EpisodeNumber: e.Number, AiredSeason: e.AiredSeason, AiredNumber: e.AiredNumber,
				Name: e.Name, AirDate: e.AirDate, RuntimeMinutes: e.RuntimeMinutes, Monitored: e.Monitored, Released: e.Released,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for _, img := range f.Images {
			row := imageRow{TitleID: string(img.Title), Kind: img.Kind, TmdbPath: img.Path, LocalPath: img.Path, FetchedAt: time.Now()}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("loading fixture: %v", err)
	}
}

func fixtureTitleRow(t *testing.T, title api.Title) titleRow {
	t.Helper()
	aliases, err := encodeAliases(title.Aliases)
	if err != nil {
		t.Fatal(err)
	}
	row := titleRow{
		ID: string(title.ID), Kind: string(title.Kind), TmdbID: int64(title.External.Tmdb),
		TvdbID: title.External.Tvdb, ImdbID: title.External.Imdb, ExternalIDSource: ExternalIDSourceTMDB,
		Name: title.Name, OriginalName: title.OriginalName, Aliases: aliases, Year: title.Year,
		RuntimeMinutes: title.RuntimeMinutes, OriginalLanguage: string(title.OriginalLanguage), SeriesStatus: title.SeriesStatus,
		ProfileID: string(title.Profile), Monitored: title.Monitored, MonitorFutureSeasons: title.MonitorFutureSeasons,
		EpisodeGroupID: title.EpisodeGroupID, Origin: string(title.Origin), AddedAt: time.Now(), MetadataRefreshedAt: time.Now(),
		ReleaseTheatricalAt: title.Dates.Theatrical, ReleaseDigitalAt: title.Dates.Digital, ReleasePhysicalAt: title.Dates.Physical,
		Released: title.Released, ReleasedAt: title.ReleasedAt,
	}
	if title.RequestedBy != nil {
		user := string(*title.RequestedBy)
		row.RequestedBy = &user
	}
	return row
}
