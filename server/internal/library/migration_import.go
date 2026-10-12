package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type importSourceRow struct {
	SourceKey domain.SourceKey   `gorm:"primaryKey"`
	MediaFile domain.MediaFileID `gorm:"column:media_file_id"`
}

func (importSourceRow) TableName() string { return "library_import_sources" }

var _ api.Importer = (*Service)(nil)

// ImportRootFolders implements api.Importer. Each folder must exist and be writable; one that is already a
// root folder is kept. The hardlink check is logged, since a migrated library may live elsewhere.
func (s *Service) ImportRootFolders(ctx context.Context, in []api.ImportRootFolder) error {
	folders, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return err
	}
	for _, root := range in {
		path := filepath.Clean(root.Path)
		if slices.ContainsFunc(folders, func(f RootFolder) bool { return f.Path == path }) {
			continue
		}
		folder, err := s.AddRootFolder(ctx, root.Kind, path)
		if errors.Is(err, ErrRootFolderExists) {
			continue
		}
		if err != nil {
			return fmt.Errorf("root folder %s: %w", path, err)
		}
		folders = append(folders, folder.RootFolder)
		if folder.Link != LinkHardlink {
			slog.InfoContext(ctx, "library: an imported root folder can't take hardlinked imports", "path", path, "link", folder.Link)
		}
	}
	return nil
}

// migratedVideo is an imported file as found on disk, inspected before the write transaction.
type migratedVideo struct {
	in       api.ImportMediaFile
	root     RootFolder
	episodes []domain.EpisodeRef
	placed   placedVideo
}

// ImportMediaFiles implements api.Importer. Files are linked where they are, without moving or renaming
// them, and each new one publishes FileImported{Origin: MIGRATION}. A file that's gone from disk or lies
// outside every root folder is left out; a path Library already holds maps to that file.
func (s *Service) ImportMediaFiles(ctx context.Context, in []api.ImportMediaFile) (map[domain.SourceKey]domain.MediaFileID, error) {
	roots, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return nil, err
	}
	known, err := s.knownImports(s.deps.DB.Read(ctx), in)
	if err != nil {
		return nil, err
	}
	sidecarsByDir := map[string][]sourceFile{}
	var videos []migratedVideo
	for _, file := range in {
		if _, ok := known[file.Source]; ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		video, ok, err := s.inspectMigrated(ctx, file, roots, sidecarsByDir)
		if err != nil {
			return nil, err
		}
		if ok {
			videos = append(videos, video)
		}
	}

	var out map[domain.SourceKey]domain.MediaFileID
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		var err error
		if out, err = s.knownImports(tx, in); err != nil {
			return err
		}
		for _, v := range videos {
			if _, ok := out[v.in.Source]; ok {
				continue
			}
			if out[v.in.Source], err = s.linkMigrated(tx, v); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// knownImports maps the sources already imported, and those whose path Library already holds, to their files.
func (s *Service) knownImports(conn *gorm.DB, in []api.ImportMediaFile) (map[domain.SourceKey]domain.MediaFileID, error) {
	sources := make([]domain.SourceKey, 0, len(in))
	paths := make([]string, 0, len(in))
	for _, file := range in {
		sources = append(sources, file.Source)
		paths = append(paths, filepath.Clean(file.Path))
	}
	var imported []importSourceRow
	if err := conn.Where("source_key IN ?", sources).Find(&imported).Error; err != nil {
		return nil, err
	}
	var held []mediaFileRow
	if err := conn.Select("id", "path").Where("path IN ?", paths).Find(&held).Error; err != nil {
		return nil, err
	}
	existing := map[domain.MediaFileID]bool{}
	byPath := map[string]domain.MediaFileID{}
	for _, row := range held {
		existing[row.ID] = true
		byPath[row.Path] = row.ID
	}
	var stillThere []domain.MediaFileID
	ids := make([]domain.MediaFileID, 0, len(imported))
	for _, row := range imported {
		ids = append(ids, row.MediaFile)
	}
	if err := conn.Model(&mediaFileRow{}).Where("id IN ?", ids).Pluck("id", &stillThere).Error; err != nil {
		return nil, err
	}
	for _, id := range stillThere {
		existing[id] = true
	}
	known := map[domain.SourceKey]domain.MediaFileID{}
	for _, row := range imported {
		if existing[row.MediaFile] {
			known[row.SourceKey] = row.MediaFile
		}
	}
	for _, file := range in {
		if id, ok := byPath[filepath.Clean(file.Path)]; ok {
			if _, done := known[file.Source]; !done {
				known[file.Source] = id
			}
		}
	}
	return known, nil
}

// inspectMigrated stats, hashes and probes a file and finds its episodes' numbering and its sidecars.
func (s *Service) inspectMigrated(ctx context.Context, file api.ImportMediaFile, roots []RootFolder, sidecarsByDir map[string][]sourceFile) (migratedVideo, bool, error) {
	path := filepath.Clean(file.Path)
	i := slices.IndexFunc(roots, func(r RootFolder) bool { return isWithin(r.Path, path) })
	if i < 0 {
		slog.WarnContext(ctx, "library: a migrated file lies outside every root folder", "path", path)
		return migratedVideo{}, false, nil
	}
	info, err := s.deps.Files.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "library: a migrated file is gone from disk", "path", path)
		return migratedVideo{}, false, nil
	}
	if err != nil {
		return migratedVideo{}, false, err
	}
	osHash, err := s.deps.Hasher.OSHash(path)
	if err != nil {
		return migratedVideo{}, false, err
	}
	media, err := s.deps.Prober.Probe(ctx, path)
	if err != nil && !errors.Is(err, ErrProbeFailed) {
		return migratedVideo{}, false, err
	}
	episodes, err := s.episodeRefs(ctx, file.Episodes)
	if err != nil {
		return migratedVideo{}, false, err
	}
	dir := filepath.Dir(path)
	sources, listed := sidecarsByDir[dir]
	if !listed {
		if sources, err = s.walk(dir); err != nil {
			return migratedVideo{}, false, err
		}
		sidecarsByDir[dir] = sources
	}
	planned := plannedVideo{
		source: sourceFile{abs: path, size: info.Size, kind: FileVideo}, episodes: episodes, info: media, target: path,
		sidecars: scannedSidecars(path, sources),
	}
	return migratedVideo{
		in: file, root: roots[i], episodes: episodes,
		placed: placedVideo{plannedVideo: planned, size: info.Size, etag: etagOf(info), osHash: osHash},
	}, true, nil
}

// episodeRefs adds each episode's current numbering. An episode Catalog doesn't know is left out.
func (s *Service) episodeRefs(ctx context.Context, ids []domain.EpisodeID) ([]domain.EpisodeRef, error) {
	refs := make([]domain.EpisodeRef, 0, len(ids))
	for _, id := range ids {
		e, err := s.deps.Catalog.Episode(ctx, id)
		if errors.Is(err, catalogapi.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		refs = append(refs, domain.EpisodeRef{Episode: e.ID, Season: e.Season, Number: e.Number})
	}
	return refs, nil
}

// linkMigrated stores one migrated file and publishes its FileImported.
func (s *Service) linkMigrated(tx *gorm.DB, v migratedVideo) (domain.MediaFileID, error) {
	importedAt := s.now().UTC()
	if !v.in.AddedAt.IsZero() {
		importedAt = v.in.AddedAt.UTC()
	}
	file := storedFile{
		MediaFile: api.MediaFile{
			ID: domain.NewID[domain.MediaFileID](), Title: v.in.Title, Episodes: v.episodes,
			RootFolder: v.root.Path, Path: v.placed.target, Size: v.placed.size, Quality: v.in.Quality,
			ReleaseName: strings.TrimSpace(v.in.ReleaseName), OSHash: v.placed.osHash, MediaInfo: v.placed.info,
			Origin: domain.OriginMigration, ImportedAt: importedAt,
		},
		RootFolderID: v.root.ID,
		ETag:         v.placed.etag,
	}
	if err := insertMediaFile(tx, file); err != nil {
		return "", err
	}
	source := importSourceRow{SourceKey: v.in.Source, MediaFile: file.ID}
	if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&source).Error; err != nil {
		return "", err
	}
	return file.ID, s.deps.Bus.Publish(tx, bus.EventOf(importedEvent(file, v.placed.sidecars)))
}
