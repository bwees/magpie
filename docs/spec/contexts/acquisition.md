# Acquisition

Finds, ranks, grabs and downloads the right release for each wanted movie or episode, and decides when a file is good enough.

- **Depends on (Readers):** `catalog.Reader`, `library.Reader`
- **Publishes:** `SearchStarted`, `SearchResultsReceived` *(ephemeral)*, `SearchCompleted`, `ReleaseGrabbed`, `ReleaseRejected`, `DownloadProgressed` *(ephemeral)*, `DownloadCompleted`, `DownloadFailed`, `DownloadRemoved`, `CutoffMet`
- **Consumes:**
  - From Catalog: `TitleAdded`, `MonitoringChanged`, `ProfileChanged`, `EpisodesAnnounced`, `WantedReleased`, `TitleRemoved`.
  - From Requests: `IssueActionRequested` (REGRAB).
  - From Library: `FileImported`, `ImportFailed`, `ImportRemoved`, `FileTranscoded`, `FileDeleted`.
- **Importer entrypoint:** `api.Importer` (migration job only): profiles, custom formats, blocklist, file scores.

Decided in: [quality model](../../../.scratch/v1-spec/issues/10-quality-model.md), [blocked files](../../../.scratch/v1-spec/issues/11-blocked-file-policy.md), [search UX](../../../.scratch/v1-spec/issues/12-progressive-search-ux.md), [import and library](../../../.scratch/v1-spec/issues/18-import-and-library.md), [Prowlarr integration](../../../.scratch/v1-spec/issues/22-indexer-engine.md), [scheduling](../../../.scratch/v1-spec/issues/24-scheduling.md), [import gaps](../../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md). Research: [03](../../../.scratch/v1-spec/research/03-release-decision-engine.md), [02](../../../.scratch/v1-spec/research/02-cardigann-indexers.md), [04](../../../.scratch/v1-spec/research/04-qbittorrent-and-import.md), [28](../../../.scratch/v1-spec/research/28-arr-import-mapping.md).

## Owns / Must not

| Owns (only Acquisition writes) | Must not |
|---|---|
| Quality definitions, quality profiles, custom formats, TRaSH sync state and user overrides | Add, remove or monitor titles (Catalog) |
| Indexer settings layered on Prowlarr's list (priority, minimum seeders, seeding mode override, seeding goals) | Configure indexers in Prowlarr (read-only use) |
| Searches and their in-memory result cache | Move, link, rename or delete library files (Library) |
| Release decisions, pending (delayed) releases, blocklist | Talk to Jellyfin |
| Downloads in qBittorrent: grab, metadata check, progress, seeding goals, removal | Trigger transcodes or subtitles directly (only via `CutoffMet` / events others consume) |
| Cutoff evaluation for library files | Write `library` or `catalog` tables |
| The ported release-title parser | |

## Data model

| Table | Key fields |
|---|---|
| `qualities` | `id` (Radarr IDs, see below), `name`, `source`, `resolution` |
| `quality_profiles` | `id`, `name` (unique, any case), `upgrade_allowed`, `cutoff_item_id` (a quality ID or a group ID), `min_format_score`, `cutoff_format_score` (upgrade-until), `min_upgrade_format_score` (increment), `propers_mode` (`PREFER_AND_UPGRADE` \| `DO_NOT_UPGRADE` \| `DO_NOT_PREFER`), `preset?` (`720P` \| `1080P` \| `2160P`), `trash_profile_id?`, `trash_score_set?` |
| `quality_profile_items` | `profile_id`, `position`, `quality_id?` or `group_id`, `allowed`, `min_mb_per_min`, `preferred_mb_per_min`, `max_mb_per_min` (sizes live on the profile) |
| `quality_groups` | `profile_id`, `id` (≥1000, numbered per profile in item order), `name`, member quality IDs (equal rank inside a group) |
| `custom_formats` | `id`, `name` (unique, any case), `trash_id?`, `trash_scores` (JSON: TRaSH score per score set), `include_when_renaming=false`, `specs` (JSON: kind, name, negate, required, value, min, max) |
| `profile_format_scores` | `profile_id`, `format_id`, `score`, `user_override bool` |
| `trash_profiles` | `trash_id`, `app`, `name`, `profile` (JSON: the guide profile), `synced_at`: the guide profiles of the last TRaSH sync |
| `indexers` | `prowlarr_id`, `name`, `privacy` (`PUBLIC` \| `SEMI_PRIVATE` \| `PRIVATE`), `caps` (JSON: search modes, ID params, categories), `status` (`OK` \| `FAILING` \| `GONE`), `last_error`, `failing_since?`, `average_response_ms`, `priority` (default 25), `min_seeders` (default 1), `seeding_mode_override` (`USE_GLOBAL` \| `SEED_UNTIL_GOALS` \| `DONT_SEED`, default `USE_GLOBAL`), `seed_ratio?`, `seed_days?` (null = the global goal), `last_ok_at?`, `synced_at` |
| `searches` | `id`, `title_id`, `episode_ids`, `trigger` (`RSS` \| `AUTO` \| `USER_AUTO` \| `INTERACTIVE` \| `REGRAB`), `started_at`, `completed_at`, counts. Results are not persisted. |
| `pending_releases` | `id`, `title_id`, `episode_ids`, `release_key` (indexer + guid, unique per title), `indexer_id`, release snapshot (the Torznab item), `trigger` (`AUTO` \| `RSS` \| `REGRAB`), `publish_at`, `reason` (`DELAYED` \| `PENDING`), `added_at` |
| `search_jobs` | `key` = `(title, episodes, trigger)`, `queued_at`: a queued search job; a second one with the key isn't queued. The job deletes its key when it starts. |
| `issue_regrabs` | `issue_id`, `handled_at`: issues whose REGRAB already blocklisted and searched |
| `blocklist` | `title_id`, `info_hash`, `release_name`, `indexer`, `reason` (`BLOCKED_FILE` \| `METADATA_TIMEOUT` \| `STALLED` \| `IMPORT_FAILED` \| `DOWNLOAD_FAILED` \| `USER` \| `ISSUE` \| `MIGRATED`), `at`, `expires_at?` (null = permanent) |
| `download_marks` | `download_id`, `progress`, `at`: when a download awaiting metadata or downloading last moved forward or waited in qBittorrent's queue; the start of its stall or metadata clock. Stored at most once a minute while it moves; rows of downloads no longer fetching are dropped |
| `downloads` | `id`, `info_hash`, `qbit_hash`, `title_id`, `episode_ids`, `release_name`, `release_id` (the search's release ID), `indexer_id`, `indexer` (name), `quality` (JSON), `cf_score`, `size_bytes`, `state`, `progress`, `seeding_mode`, `trigger`, `override`, `failure` (why it was dropped), `goal_ratio?`, `goal_time?`, `grabbed_at`, `completed_at?`, `removed_at?`, `search_id?` |
| `file_scores` | `media_file_id`, `title_id`, `episode_ids`, `quality` (JSON), `release_name`, `path`, `size_bytes` (what the score is computed from, as imported), `cf_score`, `profile_id` (the Title's profile at the last evaluation), `cutoff_met bool`, `transcoded bool`, `origin`, `evaluated_at` (Acquisition's own view of library files; built from `FileImported`/`FileTranscoded`/`FileDeleted`) |

**Qualities** use Radarr's 30 IDs. Imported Sonarr IDs are remapped (13→20, 22→21, 20→30, 21→31). Profiles and custom formats are **one pool shared by movies and TV**; the title references a profile by `ProfileID` (stored in Catalog).

## Query port

```go
package api // internal/acquisition/api

// Reader is read-only and backed by Acquisition's repository. Requests is the only other context that
// depends on it, for the default profile of new requests (see the context map). The UI's profile pickers
// call GET /api/acquisition/profiles directly.
type Reader interface {
    // Profile returns a quality profile with items, groups, sizes and format scores.
    // Returns ErrNotFound for unknown IDs.
    Profile(ctx context.Context, id domain.ProfileID) (Profile, error)
    // Profiles lists all profiles, ordered by name.
    Profiles(ctx context.Context) ([]ProfileSummary, error)
    // DefaultProfile returns the default profile for new requests ("" before the first start seeds it).
    DefaultProfile(ctx context.Context) (domain.ProfileID, error)
    // ActiveDownloads lists downloads not yet removed from qBittorrent, newest first.
    ActiveDownloads(ctx context.Context) ([]Download, error)
    // DownloadsForTitle lists every download for a title, including removed ones, newest first.
    DownloadsForTitle(ctx context.Context, title domain.TitleID) ([]Download, error)
    // Blocklist lists entries for a title (or all when title is empty), newest first, expired temporary entries included.
    Blocklist(ctx context.Context, title domain.TitleID) ([]BlocklistEntry, error)
    // CutoffStatus reports whether a library file meets its profile cutoff and whether upgrades are still allowed.
    // Returns ErrNotFound for a file Acquisition has no score for.
    CutoffStatus(ctx context.Context, file domain.MediaFileID) (CutoffStatus, error)
}

type CutoffStatus struct {
    File            domain.MediaFileID
    Quality         domain.Quality
    CFScore         int
    CutoffMet       bool // quality ≥ cutoff AND CF score ≥ cutoff_format_score
    UpgradesAllowed bool // false once transcoded, or profile.upgrade_allowed == false
}
```

- `Profile`, `ProfileSummary`, `Download`, `BlocklistEntry` and the enums are spelled out in `internal/acquisition/api`. Profile items are in preference order, most preferred first; `Profile.Cutoff` is an index into them.
- `ErrNotFound = errors.New("acquisition: not found")`.

## Events published

All durable unless marked ephemeral. Aggregate ID = title ID (searches, downloads, cutoff), so a title's events stay ordered. The two ephemeral events carry no title: `SearchResultsReceived` uses the search ID and `DownloadProgressed` the download ID.

```go
type SearchStarted struct {
    Search      domain.SearchID
    Title       domain.TitleID
    Episodes    []domain.EpisodeID // empty for movies
    Trigger     SearchTrigger      // RSS, AUTO, USER_AUTO, INTERACTIVE, REGRAB
    Interactive bool
    Indexers    []string           // names queried
}

// Ephemeral: one per indexer answer (or failure). Never persisted; drives the live search dialog.
type SearchResultsReceived struct {
    Search   domain.SearchID
    Indexer  string
    Status   IndexerRunStatus // DONE, FAILED, SKIPPED (no ID support and title search found nothing)
    Message  string           // failure reason for FAILED/SKIPPED
    Releases []ReleaseDecision
}

type ReleaseDecision struct {
    Release     domain.ReleaseID // stable per (search, indexer, guid)
    Name        string
    Indexer     string
    InfoHash    string           // may be empty until grab
    Quality     domain.Quality
    Formats     []string         // matched custom format names
    CFScore     int
    SizeBytes   int64
    Seeders     int
    PublishedAt time.Time
    Freeleech   bool
    Rejections  []Rejection      // empty = accepted
    Rank        int              // position among accepted releases at time of emit
}

type Rejection struct { Reason RejectionReason; Message string; Temporary bool }

type SearchCompleted struct {
    Search          domain.SearchID
    Title           domain.TitleID
    Accepted, Total int
    FailedIndexers  []string
    Outcome         SearchOutcome      // GRABBED | NOTHING_ACCEPTED | PENDING_DELAY | RELEASES_FOUND (interactive searches: GRABBED after a user grab, else RELEASES_FOUND when any release was accepted, else NOTHING_ACCEPTED)
    Grabbed         *domain.DownloadID // set when Outcome = GRABBED
}

type ReleaseGrabbed struct {
    Download domain.DownloadID
    Title    domain.TitleID
    Episodes []domain.EpisodeID
    Release  string
    Indexer  string
    InfoHash string
    Quality  domain.Quality
    CFScore  int
    Trigger  SearchTrigger
    Override bool // user grabbed a rejected release
}

type ReleaseRejected struct {
    Title       domain.TitleID
    Release     string
    InfoHash    string
    Indexer     string
    Reason      RejectionReason // BLOCKED_FILE, METADATA_TIMEOUT, STALLED, IMPORT_FAILED, DOWNLOAD_FAILED
    BlockedFile *string         // matched file path inside the torrent
    Download    *domain.DownloadID // the rejected download; nil when rejected before it reached qBittorrent
    Next        *domain.DownloadID // the fallback grab, if one was made
}

// Ephemeral: emitted on each qBittorrent poll that changes progress or state.
type DownloadProgressed struct {
    Download domain.DownloadID
    State    DownloadState // AWAITING_METADATA, DOWNLOADING, STALLED, COMPLETED, SEEDING, IMPORT_PENDING_USER
    Progress float64       // 0..1
    SpeedBps int64
    ETA      time.Duration
    Ratio    float64
}

type DownloadCompleted struct {
    Download    domain.DownloadID
    Title       domain.TitleID
    Episodes    []domain.EpisodeID
    ContentPath string         // as qBittorrent reports it; Library applies the remote path mapping on import
    Files       []DownloadFile // relative path + size; only files with priority > 0
    ReleaseName string
    InfoHash    string         // the torrent's info hash; Library stores it as the file's source hash
    Quality     domain.Quality
    CFScore     int
    SeedingMode SeedingMode    // SEED_UNTIL_GOALS → Library hardlinks (copy fallback); DONT_SEED → Library moves
}

type DownloadFailed struct {
    Download domain.DownloadID
    Title    domain.TitleID
    Reason   string // qBittorrent error/missingFiles, stalled past limit, etc.
}
// DownloadRemoved: the download left qBittorrent (seeding goal, a user, a drop, or gone from qBittorrent).
type DownloadRemoved struct {
    Download domain.DownloadID
    Title    domain.TitleID
}

type CutoffMet struct {
    File     domain.MediaFileID
    Title    domain.TitleID
    Episodes []domain.EpisodeID
    Quality  domain.Quality
    CFScore  int
    Origin   domain.Origin // copied from FileImported: DOWNLOAD | LIBRARY_SCAN | MIGRATION
}
```

## Events and commands consumed

Idempotency: every handler records `event.ID` in `processed_events` within the same transaction as its effects. All search work is enqueued as backlite jobs keyed by `(title, episodes, trigger)` so redelivery collapses into one job.

| Event | From | Reaction | Failure handling |
|---|---|---|---|
| `catalog.TitleAdded` | Catalog | `Origin` = REQUEST or MANUAL, with "Search when a title is added" on: one AUTO search for the monitored items whose release date has passed (`catalog.Reader.Wanted`); others wait for `WantedReleased`. `Origin` = LIBRARY_SCAN or MIGRATION: no immediate search; once Library has linked the files (`FileImported` with that origin), the normal missing (daily) and cutoff-unmet (weekly) sweeps apply. | Search job retries with backoff; Prowlarr down → `SearchCompleted` with all indexers failed. |
| `catalog.MonitoringChanged` | Catalog | Newly monitored and released → enqueue AUTO search. Unmonitored → drop matching pending releases. | — |
| `catalog.ProfileChanged` | Catalog | Re-evaluate cutoff for the Title's `file_scores` against the Title's current profile; publish `CutoffMet` for files that newly meet it. Files below the new cutoff become eligible for upgrade (unless transcoded). Handled by the same handler as the `file_scores` events below, so a title's changes apply in order. | — |
| `catalog.EpisodesAnnounced` | Catalog | No search. Episodes become eligible for RSS matching once released. | — |
| `catalog.WantedReleased` | Catalog | Enqueue AUTO search for the listed items (movie digital/physical release date or episode air date reached). | — |
| `catalog.TitleRemoved` | Catalog | Cancel running searches; drop pending releases; remove its downloads from qBittorrent (`deleteFiles=true` only if `deleteFiles`); keep blocklist rows. | qBit unreachable → retry job. |
| `requests.IssueActionRequested{REGRAB}` | Requests | Resolve the current file for `Title`/`Episode` via `library.Reader` (its source info hash and release name; without an episode, every file of the title). Blocklist that hash permanently (`ISSUE`), with the release name and the indexer of its source download when known so an indexer that reports no hash can't offer it again, and enqueue a REGRAB search for the episode or movie. A transcoded file does **not** block a REGRAB: the upgrade and repack specs don't apply to REGRAB searches. A release that already has an `ISSUE` entry gets no second one. The search doesn't count the downloads in `RemovedDownloads` as queued or recent (the admin removed them; their drop job may not have run yet). Runs once per issue ID (`acquisition_issue_regrabs`). | No source hash (scan/migration file) → blocklist by release name + the indexer of its source download if known, else just search. |
| `requests.IssueActionRequested{RERUN_SUBTITLES}` | Requests | Ignored. | — |
| `library.FileImported` | Library | Compute CF score from `ReleaseName` (fall back to file name) under the Title's profile (the default profile when the Title has none or its profile is gone). Upsert `file_scores` with `Origin`, for every origin and whether or not the download was waiting for the user. If cutoff met and the stored row didn't meet it → publish `CutoffMet{Origin}`, so a redelivery publishes nothing. A Title that is gone records nothing. If `Download` set: move the download `COMPLETED` or `IMPORT_PENDING_USER` → `IMPORTED` (once per download, however many files a pack had); if its mode is `DONT_SEED`, queue a job that removes the torrent (no data delete; Library moved it) and sets `removed_at`. `Replaces` is informational; cleanup of the old file happens on `FileDeleted`. | qBittorrent unreachable → the job retries. |
| `library.FileDeleted` | Library | Drop the file's `file_scores` row (any `Reason`). | — |
| `library.ImportFailed` | Library | `Retryable:false` only. `Manual:true`: move the download `COMPLETED` → `IMPORT_PENDING_USER`; the torrent keeps seeding with its data, nothing is blocklisted and nothing else is grabbed. Otherwise queue a job that drops the download like any rejection: remove torrent with data, state `IMPORT_FAILED` with `removed_at`, blocklist (`IMPORT_FAILED`), `ReleaseRejected`, grab next cached release or search. A download already removed or imported is left alone. | qBittorrent unreachable → the job retries. |
| `library.ImportRemoved` | Library | The user removed the import: queue the same drop job as a non-manual `ImportFailed`. A download already removed or imported is left alone, so a redelivery changes nothing. | qBittorrent unreachable → the job retries. |
| `library.FileTranscoded` | Library | Set `file_scores.transcoded = true`; upgrades stop for that file. Its CF score is **not** recomputed from the new file. | — |

## External ports

```go
// ports.go — implemented by internal/adapters/prowlarr
type Prowlarr interface {
    // Indexers lists enabled torrent indexers with Torznab caps (/{id}/api?t=caps) and Prowlarr's view of
    // their health (indexerstatus, indexerstats). Called hourly and on demand. An indexer whose caps call
    // returns a Torznab <error> comes back with nil Caps and Health.Error set; it doesn't fail the list.
    Indexers(ctx context.Context) ([]ProwlarrIndexer, error)
    // WebURL is Prowlarr's own UI ("" when no connection is set up), for the "Open Prowlarr" link.
    WebURL(ctx context.Context) (string, error)
    // Search queries ONE indexer's Torznab endpoint (/{id}/api). Never the aggregate search.
    // The query carries ID params the indexer supports, else a title query. ctx lives as long as the search:
    // the caller reports FAILED after the 60 s timeout but still takes a later answer.
    Search(ctx context.Context, indexerID int, q TorznabQuery) ([]TorznabItem, error)
    // RSS fetches an indexer's recent feed (Torznab search with no query).
    RSS(ctx context.Context, indexerID int, categories []int) ([]TorznabItem, error)
    // Download fetches a .torrent; returns MagnetRedirect when the URL 30x-redirects to a magnet.
    Download(ctx context.Context, link string) (TorrentPayload, error)
}
// Errors: ErrUnauthorized, ErrIndexerFailed{Code, Message} (Torznab <error>), ErrTimeout, ErrUnavailable.

// ports.go — implemented by internal/adapters/qbittorrent. Requires qBittorrent ≥ 5.2 (WebAPI ≥ 2.15).
type QBittorrent interface {
    Version(ctx context.Context) (WebAPIVersion, error) // startup check; < 2.15 → ErrUnsupportedVersion
    // AddTorrent adds a parsed .torrent or magnet in category "magpie" with stopCondition=MetadataReceived
    // (magnets) or stopped=true (.torrent files whose file list was already checked). Returns the info hash.
    AddTorrent(ctx context.Context, t AddRequest) (InfoHash string, err error)
    Start(ctx context.Context, hashes ...string) error
    Delete(ctx context.Context, deleteFiles bool, hashes ...string) error
    // Files returns the file list; empty until has_metadata.
    Files(ctx context.Context, hash string) ([]TorrentFile, error)
    // Sync returns incremental state since rid (sync/maindata), merged by the adapter.
    Sync(ctx context.Context, rid int64) (SyncState, error)
    // SetShareLimits always sends the full parameter set (ratio, seeding time, inactive time, action, mode).
    SetShareLimits(ctx context.Context, hash string, l ShareLimits) error
    // FreeSpace is the free bytes on the download disk (sync/maindata server_state.free_space_on_disk).
    FreeSpace(ctx context.Context) (int64, error)
}
// Auth: API key (Authorization: Bearer). Errors: ErrUnauthorized, ErrTorrentNotFound (hash gone), ErrUnavailable.

// ports.go — implemented by internal/adapters/trash. Reads TRaSH Guides' public JSON (docs/json/{radarr,sonarr}).
type TrashGuides interface {
    CustomFormats(ctx context.Context, app TrashApp) ([]TrashCustomFormat, error)     // cf/: trash_id, trash_scores, specs
    QualityProfiles(ctx context.Context, app TrashApp) ([]TrashQualityProfile, error) // quality-profiles/: items, cutoff, scores, formats (trash_ids)
    QualitySizes(ctx context.Context, app TrashApp) ([]TrashQualitySize, error)       // quality-size/movie.json or series.json: MB/min per quality
}
type TrashApp string // RADARR | SONARR
// Errors: ErrUnavailable.
```

- The TRaSH adapter lists each directory through GitHub's contents API and downloads the files from raw.githubusercontent.com, so a sync costs six API calls. Its tests run against recorded files in `internal/adapters/trash/testdata`.
- The adapter maps Sonarr's and Radarr's condition enums to Magpie values: sources to the parser's source names, plus `BLURAY_RAW` and `TV_RAW` for Sonarr's raw sources (Remux, Raw-HD), so its "Bluray but not Remux" formats keep their meaning; language IDs to ISO codes, with `ORIGINAL` for the title's original language; indexer flags to `FREELEECH` / `INTERNAL`; release types to `SINGLE_EPISODE` / `MULTI_EPISODE` / `SEASON_PACK`. A format with a condition it can't map (for example `exceptLanguage`) is left out with a warning.
- `TrashQualityProfile.Formats` lists the trash_ids of the formats the guide profile scores. Each score is that format's `trash_scores[ScoreSet]`, else its `default`.

- The qBittorrent "hash gone" error is `ErrTorrentNotFound`, so it isn't confused with `api.ErrNotFound`.
- Port data types (`ProwlarrIndexer`, `TorznabQuery`, `TorznabItem`, `TorrentPayload`, `AddRequest`, `TorrentFile`, `SyncState`, `ShareLimits`, the TRaSH types) are spelled out in `internal/acquisition/ports.go`.

The adapter maps remote paths to local ones, so `DownloadCompleted.ContentPath` is always a path the server can read.

## HTTP surface

Owned by Acquisition, under `/api/acquisition`, in the OpenAPI spec.

| Method | Path | Purpose |
|---|---|---|
| POST | `/searches` | Start a search `{title, episodes?, interactive}` → `201` with the search state below and its `searchId`. Results arrive over the live WebSocket. `interactive: false` runs a USER_AUTO search (the title page's Search button and the episode-row search icon): it grabs on its own and ignores the delay. |
| GET | `/searches/{id}` | The cached state for reconnect/refresh: indexers with `PENDING`/`DONE`/`FAILED`/`SKIPPED` and message, releases in arrival order with current ranks, `completed`, `cancelled`, and the profile's buckets (allowed items in order, quality IDs, `meetsCutoff` on the cutoff item and those above it) and penalized formats (scored below 0). 404 `search-not-found` once expired. |
| POST | `/searches/{id}/grab` | Grab `{releaseId, override?}` from the cached results → 202 `{status, download?, blockedFile?, next?}`. `status`: `CHECKING` (a magnet waits for its file list), `DOWNLOADING`, or `REJECTED` (a .torrent held a Blocked file; `next` is the fallback grab). 404 `release-not-found`; 409 `release-rejected` for a rejected release without `override`, `release-already-grabbed` for a release tried before in this search; 502 `release-unavailable` (Prowlarr couldn't fetch it), `qbittorrent-unavailable`, `qbittorrent-unsupported` (below 5.2) |
| DELETE | `/searches/{id}` | Cancel in-flight indexer queries; indexers still asked report `FAILED` ("Search cancelled") and `SearchCompleted` follows. Results stay cached. |
| GET | `/downloads` · DELETE `/downloads/{id}?deleteData=` | Queue view: downloads not yet removed, newest first, with title name and the last polled state, progress, speed, ETA and ratio. Remove (204) deletes the torrent, with its data when `deleteData`, and blocklists nothing |
| GET | `/qualities` | Radarr's 30 qualities |
| GET/POST/PUT/DELETE | `/profiles`, `/custom-formats` | Profile and CF CRUD; `POST /profiles/from-preset {preset}`. `GET /profiles` is open to every signed-in user (profile pickers); the rest is admin-only. Deleting the default profile is a 409. |
| POST | `/custom-formats/block-term` | `{term}` → a format matching release names that contain the term (literal, any case), scored −10000 in every profile |
| POST · GET | `/trash/sync` · `/trash/status` · `/trash/profiles` | Queue a TRaSH sync (202, runs as a job); last attempt, last success and error; guide profiles as of the last sync |
| GET · PUT | `/indexers` · `/indexers/{id}` | List (from Prowlarr, gone ones left out, with the last sync error and Prowlarr's URL) and per-indexer settings |
| POST | `/indexers/sync` | Pull the list from Prowlarr now; 502 `prowlarr-unavailable` / `prowlarr-unauthorized` on failure |
| GET · PUT | `/indexer-options` | Sync interval, search timeout and result cache (apply to every indexer) |
| GET · DELETE | `/blocklist` · `/blocklist/{id}` | Every entry with its title name, newest first, expired temporary ones included; remove (204) |
| GET · PUT | `/download-options` | Settings → Downloads: `blockedFilesEnabled`, `blockedPatterns` (glob or `regex:`; must compile), `metadataTimeoutMinutes` (1–60), `stalledLimitMinutes` (30–4320), and the global seeding settings `seedingMode` (`SEED_UNTIL_GOALS` \| `DONT_SEED`), `seedRatio` (0–100), `seedDays` (0–365). 400 on invalid values |
| GET · PUT | `/settings` | Delay and settle window. Quality fields so far: `defaultProfileId` (must exist), `defaultDelayMinutes` (0–1440), `settleWindowSeconds` (5–120), `trashSyncEnabled` |
| GET · PUT | `/search-options` | Settings → Downloads: `searchOnAdd` ("Search when a title is added", default on) |
| GET | `/cutoff?title={id}` | Title page → Files: each scored file's `{file, quality, cfScore, cutoffMet, upgradesAllowed}` (the Reader's `CutoffStatus` per file). 400 `title-required` without a title (admin) |
| GET · PUT | `/schedules` | Settings → General → Schedules: `rssIntervalMinutes` (10–120), `missingSweepDays` (1–7), `cutoffSweepDays` (1–30); the response adds the fixed `pendingCheckMinutes`. 400 on values out of bounds |

## Behaviour

### Indexers (Prowlarr)

- **Indexer list:** pulled from Prowlarr hourly (setting), once at startup, and on demand. Indexers are configured only in Prowlarr; Magpie stores priority, minimum seeders, seeding override and goals.
  - Only enabled torrent indexers are listed. Disabled and usenet indexers count as removed.
  - New indexers get default settings (priority 25, minimum seeders 1, `USE_GLOBAL`).
  - Listed indexers get fresh name, privacy, caps and average response. Magpie's settings are never touched by a sync.
  - Removed ones are marked `GONE`; the row and its settings stay, and come back if the indexer returns.
  - A failed pull (Prowlarr unreachable, bad API key) changes no rows; the error is shown on the Indexers page.
- **Health:** an indexer is `FAILING` while Prowlarr has backed it off after repeated failures (`disabledTill` in the future) or its caps call returns a Torznab `<error>`. Its last error and failing-since time are shown; its last known caps are kept. `last_ok_at` is the last sync or search where it was healthy.
  - A search answer writes the same health as a sync: an error or a timeout makes the indexer `FAILING` with that error, an answer clears it and moves `last_ok_at`. Answers from the cache write nothing.
- **Fan-out:** a search queries every enabled indexer whose caps cover the media kind, in parallel, one goroutine per indexer. Each result goes out as `SearchResultsReceived` as soon as that indexer answers.
- **Query tiers per indexer:**
  1. ID search with whatever the caps support: `tmdbid`, `imdbid`, `tvdbid` (from `catalog.Reader` external IDs).
  2. Title + year (movies) or title + season/episode (TV) when tier 1 is unsupported or returns nothing. Year, season and episode go in the query text (`Dune 2024`, `Severance S02E03`) when the caps lack those params.
  - Both tiers ask for the caps' categories in the kind's Newznab range (2000–2999 movies, 5000–5999 TV), or none.
  - A series search narrows to a season when every searched episode is in it, and to the episode when only one was searched.
  - A series search spanning several seasons, or the whole series, runs the tiers once per season (one season's episodes each), never as one query without a season: indexers answer that with a page of mixed episodes. The indexer's answer is every season's releases together.
  - No ID support and no title results → status `SKIPPED`.
- **Timeout:** 60 s per indexer. A late answer still streams into an open search; a timed-out indexer reports `FAILED`, and its late answer is a second `SearchResultsReceived` with `DONE`.
- **Cache:** results are kept in memory per `(indexer, query)` for 30 min, and per search for the "next best release" fallback. A restart loses it; the fallback then runs a fresh search.
  - A search's results stay for the result-cache time from its start; with the cache off (0) they still stay 30 min, so an open dialog can refresh. Expiry cancels whatever is still running.
  - Release IDs are minted per `(search, indexer, guid)`; a guid an indexer repeats is kept once.
- **Backoff:** an indexer failing repeatedly is put in backoff (`BlockedIndexer`, temporary rejection) and shown as failing.

### Parsing and quality

- **Parser:** Sonarr/Radarr's release-title parser, ported to Go (allowed under GPL-3.0). Their ~2.7k fixture titles are the parser's test suite.
- **Quality** = source × resolution + revision (proper/repack/real/version). Everything else (codec, HDR, audio, group tier) comes from **custom formats**.
- **Custom formats:** specs compile with `dlclark/regexp2` (case-insensitive) with a match timeout of 100 ms per regex. A timeout counts as "no match" and logs a warning. There are no release profiles; "must not contain" is a CF with a large negative score. The UI's "block term" creates one.
  - Matching follows Sonarr: specs of one kind are OR'd unless one is required, which must then match; kinds are AND'd; `negate` inverts a spec. A format with no specs never matches.
  - Title regexes run on the release name, group regexes on the parsed group, edition regexes on the parsed edition. A release that names no language counts as the title's original language.
- **Evaluation** is a pure function, `Evaluate(release, profile) → {Quality, QualityRank, Formats, Score, Allowed, CutoffMet}` in `internal/acquisition`. Rank follows item order (higher is better, group members share a rank, 0 = not in the profile); `Allowed` is the item's flag; `Score` sums the profile's scores of every matching format. The minimum-score check and the rest of the decision engine build on it.
- **TRaSH sync:** daily while the setting is on, plus manual. Formats and profiles are fetched and matched by `trash_id`, else by name (Sonarr/Radarr Remux names are aliased). Scores the user edited (`user_override`) are never overwritten.
  - Radarr's guide is applied first, then Sonarr's. Movies and TV share one pool, so when both guides have a format of the same name, Radarr's definition is kept and Sonarr's stands for it; Sonarr-only formats (season pack, multi-episode, …) are added.
  - A stored format without a `trash_id` and with the same name is linked to TRaSH instead of duplicated.
  - A profile is linked to a guide profile by `trash_profile_id`, else by name; it gets that guide profile's formats and score set. A preset profile gets its preset's formats. Item structure is never changed by a sync.
  - A score is `user_override` when it differs from the format's TRaSH score for the profile's score set; setting it back to TRaSH's value clears the flag. Every other score of a TRaSH-linked format follows TRaSH.
  - Fetching runs before any write; a failed fetch changes nothing and records the error for the UI.
  - The first sync is started by the first-run wizard's quality step or **Sync now**; boot never calls GitHub.
- **First-run presets** are resolution-only: `720p`, `1080p`, `2160p` ([ticket 31 follow-up](../../../.scratch/v1-spec/issues/29-assemble-spec.md)).
  - Each preset is one quality group containing every source at that resolution (WEB-DL, WEBRip, HDTV, Bluray, Remux), and that group is the cutoff. The first accepted release at that resolution meets cutoff; the file then transcodes and upgrades stop.
  - Source preference inside the group comes from TRaSH custom-format scores during selection (ranking: CF score breaks the tie inside a group).
  - No lower-resolution stopgap. Imported Sonarr/Radarr profiles keep their own structure.
  - The first start creates all three and makes `1080p` the default profile for new requests. Later starts don't recreate deleted presets.
  - Group members: every quality at the resolution with source WEB-DL, WEBRip, HDTV or Bluray, without BR-DISK and Raw-HD (Remux included). Sizes: min = the lowest TRaSH minimum of the members across the movie and series tables (720p 10, 1080p 12.5, 2160p 25 MB/min), preferred 1999, max 2000. Upgrade-until 0, increment 1, propers `PREFER_AND_UPGRADE`.
  - Preset formats, by TRaSH name, at TRaSH's default scores: Remux Tier 01–03, WEB Tier 01–03, WEB Scene, Repack/Proper, Repack2, Repack3, BR-DISK, LQ, LQ (Release Title), 3D, Extras, Upscaled, Bad Dual Groups; plus HD Bluray Tier 01–03 and x265 (HD) for 720p/1080p, UHD Bluray Tier 01–03 for 2160p. They are attached when the preset is created and on every sync.
- **Sizes:** MB per minute × runtime (from `catalog.Reader`), set per profile item. Runtime 0 → rejected with `UNKNOWN_RUNTIME`, except episodes airing in the last 24 h.

### Decision engine

Every release goes through the same specs, whatever triggered the search. Each spec decides whether it applies to the trigger. All specs in tier 0 run and every rejection is collected; tier 1 (disk) runs only if tier 0 passed. A rejection is **permanent** or **temporary**; temporary rejections go to `pending_releases`.

| Spec (tier 0 unless noted) | Kind | Applies to |
|---|---|---|
| UnknownTitle / UnknownEpisode / UnableToParse | P | all |
| QualityAllowedByProfile | P | all |
| CustomFormatAllowedByProfile (score ≥ min) | P | all |
| ReleaseLanguage (`LANGUAGE_NOT_WANTED`: audio in the original or a wanted language) | P | all |
| AcceptableSize (profile item MB/min × runtime) and MaximumSize (global) | P | all |
| NotSample, RawDisk, SplitEpisode | P | all |
| Blocklisted (by info hash; else name + indexer) | P | all |
| AlreadyImported | P | all |
| TorrentSeeding (seeders ≥ indexer minimum; AUTO, USER_AUTO and REGRAB also need `MinAutomaticSeeders` = 3) | P | all |
| BlockedIndexer (backoff) | T | all |
| UpgradeAllowed / UpgradeDisk (upgrade rule below; file transcoded → reject) | P | all but REGRAB |
| Repack (same group as existing file) and Proper (7-day window) | P | all but REGRAB / RSS |
| SameEpisodesGrab, Queue (a download already meets cutoff or is better), History | P | all / RSS |
| FullSeason (season pack only once every episode has aired) | P | all |
| MultiSeason: accepted only when every contained season is wanted | P | all |
| SeasonPackOnly (season search after the season ended) | P | search except interactive |
| Monitored | P | all except interactive |
| Searched title/season/episode match | P | search |
| Released (`catalog.Reader` release date passed) | P | auto/RSS |
| Delay (delay profile wait unless bypass) | T | auto/RSS |
| Pending (a pending release already covers these items) | T | auto/RSS |
| FreeSpace (tier 1) | P | all |

Dropped:
- Usenet-only specs: MinimumAge, Retention, Protocol.
- Anime and scene-mapping specs.
- AirDate restriction, and IndexerTag scoping.

How the specs read their inputs:
- UnknownTitle compares the parsed title and its aliases with the title's name, original name and aliases after Sonarr's cleaning (case, punctuation and the words a/an/the/and/or/of dropped); a movie's year may be off by one.
- MaximumSize reads `acquisition.maximumSize` (bytes, 0 = no cap; no UI yet). NotSample rejects a release named "sample" under 70 MB. RawDisk rejects `BDMV` / `VIDEO_TS` in the name.
- AlreadyImported matches the info hash against the title's library files (`library.Reader`). The upgrade, Repack, Proper and SameEpisodesGrab specs score each library file the release would replace by its original release name, else its path, with the library's quality.
- ReleaseLanguage reads the parsed audio languages. A release passes when any of them is the title's original language or one of its wanted languages (`catalog.Reader.WantedLanguages`), the same languages Transcode keeps. A release naming no language counts as the original; "orig" or German "DL" tags count as the original too. A multi-language release such as `ITA.ENG` passes for an English title. No original language and no wanted languages skips the check. Message: "Audio in Italian, not English".
- TorrentSeeding reads the release's own seeder count. A grabbing search (AUTO, USER_AUTO, REGRAB) also needs `MinAutomaticSeeders` (3): an older release with fewer is usually dead and stalls. RSS keeps the indexer minimum only, since its releases are minutes old. Interactive keeps the indexer minimum, so the dialog still lists everything.
- BlockedIndexer rejects releases from an indexer that was `FAILING` when the search started.
- Queue reads the title's downloads still in qBittorrent (awaiting metadata, downloading or completed); History reads its grabs from the last 12 h. Failed downloads count for neither. Each is scored against the search's profile by its release name and stored quality. Pending reads the title's `pending_releases`; like Delay it lets a release that bypasses the delay through.
- FreeSpace asks qBittorrent for the download disk's free space once per search and rejects a release that would leave less than 100 MB. Unknown free space skips the check.
- Every rejection carries a message for the dialog, such as "WEBDL-720p isn't wanted by 1080p".

**Ranking** (accepted only): quality rank (profile order; members of a group are equal) → CF score → episode count (more episodes first, a season pack first at equal count) → indexer priority (lower first) → seeders (log10 buckets) → size closest to preferred (200 MB buckets; largest first when the profile has no preferred size) → arrival. The order is total, so earlier releases never swap places as more arrive; `ReleaseDecision.Rank` is the position among accepted releases when the event is sent.

**Automatic ranking** (every search but interactive, and every automatic pick: the settle grab and the fallback) weighs seeders more:
- **Agreed seeders.** A release listed by several indexers counts with the lower median of their seeder counts. Copies match by info hash, by name (cleaned as for titles), or by size and quality. One indexer reporting 644 seeders where the others report 0 and 1 counts as 1; with two copies the smaller count wins; a release one indexer lists keeps its own count.
- **Floor.** A grabbing search (AUTO, USER_AUTO, REGRAB) skips a release whose agreed seeders are below `MinAutomaticSeeders` (3), even when its own count passed TorrentSeeding.
- **Seeder tier** after CF score and before episode count: well seeded (≥ `WellSeededSeeders` = 10), weak (3–9), below 3. A season pack with 4 seeders ranks after a single episode with 200 at the same quality and score; the remaining episodes are grabbed one by one.
- The rest of the order is as above, with the agreed seeders in the log10 tiebreak. As agreed counts change with each answer, an automatic search's ranks may swap; the interactive order never does.

**Upgrade rule**, in order:
1. Better quality while below cutoff → upgrade.
2. Worse → reject.
3. Same quality, higher revision → upgrade, unless propers are `DO_NOT_PREFER`.
4. Upgrades disabled → reject.
5. Lower revision → reject.
6. Better quality but cutoff met → reject.
7. New CF score ≤ current → reject.
8. Current ≥ upgrade-until → reject.
9. New < current + increment → reject.
10. Otherwise upgrade.

A file with `transcoded = true` is never upgraded automatically; only REGRAB or an interactive grab replaces it.

**Cutoff met** = quality ≥ cutoff AND CF score ≥ upgrade-until. Evaluated on every `FileImported`, on `ProfileChanged` for that Title, and when a profile's contents change (re-evaluated for files using it, emitting `CutoffMet` for newly met files). The resolution presets set upgrade-until to 0, so reaching the resolution is enough.
- Editing or deleting a profile queues the job `acquisition.rescore-files` for the files scored under it. Editing or deleting a custom format, blocking a term and a TRaSH sync queue it for every file. The job rescores one title per transaction from each row's stored release name (else path), quality and size.
- A transcoded file keeps its original score and is never published again.
- The weekly cutoff-unmet sweep reads `file_scores`: rows below the cutoff, not transcoded, under a profile that allows upgrades, whatever their origin.

### Search and grab

- **Triggers:**
  - AUTO: on title add, on `WantedReleased`, and in backlog sweeps.
  - USER_AUTO: the title page's Search button.
  - INTERACTIVE: the search dialog.
  - RSS: every 15 min per indexer.
  - REGRAB: issue action.
- **Automatic grab and the settle window** (AUTO, USER_AUTO, REGRAB and RSS):
  - Wait until every indexer answered or 30 s passed (setting), then grab the top accepted release. When the window passes with nothing accepted, the next accepted answer or the last indexer settles it.
  - Grab early when the top release meets the quality cutoff with the profile's **maximum attainable CF score**, because nothing can beat it. The maximum attainable score is the sum of the profile's positive format scores, an upper bound, so an early grab never passes over a better release.
  - Grab each item once per batch; a pack covers its episodes. After the best release, the best one covering only items no grab covers yet is taken, and so on. Each grab goes through `DownloadService`'s blocked-file check and fallback.
  - **Grab-time recheck.** The decision input is read once, when the search starts. Right before each automatic or fallback grab, Blocklisted, Queue and (RSS) History run again on the title's blocklist and downloads as they are then; a release they reject is skipped, not blocklisted, and the next one is tried. Downloads the same grab is dropping (a stall or failure falling back) and the search's removed downloads don't count as queued. A user's own grab from the dialog isn't rechecked.
  - **One grab at a time per title.** The settle grab, a user grab and a drop's fallback hold a per-title lock from the recheck to the stored download, so two searches settling at once can't both pass the recheck. The lock lives in memory; Magpie runs one server.
  - **Ended season, nothing accepted.** When a grabbing search of a whole season that has ended (more than one episode) accepts nothing and holds nothing, one search per episode of that season is queued with the same trigger, `MinSweepGap` (4 s) apart. Such a season search wants a pack (SeasonPackOnly), and an old season's pack is often missing or below the profile's size floor. An episode search isn't a season search, so it accepts singles.
  - The grab runs as a job (`acquisition.automatic-grab`) once the search settles. `SearchCompleted` waits for it, so its outcome says `GRABBED` or `PENDING_DELAY`.
  - RSS and the pending queue decide their releases as quiet batches: a `searches` row, no `SearchStarted`/`SearchCompleted`; only their grabs (`ReleaseGrabbed{Trigger}`) show.
- **Interactive:**
  - Nothing is grabbed automatically. The user grabs any accepted release.
  - A rejected one can be grabbed with `override=true`. Override skips the decision engine, but the blocked-file check still runs.
  - A series is searched by season or by episode, never whole: the dialog sends the season's or the episode's IDs. A season search accepts that season's packs and single episodes, so one episode can be grabbed from an ended season; an episode search accepts any release covering it, packs included. Other episodes are rejected `SEARCH_MISMATCH`.
- **Delay profiles:**
  - Wait N minutes from the indexer's publish time for a better release (default 0 = off; the 30 s settle window still applies).
  - Bypassed when the release meets cutoff, or its CF score ≥ the bypass threshold (the maximum attainable CF score, when the profile scores anything).
  - User-invoked searches ignore the delay.
  - A release rejected only by Delay or Pending is stored in `pending_releases` for items no grab covers; a grab drops the held releases of the items it covers.
  - The pending queue is checked every minute; when the oldest pending release for an item passes the delay, every pending release covering that item is decided again without the delay and the best is grabbed. Unmonitoring an item or removing the title drops its pending releases.
- **RSS:** every indexer's feed (movie and TV categories) is read as its own job. Items are parsed and matched to monitored titles that want something (`catalog.Reader.Wanted`) by cleaned title, original title and aliases, and the year for movies. They then go through the same engine with the RSS trigger, which checks S/E numbering after the episode group, monitoring and release dates.
- **Backlog sweeps:** missing items daily, cutoff-unmet items weekly (files below cutoff, not transcoded, profile allows upgrades). One AUTO search per movie or per season, queued 4 s apart (two query tiers × 2 s), so each indexer gets at most one query per 2 s on average.
- **Search jobs** are keyed by `(title, episodes, trigger)` in `search_jobs`: a redelivered `TitleAdded`, `WantedReleased` or `MonitoringChanged`, or a sweep, never queues a second job while one with the key waits. A job whose title or episodes are gone ends quietly.

### Grab and blocked-file policy

The check is on by default and can be turned off in settings.

1. **.torrent:**
   - Download through Prowlarr and parse the info dict locally.
   - If any file matches a blocked pattern, reject: blocklist the info hash permanently (`BLOCKED_FILE`), publish `ReleaseRejected`, and fall back (step 4).
   - Otherwise add stopped, then start.
2. **Magnet** (or a .torrent link that redirects to one):
   - Add with `stopCondition=MetadataReceived`.
   - Poll until `has_metadata`, then `Files` and the same check.
   - Pass → `Start`. Fail → `Delete(deleteFiles=true)` and the rejection path above.
   - No content bytes are fetched before the check.
3. **Metadata timeout:**
   - No metadata after 5 min (setting) → delete, then a **temporary** blocklist entry (`METADATA_TIMEOUT`, expires after 24 h), publish `ReleaseRejected`, fall back.
   - The clock starts at the first poll that sees the torrent and restarts at every poll that finds it waiting (`queuedDL`, `checkingDL`, `checkingResumeData`): a magnet queued behind qBittorrent's active-download limit isn't fetching yet. It is the download's progress mark, so it survives a restart.
4. **Fallback:**
   - Take the next accepted, not-yet-tried release from the search's cached ranking. Grab it and set `ReleaseRejected.Next`.
   - A release is tried once per search, whether the user grabbed it or the fallback did; the same info hash from another indexer counts as tried. A fallback .torrent that is blocked in turn is rejected the same way and the next one is taken.
   - Cache empty or expired → enqueue a fresh AUTO search.
   - A rejected download is kept with state `FAILED` and `removed_at` set, so it leaves the queue view.
5. **Patterns:**
   - Editable glob or `regex:` list.
   - Default: `*.exe *.msi *.bat *.cmd *.com *.scr *.lnk *.ps1 *.vbs *.js *.jar *.apk *.dmg *.iso *.rar *.r[0-9][0-9] *.zip *.7z`.
   - Matching is case-insensitive on the file name; folder names are not matched.
6. **Any** blocked file rejects the whole torrent. Files are never skipped selectively.

### Downloads and seeding

- **Polling:** `Sync` every 2 s while any download is active, 15 s when idle. Changes emit `DownloadProgressed` (ephemeral).
- **Completion:** at 100% (`uploading`/`stalledUP`/`stoppedUP`):
  1. **Blocked-file backstop:** re-check the final file list (priority > 0) against the blocked patterns. A match is handled exactly like a `BLOCKED_FILE` rejection: delete with data, permanent blocklist, `ReleaseRejected`, fallback. No `DownloadCompleted`.
  2. Otherwise publish `DownloadCompleted` once. `Files` = files with priority > 0 (Library drops samples and unknown extensions). `SeedingMode` = the indexer override, else the global default.
- **Failure:**
  - `error` or `missingFiles` state → `DownloadFailed`, permanent blocklist (`DOWNLOAD_FAILED`), delete with data, fall back.
  - Stalled (no progress) for longer than the stalled limit (setting, default 6 h) → `DownloadFailed`, **temporary** blocklist (`STALLED`, expires after 24 h), delete with data, fall back.
  - The stall clock is the download's progress mark (`download_marks`): it restarts when progress moves or qBittorrent has the torrent waiting (`queuedDL`, `checkingDL`, `checkingResumeData`), and is read back after a restart. A moving mark is stored at most once a minute (`MarkSaveInterval`), so after a restart a clock reads at most a minute long.
- **SEED_UNTIL_GOALS:**
  - After `FileImported`, keep seeding.
  - Every 5 min (the `acquisition.seeding-goals` periodic job), check ratio and seeding time of each `IMPORTED` download against the indexer's goals, else the global goal (`acquisition.seedingGoal`, default ratio 2.0 or 14 days, whichever first). The indexer's ratio and days override the global ones separately. When either goal is met: `Delete(deleteFiles=true)` and set `removed_at`; the state stays `IMPORTED`. The library copy is a hardlink or copy, so it survives.
  - The Downloads tab shows an `IMPORTED` download as Seeding while qBittorrent uploads it.
- **DONT_SEED:** after `FileImported`, `Delete(deleteFiles=false)`. Library has already moved the files.
- **Manual import:** a download whose import waits for the user (`IMPORT_PENDING_USER`) keeps its torrent and data. The seeding-goal check skips it, the poll treats it as finished, the Downloads tab shows it as waiting, and the decision engine counts it as queued for its items, so nothing else is grabbed for them. When the user maps it, `FileImported` moves it to `IMPORTED` and the seeding mode applies; when the user removes it, `ImportRemoved` drops it like a failed import.
- Downloads not grabbed by Magpie (other categories) are ignored.
- The poll ticker only syncs, emits progress and enqueues one check job per download that needs action (metadata arrived, metadata timeout, completion, failure, stall). The job re-reads the state, so a repeated check changes nothing. A torrent missing from qBittorrent for over a minute is marked `REMOVED`.

## Test seeds

1. **Ranking:**
   - Given accepted releases A (WEB 2160p, CF 300) and B (WEB 1080p, CF 2300) in a profile ordering 2160p above 1080p, when ranked, then A ranks first (quality beats CF score).
   - Given A and B both WEB-DL 1080p in group `WEB 1080p` with CF 1700 and 300, then the 1700 one ranks first.
2. **Ranking tiebreak:** given two equal-quality, equal-CF releases from indexers with priority 10 and 25, then the priority-10 one ranks first. With equal priority, more seeders (log10 bucket) wins.
3. **Streaming:**
   - Given a search over 3 indexers answering at 1 s, 5 s and timing out at 60 s, then exactly 3 `SearchResultsReceived` events are emitted: 2 `DONE`, 1 `FAILED`.
   - Then `SearchCompleted.FailedIndexers` lists the third.
4. **Settle window:**
   - Given an AUTO search where the first indexer returns an accepted WEB 1080p release below cutoff at 1 s, then nothing is grabbed before 30 s or before all indexers answer.
   - When a better release arrives at 10 s and all indexers finish at 12 s, then the better one is grabbed at 12 s.
5. **Early grab:** given cutoff WEB 2160p and maximum attainable CF score 2300, when a WEB 2160p release with CF 2300 arrives at 1 s, then it is grabbed immediately, before the settle window.
6. **Interactive:**
   - Given an interactive search, then no automatic grab happens.
   - Given a release rejected for `QUALITY_NOT_ALLOWED`, when grabbed with `override=true`, then `ReleaseGrabbed.Override = true`.
7. **Blocked .torrent:**
   - Given a .torrent containing `Movie.exe`, when grabbed, then it is never added to qBittorrent.
   - Then a permanent blocklist row (`BLOCKED_FILE`) exists, `ReleaseRejected.BlockedFile = "Movie.exe"`, and the next cached release is grabbed.
8. **Blocked magnet:**
   - Given a magnet whose metadata lists `Show.S01E01.rar`, when metadata arrives, then the torrent is deleted with `deleteFiles=true`.
   - Then `Start` was never called, and the release is blocklisted.
9. **Metadata timeout:**
   - Given a magnet with no metadata after 5 min, then it is deleted and a temporary `METADATA_TIMEOUT` row expiring in 24 h is written.
   - Given an empty search cache, then an AUTO search is enqueued.
10. **Blocked check off:** given the setting disabled, when a .torrent with `.rar` is grabbed, then it is added and started.
11. **Blocklist match:**
    - Given info hash H is blocklisted for title T, then any release with hash H for T is rejected `BLOCKLISTED`.
    - The same hash for a different title is not rejected.
12. **Delay:**
    - Given a 2 h delay and an RSS release published 30 min ago below cutoff, then it goes to `pending_releases`.
    - After 2 h from publish, the best pending release for that item is grabbed.
    - A release meeting cutoff bypasses the delay.
13. **Released rule:** given a movie whose digital release date is in the future, when `TitleAdded` arrives, then no search is enqueued. When `WantedReleased` arrives, one AUTO search is enqueued.
14. **Redelivery:**
    - Given `TitleAdded` E1 already processed, when E1 is delivered again, then no second search job exists.
    - Given `FileImported` E2 delivered twice, then `CutoffMet` is published once.
15. **Cutoff:**
    - Given profile cutoff WEB 2160p / upgrade-until 2000, when `FileImported` brings WEB 2160p with CF 2300, then `CutoffMet` is published.
    - With CF 1700, then not, and the file is cutoff-unmet in the weekly sweep.
16. **Profile change:**
    - Given 5 files with WEB 1080p and a profile cutoff of WEB 2160p, when the cutoff is changed to WEB 1080p, then `CutoffMet` is published for each file whose CF score is ≥ upgrade-until.
17. **Upgrade after transcode:**
    - Given a file with `FileTranscoded` processed, when RSS finds a better release, then it is rejected (`UPGRADE_NOT_ALLOWED`).
    - When an `IssueActionRequested{REGRAB}` arrives for that file, then the source info hash is blocklisted and a REGRAB search runs and may grab.
18. **Upgrade rule:** given an existing WEB 1080p file with CF 1700, upgrade-until 2000 and increment 100, then:
    - A WEB 1080p release with CF 1750 is rejected (below increment).
    - CF 1800 is accepted.
    - A REPACK from a different group is rejected.
19. **Seed until goals:**
    - Given mode `SEED_UNTIL_GOALS` with goal ratio 2.0, when `FileImported` arrives, then the torrent stays.
    - When the ratio reaches 2.0, then `Delete(deleteFiles=true)` is called.
20. **Don't seed:** given the global mode `DONT_SEED`, when `FileImported` arrives, then `Delete(deleteFiles=false)` is called immediately.
    - Given an indexer override `SEED_UNTIL_GOALS`, then the torrent stays.
21. **DownloadCompleted payload:** given a pack with `S01E01.mkv` (priority 1) and `sample.mkv` (priority 0), then `Files` contains only `S01E01.mkv` and `SeedingMode` reflects the indexer override.
22. **Import failed:** given `ImportFailed{Manual:false}` for download D, then D's hash is blocklisted (`IMPORT_FAILED`), the torrent is deleted with data, and the next cached release is grabbed.
23. **ID-then-title:**
    - Given an indexer whose caps lack `tmdbid` but support `imdbid`, then the search query uses `imdbid`.
    - Given no ID support, the query uses title + year. Given zero results, the status is `SKIPPED`.
24. **qBittorrent version:** given WebAPI 2.11 (qBittorrent 5.0), when Acquisition starts, then the health check fails with `ErrUnsupportedVersion` and no grabs are attempted.
25. **Multi-season pack:** given seasons 1–2 wanted and season 3 unmonitored, then an S01–S03 pack is rejected; given all three wanted, it is accepted.
26. **CF regex timeout:** given a custom format whose regex exceeds the 100 ms match timeout on a title, then that format doesn't match and the decision still completes.
27. **Completion backstop:** given a magnet that passed the metadata check but whose final file list now contains `extras/setup.exe` with priority 1, when it completes, then no `DownloadCompleted` is published, the torrent is deleted with data, the hash is blocklisted permanently, and the next release is grabbed.
28. **Stalled:** given the stalled limit 6 h and a download with no progress for 6 h 1 min, then `DownloadFailed` is published, a `STALLED` blocklist row expiring in 24 h is written, and the next release is grabbed. Changing the setting to 12 h means a 7 h stall is left alone.
29. **Scan origin:** given `TitleAdded{Origin: LIBRARY_SCAN}` for a released movie, then no search is enqueued. Given `FileImported{Origin: LIBRARY_SCAN}` below cutoff, then the title appears in the next cutoff-unmet sweep and `CutoffMet` carries `Origin: LIBRARY_SCAN` when met.
30. **Upgrade cleanup:** given `FileImported{Replaces: F1}` for F2 and then `FileDeleted{File: F1, Reason: UPGRADED}`, then `file_scores` has F2 and not F1; redelivering `FileDeleted` changes nothing.
31. **Resolution preset:** given the `1080p` preset, when `FileImported` brings Bluray 1080p with CF 0, then `CutoffMet` is published; a WEB 2160p release is rejected `QUALITY_NOT_ALLOWED`; between two 1080p releases with CF 1700 (WEB) and 300 (Bluray) the WEB one ranks first.
32. **SearchCompleted outcome:** given an AUTO search with no accepted releases, then `SearchCompleted{Outcome: NOTHING_ACCEPTED}`; given a delay profile holding the best release, then `Outcome: PENDING_DELAY`; given a grab, then `Outcome: GRABBED` with `Grabbed` set. Given an interactive search with an accepted release and no grab, then `Outcome: RELEASES_FOUND`.
33. **ProfileChanged:** given a Title moved from profile `2160p` to `1080p` with a WEB 1080p file, when `ProfileChanged` arrives, then `CutoffMet` is published once.
34. **Indexer sync:**
    - Given Prowlarr lists a new indexer, when the sync runs, then it is stored with priority 25, minimum seeders 1, override `USE_GLOBAL`, status `OK` and its caps.
    - Given an indexer with user priority 7, when Prowlarr reports new caps, then the caps are updated and the priority stays 7.
    - Given an indexer missing from Prowlarr's list, then its row is kept with status `GONE` and its settings; it is left out of `GET /indexers`. When it reappears, it is `OK` with the same settings.
    - Given Prowlarr backs an indexer off and its caps call answers `<error code="429">`, then it is `FAILING` with that error and its last known caps are kept. A healthy sync clears the error.
    - Given Prowlarr is unreachable, when the sync runs, then no row changes and the Indexers page shows the error.
35. **First-run presets:**
    - Given a fresh install, when the server starts, then profiles `720p`, `1080p` and `2160p` exist, each one allowed group that is the cutoff, and the default profile is `1080p`.
    - Given the `1080p` preset, then WEB-DL, WEBRip, HDTV, Bluray and Remux 1080p releases are allowed and meet the cutoff, and 720p and 2160p releases are not allowed.
    - Given the user deleted the `2160p` preset, when the server restarts, then it isn't recreated.
36. **TRaSH sync:**
    - Given Radarr's and Sonarr's guides both have "WEB Tier 01" with different trash_ids, when the sync runs, then one "WEB Tier 01" (Radarr's) exists, and Sonarr-only formats such as "Season Pack" are added.
    - Given a stored "web tier 01" format without a trash_id, then the sync links it to TRaSH instead of adding a second one.
    - Given the `1080p` preset scores "WEB Tier 01" 1700 from TRaSH and the user changes it to 1234, when TRaSH changes its scores to 1800 and the sync runs again, then "WEB Tier 01" stays 1234 and "BR-DISK" becomes 1800.
    - Given a profile named "Remux-1080p + WEB" and a Sonarr guide profile "Bluray-1080p Remux + WEB", then the sync links them by aliased name and scores the guide profile's formats; renaming the profile keeps the link.
    - Given TRaSH is unreachable, when the sync runs, then no format or score changes and the status shows the error.
37. **Block term:** given three profiles, when the term `HC.Subs` is blocked, then each profile scores the new format −10000, and a release named `….HC.Subs.…` scores −10000 while `….HCxSubs.…` scores 0.
38. **Manual import waits:** given `ImportFailed{Manual:true}` for completed download D with a cached next release, then D is `IMPORT_PENDING_USER`, its torrent is not deleted, no blocklist row or `ReleaseRejected` exists, and nothing is grabbed. When D's ratio passes the seeding goal, the seeding-goal check still leaves it. A release covering D's items is rejected `ALREADY_QUEUED`.
39. **Manual import mapped:** given D in `IMPORT_PENDING_USER` and mode `DONT_SEED`, when `FileImported{Download: D}` arrives, then D is `IMPORTED` and `Delete(deleteFiles=false)` is called.
40. **Manual import removed:** given D in `IMPORT_PENDING_USER` and a cached next release, when `ImportRemoved{D}` arrives, then D's hash is blocklisted (`IMPORT_FAILED`), the torrent is deleted with data, D is `IMPORT_FAILED` with `removed_at`, and the next release is grabbed. Redelivering `ImportRemoved` changes nothing.
41. **REGRAB once per issue:** given a REGRAB for issue I that blocklisted the file's hash and whose search ran, when `IssueActionRequested{REGRAB}` for I arrives again, then no second `ISSUE` entry and no second search exist.
42. **REGRAB past a removed waiting download:** given D in `IMPORT_PENDING_USER` covering the items, when `IssueActionRequested{REGRAB, RemovedDownloads: [D]}` arrives, then its search rejects no release `ALREADY_QUEUED`; without `RemovedDownloads` every release covering D's items is rejected `ALREADY_QUEUED`.
43. **Scoped interactive search:** given season 1 has ended, an interactive search of season 1 accepts an S01 pack and S01E02 and rejects S02E02 `SEARCH_MISMATCH`; a USER_AUTO search of season 1 rejects S01E02 `SEASON_PACK_ONLY`. An interactive search of S01E02 accepts the S01 pack and S01E02 and rejects S01E03 `SEARCH_MISMATCH`.
44. **Two searches settle at once:** given two AUTO searches of a movie that both accepted the same releases, when their automatic grabs run at the same time, then one release is grabbed; the other search's recheck finds it queued. Given a release blocklisted after the search started, a fallback skips it.
45. **Agreed seeders:** given one release listed with 644 seeders on one indexer and 0 and 1 on two others, and another release with 40 on one indexer, then an AUTO search grabs the 40-seeder release. Interactive search still lists the 644 copy with its own count.
46. **Automatic seeder floor:** a release with 2 seeders is rejected `NOT_ENOUGH_SEEDERS` in AUTO, USER_AUTO and REGRAB searches and accepted in interactive and RSS ones (indexer minimum 1).
47. **Seeder tier:** at equal quality and score, an S01 pack with 4 seeders ranks above S01E02 with 200 interactively, and below it in the automatic order.
48. **Release language:** for an English title, `iTALiAN` is rejected `LANGUAGE_NOT_WANTED` ("Audio in Italian, not English") in every search, interactive included; `ITA.ENG`, `GERMAN.DL` and a name without a language are accepted. With Italian wanted, `iTALiAN` is accepted.
49. **Ended season fallback:** given an ended three-episode season 1 and an indexer offering only S01E01, when an AUTO search of the season accepts nothing, then three AUTO searches of one episode each are queued 4 s apart. Given a season pack is offered, it is grabbed and no episode search is queued.
50. **Per-season queries:** given a series with seasons 1 and 2, a USER_AUTO search of the whole series queries each indexer once per season, never without a season.
51. **Clocks across restarts and queueing:** given a download stalled at 40 % for 5 h, when the server restarts, then it fails 1 h 1 min later. A magnet qBittorrent keeps `queuedDL` for twice the metadata timeout is left alone, then times out 5 min after qBittorrent starts fetching it; a download kept queued past the stalled limit isn't dropped.
