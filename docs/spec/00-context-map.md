# Context map and cross-context contracts

The backbone every other spec file follows. Decision record: [ADR 0001](../adr/0001-context-boundaries-and-contracts.md). Glossary: [CONTEXT.md](../../CONTEXT.md).

## Contexts

| Context | Responsible for | Owns (only it writes) | Must not |
|---|---|---|---|
| **Identity** | Who the user is and what they may do | Users, roles, per-user toggles, sessions, local admin | Know about titles or requests |
| **Catalog** | What exists and what is wanted | Titles, seasons, episodes (TMDB), external IDs, episode group choice, monitoring, the title's profile assignment, wanted languages (global + per-title), library-scan matching and its review queue, image cache, release dates | Search, download, or touch files |
| **Requests** | Users asking for titles, approval, and issues | Requests, approvals, issues, issue comments | Add titles itself; decide releases |
| **Acquisition** | Finding and downloading the right release | Quality profiles, custom formats, TRaSH sync, indexer settings (read from Prowlarr), searches, release decisions, pending releases, blocklist, downloads in qBittorrent, blocked-file checks, seeding goals, cutoff evaluation | Move files into the library; talk to Jellyfin |
| **Library** | Files on disk and what Jellyfin sees | Media files, imports, naming, hardlink/move, library scan (walking folders), file swaps, Jellyfin availability | Choose releases; decide what a folder is; run GPU work |
| **Subtitles** | Subtitle coverage per media file | Subtitle files, OpenSubtitles session/quota, sync, upgrade window, Whisper requests | Write video files; run agents directly; own the wanted-languages list |
| **Transcode** | GPU work on agents | Agents, jobs (transcode and Whisper), scheduler, leases, transfers, verification, transcode target/policy | Replace library files itself |
| **Activity** | What happened, and telling people | Timeline, global history, pipeline-status projection (stages + chips), attention list, notification URLs and subscriptions, in-app notifications | Change any other context's state |

Cross-cutting packages that are not contexts:
- `internal/domain`: shared IDs and value types only, with no behaviour that depends on a context.
- `internal/platform`:
  - event bus, outbox, per-handler dispatcher, job queue (backlite);
  - live WebSocket gateway, DB and migrations;
  - **connection config** for external services (TMDB key, qBittorrent, Prowlarr, Jellyfin, OpenSubtitles login) and their health checks;
  - the migration job;
  - `platform/parse`, the pure release-name parser shared by Acquisition and Library.
- `internal/adapters/<system>`: clients for TMDB, qBittorrent, Prowlarr, Jellyfin, OpenSubtitles, shoutrrr. Each implements port interfaces that the contexts declare.

## Package rules

- `internal/<ctx>/` contains `api/` (public contract), `service.go`, `repository.go`, `controller.go` (HTTP), `handlers.go` (event/command handlers), `ports.go` (adapter interfaces it needs, their data types and error sentinels), and `fakes/` (an in-memory fake per port).
- Another context may import only `internal/<ctx>/api`. An import lint (depguard) fails the build on any other cross-context import.
  - A context imports another's `api` only for a `Reader` along the query graph below, or for the events and commands it consumes or sends. depguard enforces the imports; a test in `internal/app` enforces that `Reader`s are used only along the graph and `Importer`s only by the migration job.
  - Adapters (`internal/adapters/<system>`) may import a context's root package, only to implement the ports in its `ports.go`. `internal/app` wires them.
- `api/` holds:
  - `Reader`, the read-only query port, implemented by the context's repository. It never mutates and never calls out to external systems.
  - The event structs the context publishes.
  - The command structs the context accepts.
  - Optionally `Importer`, the migration entrypoint (see below).
  - `Messages()`, the list of its events and commands; `internal/app` checks it against the event catalog below.
- Services never call other services, including other contexts' services. HTTP controllers call only their own context's service.
- A handler may call its own service, read other contexts' `Reader`s, and enqueue backlite jobs. It never writes another context's tables.
- **The one sanctioned exception to "no cross-context writes":** the platform migration job (`internal/platform/migrate`) calls each context's `api.Importer` (Catalog, Acquisition, Library, Activity). Importers are idempotent on a source key and run inside the owning context's own transactions ([11-migration.md](11-migration.md)). Nothing else calls an `Importer`.

## Query dependencies (acyclic)

```
Identity    ← (none)
Catalog     ← (none)
Library     → Catalog
Acquisition → Catalog, Library
Requests    → Catalog, Identity, Acquisition
Subtitles   → Catalog, Library
Transcode   → Catalog, Library
Activity    → Catalog, Identity, Requests
Platform    → Identity, Requests   (live gateway role filter only)
```

- Library never reads Acquisition: everything it needs about a download comes in the `DownloadCompleted` payload.
- Transcode never reads Subtitles or Acquisition: wanted languages come from Catalog, and cutoff arrives as `CutoffMet`.
- `acquisition.Reader` exists for Acquisition's own HTTP layer, plus Requests reading `DefaultProfile` for new requests. The UI reads profiles from `GET /api/acquisition/profiles`.
- Importing an `api` for its event structs or command is not a query dependency. For example, Library imports `acquisition/api` for `DownloadCompleted` but never uses `acquisition.Reader`, and the live gateway imports `acquisition/api` only for the search event-type constants.

Reader contents other contexts rely on:
- `catalog.Reader`:
  - titles with runtime, release dates (theatrical/digital/physical), aliases and origin;
  - episodes in the current episode-group numbering plus aired numbering;
  - external IDs (TMDB, plus optional TVDB/IMDb);
  - effective and global wanted languages.
- `library.Reader`:
  - `File(id)`: path, root folder, size, media info (HDR format, per-stream languages), `Transcoded`, origin, and the source download ID / info hash;
  - `FileForEpisode`;
  - `SourceForProcessing(id)`: read-only path, ETag, size, media info, and a `StagingDir` on the same filesystem;
  - `Coverage(titles)`;
  - `AvailableEpisodes`.

## Shared kernel (`internal/domain`)

```go
type TitleID, EpisodeID, MediaFileID, RequestID, IssueID, UserID string // ULIDs
type DownloadID, ReleaseID, SearchID, AgentID, JobID, ProfileID string   // ULIDs
type TmdbID int64

type MediaKind string      // MOVIE | SERIES
type ExternalIDs struct { Tmdb TmdbID; Tvdb, Imdb *string }
type EpisodeRef struct { Episode EpisodeID; Season, Number int }
type Quality struct { ID int; Name string; Source string; Resolution int; Revision int } // Radarr quality IDs
type Language string       // ISO 639-1

// Origin says how a title or file entered Magpie.
// Titles: REQUEST | MANUAL | LIBRARY_SCAN | MIGRATION. Files: DOWNLOAD | LIBRARY_SCAN | MIGRATION.
type Origin string

// Principal is the signed-in user, set by Identity's middleware, read by every controller.
type Principal struct { User UserID; Admin bool; Permissions struct{ AutoApproveMovies, AutoApproveTV, ViewOthersRequests, ReportIssues bool } }

// SourceKey identifies a migrated record in its source app: "sonarr:12" | "radarr:40". Importers are idempotent on it.
type SourceKey string

// Message is implemented by every event and command struct in an api package.
type Message interface {
    Type() string        // "<ctx>.<Name>", e.g. "catalog.TitleAdded"
    AggregateID() string // ordering key
    Durable() bool       // false for ephemeral events
}
```

## Messaging contract

```go
type Event struct {
    ID          string    // ULID, unique; handlers dedupe on it
    Type        string    // e.g. "acquisition.DownloadCompleted"
    OccurredAt  time.Time
    AggregateID string    // ordering key
    Payload     any       // one of the structs in <ctx>/api
}
```

- `bus.EventOf(m domain.Message)` builds the envelope from the message's own `Type()` and `AggregateID()`.

- **Durable events:** written to the `outbox` table in the same SQLite transaction as the state change, and dispatched after commit.
- **Per-handler subscriptions:**
  - Every handler has its own cursor and its own delivery state.
  - Delivery is at least once and in order per `AggregateID` **per handler**.
  - A failing handler retries with backoff and holds back only its own later events for that aggregate. It never blocks other handlers or other aggregates. After 10 failures the event is parked for that handler, visible in Activity → History.
  - Handlers record the event ID in `processed_events` and must be idempotent.
- **Ephemeral events** (`…Progressed`, `SearchResultsReceived`, `ProjectionUpdated`): published on the in-process bus only. They aren't persisted, and only the live gateway's replay buffer can resend them. No state may depend on them.
- **Commands:** same envelope, exactly one owning handler. Used only when the sender needs a specific action from the owner. v1 has exactly one: `transcode.RunAgentJob`.
- **Live gateway:** `internal/platform/live` forwards events to browser WebSocket clients, filtered by the user's role (reads `identity.Reader`, `requests.Reader`). A payload implementing `domain.Personal` (`Recipient() UserID`) goes only to that user, admins included; `activity.NotificationCreated` is the one such event. Envelope sequence numbers allow resume after reconnect ([01-architecture.md](01-architecture.md#live-updates-to-browsers)).

## Event catalog

| Event | Publisher | Key payload | Consumers → reaction |
|---|---|---|---|
| `identity.UserCreated` | Identity | user, role, source | Activity → history (no notification prefs to create: in-app is always on and URLs start empty) |
| `catalog.TitleAdded` | Catalog | title, kind, external IDs, profile, monitored episodes, released, requestedBy, requestID?, origin, scanFolder? | Acquisition → search now if released and origin REQUEST/MANUAL (never for LIBRARY_SCAN/MIGRATION); Requests → link request; Activity → timeline/pipeline |
| `catalog.TitleAddFailed` | Catalog | tmdb, kind, requestID, reason | Requests → request FAILED |
| `catalog.MonitoringChanged` | Catalog | title, episodes ±, movie?, requestID? | Acquisition → search newly monitored; Requests → link/extend request items; Activity |
| `catalog.ProfileChanged` | Catalog | title, from, to | Acquisition → re-check cutoff and upgrades |
| `catalog.LanguagesChanged` | Catalog | title? (nil = global), languages | Subtitles → recompute wanted; Transcode → refresh keep-languages for queued jobs |
| `catalog.ScanFolderMatched` | Catalog | folder, title | Library → link files in place |
| `catalog.EpisodesAnnounced` | Catalog | title, episodes | Acquisition → RSS eligibility; Requests → add items in scope |
| `catalog.WantedReleased` | Catalog | title, episodes (release point reached) | Acquisition → search; Requests → add items |
| `catalog.EpisodeGroupChanged` | Catalog | title, old→new numbering | Library → rename/re-link files |
| `catalog.TitleRemoved` | Catalog | title, deleteFiles | Acquisition → cancel; Library → delete/unlink + `FileDeleted`; Transcode → cancel jobs; Requests → close issues; Activity → drop timeline |
| `requests.RequestCreated` | Requests | request, user, kind, tmdb, scope (pending only) | Activity → admin notification + attention item |
| `requests.RequestApproved` | Requests | request, user, tmdb, kind, scope, seasons, profile, auto | Catalog → add title / monitor seasons; Activity → requester notification, clear attention |
| `requests.RequestDeclined` | Requests | request, user, reason | Activity → notify, clear attention |
| `requests.RequestCancelled` | Requests | request, user (pending request withdrawn by its requester) | Activity → clear attention |
| `requests.RequestAvailable` | Requests | request, user, title | Activity → notify |
| `requests.IssueReported` | Requests | issue, title, episode?, kind, user | Activity → notify admins, attention item |
| `requests.IssueCommented` | Requests | issue, author, reporter | Activity → notify reporter/admins |
| `requests.IssueActionRequested` | Requests | issue, title, episode?, action `REGRAB` \| `RERUN_SUBTITLES` | Acquisition → resolve file via `library.Reader`, blocklist + search (REGRAB); Subtitles → redo (RERUN_SUBTITLES) |
| `requests.IssueResolved` | Requests | issue, reporter, by | Activity → notify reporter, clear attention |
| `acquisition.SearchStarted` | Acquisition | search, title, episodes, trigger | Activity → timeline/pipeline |
| `acquisition.SearchResultsReceived` *(ephemeral)* | Acquisition | search, indexer, status, releases[] | live gateway (topic `search:<id>`) |
| `acquisition.SearchCompleted` | Acquisition | search, title, counts, failed indexers, outcome `GRABBED` \| `NOTHING_ACCEPTED` \| `PENDING_DELAY` \| `RELEASES_FOUND` (interactive only) | Activity |
| `acquisition.ReleaseGrabbed` | Acquisition | download, release, indexer, infoHash, title, episodes, quality, cfScore | Activity |
| `acquisition.ReleaseRejected` | Acquisition | release, infoHash, reason, blockedFile?, next? | Activity → timeline |
| `acquisition.DownloadProgressed` *(ephemeral)* | Acquisition | download, state, progress, speed, eta | live gateway |
| `acquisition.DownloadCompleted` | Acquisition | download, title, episodes, contentPath, files[], releaseName, infoHash, quality, cfScore, seedingMode (published only after the blocked-file backstop passes) | Library → import |
| `acquisition.DownloadFailed` | Acquisition | download, reason | Activity → notify |
| `acquisition.CutoffMet` | Acquisition | mediaFile, title, episodes, quality, cfScore, origin | Transcode → evaluate policy and queue (DOWNLOAD origin ahead of LIBRARY_SCAN/MIGRATION) |
| `library.FileImported` | Library | mediaFile, title, episodes, path, quality, releaseName, osHash, size, mediaInfo, replaces? (informational), download? (nil for scan/migration), sidecars, origin | Acquisition → cutoff check + seeding/remove; Subtitles → find subtitles; Requests → resolve REGRAB issues; Activity |
| `library.ImportFailed` | Library | download, reason, retryable | Acquisition → blocklist, try next; Activity → notify |
| `library.FileTranscoded` | Library | mediaFile, oldSize, newSize | Acquisition → stop upgrades; Subtitles → re-check embedded tracks; Transcode → job SWAPPED; Activity |
| `library.TranscodeSwapRejected` | Library | job, mediaFile, reason `SOURCE_CHANGED` \| `HASH_MISMATCH` \| `PROBE_FAILED` \| `FILE_GONE` | Transcode → job FAILED / retry |
| `library.FileDeleted` | Library | mediaFile, title, episodes, path, reason `UPGRADED` \| `REMOVED` \| `TITLE_REMOVED` | **Only cleanup signal for a removed file.** Acquisition → drop file score; Subtitles → drop sidecars; Transcode → cancel jobs; Activity |
| `library.FolderFound` | Library | path, kind, parsed title/year, tmdbHint | Catalog → match against TMDB or review queue |
| `library.ItemAvailable` | Library | title, episodes, jellyfinItem | Requests → mark available; Activity |
| `subtitles.SubtitleAdded` | Subtitles | mediaFile, language, source, synced | Requests → resolve RERUN_SUBTITLES issues; Activity |
| `subtitles.SubtitleMissing` | Subtitles | mediaFile, language, reason | Activity |
| `subtitles.CoverageSettled` | Subtitles | mediaFile, title, satisfied, missing | Activity → Subs chip |
| `transcode.JobQueued` | Transcode | job, kind, mediaFile, title, origin | Activity → Transcode chip |
| `transcode.JobStarted` | Transcode | job, kind, mediaFile, agent, attempt | Activity → Transcode chip |
| `transcode.JobProgressed` *(ephemeral)* | Transcode | job, stage, progress | live gateway |
| `transcode.TranscodeVerified` | Transcode | job, mediaFile, stagedPath, sha256, sourceETag, stats | Library → swap file |
| `transcode.TranscodeSkipped` | Transcode | mediaFile, reason (not worth it, DV, under cap, low bitrate…) | Activity |
| `transcode.JobFailed` | Transcode | job, kind, mediaFile, reason, final | Subtitles (Whisper jobs); Activity → notify + attention when final |
| `transcode.WhisperCompleted` | Transcode | job, mediaFile, language, stagedPath | Subtitles → store sidecar |
| `transcode.AgentOnline` / `AgentOffline` | Transcode | agent | Activity → notify / attention on offline, clear on online |
| `activity.NotificationCreated` | Activity | notification, user, kind, title, link | live gateway (to that user only) |
| `activity.AttentionChanged` | Activity | item, kind, ref, open, count | live gateway (admins) |
| `activity.ProjectionUpdated` *(ephemeral)* | Activity | projection `TIMELINE` \| `HISTORY` \| `PIPELINE`, title? | live gateway → views refetch the timeline, history or stage bar |
| `platform.ServiceHealthChanged` | Platform | service, healthy, error | Activity → notify + attention; Catalog → resume TMDB refresh paused by a `401` |

| Command | Sender | Owner → behaviour |
|---|---|---|
| `transcode.RunAgentJob{Kind: WHISPER, MediaFile, Language, Model}` | Subtitles | Transcode → queue a Whisper job; reply with `WhisperCompleted` or `JobFailed`. Struct: [transcode › Command accepted](contexts/transcode.md#command-accepted); durable, keyed by media file |

## Key flows

1. **Request → Available:** `RequestApproved` → Catalog `TitleAdded{REQUEST}` → Acquisition search → `ReleaseGrabbed` → `DownloadCompleted` → Library `FileImported{DOWNLOAD}` → (Subtitles, Acquisition cutoff) → Library Jellyfin refresh → `ItemAvailable` → Requests `RequestAvailable` → Activity notifies.
2. **Blocked file:** Acquisition grab → metadata check fails, or the completion backstop finds a blocked file → `ReleaseRejected` → Acquisition grabs the next cached release or searches. No other context is involved.
3. **Upgrade:** RSS/backlog in Acquisition reads `library.Reader` (current quality, transcoded?) → grab → `DownloadCompleted` → `FileImported{Replaces: old}` → `FileDeleted{old, UPGRADED}` → consumers clean up the old file.
4. **Transcode:** `CutoffMet{Origin}` → Transcode `JobQueued` → agent accepts (`JobStarted`) → `TranscodeVerified` → Library re-checks the source ETag and swaps → `FileTranscoded` (or `TranscodeSwapRejected`) → Acquisition stops upgrading.
5. **Whisper:** Subtitles finds no provider match → `RunAgentJob` → `WhisperCompleted` → Subtitles writes the sidecar → `SubtitleAdded` → `CoverageSettled`.
6. **Issue REGRAB:** `IssueActionRequested{title, episode}` → Acquisition resolves the file via `library.Reader`, blocklists its release and searches (allowed even after transcode) → normal flow 3 → `FileImported` → Requests resolves the issue.
7. **Library scan:** Library walks root folders → `FolderFound` → Catalog matches against TMDB (exact `[tmdbid-N]`, else fuzzy; low confidence → review queue) → `TitleAdded{LIBRARY_SCAN}` (new titles only) and `ScanFolderMatched` → Library links files in place → `FileImported{LIBRARY_SCAN}` → Acquisition cutoff check (no immediate search; normal sweeps) → `CutoffMet{LIBRARY_SCAN}` → Transcode queues behind downloads.

## How contracts become tests

- Each `api.Reader` has a **contract test suite** in `internal/<ctx>/api/contracttest`, exported as `Run(t, factory)`, where the factory builds a Reader holding a `Fixture`. It runs against the real repository (SQLite in a temp dir) and against the in-memory `contracttest.Fake` that other contexts use. Fake and real can't drift.
- Each port has an in-memory fake in `internal/<ctx>/fakes`. Every fake, Reader or port, embeds `scripted.Script` (`internal/platform/scripted`) so tests can inject errors and latency per method.
- Each `api.Importer` has a contract suite proving idempotency on source keys and resumability.
- Each handler gets **event-in → effect-out tests**: given these Reader fakes and this event, assert own state changes and published events, including redelivery (idempotency).
- Each context spec ends with **test seeds** in given/when/then form. Each seed is one test.
- **Flow tests** (`internal/flows_test`) wire all contexts with fake adapters and assert flows 1–7 end to end.
