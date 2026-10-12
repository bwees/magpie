# Library

Owns the media files on disk. Turns a finished download into a correctly named library file, swaps in transcoded files, and asks Jellyfin to rescan what changed.

- **Depends on (Readers):** `catalog.Reader`
- **Publishes:** `FileImported`, `ImportFailed`, `ImportRemoved`, `FileTranscoded`, `FileDeleted`, `FileRenamed`, `ItemAvailable`, `FolderFound`, `TranscodeSwapRejected`
- **Consumes:** `acquisition.DownloadCompleted`, `transcode.TranscodeVerified`, `catalog.EpisodeGroupChanged`, `catalog.TitleRemoved`, `catalog.ScanFolderMatched`
- **Importer entrypoint:** `api.Importer` (migration job only): link existing files with their original release names.

Decisions: [04](../../../.scratch/v1-spec/issues/04-qbittorrent-and-import.md), [08](../../../.scratch/v1-spec/issues/08-requests-and-jellyfin.md), [11](../../../.scratch/v1-spec/issues/11-blocked-file-policy.md), [13](../../../.scratch/v1-spec/issues/13-transcode-policy.md), [14](../../../.scratch/v1-spec/issues/14-agent-protocol.md), [18](../../../.scratch/v1-spec/issues/18-import-and-library.md), [31](../../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md).

## Owns / Must not

| Owns | Must not |
|---|---|
| Media file records and the video files under root folders | Choose, grab, blocklist or remove torrents (Acquisition) |
| Import attempts, manual-import queue | Decide cutoff or quality acceptability (Acquisition) |
| Root folders, naming, hardlink/copy/move | Talk to TMDB or decide what a folder is (Catalog) |
| Staging area for transcoded files, atomic swap | Run or schedule GPU work, serve bytes to agents (Transcode) |
| Jellyfin refresh, availability | Write or delete subtitle sidecars, except moving them on rename (Subtitles) |
| Library scan (walking folders) | Create titles (Catalog does, on `FolderFound`) |

## Data model

| Table | Key columns |
|---|---|
| `root_folders` | id, kind (MOVIE/SERIES), path, device_id |
| `media_files` | id, title_id, root_folder_id, path, size, quality (json), release_name, os_hash, media_info (json: streams, duration, HDR), etag, origin (DOWNLOAD/LIBRARY_SCAN/MIGRATION), imported_at, download_id?, info_hash?, transcoded, transcoded_at, original_size, available_at? |
| `media_file_episodes` | media_file_id, episode_id, season, number (many-to-many: multi-episode files; the numbers as imported) |
| `imports` | download_id, title_id, release_name, status (PENDING/IMPORTED/FAILED/MANUAL/REMOVED), attempts, last_error, payload (the `DownloadCompleted`), files (json: the last attempt's videos and subtitles with their sample flag and matched episodes), mapping? (json: the user's file → episodes choice), created_at, updated_at |
| `scan_folders` | path, root_folder_id, kind, status (FOUND/LINKED/IGNORED), title_id? (set on `ScanFolderMatched`), found_at, updated_at |

- `etag` = hash of (device, inode, size, mtime). It changes whenever the file is replaced or modified.
- `media_files.rename_problem` (default `''`) says why a file kept its old name after an episode-group change; a later successful rename clears it.
- `os_hash` and `release_name` are captured once at import and never recomputed. Subtitle matching depends on the original file ([16](../../../.scratch/v1-spec/issues/16-subtitle-design.md)).

## Query port

```go
package api // internal/library/api

type Reader interface {
	// FilesForTitle returns the current media files of a title, newest first. No files → empty slice.
	FilesForTitle(ctx context.Context, title domain.TitleID) ([]MediaFile, error)
	// FilesForTitles returns the files of several titles for the library tables: grouped by title in
	// input order, newest first within a title, keeping only those the filter matches. No titles → empty slice.
	FilesForTitles(ctx context.Context, titles []domain.TitleID, f FileFilter) ([]MediaFile, error)
	// FileForEpisode returns the file covering an episode, or ErrNotFound.
	FileForEpisode(ctx context.Context, ep domain.EpisodeID) (MediaFile, error)
	// File returns one media file, or ErrNotFound.
	File(ctx context.Context, id domain.MediaFileID) (MediaFile, error)
	// SourceForProcessing gives Transcode a read-only absolute path, the current ETag,
	// and a staging directory on the same filesystem. Transcode serves these bytes to agents
	// itself and writes the uploaded result into StagingDir; it never touches Path.
	SourceForProcessing(ctx context.Context, id domain.MediaFileID) (ProcessingSource, error)
	// AvailableEpisodes returns the title's episodes whose files are available (empty for a movie).
	AvailableEpisodes(ctx context.Context, title domain.TitleID) ([]domain.EpisodeID, error)
	// Coverage returns per-title file coverage for the TV page, one entry per input title in input order:
	// episodes with a file and episodes available. Titles without files return zero counts, not ErrNotFound.
	Coverage(ctx context.Context, titles []domain.TitleID) ([]TitleCoverage, error)
}

type MediaFile struct {
	ID          domain.MediaFileID
	Title       domain.TitleID
	Episodes    []domain.EpisodeRef
	RootFolder  string
	Path        string
	Size        int64
	Quality     domain.Quality
	ReleaseName string    // original grabbed release; empty for scanned files with no name
	OSHash      string    // OpenSubtitles hash of the original file
	MediaInfo   MediaInfo // streams (codec, language, forced, default, image/text, start time), duration, HDR format, bitrate
	Origin      domain.Origin      // DOWNLOAD | LIBRARY_SCAN | MIGRATION
	Download    *domain.DownloadID // source download, nil for scanned files
	InfoHash    string             // source torrent hash; "" if unknown (scan, some migrations)
	Transcoded  bool
	ImportedAt  time.Time
	Available   bool
}

// FileFilter narrows FilesForTitles. Transcoded nil keeps every file.
type FileFilter struct { Transcoded *bool }

type TitleCoverage struct {
	Title                domain.TitleID
	EpisodesWithFile     int
	EpisodesAvailable    int
	SizeBytes            int64
}

type ProcessingSource struct {
	Path       string // absolute, read-only for the caller
	ETag       string
	Size       int64
	StagingDir string // same filesystem as Path; caller may create files here only
	MediaInfo  MediaInfo
}

var ErrNotFound = errors.New("library: not found")
```

## Events published

```go
type FileImported struct {
	MediaFile   domain.MediaFileID
	Title       domain.TitleID
	Episodes    []domain.EpisodeRef
	Path        string
	Quality     domain.Quality
	ReleaseName string
	OSHash      string
	Size        int64
	MediaInfo   MediaInfo
	Replaces    *domain.MediaFileID // informational: the file this upgrade replaced. Cleanup is driven by FileDeleted{UPGRADED}
	Download    *domain.DownloadID  // nil for scanned/migrated files
	Sidecars    []Sidecar           // subtitles shipped in the release, already placed
	Origin      domain.Origin       // DOWNLOAD | LIBRARY_SCAN | MIGRATION
}

type ImportFailed struct {
	Download  domain.DownloadID
	Reason    string
	Retryable bool
	Manual    bool // MANUAL: the import waits for the user to map or remove it, so the download keeps its data
}
// ImportRemoved: the user chose Remove & blocklist on a FAILED or MANUAL import.
type ImportRemoved struct { Download domain.DownloadID; Title domain.TitleID; ReleaseName string }
type FileTranscoded struct { MediaFile domain.MediaFileID; Path string; OldSize, NewSize int64 }
type FileDeleted struct {
	MediaFile domain.MediaFileID
	Title     domain.TitleID
	Episodes  []domain.EpisodeRef
	Path      string
	Reason    DeleteReason // UPGRADED | REMOVED | TITLE_REMOVED
}
// FileRenamed: a media file moved to a new path, with every file sharing its basename.
type FileRenamed struct {
	MediaFile domain.MediaFileID
	Title     domain.TitleID
	Episodes  []domain.EpisodeRef // the numbers after the rename
	OldPath   string
	NewPath   string
	Sidecars  []SidecarMove // { OldPath, NewPath string }
}
type ItemAvailable struct { Title domain.TitleID; Episodes []domain.EpisodeID }
type FolderFound struct { Path string; Kind domain.MediaKind; ParsedTitle string; Year *int; TmdbHint *domain.TmdbID }
type TranscodeSwapRejected struct { Job domain.JobID; MediaFile domain.MediaFileID; Reason SwapRejectReason } // SOURCE_CHANGED | HASH_MISMATCH | PROBE_FAILED | FILE_GONE
```

- All of these are durable. `FileImported` is published in the same transaction that inserts the `media_files` row. Aggregate key: title ID; events without a title use their own subject (`ImportFailed` and `ImportRemoved` → download ID, `FileTranscoded` and `TranscodeSwapRejected` → media file ID, `FolderFound` → folder path).
- `FileDeleted` is the **only** cleanup signal for a removed file, whatever the reason. Consumers (Acquisition, Subtitles, Transcode) never infer deletion from `FileImported.Replaces`.
- `FileRenamed` is published in the transaction that records the new path, for every move of a stored media file. `Sidecars` lists the files that moved with it (subtitles and anything else named `<video basename>.…`). Moves that happen before a file is stored (rename-on-scan) need no event: `FileImported` already carries the new paths. A transcode swap that changes the extension is reported by `FileTranscoded.Path`; its sidecars don't move.

## Events and commands consumed

| Event | Reaction | Idempotency key | Failure handling |
|---|---|---|---|
| `acquisition.DownloadCompleted` | Run import (below) as a backlite job | download ID (`imports` row) | Retry 3× with backoff for IO errors. Then `ImportFailed{Retryable:false}` and the import enters the manual queue. |
| `transcode.TranscodeVerified` | Swap (below) | job ID | On check failure, publish `TranscodeSwapRejected`; the staged file is deleted. |
| `catalog.EpisodeGroupChanged` | Rename affected files and their sidecars to new S/E numbers; relink `media_file_episodes`; publish `FileRenamed` per moved file | event ID | Per-file errors are logged; the file keeps its old name and is flagged for review. |
| `catalog.TitleRemoved` | Queue the job `library.remove-title`. If `deleteFiles`, it deletes each file from disk; either way it then drops the record and publishes `FileDeleted{Reason: TITLE_REMOVED}`, one transaction per file. Unlinking only leaves the files on disk. Sidecars are Subtitles' to delete. | event ID; a rerun finds only the files still recorded | Missing files are treated as already deleted. A disk error fails the job, which retries. |
| `catalog.ScanFolderMatched` | Link the files in that folder to the Title in place (no move or rename unless opted in); publish `FileImported{Origin: LIBRARY_SCAN}` per video | scan folder path | A folder with no playable video is marked IGNORED. |

## External ports

```go
// FileSystem wraps all disk mutation so tests can use an in-memory fake.
type FileSystem interface {
	Stat(path string) (FileInfo, error)          // FileInfo includes device and inode
	SameDevice(a, b string) (bool, error)
	Hardlink(src, dst string) error
	Copy(ctx context.Context, src, dst string) error
	Move(src, dst string) error                  // rename, or copy+delete across devices
	Rename(src, dst string) error                // must be atomic; ErrCrossDevice across devices
	Remove(path string) error
	MkdirAll(path string) error
	Walk(root string, fn func(path string, info FileInfo) error) error
}

// DownloadLocation is implemented by the qBittorrent adapter, for the hardlink check.
type DownloadLocation interface {
	// SavePath is where qBittorrent saves Magpie's category (else its default), path mapping applied.
	SavePath(ctx context.Context) (string, error)
}

type MediaProber interface {
	// Probe runs ffprobe. probesize 50 MB, retried at 150 MB with longer analyzeduration. ErrProbeFailed if unreadable.
	Probe(ctx context.Context, path string) (MediaInfo, error)
}

type Hasher interface {
	OSHash(path string) (string, error) // size + uint64 LE sum of the first and last 64 KiB
	SHA256(path string) (string, error)
}

type JellyfinLibrary interface {
	// RefreshPath calls POST /Library/Media/Updated for one path.
	RefreshPath(ctx context.Context, path string) error
}

var ErrCrossDevice, ErrProbeFailed, ErrJellyfinUnavailable error
```

- `MediaInfo` (streams, duration, HDR format, bitrate, size) lives in `library/api`; `FileInfo` is spelled out in `internal/library/ports.go`.

- Every Jellyfin request sends only `Authorization: MediaBrowser Token="…"`. The legacy headers are disabled by default on Jellyfin 12 ([08](../../../.scratch/v1-spec/issues/08-requests-and-jellyfin.md)).
- The release-name parser is a pure shared package, `internal/platform/parse` (no state, no I/O). Acquisition and Library both use it; Library uses it to map season-pack files to episodes.

## HTTP surface

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/library/files?title={id}` | Files for the title page Files tab; each has `available` and `renameProblem` (set when an episode-group rename failed) |
| GET | `/api/library/files?titles={id}&titles={id}…&transcoded={bool}` | Batch files (same shape as above) for the Movies page, grouped by title in input order (`FilesForTitles`); `transcoded` filter optional. Pass exactly one of `title` or `titles`; more than 500 titles is 400 `too-many-titles` (admin) |
| GET | `/api/library/files/{id}` | One file with media info |
| GET | `/api/library/coverage?titles={id}&titles={id}…` | Episodes with a file / available and size per title, in input order (TV page; at most 500 titles, admin) |
| DELETE | `/api/library/files/{id}` | Delete a file from disk → `FileDeleted{REMOVED}` (admin). 204; 404 `file-not-found` for an unknown or already deleted file |
| GET | `/api/library/root-folders` · POST · DELETE `/{id}` | Root folders, each with its hardlink check (`link`: `HARDLINK` \| `COPY` \| `UNKNOWN`); POST validates the path is absolute, exists and is writable (400 `invalid-root-folder`, 409 `root-folder-exists`); DELETE refuses a folder that still holds media files (409 `root-folder-in-use`) |
| GET | `/api/library/imports?status=FAILED,MANUAL` | Failed and manual imports, with each download's videos and subtitles, the matched episodes, and the series' episodes for the picker |
| POST | `/api/library/imports/{downloadId}/manual` | Body `{files: [{path, episodes}]}`: imports the listed videos as those episodes (a movie's video has none) and skips the rest; 202, runs as the import job. 400 `invalid-file-mapping`, 409 `import-not-stopped` unless FAILED or MANUAL |
| POST | `/api/library/imports/{downloadId}/retry` | Runs a FAILED or MANUAL import again with automatic mapping; 202 |
| DELETE | `/api/library/imports/{downloadId}` | Remove & blocklist: marks a FAILED or MANUAL import `REMOVED` and publishes `ImportRemoved` in the same transaction; Acquisition then blocklists the release, deletes the torrent with its data and grabs the next one (a FAILED import's release is already blocklisted). 204; 409 `import-not-stopped` otherwise |
| POST | `/api/library/scan` | Start a library scan (backlite job); 202 with the scan status. Nothing new starts while one runs (a run older than an hour counts as stale) |
| GET | `/api/library/scan` | `{running, startedAt, finishedAt, error, reported, found, linked, ignored, renameMatched}`: the last scan, how many folders it reported, and the scan folders by status (admin) |
| PUT | `/api/library/scan/options` | `{renameMatched}`: "Rename matched folders on scan" (setting `library.scan.options`); answers the scan status (admin) |
| GET | `/api/library/hardlink-check` | Whether the qBittorrent download path (from `DownloadLocation`) and each root folder share a device; `problem` says why the result is `UNKNOWN` |

## Behaviour

**Import** (on `DownloadCompleted`):
0. The handler stores an `imports` row keyed by download ID (a redelivery finds it and does nothing) and queues the import job in the same transaction. The job runs only a `PENDING` import, and moves it to `IMPORTED` in the transaction that inserts the `media_files` rows.
1. Resolve `contentPath` and list files. Keep only video files (`mkv mp4 m4v avi ts`) and subtitle files (`srt ass ssa sub idx sup vtt`). Anything else is ignored.
   - The blocked-file backstop runs in Acquisition before it publishes `DownloadCompleted`. Library never imports unknown extensions and never reads Acquisition's block list.
   - A single-file torrent's `contentPath` is the file. `Files` paths are relative to `contentPath`, or to its parent when they start with its folder name (as qBittorrent lists them).
2. Probe each video. Skip samples: runtime ≤ the threshold for the expected runtime (≤3 min → 15 s, ≤10 → 90 s, ≤30 → 300 s, else 600 s). A video ffprobe can't read counts as a sample ([04](../../../.scratch/v1-spec/issues/04-qbittorrent-and-import.md)). The expected runtime is the episode's, else the title's, else 45 min.
3. Map videos to episodes. A single file maps to the payload's episodes. Season packs parse each file name, falling back to the folder name, then the release name, and match the current numbering, then TMDB's aired order. Any video that can't be mapped, two videos for one episode, or several videos for a movie make the import `MANUAL` (no partial import). No video at all (or only samples), no root folder, or a title that's gone makes it `FAILED`. Both publish `ImportFailed{Retryable:false}`; `Manual` is set for `MANUAL`, and Acquisition keeps that download's torrent and data until the user maps (Map files, Retry) or removes (Remove & blocklist) the import.
4. Build the target path from `catalog.Reader` (fixed Jellyfin naming, no editor):
   - Movie: `<movies root>/<Title> (<Year>) [tmdbid-<id>]/<Title> (<Year>).<ext>`
   - Episode: `<tv root>/<Show> (<Year>) [tmdbid-<id>]/Season <NN>/<Show> S<NN>E<NN>[-E<NN>].<ext>`
   - Names are sanitised for the filesystem; numbering follows the title's current episode group.
5. Place the file. If `seedingMode = SEED_UNTIL_GOALS`: hardlink, falling back to copy when the devices differ. If `DONT_SEED`: move. Hardlink/copy/move happens into a temp name, then an atomic rename.
6. Compute OSHash and ETag. Store `release_name` and the download's info hash from the payload.
7. Upgrade: a new file replaces the title's existing files covering the same episodes. For a movie that is any file of the title. For a series it is any file sharing an episode with it, but only when the import's videos together cover **all** of that file's episodes.
   - A series import that shares some but not all episodes of an existing file (a new `S01E01` over an old `S01E01-E02`) is `MANUAL` with the reason "Would split the multi-episode file `<name>`". This is checked after mapping (step 3), so nothing is placed and the old file is untouched. The user can map the video to all the old file's episodes, remove the import, or delete the old file and retry.
   - In the transaction that inserts the new row, the old rows are dropped and `FileImported{Replaces: old}` is published for the new file, then `FileDeleted{Reason: UPGRADED}` for each old one. `Replaces` names the first when there are several.
   - After the commit the old file is removed from disk. An old file at the new file's path was already overwritten by the placement, so it isn't removed. A removal error is logged; the record is gone either way.
   - Sidecars of the old file are Subtitles' to remove on `FileDeleted`.
8. Release subtitles go to `<basename>.<lang>[.forced|.sdh].<ext>`, reported in `Sidecars`. The language (ISO 639-1/2 code or English name) is parsed from the end of the file name; files with no language are dropped. A subtitle belongs to the video whose name it starts with, the only video, or the video covering the episodes its own name gives.
9. Commit and publish `FileImported` and `ItemAvailable`. Then refresh Jellyfin for the parent folder.

**Availability:**
- A file is available once it is in the library. Every new file (import, scan link, migration) sets `available_at` and publishes `FileImported` then `ItemAvailable` in the same transaction. Magpie doesn't check that Jellyfin picked it up.
- After each import, transcode swap or rename, call `RefreshPath` once per parent folder. A refresh error is logged and never fails the import.

**Transcode swap** (on `TranscodeVerified`): the handler queues the backlite job `library.transcode-swap`, which does the disk work. A redelivered event whose staged file is gone while the file is already transcoded is a no-op.
1. Load the file. If it's gone (record or disk), reject (`FILE_GONE`).
2. Current ETag must equal `sourceETag`, both the stored one and one computed from the file on disk now, else reject (`SOURCE_CHANGED`, e.g. an upgrade landed meanwhile).
3. The staged file must sit in that file's `StagingDir`, and its SHA-256 must equal the payload's, else reject (`HASH_MISMATCH`).
4. Probe the staged file. The duration must be within 1 s of the original and the stream count must match `stats`, else reject (`PROBE_FAILED`).
5. Atomic rename staged → final path (always `.mkv`). If the extension changed, remove the old file after the rename. Sidecars keep their basename, so they stay valid.
6. Update path, size, media info (the staged file's probe), ETag, `transcoded=true`, `transcoded_at`, `original_size` (the first original's size is kept across transcodes). Keep `os_hash` and `release_name`. Publish `FileTranscoded`, then refresh Jellyfin for the file's folder; the new Jellyfin item publishes `ItemAvailable` again.
7. A rejection deletes the staged file and publishes `TranscodeSwapRejected`.

**Library scan:** walk the root folders. For each top-level folder not linked to a title, publish `FolderFound` with the parsed title/year and any `[tmdbid-N]`/`{tmdb-N}` hint. Catalog decides the match (exact ID, else fuzzy with a review queue it owns) and publishes `ScanFolderMatched` (plus `TitleAdded{Origin: LIBRARY_SCAN}` for new Titles). Library then links that folder's files in place, without moving or renaming unless the user opted in ([18](../../../.scratch/v1-spec/issues/18-import-and-library.md)), and publishes `FileImported{Origin: LIBRARY_SCAN}`.
- The scan skips hidden folders (`.magpie-staging`), folders that already hold a media file (imports) and `LINKED` scan folders. `FOUND` folders are reported again (Catalog ignores folders it has seen). An `IGNORED` folder whose title is known is relinked without asking Catalog, in case a video was added.
- Folder names: the `[tmdbid-N]`, `{tmdb-N}` (also with `=`) tag gives the hint; ID tags are dropped from the title; `Title (Year)` gives title and year; else the release parser reads a release-style name; else the whole name is the title.
- Linking: videos that aren't samples and aren't media files yet. A movie keeps its largest video. A series maps each video by its name (current numbering, then aired order); a video it can't map, or a second one for an episode, is left out. Quality comes from the name, else from the picture width (HDTV at that resolution, as Sonarr does). `release_name` keeps a release-style file name, else `""`. Subtitles named `<video basename>.<lang>…` are reported in `Sidecars`. No playable video → `IGNORED`. The rows, `FileImported`s and `LINKED` commit together; a redelivered `ScanFolderMatched` for a `LINKED` folder does nothing.
- With "Rename matched folders on scan" on, each video and the files sharing its basename move to the fixed naming in the same root (temp name, then rename). A video that can't move is linked where it is.

**Manual delete:** deleting a file from the UI (title page → Files, with a confirmation) removes the video from disk, then drops its record and publishes `FileDeleted{Reason: REMOVED}`, and refreshes Jellyfin for its folder. A video already gone from disk counts as deleted.

**Episode-group change:** rename each affected file and every sidecar sharing its basename to the new numbering, update links, then refresh Jellyfin.
- A file is affected when one of its episodes is in `Moves` at its stored `From` numbers. Its new path keeps its show folder (the top folder under the root; a file loose in the root moves to the title's folder) and is `Season <NN>/<Show> S<NN>E<NN>[-E<NN>].<ext>` within it.
- Every file first moves to a temporary name in the target folder, then to its final name, so episodes can trade names. A move that would overwrite a file outside the batch, or fails, is put back: the file keeps its old name and numbers and gets `rename_problem`. A rename an interrupted run already made (old path gone, new one there) is just recorded.
- Each moved file's new path and numbers are recorded with a `FileRenamed` listing its sidecars' old and new paths, so Subtitles keeps its stored sidecar paths current. A file whose path stays the same publishes nothing.
- Then `RefreshPath` runs for the old and new folders, and emptied folders are removed.

## Test seeds

1. **Given** SEED mode and the download on the same device as the root folder, **when** `DownloadCompleted` arrives for a movie, **then** the file is hardlinked to `Movies/Title (Year) [tmdbid-N]/Title (Year).mkv`, the source remains, and `FileImported` carries the OSHash and release name.
2. **Given** SEED mode on different devices, **when** importing, **then** the file is copied and the source remains.
3. **Given** DONT_SEED mode, **when** importing, **then** the file is moved and the source no longer exists.
4. **Given** a season pack with E01–E10 and a 40 s `sample.mkv`, **when** importing, **then** ten files are imported, the sample is skipped, and one `FileImported` is published per file.
5. **Given** a pack where one video can't be mapped to an episode, **when** importing, **then** nothing is imported, the import is `MANUAL`, and `ImportFailed{Retryable:false, Manual:true}` is published. A pack with only a sample is `FAILED` and publishes `ImportFailed{Manual:false}`.
6. **Given** `DownloadCompleted` is delivered twice, **when** handled, **then** only one `media_files` row exists and `FileImported` is published once.
7. **Given** an existing file F1 for S01E01, **when** an upgrade imports F2, **then** F1 is deleted from disk, `FileImported{MediaFile: F2, Replaces: F1}` and then `FileDeleted{MediaFile: F1, Reason: UPGRADED}` are published in that order, once each. A new `S01E01-E02` video replaces F1 and an old `S01E02` file. A season pack replaces an old `S01E01-E02` file. A new movie file replaces the movie's old file. **Given** an existing `S01E01-E02` file, **when** a single `S01E01` video is imported, **then** the import is `MANUAL` with a reason naming the old file, nothing is placed, and the old file keeps its record and stays on disk.
8. **Given** a release containing `Movie.en.srt` and `Movie.nfo`, **when** importing, **then** the sidecar `Title (Year).en.srt` exists, the nfo is not copied, and `Sidecars` lists the srt.
9. **Given** `TranscodeVerified` with a matching ETag and hash, **when** handled, **then** the path ends `.mkv`, the old `.mp4` is removed, `transcoded=true`, `os_hash` is unchanged, and `FileTranscoded` is published.
10. **Given** the file was upgraded after the transcode started (ETag differs), **when** `TranscodeVerified` arrives, **then** the file is unchanged, the staged file is deleted, and `TranscodeSwapRejected{SOURCE_CHANGED}` is published.
11. **Given** a staged file whose SHA-256 differs from the payload, **when** handled, **then** `TranscodeSwapRejected{HASH_MISMATCH}` is published.
12. **Given** a movie download, **when** imported, **then** the file is available and `ItemAvailable` is published once for the movie.
13. **Given** a season pack, **when** imported, **then** `ItemAvailable` covers every imported episode.
14. **Given** `EpisodeGroupChanged` maps S02E01→S01E11, **when** handled, **then** the video and its `.en.srt` are renamed to `S01E11` and the links are updated.
15. **Given** a root folder containing `Dune (2021) [tmdbid-438631]`, **when** a scan runs, **then** `FolderFound{TmdbHint: 438631}` is published, and after `ScanFolderMatched{Folder, Title}` the file is linked in place without being moved and `FileImported{Origin: LIBRARY_SCAN, Download: nil}` is published; a redelivered `ScanFolderMatched` creates no second row.
16. **Given** the fake Jellyfin port returns an error on `RefreshPath`, **when** importing, **then** the import still commits and the file is available.
17. **Given** `TitleRemoved{DeleteFiles: false}` for a title with two files, **then** the files stay on disk, their records are gone, and two `FileDeleted{Reason: TITLE_REMOVED}` are published.
18. **Given** a file, **when** `SourceForProcessing` is called, **then** it returns the current ETag and a `StagingDir` on the same device as the file; after the file is replaced by an upgrade, the ETag differs.
19. **Given** a series with 10 episodes, 7 with files and 6 of them available, **then** `Coverage` returns `EpisodesWithFile = 7`, `EpisodesAvailable = 6`.
20. **Given** a `MANUAL` import, **when** the user removes it (Remove & blocklist), **then** the import is `REMOVED` and leaves the imports queue, `ImportRemoved{Download}` is published once, and removing it again answers 409 `import-not-stopped`.
21. **Given** an S02E01 file with a `.en.srt` sidecar, **when** `EpisodeGroupChanged` maps S02E01→S01E11, **then** `FileRenamed` is published once with the video's and the sidecar's old and new paths and the new numbers; a redelivered `EpisodeGroupChanged` publishes no second one.
