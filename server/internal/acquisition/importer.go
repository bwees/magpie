package acquisition

import (
	"context"
	"fmt"
	"strings"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/platform/settings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type importSourceKind string

const (
	importSourceFormat    importSourceKind = "CUSTOM_FORMAT"
	importSourceProfile   importSourceKind = "QUALITY_PROFILE"
	importSourceBlocklist importSourceKind = "BLOCKLIST"
)

type importSourceRow struct {
	SourceKey domain.SourceKey `gorm:"primaryKey"`
	Kind      importSourceKind
	TargetID  string
}

func (importSourceRow) TableName() string { return "acquisition_import_sources" }

// Importer is the migration's way into Acquisition. Every call is one transaction and idempotent on the
// source keys. A format or profile whose name is already stored is merged into it: the stored one is kept,
// so a TRaSH-synced format stays TRaSH's.
type Importer struct {
	db  *db.DB
	now func() time.Time
}

var _ api.Importer = (*Importer)(nil)

func NewImporter(d *db.DB) *Importer {
	return &Importer{db: d, now: time.Now}
}

func (i *Importer) ImportCustomFormats(ctx context.Context, in []api.ImportCustomFormat) (map[domain.SourceKey]api.CustomFormatID, error) {
	out := map[domain.SourceKey]api.CustomFormatID{}
	err := i.db.Write(ctx, func(tx *gorm.DB) error {
		formats, err := loadFormats(tx)
		if err != nil {
			return err
		}
		byName := map[string]api.CustomFormatID{}
		for _, f := range formats {
			byName[nameKey(f.Name)] = f.ID
		}
		for _, imported := range in {
			id, err := importedTarget(tx, imported.Source, importSourceFormat)
			if err != nil {
				return err
			}
			if id == "" {
				if id, err = i.importFormat(tx, imported, byName); err != nil {
					return err
				}
			}
			out[imported.Source] = api.CustomFormatID(id)
		}
		return nil
	})
	return out, err
}

func (i *Importer) importFormat(tx *gorm.DB, in api.ImportCustomFormat, byName map[string]api.CustomFormatID) (string, error) {
	name := strings.TrimSpace(in.Name)
	id, merged := byName[nameKey(name)]
	if !merged {
		f := CustomFormat{ID: domain.NewID[api.CustomFormatID](), Name: name, IncludeWhenRenaming: in.IncludeWhenRenaming, Specs: in.Specs}
		if _, err := CompileFormat(f); err != nil {
			return "", err
		}
		if err := saveFormat(tx, f, i.now()); err != nil {
			return "", err
		}
		id = f.ID
		byName[nameKey(name)] = id
	}
	return string(id), saveImportSource(tx, in.Source, importSourceFormat, string(id))
}

func (i *Importer) ImportQualityProfiles(ctx context.Context, in []api.ImportQualityProfile) (map[domain.SourceKey]domain.ProfileID, error) {
	out := map[domain.SourceKey]domain.ProfileID{}
	err := i.db.Write(ctx, func(tx *gorm.DB) error {
		for _, imported := range in {
			id, err := importedTarget(tx, imported.Source, importSourceProfile)
			if err != nil {
				return err
			}
			if id == "" {
				if id, err = i.importProfile(tx, imported); err != nil {
					return err
				}
			}
			out[imported.Source] = domain.ProfileID(id)
		}
		return nil
	})
	return out, err
}

func (i *Importer) importProfile(tx *gorm.DB, in api.ImportQualityProfile) (string, error) {
	name := strings.TrimSpace(in.Name)
	var named []profileRow
	if err := tx.Where("name = ?", name).Limit(1).Find(&named).Error; err != nil {
		return "", err
	}
	if len(named) > 0 {
		id := string(named[0].ID)
		return id, saveImportSource(tx, in.Source, importSourceProfile, id)
	}
	p := api.Profile{
		ID:                    domain.NewID[domain.ProfileID](),
		Name:                  name,
		UpgradeAllowed:        in.UpgradeAllowed,
		Cutoff:                in.Cutoff,
		MinFormatScore:        in.MinFormatScore,
		CutoffFormatScore:     in.CutoffFormatScore,
		MinUpgradeFormatScore: in.MinUpgradeFormatScore,
		PropersMode:           api.PropersPreferAndUpgrade,
		Items:                 make([]api.ProfileItem, 0, len(in.Items)),
		FormatScores:          make([]api.FormatScore, 0, len(in.FormatScores)),
	}
	for _, item := range in.Items {
		profileItem := api.ProfileItem{Qualities: item.Qualities, Allowed: item.Allowed, Sizes: item.Sizes}
		if item.Group != "" {
			profileItem.Group = &api.QualityGroup{Name: item.Group}
		}
		p.Items = append(p.Items, profileItem)
	}
	for format, score := range in.FormatScores {
		p.FormatScores = append(p.FormatScores, api.FormatScore{Format: format, Score: score})
	}
	if err := validateProfile(p); err != nil {
		return "", fmt.Errorf("profile %q: %w", name, err)
	}
	formats, err := loadFormats(tx)
	if err != nil {
		return "", err
	}
	if err := applyOverrides(&p, formatsByID(formats)); err != nil {
		return "", fmt.Errorf("profile %q: %w", name, err)
	}
	if err := saveProfile(tx, p, i.now()); err != nil {
		return "", err
	}
	return string(p.ID), saveImportSource(tx, in.Source, importSourceProfile, string(p.ID))
}

// ImportDelayProfiles sets the default delay to the shortest untagged delay. Magpie has no tag-scoped
// delays, so tagged profiles change nothing.
func (i *Importer) ImportDelayProfiles(ctx context.Context, in []api.ImportDelayProfile) error {
	var shortest *time.Duration
	for _, p := range in {
		if len(p.Tags) == 0 && (shortest == nil || p.Delay < *shortest) {
			delay := min(max(p.Delay, MinDefaultDelay), MaxDefaultDelay)
			shortest = &delay
		}
	}
	if shortest == nil {
		return nil
	}
	return i.db.Write(ctx, func(tx *gorm.DB) error { return DefaultDelayKey.SetTx(tx, settings.Duration(*shortest)) })
}

// ImportBlocklist stores each row once as a permanent MIGRATED entry.
func (i *Importer) ImportBlocklist(ctx context.Context, in []api.ImportBlockedRelease) error {
	return i.db.Write(ctx, func(tx *gorm.DB) error {
		for _, row := range in {
			id, err := importedTarget(tx, row.Source, importSourceBlocklist)
			if err != nil {
				return err
			}
			if id != "" {
				continue
			}
			entry, err := insertBlocklistEntry(tx, api.BlocklistEntry{
				Title: row.Title, InfoHash: strings.ToLower(row.InfoHash), ReleaseName: row.ReleaseName,
				Indexer: row.Indexer, Reason: api.BlocklistMigrated, At: row.At.UTC(),
			})
			if err != nil {
				return err
			}
			if err := saveImportSource(tx, row.Source, importSourceBlocklist, entry.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func nameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// importedTarget is what a source was imported as, or "" when it wasn't.
func importedTarget(tx *gorm.DB, source domain.SourceKey, kind importSourceKind) (string, error) {
	var rows []importSourceRow
	err := tx.Where("source_key = ? AND kind = ?", source, kind).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[0].TargetID, nil
}

func saveImportSource(tx *gorm.DB, source domain.SourceKey, kind importSourceKind, target string) error {
	row := importSourceRow{SourceKey: source, Kind: kind, TargetID: target}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}
