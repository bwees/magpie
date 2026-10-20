# Catalog

Knows what exists (from TMDB) and what is wanted: Titles, seasons, episodes, their IDs, release dates, monitoring and wanted languages.

- **Depends on (Readers):** none.
- **Publishes:** `TitleAdded`, `TitleAddFailed`, `MonitoringChanged`, `ProfileChanged`, `LanguagesChanged`, `ScanFolderMatched`, `EpisodesAnnounced`, `WantedReleased`, `EpisodeGroupChanged`, `TitleRemoved`.
- **Consumes:** `requests.RequestApproved`, `library.FolderFound`, `acquisition.ProfileDeleted`, `platform.ServiceHealthChanged` (TMDB healthy → resume refresh).
- **Importer entrypoint:** `api.Importer` (migration job only, see [11-migration.md](../11-migration.md)).

## Owns / Must not

| Owns | Must not |
|---|---|
| Titles (movie, series), seasons, episodes, TMDB metadata | Search indexers, grab releases, or decide quality |
| External IDs: TMDB (source of record), TVDB and IMDb (optional) | Touch files, folders or Jellyfin |
| Episode group choice per series | Validate that a profile exists (it stores the reference only) |
| Monitoring per movie, season and episode | Know about requests beyond the correlation ID it echoes back |
| Profile assignment per Title (opaque `ProfileID`) | Store connection credentials (the TMDB key is platform config) |
| Wanted languages: global ordered list + per-Title override | |
| Library-scan matching and its review queue | |
| Image cache (poster, backdrop, logo) | |
| Release dates and the "released" decision | |
| Settings: metadata language, release region | |

## Data model

| Table | Key columns |
|---|---|
| `catalog_titles` | `id`, `kind`, `tmdbId` (unique per kind), `tvdbId?`, `imdbId?`, `externalIdSource` (`TMDB` \| `IMPORT`), `name`, `originalName`, `aliases` JSON, `year`, `overview`, `originalLanguage`, `runtimeMinutes?`, `seriesStatus?` (`CONTINUING` \| `ENDED`), `profileId`, `monitored`, `monitorFutureSeasons`, `episodeGroupId?`, `languagesOverride?` JSON, `origin`, `requestedBy?`, `requestId?` (the request that created it; a redelivered request is a no-op), `addedAt`, `metadataRefreshedAt`, `releaseDigitalAt?`, `releasePhysicalAt?`, `releaseTheatricalAt?`, `released`, `releasedAt?` |
| `catalog_seasons` | `id`, `titleId`, `number` (in the current numbering), `monitored` |
| `catalog_episodes` | `id`, `titleId`, `tmdbEpisodeId` (unique), `seasonNumber`, `episodeNumber` (current numbering), `airedSeason`, `airedNumber` (TMDB aired order), `name`, `airDate?`, `runtimeMinutes?`, `monitored`, `released`, `releasedAt?` |
| `catalog_images` | `titleId`, `kind` (`POSTER` \| `BACKDROP` \| `LOGO`), `tmdbPath`, `localPath`, `fetchedAt` |
| `catalog_languages` | `position`, `language`, `forced`, `hearingImpaired` (global wanted languages, ordered) |
| `catalog_scan_review` | `id`, `folderPath` (unique), `kind`, `parsedTitle`, `parsedYear?`, `tmdbHint?`, `candidates` JSON (`tmdbId`, `kind`, `name`, `year`, `overview`, `posterPath`, `score`), `state` (`PENDING` \| `MATCHED` \| `IGNORED`), `titleId?` (set when `MATCHED`), `createdAt`, `updatedAt`. Every folder Catalog has seen has a row: an automatic match is stored `MATCHED`. |

- Columns are snake_case in SQL (`tmdb_id`), like the platform tables.
- Settings live in the platform settings store, not a table: `catalog.metadata` (`language`, default `en-US`; `releaseRegion`, default `US`), `catalog.schedule.movieRefresh` (`168h`), `catalog.schedule.continuingSeriesRefresh` (`24h`), `catalog.schedule.endedSeriesRefresh` (`168h`), `catalog.schedule.releaseClock` (`1h`) and `catalog.refresh.paused` (set by a `401`).
- Cached images are files under `<data dir>/images/<titleId>/`, with resized variants beside them.
- `EpisodeID` is stable across episode-group changes: it is tied to `tmdbEpisodeId`, not to season/number.
- `seasonNumber` and `episodeNumber` always reflect the selected episode group, or aired order when none is selected.

## Query port

```go
package api // internal/catalog/api

var ErrNotFound = errors.New("catalog: not found")

type Title struct {
    ID                   domain.TitleID
    Kind                 domain.MediaKind
    External             domain.ExternalIDs // Tmdb always set; Tvdb/Imdb may be nil
    Name, OriginalName   string
    Aliases              []string // alternative titles from TMDB, for release matching
    Year                 int
    RuntimeMinutes       int      // movie runtime, or typical episode runtime; 0 if unknown
    OriginalLanguage     domain.Language
    SeriesStatus         string // "" for movies
    Profile              domain.ProfileID
    Monitored            bool
    MonitorFutureSeasons bool
    EpisodeGroupID       string // "" = TMDB aired order
    Dates                ReleaseDates
    Released             bool   // movies: global "released" rule; series: any episode released
    ReleasedAt           *time.Time
    Origin               domain.Origin // REQUEST | MANUAL | LIBRARY_SCAN | MIGRATION
    RequestedBy          *domain.UserID
}

type ReleaseDates struct {
    Theatrical, Digital, Physical *time.Time // earliest in releaseRegion, else any region
}

type WantedLanguage struct {
    Language        domain.Language
    Forced          bool // also want a forced track
    HearingImpaired bool // HI variant acceptable
}

type Episode struct {
    ID            domain.EpisodeID
    Title         domain.TitleID
    Season, Number int // current numbering (episode group if set)
    AiredSeason, AiredNumber int // TMDB aired order, for release-name matching
    Name          string
    AirDate       *time.Time
    RuntimeMinutes int
    Monitored     bool
    Released      bool
}

type TitleFilter struct {
    Kind      *domain.MediaKind
    Monitored *bool
    Released  *bool
    IDs       []domain.TitleID
}

type Reader interface {
    // Title returns one Title. ErrNotFound if the ID is unknown or removed.
    Title(ctx context.Context, id domain.TitleID) (Title, error)
    // TitleByTmdb looks up by TMDB ID and kind. ErrNotFound if not tracked.
    TitleByTmdb(ctx context.Context, kind domain.MediaKind, id domain.TmdbID) (Title, error)
    // Titles lists Titles matching the filter, ordered by name then year. Empty filter = all.
    Titles(ctx context.Context, f TitleFilter) ([]Title, error)
    // Episodes lists a series' episodes ordered by (Season, Number) in the current numbering.
    // Movies return an empty slice. ErrNotFound if the Title is unknown.
    Episodes(ctx context.Context, title domain.TitleID) ([]Episode, error)
    // Episode returns one episode. ErrNotFound if unknown.
    Episode(ctx context.Context, id domain.EpisodeID) (Episode, error)
    // EpisodeByNumber resolves a season/number in the current numbering. ErrNotFound if no match.
    EpisodeByNumber(ctx context.Context, title domain.TitleID, season, number int) (Episode, error)
    // Wanted returns what is wanted for a Title. Series: its monitored, released episodes ordered by
    // (Season, Number), with ok = at least one; an unmonitored series wants nothing. Movies: an empty slice,
    // with ok = the movie is monitored and released. Callers decide whether a file is missing.
    // ErrNotFound if the Title is unknown.
    Wanted(ctx context.Context, title domain.TitleID) (episodes []Episode, ok bool, err error)
    // ImagePath returns the local cached image path for kind POSTER | BACKDROP | LOGO,
    // or ErrNotFound if not cached yet.
    ImagePath(ctx context.Context, title domain.TitleID, kind string) (string, error)
    // WantedLanguages returns the effective ordered list for a Title: its override if set,
    // else the global list. ErrNotFound if the Title is unknown.
    WantedLanguages(ctx context.Context, title domain.TitleID) ([]WantedLanguage, error)
    // GlobalLanguages returns the global ordered list (never empty after setup).
    GlobalLanguages(ctx context.Context) ([]WantedLanguage, error)
}

// Importer is the migration entrypoint (platform migration job only). Idempotent on the
// source key and on (kind, tmdbId). Full input types are in 11-migration.md.
type Importer interface {
    ImportTitles(ctx context.Context, in []ImportTitle) (ImportedTitles, error)
}

// ImportedTitles maps each ImportTitle.Source and ImportEpisode.Source to its Magpie ID.
type ImportedTitles struct {
    Titles   map[domain.SourceKey]domain.TitleID
    Episodes map[domain.SourceKey]domain.EpisodeID
}
```

- Readers never call TMDB. They return what is stored.

## Events published

All durable (written through the outbox in the same transaction as the change). Aggregate ID: the title ID; `TitleAddFailed` uses its request ID, and `LanguagesChanged` for the global list uses `GLOBAL`.

```go
// TitleAdded: a new Title was created and its metadata stored.
type TitleAdded struct {
    Title       domain.TitleID
    Kind        domain.MediaKind
    External    domain.ExternalIDs
    Profile     domain.ProfileID
    Monitored   []domain.EpisodeID // empty for movies
    Released    bool
    RequestedBy *domain.UserID
    RequestID   *domain.RequestID // correlation for Requests; nil unless Origin = REQUEST
    Origin      domain.Origin     // REQUEST | MANUAL | LIBRARY_SCAN | MIGRATION
    ScanFolder  string            // set when Origin = LIBRARY_SCAN: the folder Library reported
}

// TitleAddFailed: a RequestApproved could not create the Title.
type TitleAddFailed struct {
    Tmdb      domain.TmdbID
    Kind      domain.MediaKind
    RequestID *domain.RequestID
    Reason    string // "TMDB_NOT_FOUND" | "TMDB_UNAUTHORIZED" | "TMDB_UNAVAILABLE"
}

// MonitoringChanged: episodes or the movie were (un)monitored.
type MonitoringChanged struct {
    Title     domain.TitleID
    Added     []domain.EpisodeID
    Removed   []domain.EpisodeID
    Movie     *bool             // set for movies
    RequestID *domain.RequestID // set when caused by a request
}

// ProfileChanged: the Title's quality profile assignment changed.
type ProfileChanged struct {
    Title    domain.TitleID
    From, To domain.ProfileID
}

// ScanFolderMatched: a folder reported by library.FolderFound now belongs to a Title
// (auto-matched or confirmed in the review queue). Library links the files.
type ScanFolderMatched struct {
    Folder string
    Title  domain.TitleID
}

// LanguagesChanged: the global wanted-language list or a Title's override changed.
type LanguagesChanged struct {
    Title     *domain.TitleID         // nil = global list changed
    Languages []WantedLanguage        // new effective list (for Title) or new global list
}

// EpisodesAnnounced: a metadata refresh found new episodes for a series.
type EpisodesAnnounced struct {
    Title    domain.TitleID
    Episodes []domain.EpisodeRef
}

// WantedReleased: monitored items crossed their release point.
type WantedReleased struct {
    Title    domain.TitleID
    Episodes []domain.EpisodeID // empty for a movie
}

// EpisodeGroupChanged: numbering of a series changed.
type EpisodeGroupChanged struct {
    Title   domain.TitleID
    GroupID string // "" = aired order
    Moves   []EpisodeMove
}
type EpisodeMove struct {
    Episode            domain.EpisodeID
    FromSeason, FromNo int
    ToSeason, ToNo     int
}

// TitleRemoved: Title deleted from Magpie.
type TitleRemoved struct {
    Title       domain.TitleID
    DeleteFiles bool
}
```

## Events and commands consumed

| Message | From | Reaction | Idempotency | Failure |
|---|---|---|---|---|
| `requests.RequestApproved` | Requests | Title not tracked: fetch from TMDB, create it with the request's profile, and monitor the scope: the movie, all seasons plus future seasons, or only the chosen seasons. Then publish `TitleAdded{RequestID, Origin: REQUEST}`. Title tracked: monitor the requested seasons (or the movie) and publish `MonitoringChanged{RequestID}`; the existing profile is kept. | Event ID in `processed_events`; natural key `(kind, tmdbId)` prevents duplicate Titles; `requestId` on the Title makes a repeated request a no-op | The handler queues a backlite job for the TMDB fetch (handlers can't do network I/O). TMDB error → retry 3× after 30 s, 2 min, 10 min, then `TitleAddFailed`; a `404` fails at once |
| `library.FolderFound` | Library (scan) | Match the folder to TMDB: a `[tmdbid-N]` tag (or `TmdbHint`) is exact; otherwise search TMDB by parsed title + year. One candidate with score ≥ 0.9 → match. Anything else → a `catalog_scan_review` row; the admin picks a candidate or ignores the folder. On a match: create the Title if new (default profile, monitored) and publish `TitleAdded{Origin: LIBRARY_SCAN, ScanFolder}`; then, new or existing, publish `ScanFolderMatched{Folder, Title}`. | Event ID; `(kind, tmdbId)`; review rows unique per `folderPath` | TMDB unavailable → retry with backoff; stays queued |
| `acquisition.ProfileDeleted` | Acquisition | Move every Title on `Profile` to `ReplacedBy` through the same update as `PATCH profileId`, so each moved Title publishes `ProfileChanged{From: Profile, To: ReplacedBy}`. | Event ID; a redelivery finds no Title left on `Profile` and publishes nothing | Retry with backoff; all Titles move in one transaction |

Migration writes go through `api.Importer.ImportTitles` (not an event). It applies the same creation rules with `Origin: MIGRATION` and publishes `TitleAdded{Origin: MIGRATION}`. TVDB/IMDb IDs it supplies are stored only when TMDB has none (`externalIdSource = IMPORT`). Aliases it supplies (Sonarr's alternate titles, opt-in) are kept in `importedAliases` and read with TMDB's, so a refresh doesn't drop them. Source keys are kept in `catalog_import_sources`, so a re-run returns the same IDs; a title its `(kind, tmdbId)` already names is reused unchanged.

## External ports

```go
// TMDB is implemented by internal/adapters/tmdb.
type TMDB interface {
    Movie(ctx context.Context, id domain.TmdbID, lang string) (TmdbMovie, error)    // details + release_dates + external_ids + images
    Series(ctx context.Context, id domain.TmdbID, lang string) (TmdbSeries, error)  // details + seasons summary + external_ids + images
    Season(ctx context.Context, id domain.TmdbID, season int, lang string) (TmdbSeason, error)
    EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]TmdbEpisodeGroupSummary, error)
    EpisodeGroup(ctx context.Context, groupID string) (TmdbEpisodeGroup, error)
    Find(ctx context.Context, source string, externalID string) (TmdbFindResult, error) // "tvdb_id" | "imdb_id"
    Search(ctx context.Context, query string, page int) (TmdbSearchPage, error)        // movies + tv
    Trending(ctx context.Context, window string) ([]TmdbSearchItem, error)              // "week"
    Changes(ctx context.Context, kind domain.MediaKind, since time.Time) ([]domain.TmdbID, error)
    Image(ctx context.Context, path string, size string) (io.ReadCloser, error)
    ImageURL(path string, size string) string // public URL, for hotlinking discovery results
}

var (
    ErrTmdbNotFound     = errors.New("tmdb: not found")
    ErrTmdbUnauthorized = errors.New("tmdb: invalid api key")
    ErrTmdbUnavailable  = errors.New("tmdb: unavailable") // after retries
)
```

- The `Tmdb*` result types (movie with release dates per region and type, series with seasons summary, season episodes with TMDB episode IDs, episode groups, find and search results) are declared in `internal/catalog/ports.go`.
- The adapter rate-limits to 40 requests/s with a token bucket. On `429` it waits for `Retry-After` (or 1s, doubling up to 30s) and retries up to 5 times. Then it returns `ErrTmdbUnavailable`. `5xx` answers and network errors retry the same way.
- The key may be a v3 API key (sent as `api_key`) or a v4 read access token (sent as `Authorization: Bearer`). A missing key is `ErrTmdbUnauthorized`.
- `ErrTmdbUnauthorized` sets the TMDB connection status to failed (Settings → Connections) and pauses refresh jobs until the key changes.

## HTTP surface

Routes under `/api/catalog`. Schemas come from fuego's OpenAPI.

- `GET /titles` list Titles (filters: `q` on name, original name and aliases, `kind`, `monitored`, `released`, `ids` (repeated, at most 500); `sort`: `NAME` (default), `ADDED` or `YEAR`, both newest first).
  - Each series also carries `seasonCount` (numbered seasons, no specials), `wantedEpisodeCount` (monitored, released episodes; 0 when the series is unmonitored) and `nextEpisode` (`season`, `number`, `airDate` of the first announced, unreleased episode; left out when there is none). Movies have 0, 0 and no `nextEpisode`. The TV page reads them.
  - Catalog can't read Activity, so there is no activity sort: Home's "Recently active" order comes from `GET /api/activity/pipeline?recent=true`, and its names and posters from `GET /titles?ids=`.
- `GET /titles/{id}` one Title with seasons and episodes.
- `PATCH /titles/{id}` set `profileId`, `monitored`, `monitorFutureSeasons` (admin).
- `PATCH /titles/{id}/seasons/{n}` set season monitoring (admin).
- `PATCH /titles/{id}/episodes/{episodeId}` set episode monitoring (admin).
- `GET /titles/{id}/episode-groups` list TMDB episode groups for a series.
- `PUT /titles/{id}/episode-group` select a group, or `""` for aired order (admin).
- `POST /titles/{id}/refresh` refresh metadata now (admin).
- `DELETE /titles/{id}?deleteFiles=bool` remove a Title (admin).
- `GET /images/{titleId}/{kind}?w=` serve a cached image, resized. `w` rounds up to 92, 154, 185, 342, 500, 780 or 1280; no `w`, or one wider than the image, serves the cached file.
- `GET /discover?q=` TMDB search, each item marked `inLibrary` (with its `titleId`); an empty `q` returns TMDB trending this week (`trending: true`). TMDB failures answer `503` with problem type `tmdb-unavailable`.
- `GET /discover/{kind}/{tmdbId}` one movie or series as a discover item plus its `seasons` (`number`, `episodeCount`, `monitored`), for request rows and the request scope picker. A tracked Title is read from the library (cached poster, its seasons); anything else from TMDB (`404` if TMDB has no such ID).
- `GET /settings`, `PUT /settings` metadata language, release region (admin). The TMDB key is platform connection config (`/api/platform/connections`).
- `GET /languages` the global wanted languages, ordered, plus `available`: the ISO 639-1 codes and English names a list may hold. `PUT /languages {languages}` replaces the list (admin): at least one language, each a known ISO 639-1 code, no duplicates, else `400 invalid-wanted-languages`. Shown in Settings → Subtitles and the setup wizard.
- `PUT /titles/{id}/languages {languages}` set a Title's override, or clear it with `null` (admin). Same validation. `TitleResponse.languagesOverride` is `null` when the Title uses the global list.
- `PATCH /titles/{id}/seasons/{n}` and `PATCH /titles/{id}/episodes/{episodeId}` take `{monitored}` and return the Title. `n` is in the current numbering. Unknown seasons and episodes answer `404`; a movie answers `409 not-a-series`. So do the episode-group routes.
- `GET /titles/{id}/episode-groups` items: `id`, `name`, `type` (`ORIGINAL_AIR_DATE` \| `ABSOLUTE` \| `DVD` \| `DIGITAL` \| `STORY_ARC` \| `PRODUCTION` \| `TV`), `episodeCount`, `groupCount`. `PUT /titles/{id}/episode-group {groupId}` answers `400 unknown-episode-group` for a group TMDB doesn't list for the series. `TitleResponse.episodeGroupId` is the selected group (`""` = aired order).
- `GET /scan-review`, `POST /scan-review/{id}/match {tmdbId}`, `POST /scan-review/{id}/ignore` library-scan review queue (admin).
  - `GET` lists `PENDING` rows, oldest first: `id`, `folderPath`, `kind`, `parsedTitle`, `parsedYear`, `tmdbHint`, `state`, `titleId`, `createdAt`, and `candidates` (`tmdbId`, `kind`, `name`, `year`, `overview`, `posterUrl` hotlinked from TMDB, `score` 0–1).
  - `match` takes any TMDB ID of the row's kind (a candidate, or one the admin found with `/discover`). It fetches the Title synchronously, then creates it if new and publishes `ScanFolderMatched` like an automatic match. Both answer the row: `404 scan-review-not-found`, `409 scan-review-resolved` unless `PENDING`, `404 title-not-found` when TMDB has no such ID, `503 tmdb-unavailable`. A failed match leaves the row `PENDING`.
- `GET /attribution` TMDB logo and notice for the About page.

Adding a Title from the top-bar search goes through Requests (`POST /api/requests`). An admin's request is approved immediately. Catalog has no direct add endpoint.

## Behaviour

**Source of record** ([ticket 09](../../../.scratch/v1-spec/issues/09-choose-metadata-source.md))
- TMDB is the only metadata source. The user enters their own TMDB API key during setup; Magpie ships none.
- The About page shows TMDB attribution: the TMDB logo and the notice "This product uses the TMDB API but is not endorsed or certified by TMDB."

**External IDs** ([ticket 31](../../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md))
- `tvdbId` and `imdbId` come from TMDB `external_ids`.
- When TMDB has none, an ID supplied by `Importer.ImportTitles` (migration) is stored with `externalIdSource = IMPORT`.
- A later refresh that finds a TMDB-provided ID replaces an `IMPORT` ID.

**Episode groups** ([ticket 09](../../../.scratch/v1-spec/issues/09-choose-metadata-source.md))
- Default numbering is TMDB aired order. An admin may select one TMDB episode group per series.
- Numbering matches Jellyfin's ([PR 16052](https://github.com/jellyfin/jellyfin/pull/16052)): an episode's season is its group's `order` and its number is its `order` inside the group + 1.
- Selecting a group recomputes `seasonNumber`/`episodeNumber` for every episode, rebuilds `catalog_seasons` with monitoring carried by episode (a season is monitored when any of its episodes is), and publishes `EpisodeGroupChanged` listing every episode whose numbering moved.
- Episodes the group doesn't place go to season 0, in aired order, numbered after the group's own season-0 episodes. Unplaced specials therefore keep their aired numbers unless the group has its own specials.
- A refresh re-applies the selected group. Existing episodes it moves publish `EpisodeGroupChanged`. If TMDB no longer has the group, the series falls back to aired order and publishes `EpisodeGroupChanged{GroupID: ""}`.
- The selected group ID is exposed on `Title.EpisodeGroupID`, so Library can tell the user to pick the same group in Jellyfin.

**Monitoring**
- A movie has one `monitored` flag.
- A series has per-season and per-episode flags.
  - Monitoring a season monitors all its episodes.
  - Episodes added later by a refresh inherit their season's flag.
  - With `monitorFutureSeasons`, new seasons are monitored on arrival. Season 0 (specials) never is.
  - The Title's own `monitored` flag gates the series: an unmonitored series wants nothing (`Reader.Wanted`) and its episodes publish no `WantedReleased`. Turning it off or on publishes `MonitoringChanged` with every monitored episode in `Removed` or `Added`.
  - Season and episode changes publish `MonitoringChanged` with the episodes whose flag changed. An episode change leaves its season's flag alone.
- Whole-series requests set every season but season 0 monitored and `monitorFutureSeasons = true`, like Sonarr. Season requests monitor only the chosen seasons, by number in the current numbering. A request for a tracked series also turns the Title's flag on.

**Released rule** ([ticket 31](../../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md))
- A movie is released when today ≥ the earliest TMDB Digital (type 4) or Physical (type 5) release date in `releaseRegion`.
  - If the region has neither, the earliest Digital/Physical date in any region counts.
  - With no Digital/Physical date anywhere, the movie counts as released at its earliest Theatrical date + 90 days. Theatrical means TMDB types 2 (limited) and 3, from the region if it has one, else any region.
  - With no date at all, it stays unreleased. A manual search is still allowed.
- An episode is released when its `airDate` (a date, read in the server's time zone) is today or earlier. Episodes that have aired when a series is added are released at once. Episodes a refresh finds are released by the clock, so already-aired ones publish `WantedReleased` on its next run. A refresh that moves a released episode's air date into the future makes it unreleased again, so the clock releases it on the new date. A series is released once any episode is.
- An hourly release-clock job finds monitored items that just crossed their release point, sets `releasedAt`, and publishes one `WantedReleased` per Title.
- `TitleAdded.Released` tells Acquisition whether to search straight away (only for `Origin` REQUEST or MANUAL).

**Metadata refresh** ([ticket 24](../../../.scratch/v1-spec/issues/24-scheduling.md))
- Continuing series: daily. Ended series and movies: weekly.
- Each run first calls TMDB `/changes` for the span since the previous run (its interval, at most TMDB's 14 days) and skips unchanged Titles. Every Title is still fully refreshed at least once every 30 days. Unchanged Titles only re-fetch stale images.
- Continuing and ended series run as two jobs, each checking `/changes` over its own interval.
- New episodes produce `EpisodesAnnounced`. Changed air dates feed the release clock. Episodes TMDB drops stay in Catalog.

**Image cache** ([ticket 09](../../../.scratch/v1-spec/issues/09-choose-metadata-source.md))
- Poster, backdrop and logo are downloaded when a Title is added, and served resized from `/api/catalog/images`.
- Images are re-fetched when `fetchedAt` is older than 5 months, staying inside TMDB's 6-month cache limit.
- Discovery results for Titles not in the library hotlink `image.tmdb.org`.

**Wanted languages** ([ticket 16](../../../.scratch/v1-spec/issues/16-subtitle-design.md))
- One global ordered list with per-language `forced` and `hearingImpaired` flags, set in the setup wizard. Each Title may override it.
- Subtitles uses it to decide which subtitles to find. Transcode uses it to decide which audio and subtitle streams to keep (plus the original language).
- Any change publishes `LanguagesChanged`.

**Library scan matching** ([ticket 18](../../../.scratch/v1-spec/issues/18-import-and-library.md))
- Catalog owns matching and the review queue; Library only reports folders (`FolderFound`) and links files after `ScanFolderMatched`.
- Titles created by a scan get `Origin = LIBRARY_SCAN`. Acquisition doesn't search them until Library has linked their files.
- `FolderFound` queues a `catalog.match-folder` job (handlers can't call TMDB). A folder with a review row is left alone, so a redelivery or a rescan publishes nothing.
- A tag (`TmdbHint`) is fetched directly; if TMDB doesn't know it, the folder is searched instead.
- Search: TMDB `/search/multi` for the parsed title, results of the folder's kind. Score = name similarity (1 − Levenshtein distance / longer length, over lowercased names with punctuation as spaces and `&` as "and"; the better of name and original name), × 0.95 when the years differ by one or TMDB has no year, × 0.6 when they differ by more; no folder year leaves it as is. Exactly one result ≥ 0.9 matches; otherwise the best five are stored as candidates on a `PENDING` row.
- A new Title is created on the default profile (`Profile` empty, which Acquisition reads as its default profile), monitored; a series monitors every regular season and future seasons, like a whole-series request.
- TMDB errors other than not-found retry the job after 30 s, doubling up to 1 h, until TMDB answers.
- Removing a Title deletes the review rows matched to it, so a later scan can match the folder again.

**Origin**
- `REQUEST`: created from `RequestApproved` (including admin adds from the top-bar search, which go through Requests).
- `LIBRARY_SCAN`, `MIGRATION`: see above.
- `MANUAL`: reserved for admin adds outside Requests; no v1 screen creates it.

**Removal**
- `DELETE /titles/{id}` deletes Catalog rows and cached images, then publishes `TitleRemoved{DeleteFiles}`. Other contexts clean up their own data.

## Test seeds

1. **Given** no Title for TMDB movie 974576, **when** `RequestApproved{kind: MOVIE, tmdb: 974576, profile: P}` arrives, **then** one Title exists with profile P and `monitored = true`, and `TitleAdded{RequestID, Origin: REQUEST}` is published once.
2. **Given** the same `RequestApproved` event is delivered twice, **then** one Title exists and one `TitleAdded` is published.
3. **Given** a series is already tracked with season 1 monitored, **when** `RequestApproved{seasons: [2]}` arrives, **then** season 2 episodes become monitored, the profile is unchanged, and `MonitoringChanged{Added: S2 episodes, RequestID}` is published (no `TitleAdded`).
4. **Given** TMDB returns 404 for the requested ID, **when** `RequestApproved` arrives, **then** no Title is created and `TitleAddFailed{Reason: "TMDB_NOT_FOUND", RequestID}` is published.
5. **Given** TMDB returns 429 twice then 200, **when** a Title is added, **then** the adapter retries and the add succeeds without error.
6. **Given** a whole-series request, **when** a refresh finds a new season 3, **then** season 3 is monitored and `EpisodesAnnounced` lists its episodes.
7. **Given** a movie whose only Digital date is in another region (yesterday), **when** the release clock runs, **then** `Released = true` and `WantedReleased{Title}` is published once.
8. **Given** a movie with only a Theatrical date 89 days ago, **when** the release clock runs, **then** it stays unreleased; **when** it runs the next day (90 days), **then** `Released = true` and `WantedReleased` is published once. A movie with no dates at all never becomes released.
9. **Given** a monitored episode with `airDate` = today, **when** the release clock runs twice, **then** exactly one `WantedReleased` includes it.
10. **Given** a series with episodes S01E01–S01E10, **when** an episode group that splits them into S01E01–05 and S02E01–05 is selected, **then** EpisodeIDs are unchanged, `EpisodeByNumber(2,1)` returns the old S01E06, and `EpisodeGroupChanged` lists 5 moves.
11. **Given** `Importer.ImportTitles([{Source: "sonarr:12", External.Tvdb: "123"}])` and TMDB has no `tvdb_id`, **then** the Title stores `tvdbId = 123` with `externalIdSource = IMPORT` and `TitleAdded{Origin: MIGRATION}` is published; calling it again returns the same TitleID and publishes nothing; **when** a later refresh finds `tvdb_id = 456`, **then** `tvdbId = 456` and the source is `TMDB`.
12. **Given** a Title, **when** `PATCH profileId` changes it from P to Q, **then** `ProfileChanged{From: P, To: Q}` is published.
13. **Given** an unknown TitleID, **then** `Reader.Title` and `Reader.Episodes` return `ErrNotFound`.
14. **Given** a cached image with `fetchedAt` 5 months and 1 day ago, **when** the refresh job runs, **then** the image is re-downloaded.
15. **Given** an invalid TMDB key, **when** any TMDB call returns 401, **then** refresh jobs pause, the connection status is failed, and Readers keep serving stored data.
16. **Given** a Title, **when** `DELETE ?deleteFiles=true`, **then** Catalog rows and images are gone and `TitleRemoved{DeleteFiles: true}` is published.
17. **Given** `FolderFound{Folder: ".../Dune Part Two (2024) [tmdbid-693134]"}` and no such Title, **then** a Title is created and `TitleAdded{Origin: LIBRARY_SCAN, ScanFolder}` then `ScanFolderMatched{Folder, Title}` are published; redelivery publishes nothing new.
18. **Given** `FolderFound{Folder: ".../Dune"}` with no tag and two TMDB candidates scoring < 0.9, **then** one `catalog_scan_review` row exists and no event is published; **when** the admin matches it, **then** `ScanFolderMatched` is published.
19. **Given** an existing Title and a `FolderFound` that matches it exactly, **then** no `TitleAdded` is published but `ScanFolderMatched` is.
20. **Given** global languages [en, es], **when** a Title override [ja, en] is set, **then** `WantedLanguages(title)` returns [ja, en], `GlobalLanguages` still returns [en, es], and `LanguagesChanged{Title}` is published; clearing the override returns the global list again.
21. **Given** Titles A on profile P and B on Q, **when** `ProfileDeleted{Profile: P, ReplacedBy: R}` is delivered twice, **then** A is on R, B stays on Q, and one `ProfileChanged{Title: A, From: P, To: R}` is published.
