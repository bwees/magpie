package activity

import (
	"context"
	"errors"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/db"

	"gorm.io/gorm"
)

const migratedSuffix = " (Sonarr/Radarr)"

// importedKinds is how each kind of history row reads: the event type it stands for, its severity and text.
var importedKinds = map[api.ImportKind]struct {
	eventType, severity, text string
}{
	api.ImportGrabbed:        {eventType: acquisitionapi.TypeReleaseGrabbed, severity: api.SeverityInfo, text: "Grabbed"},
	api.ImportImported:       {eventType: libraryapi.TypeFileImported, severity: api.SeverityInfo, text: "Imported"},
	api.ImportDownloadFailed: {eventType: acquisitionapi.TypeDownloadFailed, severity: api.SeverityError, text: "Download failed"},
	api.ImportFileDeleted:    {eventType: libraryapi.TypeFileDeleted, severity: api.SeverityInfo, text: "File deleted"},
}

// Importer writes Sonarr/Radarr history onto timelines and the global history with the rows' own
// timestamps. It touches neither the pipeline, attention items nor notifications.
type Importer struct {
	repo    *Repository
	db      *db.DB
	catalog catalogapi.Reader
	service *Service
}

var _ api.Importer = (*Importer)(nil)

func NewImporter(p ServiceParams, s *Service) *Importer {
	return &Importer{repo: p.Repository, db: p.DB, catalog: p.Catalog, service: s}
}

// ImportTimeline implements api.Importer. A row is stored under its source key, so importing it again
// changes nothing. Rows of unknown kinds and of removed titles are left out.
func (i *Importer) ImportTimeline(ctx context.Context, in []api.ImportTimelineEvent) error {
	names := map[domain.TitleID]string{}
	for _, row := range in {
		if _, seen := names[row.Title]; seen {
			continue
		}
		title, err := i.catalog.Title(ctx, row.Title)
		if err != nil && !errors.Is(err, catalogapi.ErrNotFound) {
			return err
		}
		names[row.Title] = title.Name
	}
	return i.db.Write(ctx, func(tx *gorm.DB) error {
		touched := map[domain.TitleID]bool{}
		for _, row := range in {
			kind, ok := importedKinds[row.Kind]
			if !ok {
				continue
			}
			if removed, err := i.repo.removed(tx, row.Title); err != nil || removed {
				if err != nil {
					return err
				}
				continue
			}
			title := row.Title
			entry := api.TimelineEntry{
				EventID: string(row.Source), Type: kind.eventType, Severity: kind.severity, Title: &title,
				TitleName: names[row.Title], Episodes: row.Episodes, Text: kind.text + migratedSuffix,
				Detail: importedDetail(row), At: row.At,
			}
			timeline, err := newTimelineRow(title, entry)
			if err != nil {
				return err
			}
			if err := i.repo.addTimeline(tx, timeline); err != nil {
				return err
			}
			history, err := newHistoryRow(entry)
			if err != nil {
				return err
			}
			if err := i.repo.addHistory(tx, history); err != nil {
				return err
			}
			touched[title] = true
		}
		for title := range touched {
			if err := i.service.announce(tx, api.ProjectionTimeline, &title); err != nil {
				return err
			}
		}
		if len(touched) == 0 {
			return nil
		}
		return i.service.announce(tx, api.ProjectionHistory, nil)
	})
}

// importedDetail is the release name, followed by the reason a failed or deleted row has.
func importedDetail(row api.ImportTimelineEvent) string {
	switch {
	case row.Reason == "":
		return row.ReleaseName
	case row.ReleaseName == "":
		return row.Reason
	}
	return row.ReleaseName + missingSeparator + row.Reason
}
