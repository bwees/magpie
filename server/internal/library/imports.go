package library

import (
	"context"
	"errors"
	"fmt"
	"slices"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
)

var (
	ErrImportNotStopped = errors.New("library: only a failed or manual import can be retried or removed")
	ErrInvalidMapping   = errors.New("library: invalid file mapping")
)

// stoppedImports are the statuses of the imports queue in Activity.
var stoppedImports = []ImportStatus{ImportFailed, ImportManual}

// ImportView is an import with what the imports queue shows about its title.
type ImportView struct {
	Import
	TitleName string
	Kind      domain.MediaKind
	// Episodes are the series' episodes, for the file → episode picker; empty for a movie.
	Episodes []catalogapi.Episode
}

// Imports lists imports in the given statuses, most recently changed first.
func (s *Service) Imports(ctx context.Context, statuses []ImportStatus) ([]ImportView, error) {
	imports, err := s.deps.Repo.Imports(ctx, statuses)
	if err != nil {
		return nil, err
	}
	views := make([]ImportView, 0, len(imports))
	for _, imp := range imports {
		v, err := s.view(ctx, imp)
		if err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, nil
}

func (s *Service) view(ctx context.Context, imp Import) (ImportView, error) {
	v := ImportView{Import: imp, Episodes: []catalogapi.Episode{}}
	title, err := s.deps.Catalog.Title(ctx, imp.Title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return v, nil
	}
	if err != nil {
		return ImportView{}, err
	}
	v.TitleName, v.Kind = title.Name, title.Kind
	if title.Kind == domain.MediaKindSeries {
		if v.Episodes, err = s.deps.Catalog.Episodes(ctx, title.ID); err != nil {
			return ImportView{}, err
		}
	}
	return v, nil
}

// RetryImport runs a failed or manual import again with automatic mapping.
func (s *Service) RetryImport(ctx context.Context, id domain.DownloadID) (ImportView, error) {
	return s.restart(ctx, id, nil)
}

// ImportManually runs a failed or manual import with the user's file → episodes mapping. Videos left out
// of the mapping aren't imported.
func (s *Service) ImportManually(ctx context.Context, id domain.DownloadID, mapping []FileMapping) (ImportView, error) {
	imp, err := s.deps.Repo.Import(ctx, id)
	if err != nil {
		return ImportView{}, err
	}
	if err := s.checkMapping(ctx, imp, mapping); err != nil {
		return ImportView{}, err
	}
	return s.restart(ctx, id, mapping)
}

func (s *Service) checkMapping(ctx context.Context, imp Import, mapping []FileMapping) error {
	if len(mapping) == 0 {
		return fmt.Errorf("%w: map at least one video", ErrInvalidMapping)
	}
	title, err := s.deps.Catalog.Title(ctx, imp.Title)
	if err != nil {
		return err
	}
	if title.Kind == domain.MediaKindMovie && len(mapping) > 1 {
		return fmt.Errorf("%w: a movie takes one video", ErrInvalidMapping)
	}
	var episodes []catalogapi.Episode
	if title.Kind == domain.MediaKindSeries {
		if episodes, err = s.deps.Catalog.Episodes(ctx, title.ID); err != nil {
			return err
		}
	}
	for _, m := range mapping {
		if !slices.ContainsFunc(imp.Files, func(f ImportFile) bool { return f.Path == m.Path && f.Kind == FileVideo }) {
			return fmt.Errorf("%w: the download has no video %s", ErrInvalidMapping, m.Path)
		}
		if title.Kind == domain.MediaKindSeries && len(m.Episodes) == 0 {
			return fmt.Errorf("%w: pick the episodes of %s", ErrInvalidMapping, m.Path)
		}
		for _, ep := range m.Episodes {
			if !slices.ContainsFunc(episodes, func(e catalogapi.Episode) bool { return e.ID == ep }) {
				return fmt.Errorf("%w: episode %s isn't in this series", ErrInvalidMapping, ep)
			}
		}
	}
	return nil
}

func (s *Service) restart(ctx context.Context, id domain.DownloadID, mapping []FileMapping) (ImportView, error) {
	encoded, err := encodeMapping(mapping)
	if err != nil {
		return ImportView{}, err
	}
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		if _, err := importIn(tx, id); err != nil {
			return err
		}
		moved, err := moveImport(tx, id, stoppedImports, map[string]any{
			"status": ImportPending, "mapping": encoded, "last_error": "", "updated_at": s.now().UTC(),
		})
		if err != nil {
			return err
		}
		if !moved {
			return ErrImportNotStopped
		}
		return s.deps.Jobs.EnqueueTx(tx, newImportTask(id))
	})
	if err != nil {
		return ImportView{}, err
	}
	imp, err := s.deps.Repo.Import(ctx, id)
	if err != nil {
		return ImportView{}, err
	}
	return s.view(ctx, imp)
}

// RemoveImport drops a failed or manual import from the queue. Acquisition already blocklisted its
// release when the import failed.
func (s *Service) RemoveImport(ctx context.Context, id domain.DownloadID) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		if _, err := importIn(tx, id); err != nil {
			return err
		}
		result := tx.Where("download_id = ? AND status IN ?", id, stoppedImports).Delete(&importRow{})
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrImportNotStopped
		}
		return result.Error
	})
}
