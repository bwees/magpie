package library

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/parse"
	"magpie/internal/platform/settings"

	"gorm.io/gorm"
)

const (
	hiddenPrefix = "."
	// scanStaleAfter lets a new scan start when one that claims to be running hasn't finished in this long,
	// as after a crash.
	scanStaleAfter = time.Hour
	// firstRevision is the revision of a scanned file's quality: nothing says it's a repack.
	firstRevision = 1
)

var (
	// tmdbHintPattern finds Jellyfin's [tmdbid-N] and Plex's {tmdb-N}, also with "=".
	tmdbHintPattern = regexp.MustCompile(`(?i)[\[{]tmdb(?:id)?[-=](\d+)[\]}]`)
	// idTagPattern finds any provider ID tag, to drop from the title.
	idTagPattern     = regexp.MustCompile(`(?i)\s*[\[{](?:tmdb|tvdb|imdb)(?:id)?[-=][^\]}]*[\]}]`)
	titleYearPattern = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)$`)
)

// ScanOptions are the Settings → Library choices for scans.
type ScanOptions struct {
	// RenameMatched moves a matched folder's videos and their sidecars to the fixed naming.
	RenameMatched bool `json:"renameMatched"`
}

// ScanState is the last scan's run.
type ScanState struct {
	Running    bool       `json:"running"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
	// Reported is how many folders the last run reported or relinked.
	Reported int    `json:"reported"`
	Error    string `json:"error"`
}

// ScanStatus is the last scan and the folders scans have found so far.
type ScanStatus struct {
	ScanState
	ScanCounts
	Options ScanOptions
}

var (
	scanOptionsKey = settings.NewKey(owner.Library, "scan.options", ScanOptions{})
	scanStateKey   = settings.NewKey(owner.Library, "scan.state", ScanState{})
)

// parseFolder reads a library folder's name: the TMDB tag, then "Title (Year)", else a release-style name.
func parseFolder(name string, kind domain.MediaKind) (title string, year *int, hint *domain.TmdbID) {
	if m := tmdbHintPattern.FindStringSubmatch(name); m != nil {
		if id, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			hint = ptr(domain.TmdbID(id))
		}
	}
	bare := strings.TrimSpace(idTagPattern.ReplaceAllString(name, ""))
	if m := titleYearPattern.FindStringSubmatch(bare); m != nil {
		y, _ := strconv.Atoi(m[2])
		return strings.TrimSpace(m[1]), &y, hint
	}
	if r, err := parse.ParseRelease(bare, kind); err == nil && r.Title != "" && r.Year > 0 {
		return r.Title, &r.Year, hint
	}
	return bare, nil, hint
}

func ptr[T any](v T) *T { return &v }

// StartScan queues a library scan unless one is running.
func (s *Service) StartScan(ctx context.Context) (ScanStatus, error) {
	now := s.now().UTC()
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		state, err := scanStateKey.GetTx(tx)
		if err != nil {
			return err
		}
		if state.Running && state.StartedAt != nil && now.Sub(*state.StartedAt) < scanStaleAfter {
			return nil
		}
		if err := scanStateKey.SetTx(tx, ScanState{Running: true, StartedAt: &now}); err != nil {
			return err
		}
		return s.deps.Jobs.EnqueueTx(tx, scanTask{})
	})
	if err != nil {
		return ScanStatus{}, err
	}
	return s.ScanStatus(ctx)
}

// ScanStatus is the last scan and the scan folders by status.
func (s *Service) ScanStatus(ctx context.Context) (ScanStatus, error) {
	state, err := scanStateKey.Get(ctx, s.deps.Store)
	if err != nil {
		return ScanStatus{}, err
	}
	counts, err := s.deps.Repo.scanCounts(ctx)
	if err != nil {
		return ScanStatus{}, err
	}
	options, err := scanOptionsKey.Get(ctx, s.deps.Store)
	return ScanStatus{ScanState: state, ScanCounts: counts, Options: options}, err
}

func (s *Service) SetScanOptions(ctx context.Context, options ScanOptions) error {
	return scanOptionsKey.Set(ctx, s.deps.Store, options)
}

// RunScan walks the root folders and reports each top-level folder that holds no media file yet. The outcome
// is recorded even when a shutdown stops it, so the status never stays running.
func (s *Service) RunScan(ctx context.Context) error {
	reported, scanErr := s.scanRoots(ctx)
	state, err := scanStateKey.Get(context.WithoutCancel(ctx), s.deps.Store)
	if err != nil {
		return errors.Join(scanErr, err)
	}
	finished := s.now().UTC()
	state.Running, state.FinishedAt, state.Reported, state.Error = false, &finished, reported, ""
	if scanErr != nil {
		state.Error = scanErr.Error()
	}
	return errors.Join(scanErr, scanStateKey.Set(context.WithoutCancel(ctx), s.deps.Store, state))
}

// scanRoots publishes FolderFound for new folders and folders still waiting for Catalog, and relinks matched
// folders that had no playable video before.
func (s *Service) scanRoots(ctx context.Context) (int, error) {
	roots, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return 0, err
	}
	known, err := s.deps.Repo.scanFolders(ctx)
	if err != nil {
		return 0, err
	}
	byPath := map[string]scanFolderRow{}
	for _, row := range known {
		byPath[row.Path] = row
	}
	held, err := s.foldersHoldingFiles(ctx, roots)
	if err != nil {
		return 0, err
	}
	var found []scanFolderRow
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		dirs, err := s.topFolders(root.Path)
		if err != nil {
			return 0, err
		}
		for _, dir := range dirs {
			row, seen := byPath[dir]
			if held[dir] || row.Status == ScanFolderLinked {
				continue
			}
			if !seen {
				row = scanFolderRow{Path: dir, RootFolderID: root.ID, Kind: root.Kind}
			}
			found = append(found, row)
		}
	}
	return len(found), s.reportFolders(ctx, found)
}

// reportFolders asks Catalog about each folder, or relinks one Catalog already matched.
func (s *Service) reportFolders(ctx context.Context, folders []scanFolderRow) error {
	now := s.now().UTC()
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		for _, row := range folders {
			matched := row.Status == ScanFolderIgnored && row.TitleID != nil
			row.Status, row.UpdatedAt = ScanFolderFound, now
			row.FoundAt = cmp.Or(row.FoundAt, now)
			if err := saveScanFolder(tx, row); err != nil {
				return err
			}
			if matched {
				if err := s.deps.Jobs.EnqueueTx(tx, linkTask{Folder: row.Path}); err != nil {
					return err
				}
				continue
			}
			title, year, hint := parseFolder(filepath.Base(row.Path), row.Kind)
			found := api.FolderFound{Path: row.Path, Kind: row.Kind, ParsedTitle: title, Year: year, TmdbHint: hint}
			if err := s.deps.Bus.Publish(tx, bus.EventOf(found)); err != nil {
				return err
			}
		}
		return nil
	})
}

// topFolders lists a root's folders, leaving out hidden ones such as the staging folder.
func (s *Service) topFolders(root string) ([]string, error) {
	var dirs []string
	err := s.deps.Files.Walk(root, func(path string, info FileInfo) error {
		switch {
		case path == root:
			return nil
		case !info.IsDir():
			return nil
		case !strings.HasPrefix(filepath.Base(path), hiddenPrefix):
			dirs = append(dirs, path)
		}
		return fs.SkipDir
	})
	return dirs, err
}

// foldersHoldingFiles are the top-level folders that already hold a media file.
func (s *Service) foldersHoldingFiles(ctx context.Context, roots []RootFolder) (map[string]bool, error) {
	paths, err := s.deps.Repo.mediaFilePaths(ctx)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, path := range paths {
		for _, root := range roots {
			if top, ok := topFolderOf(root.Path, path); ok {
				held[top] = true
			}
		}
	}
	return held, nil
}

// topFolderOf is the folder directly under root that holds path.
func topFolderOf(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	first, _, nested := strings.Cut(filepath.ToSlash(rel), pathSeparator)
	if !nested {
		return "", false
	}
	return filepath.Join(root, first), true
}

// QueueLink records the title Catalog matched a scan folder to and queues linking its files, unless they
// are linked already.
func (s *Service) QueueLink(ctx context.Context, tx *gorm.DB, matched catalogapi.ScanFolderMatched) error {
	row, err := scanFolderIn(tx, matched.Folder)
	switch {
	case errors.Is(err, errScanFolderMissing):
		if row, err = s.unreportedFolder(ctx, matched.Folder); err != nil {
			return err
		}
	case err != nil:
		return err
	case row.Status == ScanFolderLinked:
		return nil
	}
	row.TitleID, row.Status, row.UpdatedAt = &matched.Title, ScanFolderFound, s.now().UTC()
	if err := saveScanFolder(tx, row); err != nil {
		return err
	}
	return s.deps.Jobs.EnqueueTx(tx, linkTask{Folder: matched.Folder})
}

// unreportedFolder is a scan folder row for a folder no scan reported, found by the root folder holding it.
func (s *Service) unreportedFolder(ctx context.Context, folder string) (scanFolderRow, error) {
	roots, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return scanFolderRow{}, err
	}
	for _, root := range roots {
		if filepath.Dir(folder) == root.Path {
			now := s.now().UTC()
			return scanFolderRow{Path: folder, RootFolderID: root.ID, Kind: root.Kind, FoundAt: now}, nil
		}
	}
	return scanFolderRow{}, ErrRootFolderNotFound
}

// linkPlan is what linking a scan folder will store.
type linkPlan struct {
	folder scanFolderRow
	title  catalogapi.Title
	root   RootFolder
	videos []plannedVideo
}

// LinkFolder links a matched folder's playable videos to its title where they are, or at the fixed naming
// when the admin opted in, and publishes FileImported{LIBRARY_SCAN} for each. A folder with none is IGNORED.
func (s *Service) LinkFolder(ctx context.Context, folder string) error {
	row, err := scanFolderIn(s.deps.DB.Read(ctx), folder)
	if errors.Is(err, errScanFolderMissing) {
		return nil
	}
	if err != nil || row.Status == ScanFolderLinked || row.TitleID == nil {
		return err
	}
	title, err := s.deps.Catalog.Title(ctx, *row.TitleID)
	if errors.Is(err, catalogapi.ErrNotFound) {
		slog.WarnContext(ctx, "library: the title a scan folder matched is gone", "folder", folder, "title", *row.TitleID)
		return nil
	}
	if err != nil {
		return err
	}
	roots, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(roots, func(r RootFolder) bool { return r.ID == row.RootFolderID })
	if i < 0 {
		return nil
	}
	plan := linkPlan{folder: row, title: title, root: roots[i]}
	if err := s.planLink(ctx, &plan); err != nil {
		return err
	}
	if len(plan.videos) == 0 {
		return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
			return setScanFolderStatus(tx, folder, ScanFolderIgnored, s.now().UTC())
		})
	}
	options, err := scanOptionsKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	if options.RenameMatched {
		s.renameLinked(&plan)
	}
	placed, err := s.inspect(ctx, plan.videos)
	if err != nil {
		return err
	}
	// Renamed files are already in their new place, so their records must be written even during shutdown.
	if err := s.commitLink(context.WithoutCancel(ctx), plan, placed); err != nil {
		return err
	}
	targets := make([]string, 0, len(placed))
	for _, v := range placed {
		targets = append(targets, v.target)
	}
	s.refreshFolders(ctx, targets)
	return nil
}

// planLink picks the folder's videos to link: not samples, not linked already, the largest one for a movie,
// and the ones whose names give an episode for a series. Subtitles named after a video become its sidecars.
func (s *Service) planLink(ctx context.Context, plan *linkPlan) error {
	sources, err := s.walk(plan.folder.Path)
	if err != nil {
		return err
	}
	stored, err := s.deps.Repo.mediaFilePaths(ctx)
	if err != nil {
		return err
	}
	var index episodeIndex
	if plan.title.Kind == domain.MediaKindSeries {
		if index.episodes, err = s.deps.Catalog.Episodes(ctx, plan.title.ID); err != nil {
			return err
		}
	}
	covered := map[domain.EpisodeID]bool{}
	for _, source := range sources {
		if source.kind != FileVideo || slices.Contains(stored, source.abs) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		video, ok, err := s.scannedVideo(ctx, plan.title, source, index)
		if err != nil {
			return err
		}
		if !ok || slices.ContainsFunc(video.episodes, func(e domain.EpisodeRef) bool { return covered[e.Episode] }) {
			continue
		}
		for _, e := range video.episodes {
			covered[e.Episode] = true
		}
		video.sidecars = scannedSidecars(video.source.abs, sources)
		plan.videos = append(plan.videos, video)
	}
	if plan.title.Kind == domain.MediaKindMovie && len(plan.videos) > 1 {
		largest := slices.MaxFunc(plan.videos, func(a, b plannedVideo) int { return cmp.Compare(a.source.size, b.source.size) })
		plan.videos = []plannedVideo{largest}
	}
	return nil
}

// scannedVideo probes a video and maps it to its episodes; ok is false for a sample or an episode it can't tell.
func (s *Service) scannedVideo(ctx context.Context, title catalogapi.Title, source sourceFile, index episodeIndex) (plannedVideo, bool, error) {
	info, err := s.deps.Prober.Probe(ctx, source.abs)
	if err != nil && !errors.Is(err, ErrProbeFailed) {
		return plannedVideo{}, false, err
	}
	video := plannedVideo{source: source, info: info, target: source.abs, episodes: []domain.EpisodeRef{}}
	minutes := title.RuntimeMinutes
	if title.Kind == domain.MediaKindSeries {
		episodes, ok := index.fromName(source.abs, "")
		if !ok {
			slog.InfoContext(ctx, "library: left out a scanned video whose episode its name doesn't give", "path", source.abs)
			return plannedVideo{}, false, nil
		}
		video.episodes = refs(episodes)
		minutes = cmp.Or(episodes[0].RuntimeMinutes, minutes)
	}
	return video, !isSample(info.Duration, time.Duration(minutes)*time.Minute), nil
}

// scannedSidecars are the subtitles beside a video, named after it, with a language in their names.
func scannedSidecars(video string, sources []sourceFile) []plannedSidecar {
	prefix := stem(video) + sidecarJoin
	var sidecars []plannedSidecar
	for _, source := range sources {
		if source.kind != FileSubtitle || !strings.HasPrefix(source.abs, prefix) {
			continue
		}
		if tag, ok := parse.ParseSubtitleLanguage(source.abs); ok {
			sidecars = append(sidecars, plannedSidecar{source: source, sidecar: api.Sidecar{
				Path: source.abs, Language: tag.Language, Forced: tag.Forced, SDH: tag.SDH,
			}})
		}
	}
	return sidecars
}

// renameLinked moves the plan's videos and their sidecars to the fixed naming. A video that can't move stays
// where it is and is linked there.
func (s *Service) renameLinked(plan *linkPlan) {
	moves := make([]fileMove, len(plan.videos))
	for i, v := range plan.videos {
		ext := strings.ToLower(filepath.Ext(v.source.abs))
		moves[i] = fileMove{from: v.source.abs, to: moviePath(plan.root.Path, plan.title, ext)}
		if plan.title.Kind == domain.MediaKindSeries {
			moves[i].to = episodePath(plan.root.Path, plan.title, v.episodes, ext)
		}
	}
	for i, err := range s.moveFiles(moves) {
		if err != nil {
			slog.Warn("library: a scanned video stays where it is", "path", moves[i].from, "want", moves[i].to, "error", err)
			continue
		}
		v := &plan.videos[i]
		v.target = moves[i].to
		for j := range v.sidecars {
			sc := &v.sidecars[j].sidecar
			sc.Path = stem(v.target) + strings.TrimPrefix(sc.Path, stem(moves[i].from))
		}
		s.removeEmptyDirs(filepath.Dir(moves[i].from), filepath.Dir(plan.folder.Path))
	}
}

// inspect reads each video's size, ETag and OSHash where it now lies.
func (s *Service) inspect(ctx context.Context, videos []plannedVideo) ([]placedVideo, error) {
	placed := make([]placedVideo, 0, len(videos))
	for _, v := range videos {
		if err := ctx.Err(); err != nil {
			return nil, err
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

// commitLink stores the videos as LIBRARY_SCAN media files with no download, publishes FileImported for each,
// and marks the folder LINKED, all at once.
func (s *Service) commitLink(ctx context.Context, plan linkPlan, placed []placedVideo) error {
	now := s.now().UTC()
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		row, err := scanFolderIn(tx, plan.folder.Path)
		if err != nil || row.Status == ScanFolderLinked {
			return err
		}
		for _, v := range placed {
			file := storedFile{
				MediaFile: api.MediaFile{
					ID: domain.NewID[domain.MediaFileID](), Title: plan.title.ID, Episodes: v.episodes,
					RootFolder: plan.root.Path, Path: v.target, Size: v.size,
					Quality: scannedQuality(v.source.abs, plan.title.Kind, v.info), ReleaseName: scannedReleaseName(v.source.abs, plan.title.Kind),
					OSHash: v.osHash, MediaInfo: v.info, Origin: domain.OriginLibraryScan, ImportedAt: now,
				},
				RootFolderID: plan.root.ID,
				ETag:         v.etag,
			}
			if err := insertMediaFile(tx, file); err != nil {
				return err
			}
			if err := s.deps.Bus.Publish(tx, bus.EventOf(importedEvent(file, v.sidecars))); err != nil {
				return err
			}
		}
		return setScanFolderStatus(tx, plan.folder.Path, ScanFolderLinked, now)
	})
}

type widthQuality struct {
	minWidth int
	quality  parse.QualityID
}

// widthQualities guess a quality from the picture width when the name gives none, as Sonarr does: HDTV at
// that resolution. Widths allow for cropped and scope pictures.
var widthQualities = []widthQuality{
	{minWidth: 3200, quality: parse.QualityHDTV2160p},
	{minWidth: 1800, quality: parse.QualityHDTV1080p},
	{minWidth: 1200, quality: parse.QualityHDTV720p},
	{minWidth: 1, quality: parse.QualitySDTV},
}

// scannedQuality is the quality the file's name gives, else one guessed from its width.
func scannedQuality(path string, kind domain.MediaKind, info api.MediaInfo) domain.Quality {
	if r, err := parse.ParseFile(path, kind); err == nil && r.Quality.ID != int(parse.QualityUnknown) {
		return r.Quality
	}
	id := parse.QualityUnknown
	if i := slices.IndexFunc(widthQualities, func(w widthQuality) bool { return info.Width >= w.minWidth }); i >= 0 {
		id = widthQualities[i].quality
	}
	def, _ := parse.QualityByID(id)
	return def.Quality(firstRevision)
}

// scannedReleaseName keeps a file name that reads like a release, for subtitle matching; other names give "".
func scannedReleaseName(path string, kind domain.MediaKind) string {
	name := filepath.Base(stem(path))
	if r, err := parse.ParseRelease(name, kind); err == nil && r.Group != "" {
		return name
	}
	return ""
}
