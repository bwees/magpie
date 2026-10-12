package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/parse"

	"gorm.io/gorm"
)

// importProblem stops an import until someone steps in: the user maps the files, retries or removes it.
type importProblem struct {
	status ImportStatus
	reason string
	files  []ImportFile
}

func (p *importProblem) Error() string { return p.reason }

func manual(reason string, files []ImportFile) *importProblem {
	return &importProblem{status: ImportManual, reason: reason, files: files}
}

func failed(reason string, files []ImportFile) *importProblem {
	return &importProblem{status: ImportFailed, reason: reason, files: files}
}

// sourceFile is a video or subtitle of the download.
type sourceFile struct {
	// rel is the path as the download lists it.
	rel  string
	abs  string
	size int64
	kind FileKind
}

type plannedVideo struct {
	source   sourceFile
	episodes []domain.EpisodeRef
	info     api.MediaInfo
	target   string
	sidecars []plannedSidecar
}

type plannedSidecar struct {
	source  sourceFile
	sidecar api.Sidecar
}

type importPlan struct {
	imp    Import
	title  catalogapi.Title
	root   RootFolder
	videos []plannedVideo
	files  []ImportFile
}

// QueueImport records a finished download and queues its import, once per download.
func (s *Service) QueueImport(tx *gorm.DB, completed acquisitionapi.DownloadCompleted) error {
	inserted, err := insertImport(tx, completed, s.now().UTC())
	if err != nil || !inserted {
		return err
	}
	return s.deps.Jobs.EnqueueTx(tx, newImportTask(completed.Download))
}

// importDownload runs a pending import. It returns an *importProblem when the user has to step in, and
// any other error when trying again may help.
func (s *Service) importDownload(ctx context.Context, id domain.DownloadID) error {
	imp, err := s.deps.Repo.Import(ctx, id)
	if errors.Is(err, ErrImportNotFound) {
		return nil
	}
	if err != nil || imp.Status != ImportPending {
		return err
	}
	plan, err := s.plan(ctx, imp)
	if err != nil {
		return err
	}
	placed, err := s.place(ctx, plan)
	if err != nil {
		return err
	}
	// The files are in the library by now, so their records must be written even during shutdown.
	retired, err := s.commitImport(context.WithoutCancel(ctx), plan, placed)
	if err != nil {
		return err
	}
	s.retireReplaced(ctx, retired)
	targets := make([]string, 0, len(placed)+len(retired))
	for _, v := range placed {
		targets = append(targets, v.target)
	}
	s.refreshFolders(ctx, append(targets, retired...))
	return nil
}

func (s *Service) plan(ctx context.Context, imp Import) (importPlan, error) {
	title, err := s.deps.Catalog.Title(ctx, imp.Title)
	if errors.Is(err, catalogapi.ErrNotFound) {
		return importPlan{}, failed("The title is no longer in Magpie", nil)
	}
	if err != nil {
		return importPlan{}, err
	}
	root, err := s.deps.Repo.rootFolderFor(ctx, title.Kind)
	if errors.Is(err, ErrNoRootFolder) {
		return importPlan{}, failed(noRootFolderReason(title.Kind), nil)
	}
	if err != nil {
		return importPlan{}, err
	}
	sources, err := s.sourceFiles(ctx, imp.Payload)
	if err != nil {
		return importPlan{}, err
	}
	var index episodeIndex
	if title.Kind == domain.MediaKindSeries {
		if index.episodes, err = s.deps.Catalog.Episodes(ctx, title.ID); err != nil {
			return importPlan{}, err
		}
	}
	plan := importPlan{imp: imp, title: title, root: root}
	if err := s.probeVideos(ctx, &plan, sources, index); err != nil {
		return importPlan{}, err
	}
	if imp.Mapping != nil {
		err = plan.mapManually(index)
	} else {
		err = plan.mapAutomatically(index)
	}
	if err != nil {
		return importPlan{}, err
	}
	if err := s.refuseSplits(ctx, plan); err != nil {
		return importPlan{}, err
	}
	plan.assignSidecars(sources, index)
	return plan, nil
}

func noRootFolderReason(kind domain.MediaKind) string {
	if kind == domain.MediaKindSeries {
		return "Add a TV root folder in Settings → Library, then retry"
	}
	return "Add a Movies root folder in Settings → Library, then retry"
}

// sourceFiles lists the download's videos and subtitles. A single-file torrent's content path is the
// file itself; a folder's files are listed relative to it, or to its parent when they start with the
// folder's own name, as qBittorrent lists them. The content path is mapped on every run, so a retry
// picks up a corrected path mapping.
func (s *Service) sourceFiles(ctx context.Context, p acquisitionapi.DownloadCompleted) ([]sourceFile, error) {
	root, err := s.deps.Downloads.LocalPath(ctx, p.ContentPath)
	if err != nil {
		return nil, err
	}
	content, err := s.deps.Files.Stat(root)
	if err != nil {
		return nil, err
	}
	if !content.IsDir() {
		return keepMedia([]sourceFile{{rel: filepath.Base(root), abs: root, size: content.Size}}), nil
	}
	if len(p.Files) == 0 {
		return s.walk(root)
	}
	files := make([]sourceFile, 0, len(p.Files))
	for _, f := range p.Files {
		if fileKind(f.Path) == FileIgnored {
			continue
		}
		abs, err := s.resolve(root, f.Path)
		if err != nil {
			return nil, err
		}
		files = append(files, sourceFile{rel: f.Path, abs: abs, size: f.Size})
	}
	return keepMedia(files), nil
}

func (s *Service) resolve(content, rel string) (string, error) {
	candidates := []string{filepath.Join(content, rel)}
	if first, _, nested := strings.Cut(filepath.ToSlash(rel), "/"); nested && first == filepath.Base(content) {
		candidates = append(candidates, filepath.Join(filepath.Dir(content), rel))
	}
	var errs []error
	for _, candidate := range candidates {
		_, err := s.deps.Files.Stat(candidate)
		if err == nil {
			return candidate, nil
		}
		errs = append(errs, err)
	}
	return "", errors.Join(errs...)
}

func (s *Service) walk(root string) ([]sourceFile, error) {
	var files []sourceFile
	err := s.deps.Files.Walk(root, func(path string, info FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{rel: rel, abs: path, size: info.Size})
		return nil
	})
	return keepMedia(files), err
}

func keepMedia(files []sourceFile) []sourceFile {
	kept := make([]sourceFile, 0, len(files))
	for _, f := range files {
		f.kind = fileKind(f.rel)
		if f.kind != FileIgnored {
			kept = append(kept, f)
		}
	}
	return kept
}

// probeVideos reads every video's media info and lists the download's files for the imports queue.
// Samples are left out of the plan.
func (s *Service) probeVideos(ctx context.Context, plan *importPlan, sources []sourceFile, index episodeIndex) error {
	for _, source := range sources {
		file := ImportFile{Path: source.rel, Size: source.size, Kind: source.kind, Episodes: []domain.EpisodeRef{}}
		if source.kind != FileVideo {
			plan.files = append(plan.files, file)
			continue
		}
		info, err := s.deps.Prober.Probe(ctx, source.abs)
		if err != nil && !errors.Is(err, ErrProbeFailed) {
			return err
		}
		file.Sample = isSample(info.Duration, plan.expectedRuntime(source, index))
		plan.files = append(plan.files, file)
		plan.videos = append(plan.videos, plannedVideo{source: source, info: info})
	}
	return nil
}

// expectedRuntime is the runtime of the episode a video's name points at, else the title's.
func (p *importPlan) expectedRuntime(source sourceFile, index episodeIndex) time.Duration {
	minutes := p.title.RuntimeMinutes
	if episodes, ok := index.fromName(source.abs, p.imp.ReleaseName); ok && episodes[0].RuntimeMinutes > 0 {
		minutes = episodes[0].RuntimeMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func (p *importPlan) isSample(v plannedVideo) bool {
	i := slices.IndexFunc(p.files, func(f ImportFile) bool { return f.Path == v.source.rel })
	return i >= 0 && p.files[i].Sample
}

// mapAutomatically keeps the videos that aren't samples and maps them to episodes. Any video it can't
// map stops the whole import.
func (p *importPlan) mapAutomatically(index episodeIndex) error {
	p.videos = slices.DeleteFunc(p.videos, p.isSample)
	switch {
	case len(p.videos) == 0:
		return failed("The download has no video to import, or only samples", p.files)
	case p.title.Kind == domain.MediaKindMovie && len(p.videos) > 1:
		return manual("The download has several videos; pick the movie", p.files)
	case p.title.Kind == domain.MediaKindMovie:
		p.videos[0].episodes = []domain.EpisodeRef{}
		return p.checkMapped()
	case len(p.videos) == 1:
		return p.mapToPayload(index)
	}
	var unmapped []string
	for i := range p.videos {
		v := &p.videos[i]
		episodes, ok := index.fromName(v.source.abs, p.imp.ReleaseName)
		if !ok {
			unmapped = append(unmapped, filepath.Base(v.source.rel))
			continue
		}
		v.episodes = refs(episodes)
		p.noteEpisodes(*v)
	}
	if len(unmapped) > 0 {
		return manual(fmt.Sprintf("Couldn't tell which episode %s is", strings.Join(unmapped, ", ")), p.files)
	}
	return p.checkMapped()
}

// noteEpisodes records a video's episodes in the file list the imports queue shows.
func (p *importPlan) noteEpisodes(v plannedVideo) {
	i := slices.IndexFunc(p.files, func(f ImportFile) bool { return f.Path == v.source.rel })
	p.files[i].Episodes = v.episodes
}

// mapToPayload maps a download's only video to the episodes it was grabbed for.
func (p *importPlan) mapToPayload(index episodeIndex) error {
	episodes := make([]catalogapi.Episode, 0, len(p.imp.Payload.Episodes))
	for _, id := range p.imp.Payload.Episodes {
		if e, ok := index.byID(id); ok {
			episodes = append(episodes, e)
		}
	}
	if len(episodes) == 0 {
		return manual("The download's episodes are no longer in Magpie", p.files)
	}
	p.videos[0].episodes = refs(episodes)
	return p.checkMapped()
}

// mapManually imports the videos the user mapped, and only those.
func (p *importPlan) mapManually(index episodeIndex) error {
	mapped := make([]plannedVideo, 0, len(p.imp.Mapping))
	for _, m := range p.imp.Mapping {
		i := slices.IndexFunc(p.videos, func(v plannedVideo) bool { return v.source.rel == m.Path })
		if i < 0 {
			return manual(fmt.Sprintf("The download has no video %s", m.Path), p.files)
		}
		v := p.videos[i]
		v.episodes = []domain.EpisodeRef{}
		episodes := make([]catalogapi.Episode, 0, len(m.Episodes))
		for _, id := range m.Episodes {
			e, ok := index.byID(id)
			if !ok {
				return manual("The mapping names an episode that isn't in this series", p.files)
			}
			episodes = append(episodes, e)
		}
		if p.title.Kind == domain.MediaKindSeries {
			if len(episodes) == 0 {
				return manual(fmt.Sprintf("Pick the episodes of %s", m.Path), p.files)
			}
			v.episodes = refs(episodes)
		}
		mapped = append(mapped, v)
	}
	if len(mapped) == 0 || (p.title.Kind == domain.MediaKindMovie && len(mapped) > 1) {
		return manual("Pick one video to import", p.files)
	}
	p.videos = mapped
	return p.checkMapped()
}

// checkMapped refuses two videos for one episode and records the mapping for the imports queue.
func (p *importPlan) checkMapped() error {
	seen := map[domain.EpisodeID]bool{}
	for _, v := range p.videos {
		for _, ref := range v.episodes {
			if seen[ref.Episode] {
				return manual(fmt.Sprintf("Two videos are S%02dE%02d", ref.Season, ref.Number), p.files)
			}
			seen[ref.Episode] = true
		}
		p.noteEpisodes(v)
	}
	for i := range p.videos {
		v := &p.videos[i]
		ext := strings.ToLower(filepath.Ext(v.source.rel))
		if p.title.Kind == domain.MediaKindMovie {
			v.target = moviePath(p.root.Path, p.title, ext)
		} else {
			v.target = episodePath(p.root.Path, p.title, v.episodes, ext)
		}
	}
	return nil
}

// assignSidecars gives each subtitle with a language in its name to the video it belongs to: the one
// whose name it starts with, the only one, or the one covering the episodes its own name gives.
// Subtitles without a language or a video are dropped.
func (p *importPlan) assignSidecars(sources []sourceFile, index episodeIndex) {
	for _, source := range sources {
		if source.kind != FileSubtitle {
			continue
		}
		tag, ok := parse.ParseSubtitleLanguage(source.rel)
		if !ok {
			continue
		}
		v := p.ownerOf(source, index)
		if v == nil {
			continue
		}
		target := sidecarPath(v.target, tag, source.rel)
		if slices.ContainsFunc(v.sidecars, func(s plannedSidecar) bool { return s.sidecar.Path == target }) {
			continue
		}
		v.sidecars = append(v.sidecars, plannedSidecar{source: source, sidecar: api.Sidecar{
			Path: target, Language: tag.Language, Forced: tag.Forced, SDH: tag.SDH,
		}})
	}
}

func (p *importPlan) ownerOf(subtitle sourceFile, index episodeIndex) *plannedVideo {
	if len(p.videos) == 1 {
		return &p.videos[0]
	}
	stem := func(path string) string { return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) }
	for i := range p.videos {
		if strings.HasPrefix(stem(subtitle.rel), stem(p.videos[i].source.rel)) {
			return &p.videos[i]
		}
	}
	episodes, ok := index.fromName(subtitle.abs, "")
	if !ok {
		return nil
	}
	want := refs(episodes)
	for i := range p.videos {
		if slices.Equal(p.videos[i].episodes, want) {
			return &p.videos[i]
		}
	}
	return nil
}

const (
	sidecarSeparator = "."
	forcedSuffix     = "forced"
	sdhSuffix        = "sdh"
)

// sidecarPath is <video basename>.<lang>[.forced|.sdh].<ext>.
func sidecarPath(video string, tag parse.SubtitleLanguage, subtitle string) string {
	parts := []string{strings.TrimSuffix(video, filepath.Ext(video)), string(tag.Language)}
	switch {
	case tag.Forced:
		parts = append(parts, forcedSuffix)
	case tag.SDH:
		parts = append(parts, sdhSuffix)
	}
	return strings.Join(parts, sidecarSeparator) + strings.ToLower(filepath.Ext(subtitle))
}

// placedVideo is a video in its library place, with what Library records about it.
type placedVideo struct {
	plannedVideo
	size   int64
	etag   string
	osHash string
}

// place hardlinks, copies or moves every planned file into the library. Re-running it after a failure
// places the same files again.
func (s *Service) place(ctx context.Context, plan importPlan) ([]placedVideo, error) {
	mode := plan.imp.Payload.SeedingMode
	placed := make([]placedVideo, 0, len(plan.videos))
	for _, v := range plan.videos {
		if err := s.placeFile(ctx, v.source.abs, v.target, mode); err != nil {
			return nil, err
		}
		for _, sc := range v.sidecars {
			if err := s.placeFile(ctx, sc.source.abs, sc.sidecar.Path, mode); err != nil {
				return nil, err
			}
		}
		info, err := s.deps.Files.Stat(v.target)
		if err != nil {
			return nil, err
		}
		osHash, err := s.deps.Hasher.OSHash(v.target)
		if err != nil {
			return nil, err
		}
		placed = append(placed, placedVideo{plannedVideo: v, size: info.Size, etag: etagOf(info), osHash: osHash})
	}
	return placed, nil
}

const tempPrefix = ".magpie-import-"

// placeFile puts src at dst through a temporary name in dst's folder, so dst appears whole or not at all.
// Seeding downloads are hardlinked, or copied across devices; others are moved.
func (s *Service) placeFile(ctx context.Context, src, dst string, mode acquisitionapi.SeedingMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(dst) {
		return fmt.Errorf("library: target %q isn't an absolute path", dst)
	}
	dir := filepath.Dir(dst)
	if err := s.deps.Files.MkdirAll(dir); err != nil {
		return err
	}
	temp := filepath.Join(dir, tempPrefix+filepath.Base(dst))
	if err := s.deps.Files.Remove(temp); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := s.transfer(ctx, src, temp, mode); err != nil {
		return err
	}
	return s.deps.Files.Rename(temp, dst)
}

func (s *Service) transfer(ctx context.Context, src, dst string, mode acquisitionapi.SeedingMode) error {
	if mode == acquisitionapi.DontSeed {
		return s.deps.Files.Move(src, dst)
	}
	err := s.deps.Files.Hardlink(src, dst)
	if errors.Is(err, ErrCrossDevice) {
		return s.deps.Files.Copy(ctx, src, dst)
	}
	return err
}

// commitImport records the placed videos. A video replaces the files it shares an episode with, when the
// import covers all their episodes: their records go in the same transaction, each FileImported followed
// by FileDeleted{UPGRADED} for the files it replaced. It returns the replaced files still on disk; a
// replaced file the new one landed on is already gone.
func (s *Service) commitImport(ctx context.Context, plan importPlan, placed []placedVideo) ([]string, error) {
	now := s.now().UTC()
	files, err := encodeFiles(plan.files)
	if err != nil {
		return nil, err
	}
	payload := plan.imp.Payload
	download := payload.Download
	targets := make([]string, 0, len(placed))
	for _, v := range placed {
		targets = append(targets, v.target)
	}
	covered := coveredBy(plan.videos)
	var retired []string
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		retired = nil
		moved, err := moveImport(tx, download, []ImportStatus{ImportPending}, map[string]any{
			"status": ImportImported, "last_error": "", "files": files, "updated_at": now,
		})
		if err != nil || !moved {
			return err
		}
		existing, err := titleFiles(tx, plan.title.ID)
		if err != nil {
			return err
		}
		for _, v := range placed {
			file := storedFile{
				MediaFile: api.MediaFile{
					ID: domain.NewID[domain.MediaFileID](), Title: plan.title.ID, Episodes: v.episodes,
					RootFolder: plan.root.Path, Path: v.target, Size: v.size, Quality: payload.Quality,
					ReleaseName: payload.ReleaseName, OSHash: v.osHash, MediaInfo: v.info, Origin: domain.OriginDownload,
					Download: &download, InfoHash: payload.InfoHash, ImportedAt: now, Available: true,
				},
				RootFolderID: plan.root.ID,
				ETag:         v.etag,
			}
			if err := insertMediaFile(tx, file); err != nil {
				return err
			}
			replaced := replacedBy(existing, plan.title.Kind, v.plannedVideo, covered)
			imported := importedEvent(file, v.sidecars)
			if len(replaced) > 0 {
				imported.Replaces = &replaced[0].ID
			}
			if err := s.publishImported(tx, imported); err != nil {
				return err
			}
			for _, old := range replaced {
				if err := s.forgetFile(tx, old, api.DeleteUpgraded); err != nil {
					return err
				}
				if !slices.Contains(targets, old.Path) {
					retired = append(retired, old.Path)
				}
			}
			existing = slices.DeleteFunc(existing, func(f api.MediaFile) bool {
				return slices.ContainsFunc(replaced, func(old api.MediaFile) bool { return old.ID == f.ID })
			})
		}
		return nil
	})
	return retired, err
}

func importedEvent(f storedFile, sidecars []plannedSidecar) api.FileImported {
	placed := make([]api.Sidecar, 0, len(sidecars))
	for _, sc := range sidecars {
		placed = append(placed, sc.sidecar)
	}
	return api.FileImported{
		MediaFile: f.ID, Title: f.Title, Episodes: f.Episodes, Path: f.Path, Quality: f.Quality,
		ReleaseName: f.ReleaseName, OSHash: f.OSHash, Size: f.Size, MediaInfo: f.MediaInfo,
		Download: f.Download, Sidecars: placed, Origin: f.Origin,
	}
}

// failImport stops an import with a problem the user has to look at and tells Acquisition, once.
func (s *Service) failImport(ctx context.Context, id domain.DownloadID, problem *importProblem) error {
	changes := map[string]any{
		"status": problem.status, "last_error": problem.reason, "attempts": gorm.Expr("attempts + 1"),
		"updated_at": s.now().UTC(),
	}
	if problem.files != nil {
		files, err := encodeFiles(problem.files)
		if err != nil {
			return err
		}
		changes["files"] = files
	}
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		moved, err := moveImport(tx, id, []ImportStatus{ImportPending}, changes)
		if err != nil || !moved {
			return err
		}
		failed := api.ImportFailed{Download: id, Reason: problem.reason, Retryable: false, Manual: problem.status == ImportManual}
		return s.deps.Bus.Publish(tx, bus.EventOf(failed))
	})
}

// noteAttempt records a failed attempt that will be retried.
func (s *Service) noteAttempt(ctx context.Context, id domain.DownloadID, cause error) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		_, err := moveImport(tx, id, []ImportStatus{ImportPending}, map[string]any{
			"last_error": cause.Error(), "attempts": gorm.Expr("attempts + 1"), "updated_at": s.now().UTC(),
		})
		return err
	})
}
