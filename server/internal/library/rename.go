package library

import (
	"cmp"
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

	"gorm.io/gorm"
)

const (
	renameTempPrefix = ".magpie-rename-"
	sidecarJoin      = "."
	pathSeparator    = "/"
)

var errTargetExists = errors.New("library: another file already has that name")

// fileMove renames a video, and every file sharing its basename, to a new video path.
type fileMove struct {
	from, to string
}

// pathMove is one file of a fileMove with the temporary name it passes through.
type pathMove struct {
	from, to, temp string
}

func stem(path string) string { return strings.TrimSuffix(path, filepath.Ext(path)) }

// pathsOf lists the video and its sidecars with where each goes; a sidecar keeps what follows the video's basename.
func (s *Service) pathsOf(m fileMove) ([]pathMove, error) {
	sidecars, err := s.sidecarsOf(m.from)
	if err != nil {
		return nil, err
	}
	paths := []pathMove{{from: m.from, to: m.to}}
	for _, sidecar := range sidecars {
		paths = append(paths, pathMove{from: sidecar, to: stem(m.to) + strings.TrimPrefix(sidecar, stem(m.from))})
	}
	for i := range paths {
		paths[i].temp = filepath.Join(filepath.Dir(paths[i].to), renameTempPrefix+filepath.Base(paths[i].to))
	}
	return paths, nil
}

// sidecarsOf lists the files beside a video whose names are its basename, a dot and more; other videos are left out.
func (s *Service) sidecarsOf(video string) ([]string, error) {
	dir := filepath.Dir(video)
	prefix := filepath.Base(stem(video)) + sidecarJoin
	var sidecars []string
	err := s.deps.Files.Walk(dir, func(path string, info FileInfo) error {
		switch name := filepath.Base(path); {
		case path == dir:
		case info.IsDir():
			return fs.SkipDir
		case path != video && strings.HasPrefix(name, prefix) && fileKind(name) != FileVideo:
			sidecars = append(sidecars, path)
		}
		return nil
	})
	return sidecars, err
}

// moveFiles renames each video with its sidecars and returns each move's error. Every file passes through a
// temporary name first, so videos can trade names. A move that fails is put back, and the others go ahead.
func (s *Service) moveFiles(moves []fileMove) []error {
	errs := make([]error, len(moves))
	staged := make([][]pathMove, len(moves))
	sources := make(map[string]bool, len(moves))
	for _, m := range moves {
		sources[m.from] = true
	}
	for i, m := range moves {
		if m.from == m.to || (!sources[m.to] && s.alreadyMoved(m)) {
			continue
		}
		staged[i], errs[i] = s.pathsOf(m)
		if errs[i] == nil {
			errs[i] = s.stage(staged[i])
		}
	}
	for i, paths := range staged {
		if errs[i] == nil {
			errs[i] = s.unstage(paths)
		}
	}
	return errs
}

// stage moves every path to its temporary name, or none of them.
func (s *Service) stage(paths []pathMove) error {
	for i, p := range paths {
		err := s.deps.Files.MkdirAll(filepath.Dir(p.to))
		if err == nil {
			err = s.deps.Files.Rename(p.from, p.temp)
		}
		if err != nil {
			s.putBack(paths[:i], func(p pathMove) string { return p.temp })
			return err
		}
	}
	return nil
}

// unstage gives every path its final name without overwriting another file; if one can't, all go back.
func (s *Service) unstage(paths []pathMove) error {
	for i, p := range paths {
		err := s.vacant(p.to)
		if err == nil {
			err = s.deps.Files.Rename(p.temp, p.to)
		}
		if err != nil {
			s.putBack(paths[:i], func(p pathMove) string { return p.to })
			s.putBack(paths[i:], func(p pathMove) string { return p.temp })
			return err
		}
	}
	return nil
}

func (s *Service) vacant(path string) error {
	_, err := s.deps.Files.Stat(path)
	switch {
	case err == nil:
		return fmt.Errorf("%w: %s", errTargetExists, path)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	}
	return err
}

func (s *Service) putBack(paths []pathMove, current func(pathMove) string) {
	for _, p := range paths {
		if err := s.deps.Files.Rename(current(p), p.from); err != nil {
			slog.Error("library: a file couldn't be put back after a failed rename", "path", current(p), "want", p.from, "error", err)
		}
	}
}

// removeEmptyDirs removes dir and its parents up to, not including, stop, while they are empty.
func (s *Service) removeEmptyDirs(dir, stop string) {
	for ; dir != stop && isWithin(stop, dir); dir = filepath.Dir(dir) {
		if s.deps.Files.Remove(dir) != nil {
			return
		}
	}
}

func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// renumbered is a media file whose episodes moved, with its new numbers and name.
type renumbered struct {
	file     storedFile
	episodes []domain.EpisodeRef
	target   string
}

// RenameForGroup renames the files of the episodes an episode-group change moved, with their sidecars, to the
// new numbers. A file that can't be renamed keeps its name and numbers and is flagged.
func (s *Service) RenameForGroup(ctx context.Context, changed catalogapi.EpisodeGroupChanged) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	title, err := s.deps.Catalog.Title(ctx, changed.Title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	files, err := s.deps.Repo.files(ctx, "title_id = ?", changed.Title)
	if err != nil {
		return err
	}
	moves := map[domain.EpisodeID]catalogapi.EpisodeMove{}
	for _, m := range changed.Moves {
		moves[m.Episode] = m
	}
	var plans []renumbered
	for _, f := range files {
		if episodes, moved := renumber(f.Episodes, moves); moved {
			plans = append(plans, renumbered{file: f, episodes: episodes, target: renamedEpisodePath(f, title, episodes)})
		}
	}
	if len(plans) == 0 {
		return nil
	}
	// Once files have moved, their records must follow even during shutdown.
	errs := s.moveFiles(movesOf(plans))
	return s.commitRenames(context.WithoutCancel(ctx), plans, errs)
}

// renumber applies the moves to a file's episodes; moved is false when none of them moved.
func renumber(episodes []domain.EpisodeRef, moves map[domain.EpisodeID]catalogapi.EpisodeMove) (out []domain.EpisodeRef, moved bool) {
	out = slices.Clone(episodes)
	for i, ref := range out {
		m, ok := moves[ref.Episode]
		if ok && ref.Season == m.FromSeason && ref.Number == m.FromNo {
			out[i].Season, out[i].Number = m.ToSeason, m.ToNo
			moved = true
		}
	}
	slices.SortFunc(out, func(a, b domain.EpisodeRef) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number))
	})
	return out, moved
}

// renamedEpisodePath keeps the file in its show's folder under the root and renames it within, by the
// fixed naming. A file lying loose in the root moves into the title's conventional folder.
func renamedEpisodePath(f storedFile, t catalogapi.Title, episodes []domain.EpisodeRef) string {
	show := titleFolder(f.RootFolder, t)
	if rel, err := filepath.Rel(f.RootFolder, f.Path); err == nil && strings.Contains(filepath.ToSlash(rel), pathSeparator) && !strings.HasPrefix(rel, "..") {
		show = filepath.Join(f.RootFolder, strings.Split(filepath.ToSlash(rel), pathSeparator)[0])
	}
	return filepath.Join(show, episodeInShow(t, episodes, filepath.Ext(f.Path)))
}

func movesOf(plans []renumbered) []fileMove {
	moves := make([]fileMove, len(plans))
	for i, p := range plans {
		moves[i] = fileMove{from: p.file.Path, to: p.target}
	}
	return moves
}

// alreadyMoved recognises a rename an interrupted run already made: the old path is gone and the new one is there.
func (s *Service) alreadyMoved(m fileMove) bool {
	_, fromErr := s.deps.Files.Stat(m.from)
	_, toErr := s.deps.Files.Stat(m.to)
	return errors.Is(fromErr, fs.ErrNotExist) && toErr == nil
}

// commitRenames records the files that moved and flags the ones that didn't, then asks Jellyfin to rescan
// the folders on both sides.
func (s *Service) commitRenames(ctx context.Context, plans []renumbered, errs []error) error {
	var touched []string
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		touched = touched[:0]
		for i, p := range plans {
			if errs[i] != nil {
				slog.WarnContext(ctx, "library: a file kept its name after an episode-group change", "path", p.file.Path, "want", p.target, "error", errs[i])
				if err := flagRenameProblem(tx, p.file.ID, renameProblem(p.target, errs[i])); err != nil {
					return err
				}
				continue
			}
			etag := p.file.ETag
			if info, err := s.deps.Files.Stat(p.target); err == nil {
				etag = etagOf(info)
			}
			if err := moveMediaFile(tx, p.file.ID, p.target, etag, p.episodes); err != nil {
				return err
			}
			touched = append(touched, p.file.Path, p.target)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i, p := range plans {
		if errs[i] == nil {
			s.removeEmptyDirs(filepath.Dir(p.file.Path), p.file.RootFolder)
		}
	}
	s.refreshFolders(ctx, touched)
	return nil
}

func renameProblem(target string, err error) string {
	return fmt.Sprintf("Couldn't rename it to %s: %v", filepath.Base(target), err)
}

// RenameProblems are the reasons a title's files kept their old names after a renumbering, by file.
func (s *Service) RenameProblems(ctx context.Context, title domain.TitleID) (map[domain.MediaFileID]string, error) {
	return s.deps.Repo.RenameProblems(ctx, title)
}
