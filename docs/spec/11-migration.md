# Migration from Sonarr and Radarr

A one-time import of an existing Sonarr v4 / Radarr setup into Magpie.

Decided in:
- [Sonarr/Radarr one-time import mapping](../../.scratch/v1-spec/issues/28-arr-import-mapping.md) (field-level detail: [research 28](../../.scratch/v1-spec/research/28-arr-import-mapping.md))
- [Import wizard decisions and model gaps](../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md)
- [Import and library management](../../.scratch/v1-spec/issues/18-import-and-library.md)

Out of scope:
- Prowlarr: Magpie reads it directly ([ticket 22](../../.scratch/v1-spec/issues/22-indexer-engine.md)).
- Seerr requests, Sonarr v3, Usenet data, anime/scene numbering, naming formats, import lists, notifications, download clients, remote path mappings.

## Shape

- The import is a **platform job** (`internal/platform/migrate`), not a context. It reads Sonarr/Radarr through their v3 APIs (read-only, `X-Api-Key`) and TMDB through the Catalog TMDB port.
- It writes **only** through each context's import entrypoint below, never to another context's tables. This is the single sanctioned exception to "no cross-context writes" ([00-context-map.md](00-context-map.md#package-rules)).
- It runs in two phases:
  1. **Dry run:** read everything, build the mapping in memory or scratch tables, show the report. Nothing is written.
  2. **Commit:** call each context's import entrypoint in the order of the table below. Acquisition CFs and profiles go first, because titles reference profiles. Each call is one transaction inside its context. A failure stops the job; already-committed contexts stay, and the job can be resumed from the failed step.
- Imported records carry `Origin: MIGRATION` so downstream handlers can tell them apart (see "Effects after commit").

## Import entrypoints (contract)

Each context exposes an `api.Importer` that the migration job calls. Entrypoints are idempotent on a source key (`domain.SourceKey`: `"sonarr:{id}"`, `"radarr:{id}"`), so a resumed run doesn't duplicate. `SourceKey` below means `domain.SourceKey`.

```go
// internal/catalog/api
type Importer interface {
    ImportTitles(ctx context.Context, in []ImportTitle) (ImportedTitles, error)
}
// ImportedTitles maps source keys to Magpie IDs: titles by their Source, episodes by theirs.
type ImportedTitles struct {
    Titles   map[SourceKey]domain.TitleID
    Episodes map[SourceKey]domain.EpisodeID
}
type ImportTitle struct {
    Source        SourceKey          // "sonarr:12" | "radarr:40"
    Kind          domain.MediaKind
    Tmdb          domain.TmdbID
    External      domain.ExternalIDs // keeps Sonarr's tvdbId / imdbId even when TMDB lacks the link
    Profile       domain.ProfileID   // already imported by Acquisition
    Monitored     bool
    MonitorNewSeasons bool
    EpisodeGroup  *string            // TMDB episode group id, when the user confirmed one
    Episodes      []ImportEpisode    // series only, already translated to Magpie numbering
    Tags          []string           // not stored: Magpie titles have no tags
    Aliases       []string           // Sonarr alternate titles, when the user opted in; kept across refreshes
    AddedAt       time.Time
}
type ImportEpisode struct { Source SourceKey; Season, Number int; Monitored bool }

// internal/acquisition/api
type Importer interface {
    ImportCustomFormats(ctx context.Context, in []ImportCustomFormat) (map[SourceKey]CustomFormatID, error)
    ImportQualityProfiles(ctx context.Context, in []ImportQualityProfile) (map[SourceKey]domain.ProfileID, error)
    ImportDelayProfiles(ctx context.Context, in []ImportDelayProfile) error
    ImportBlocklist(ctx context.Context, in []ImportBlockedRelease) error
}
// Input types (CustomFormatID, ImportCustomFormat, FormatSpec, ImportProfileItem, ImportQualityProfile,
// ImportDelayProfile, ImportBlockedRelease) are spelled out in server/internal/acquisition/api/importer.go.

// internal/library/api
type Importer interface {
    ImportRootFolders(ctx context.Context, in []ImportRootFolder) error
    ImportMediaFiles(ctx context.Context, in []ImportMediaFile) (map[SourceKey]domain.MediaFileID, error)
}
type ImportRootFolder struct { Source SourceKey; Kind domain.MediaKind; Path string } // Path after rewrite
type ImportMediaFile struct {
    Source      SourceKey
    Title       domain.TitleID
    Episodes    []domain.EpisodeID
    Path        string            // after path rewrite
    Size        int64
    Quality     domain.Quality    // Magpie (Radarr) quality IDs
    ReleaseName string            // *arr sceneName
    ReleaseGroup string
    Languages   []domain.Language
    AddedAt     time.Time
    Rename      bool              // move to the fixed naming with its sidecars (the "rename folders" option)
}

// internal/activity/api
type Importer interface {
    ImportTimeline(ctx context.Context, in []ImportTimelineEvent) error // history rows → timeline/history
}
type ImportKind string // GRABBED | IMPORTED | DOWNLOAD_FAILED | FILE_DELETED (history table below)
type ImportTimelineEvent struct {
    Source      SourceKey          // "sonarr:history:812"
    Title       domain.TitleID
    Episodes    []domain.EpisodeID // empty for movies
    Kind        ImportKind
    ReleaseName string
    Reason      string             // failure or deletion reason, when the row has one
    At          time.Time          // original timestamp
}
```

| Context | Entrypoint | Receives | Behaviour and guarantees |
|---|---|---|---|
| Acquisition | `ImportCustomFormats` | CFs from both apps, specs translated | Same name and same specs → one CF. Same name and different specs → the user picked one in the wizard diff. A name already stored (e.g. a TRaSH-synced CF) is reused as it is, so TRaSH sync keeps owning it; other TRaSH names are linked by the next sync. Runs first (profiles reference CFs). |
| Acquisition | `ImportQualityProfiles` | Profiles with quality IDs remapped, CF IDs remapped, sizes copied in | One shared pool for movies and TV. Same name → merged per the user's diff choice; a name already stored is reused. Propers mode `PREFER_AND_UPGRADE`. |
| Acquisition | `ImportDelayProfiles` | Torrent delay, bypass rules, tags | Usenet fields dropped. The shortest untagged torrent delay becomes the default delay; tagged profiles change nothing (Magpie has one delay). |
| Catalog | `ImportTitles` | Matched titles with Magpie numbering | Doesn't emit `TitleAdded` for search purposes (see below). External IDs stored. |
| Library | `ImportRootFolders` | Rewritten paths, media type | Checks access and runs the hardlink test; inaccessible roots fail the step. |
| Library | `ImportMediaFiles` | File records linked to Magpie titles/episodes | Doesn't move or rename files unless `Rename` is set. Probes media info, hashes and lists sidecars before its transaction. A file gone from disk or outside every root folder is left out of the result. Publishes `library.FileImported{Origin: MIGRATION}` per file. |
| Acquisition | `ImportBlocklist` | Blocklist rows with recovered hashes | Rows without a hash match by title + indexer. Usenet rows skipped. |
| Activity | `ImportTimeline` | All grab/import/fail/delete history rows | Written with their original timestamps. Doesn't create notifications or attention items. |

Identity, Requests, Subtitles and Transcode have no import entrypoint:
- Users sign in with Jellyfin.
- Requests start fresh.
- Subtitles discovers existing sidecars from `FileImported`.
- Transcode reacts to `CutoffMet`.

## Wizard steps

1. **Connect:** Sonarr and Radarr URLs and API keys (either can be skipped). Show detected versions. Block Sonarr < 4.0. Probe v5 fields (`seasonType`, per-profile sizes) if present.
2. **Paths:** prefix rewrite per root folder (e.g. `/tv` → `/data/media/tv`), with a live access check per path.
3. **Cutover checklist (manual):**
   - In Sonarr and Radarr: disable RSS sync, disable automatic search, unmonitor or stop.
   - Let in-flight downloads finish in the old apps. Magpie doesn't adopt their torrents.
   - Confirm before commit. The checkbox gates the Commit button.
4. **Dry run:** a background job with a progress bar, which may run for minutes on large libraries. The report:
   - Titles: matched / needs review / skipped, per app.
   - Series with numbering mismatches and the suggested episode group.
   - Episodes unmapped (count).
   - Profiles and CFs: merged / conflicting / untranslatable specs.
   - Release profiles converted to CFs, and the scoping lost.
   - Blocklist rows with and without a hash.
   - History rows to import.
5. **Review queues:** each must be cleared or explicitly skipped.
   - **Unmatched titles:** pick a TMDB match, split into several titles (one-to-many), or skip.
   - **Episode numbering:** confirm the suggested TMDB episode group or keep aired order, per series.
   - **Profile/CF clashes:** side-by-side diff; pick the Sonarr or Radarr version.
   - **CF specs that didn't translate:** edit or drop.
   - **Daily series:** import as standard or skip. Anime-type series: skip, or import as standard with a warning.
6. **Options:**
   - Rename existing folders to `Title (Year) [tmdbid-N]` (opt-in).
   - Import Sonarr alternate titles as aliases.
7. **Commit**, then a summary with links to the review leftovers and the library scan for unmapped files.

The wizard is the optional last first-run step and a card in Settings → General.

### Job and HTTP

- One migration per install, stored as one document (`platform_migration`). Phases: `SETUP` → `DRY_RUN_RUNNING` → `REVIEW` (or `DRY_RUN_FAILED`) → `COMMIT_RUNNING` → `DONE` (or `COMMIT_FAILED`). The dry run may run again until the commit starts; it replaces the plan and its decisions.
- Both runs are backlite jobs (`platform.migration`, one attempt). They honour cancellation: a shutdown marks the run failed with "start it again"; a crash is marked the same at the next boot.
- A resumed commit runs every step again. Each Importer is idempotent on source keys, so finished steps change nothing and return the IDs later steps need.
- Source keys: titles `sonarr:{id}` / `radarr:{id}`, a split series' parts `sonarr:{id}/tmdb:{tmdbId}`, other records `{app}:{record}:{id}` (`episode`, `episodefile`, `moviefile`, `customformat`, `qualityprofile`, `delayprofile`, `rootfolder`, `blocklist`, `history`), and derived formats `…:releaseprofile:{id}:ignored|required`, `…:qualityprofile:{id}:language`.
- Root folders are imported only when an imported title lies under them, so an unused, unmounted *arr root doesn't fail the commit.
- Skipping a queue settles its open items: unmatched titles and daily/anime series are skipped, numbering keeps the preselected group (or aired order), clashing items are kept under both names with ` (TV)` / ` (Movies)` suffixes, untranslated specs are dropped.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/platform/migration` | Phase, connections (no API keys), paths with access, options, progress, report, review queues, summary |
| DELETE | `/api/platform/migration` | Forget the migration (204); committed records stay. 409 `migration-busy` while a run is going |
| PUT | `/api/platform/migration/connections` | `{app, url, apiKey, skip}`: checks the version (400 `unsupported-version` for Sonarr < 4.0; 502 `arr-unreachable`) and reads the root folders |
| PUT | `/api/platform/migration/paths` | `{paths: [{app, from, to}]}` |
| POST | `/api/platform/migration/paths/check` | `{path}` → `{path, access}`: `OK` \| `MISSING` \| `NOT_A_FOLDER` \| `NOT_WRITABLE` \| `NOT_ABSOLUTE` |
| PUT | `/api/platform/migration/cutover` | `{confirmed}` |
| PUT | `/api/platform/migration/options` | `{renameFolders, importAliases}` |
| POST | `/api/platform/migration/dry-run` | 202; 409 `migration-not-ready` with no app connected or after the commit started |
| POST | `/api/platform/migration/reviews` | `{queue, id, …decision}` settles one item; 400 `invalid-decision`, 404 for an unknown item |
| POST | `/api/platform/migration/reviews/skip` | `{queue}` settles the rest of a queue with its default |
| GET | `/api/platform/migration/search?kind=&q=` | TMDB candidates for an unmatched title |
| POST | `/api/platform/migration/commit` | 202; starts or resumes the commit. 409 `migration-not-ready` until the queues are settled and the cutover is confirmed |

Progress goes to admins as the ephemeral `platform.MigrationProgressed{phase, step, done, total}`; the wizard refetches when the phase changes.

## Mapping rules

### Titles

- **Movies:** Radarr `tmdbId`, one-to-one.
- **Series, in order:**
  1. Sonarr `tmdbId`, verified against TMDB `external_ids.tvdb_id`.
  2. TMDB `/find?external_source=tvdb_id`.
  3. TMDB `/find?external_source=imdb_id`.
  4. Title + year search → review queue.
- Sonarr `tvdbId` / `imdbId` and Radarr `imdbId` are always stored as external IDs, used for indexer ID search when TMDB has no link.
- Metadata (names, overview, images, dates) is refetched from TMDB. *arr values are used only for matching.
- Radarr `minimumAvailability` is dropped. Magpie uses its single global rule: movies aren't searched before their TMDB digital/physical release date (theatrical + 90 days when neither exists).

### Episodes

1. Compare per-season episode counts (specials excluded) between Sonarr and TMDB aired order. If they're equal, map S/E directly.
2. If not, or if `useSceneNumbering` was on, or v5 `seasonType ≠ official`: score each TMDB episode group against Sonarr's structure and preselect the best (DVD → type 3, absolute → type 2). The user confirms in review.
3. Per-episode crosswalk, only for series that failed step 1, and always for specials:
   1. TVDB episode ID via `/find` (`tv_episode_results`).
   2. Air date.
   3. Normalised title.
4. Unmapped episodes lose their monitored flag and file link. Their files go to the library scan review.
5. TMDB calls are rate-limited to about 35/s. They run as a background job with progress.

### Quality IDs

Magpie uses Radarr's quality IDs. Radarr data imports unchanged. Sonarr IDs are remapped:

| Sonarr ID | Sonarr name | Magpie ID |
|---|---|---|
| 13 | Bluray-480p | 20 |
| 22 | Bluray-576p | 21 |
| 20 | Bluray-1080p Remux | 30 (Remux-1080p) |
| 21 | Bluray-2160p Remux | 31 (Remux-2160p) |

All other Sonarr IDs are identical. Quality group IDs (≥ 1000) are reassigned. Each imported profile gets the source app's global size table copied into its per-profile sizes. Movie-only qualities are hidden in profiles used only by series.

### Custom format conditions

| Condition | Rule |
|---|---|
| Release title, release group, size, resolution, year, edition, release type, quality modifier | Copy |
| Source | Translate by meaning. Sonarr `TelevisionRaw` → Source TV + modifier RAWHD; `BlurayRaw` → Source Bluray + modifier REMUX. Exact only if the CF has no other Source condition; otherwise flag for review. |
| Language | Translate by name (IDs differ from 26 up) |
| Indexer flag | Translate by name; flags with no counterpart are dropped with a warning |

- CFs are linked to TRaSH by name match, so TRaSH sync owns them afterwards.
- Release profiles become CFs: ignored terms → a title condition scored −10000; required terms → a negated title condition scored −10000. Tag/indexer scoping is lost and listed in the report.
- Radarr profile `language` becomes a "Language: not X" CF scored −10000 in that profile.

### Blocklist and history

- **Blocklist:** every row is imported. The torrent hash is recovered from the matching `grabbed` history row (`torrentInfoHash`). Indexer names have the " (Prowlarr)" suffix stripped and are matched to Prowlarr indexers.
- **History:** every row is imported onto title timelines:

| History type | Timeline event |
|---|---|
| `grabbed` | Grabbed |
| `downloadFolderImported`, `seriesFolderImported`, `movieFolderImported` | Imported |
| `downloadFailed` | Download failed |
| `episodeFileDeleted` / `movieFileDeleted` | File deleted (reason) |

Renamed, ignored and unknown types are dropped.

## Effects after commit

- `library.FileImported{Origin: MIGRATION}` → **Acquisition** evaluates cutoff:
  - **Below cutoff:** joins the normal cutoff-unmet sweep. No immediate search, so a 1,000-title import doesn't hit indexers at once. Imported `lastSearchTime` spreads the first sweep.
  - **Meets cutoff:** `acquisition.CutoffMet{Origin: MIGRATION}`.
- → **Transcode** queues migrated files at **backlog priority**, at the normal throttled pace. Files from Magpie's own downloads always come first.
- → **Subtitles** records existing sidecars and embedded tracks. It searches providers only for wanted languages that are missing, through the normal daily-quota queue.
- **Catalog** doesn't emit search-triggering `TitleAdded` for migrated titles. It emits `catalog.TitleAdded{Origin: MIGRATION}`, which Acquisition ignores for immediate search. Once the files are linked, the normal missing (daily) and cutoff-unmet (weekly) sweeps apply.
- Imported quality profiles keep their own structure. The resolution-only presets (`720p` / `1080p` / `2160p`) are only the first-run defaults for fresh installs.
- No notifications or attention items are created for history rows.

## Test seeds

- Given a Radarr movie with `tmdbId` 974576, when the dry run runs, then it is "matched" with no TMDB `/find` call.
- Given a Sonarr series with `tmdbId` 0 and `tvdbId` 371980 that TMDB `/find` resolves, then the title is matched and `External.Tvdb` = "371980".
- Given a Sonarr series whose season counts differ from TMDB aired order and a TMDB "DVD" group matching exactly, then the review queue preselects that group.
- Given Sonarr quality 20 on a file, when imported, then the Magpie file has quality 30 "Remux-1080p".
- Given a Sonarr CF with Source = 7 (BlurayRaw) and no other Source condition, then the Magpie CF has Source Bluray AND modifier REMUX, and no review flag.
- Given profiles named "HD-1080p" in both apps with different items, then the dry run lists a clash and commit is blocked until the user picks one.
- Given a blocklist row and a `grabbed` history row with the same series and source title, then the imported blocklist entry has the history row's info hash.
- Given an imported file that meets cutoff, then Transcode queues it at backlog priority and a freshly downloaded file queued later starts first.
- Given the cutover checkbox is unticked, then Commit is disabled.
- Given the job fails during `Library.ImportMediaFiles`, when it is resumed, then Catalog and Acquisition entrypoints are no-ops (idempotent on source keys) and Library continues.
