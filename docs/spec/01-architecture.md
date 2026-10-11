# Architecture

Magpie is one self-hosted server that finds, downloads, imports, subtitles and transcodes movies and TV for a Jellyfin library. It replaces Sonarr, Radarr, Bazarr, Seerr and Tdarr. qBittorrent and Prowlarr stay external. GPU work runs on separate agent containers.

Context boundaries, events and package rules are in [00-context-map.md](00-context-map.md). This file covers the platform under them.

## Stack

| Layer | Choice |
|---|---|
| Server | Go, [fuego](https://github.com/go-fuego/fuego) HTTP (generates OpenAPI), [uber/fx](https://github.com/uber-go/fx) DI |
| Persistence | SQLite via ncruces `gormlite` (cgo-free). GORM for queries only; schema owned by goose SQL migrations |
| Jobs | [backlite](https://github.com/mikestefanello/backlite) on the same SQLite file |
| Web | SvelteKit SPA (`adapter-static`), shadcn-svelte, Tailwind |
| API client | oazapfts client generated from the fuego OpenAPI spec |
| Tooling | mise for tool versions and tasks; goreleaser for images |
| Licence | GPL-3.0 ([ticket 21](../../.scratch/v1-spec/issues/21-licence.md)). Allows porting Sonarr/Radarr parser and decision code and bundling alass |

Base template: [bwees/stack](https://github.com/bwees/stack). One change to it: replace `glebarez/sqlite` with ncruces `gormlite` ([ticket 20](../../.scratch/v1-spec/issues/20-postgres-in-v1.md)).

## Package layout

```
server/
  main.go                      fx.New(app.Module)
  internal/
    app/                       app.Module = platform.Module + every context's Module + port fakes (main and flow tests); Messages(), the event catalog in code
    domain/                    shared IDs and value types
    platform/
      db/                      writer + reader pools, goose runner, backups
      bus/                     in-process event bus, outbox dispatcher, processed_events
      jobs/                    backlite queue, periodic schedules, tickers
      live/                    browser WebSocket gateway
      config/                  env bootstrap
      settings/                settings store, platform.connections.*
      backup/                  daily backup job, list/restore endpoints
      owner/                   names of the platform and contexts (migration order, setting prefixes)
      api/                     events the platform publishes (ServiceHealthChanged)
      scripted/                error and latency injection embedded in every fake
    flows_test/                flow tests: all modules, fake adapters, temp SQLite
    adapters/
      tmdb/ qbittorrent/ prowlarr/ trash/ jellyfin/ opensubtitles/ shoutrrr/ subsync/
    identity/ catalog/ requests/ acquisition/ library/ subtitles/ transcode/ activity/
      api/                     Reader, events, commands  (only importable part)
      api/contracttest/        Reader contract suite (Run) + in-memory Fake
      fakes/                   in-memory fake per port
      service.go repository.go controller.go handlers.go ports.go module.go
  agent/                       transcode agent binary (separate image)
web/                           SvelteKit SPA
```

- Each context exports one `fx.Module` that provides its service, repository (also bound as `api.Reader`), controller and handlers, and registers handlers on the bus.
- Adapters are provided `fx.As` the port interfaces each context declares in `ports.go`. `internal/app` provides them, since an adapter imports the context it serves. Until a slice adds the real adapter, `internal/app` provides the port's in-memory fake from `<ctx>/fakes`.
- Import lint (golangci-lint `depguard`):
  - `internal/<ctx>/**` may import `internal/<other>/api` only, and only for the contexts it reads or whose messages it consumes or sends (one `<ctx>-dependencies` rule per context).
  - `api/` packages import only `internal/domain` and the standard library.
  - Lint fails on any other cross-context import. Only `internal/app` (module wiring), `internal/flows_test` and `internal/adapters` (to implement ports) may import a context's root package.
  - `api/contracttest` may import its own context, but doesn't need to: the real repository's test calls `contracttest.Run`.
  - `mise run lint:boundaries` proves the rules by linting fixtures that must each fail (`internal/lintfixture/*`, build tag `lintfixture`).
  - depguard can't tell a `Reader` from an event struct, so `internal/app` has a test that parses the query graph in 00-context-map.md and fails on any `Reader` use outside it, and on any `Importer` use outside `platform/migrate`.
- Agent and server share the protocol types in `internal/transcode/api/agentproto`. It falls under the `api/` rule (standard library and `internal/domain` only), so it repeats the few enums it shares with `transcode/api`, such as the job kind.

## HTTP

- All routes live under `/api/<context>/…`. Each context's controller registers its own routes. Every route has a stable `OptionOperationID`, which becomes the client function name.
- `mise run openapi` regenerates `server/openapi.json` and `web/src/lib/sdk/client.ts`. CI fails on drift.
- Every event and command struct (`app.Messages()`) is also an OpenAPI component schema, so the web client gets a TypeScript type for each live-event payload. A test in `internal/app` fails when a struct changed without regenerating.
- Auth: session cookie (HttpOnly, SameSite=Lax), issued by Identity. Middleware resolves the user and role and puts them on the request context. Admin-only routes are marked per route.
- Errors: RFC 9457 problem JSON with a stable `type` per domain error.

## Database

[Ticket 20](../../.scratch/v1-spec/issues/20-postgres-in-v1.md), research [19](../../.scratch/v1-spec/research/19-sqlite-and-postgres.md).

- One SQLite file in the data dir. WAL mode, `synchronous=NORMAL`, `foreign_keys=ON`. The data dir must be on local disk (WAL doesn't work over NFS/SMB). Startup refuses a network filesystem.
- **Writer pool:** `SetMaxOpenConns(1)`, DSN `_txlock=immediate`. All write transactions go through `db.Write(ctx, func(tx) error)`.
- **Reader pool:** N connections (default `GOMAXPROCS*2`), read-only. Long reads must not hold a transaction across network I/O, which avoids WAL checkpoint starvation.
- **Ownership:** each context owns its tables, prefixed with the context name (`catalog_titles`, `acquisition_downloads`, …). No cross-context foreign keys; references are plain ID columns.
- **Migrations:** goose SQL files, one directory per context, run in a fixed order at startup after a pre-migration backup. No GORM AutoMigrate.
  - Order: platform, identity, catalog, requests, library, acquisition, subtitles, transcode, activity.
  - Each owner has its own goose version table (`<owner>_goose_db_version`), so versions are numbered per owner.
  - A context registers its directory with `db.ProvideMigrations` in its module once it has a table.
- **Platform tables:**

| Table | Columns | Use |
|---|---|---|
| `platform_outbox` | `seq` (autoincrement, commit order), `id` (event ULID), `type`, `aggregate_id`, `payload` JSON, `occurred_at` | Durable events, written in the same tx as the state change |
| `platform_subscriptions` | `handler`, `cursor` (last outbox `seq` seen) | One cursor per registered handler; its event types live in code |
| `platform_handler_queue` | `handler`, `event_id`, `seq`, `aggregate_id`, `attempts`, `next_attempt_at`, `parked_at`, `last_error` | Per-handler delivery state for failing events and the events held behind them |
| `platform_processed_events` | `handler`, `event_id`, `processed_at` (PK handler+event_id) | Handler idempotency |
| `platform_settings` | `key`, `value` JSON, `updated_at` | Settings store; each key is owned by one context or by platform |
| `platform_schedules` | `name`, `next_run_at` | Next run of each periodic job |
| backlite tables | as shipped | Jobs |

- Cursors use the outbox `seq`, not the event ULID: ULIDs are made before the transaction starts, so they can commit out of order.

- **Dispatcher (per-handler subscriptions):**
  - Every handler registers for event types and gets its own cursor. Delivery to one handler never waits on another.
  - Ordering is per `aggregate_id` per handler: a handler sees one aggregate's events in outbox order.
  - At least once. A handler error retries that event with backoff (1 s, 5 s, 30 s, 2 min, then every 10 min) and holds only that handler's later events for the same aggregate. Other aggregates and other handlers continue.
  - After 10 failures the event is parked for that handler and logged. A parked event stops holding back later events for its aggregate. Admins see parked events in Activity → History and can retry them (`GET /api/platform/parked-events`, `POST /api/platform/parked-events/retry`); a retry starts a fresh set of attempts.
  - A handler runs inside a write transaction. Its changes, the events it publishes and its `processed_events` row commit together; the cursor moves in a second transaction, so a crash between the two is deduped on redelivery.
- **Pruning:** outbox rows are deleted once every subscribed handler's cursor has passed them and 7 days have gone by. Rows still queued for a handler stay. Processed-event rows go after 30 days.
- **Transient data** (search results, progress ticks) stays in memory. Only outcomes are stored.

## Background work

[Ticket 24](../../.scratch/v1-spec/issues/24-scheduling.md). Every interval is a setting.

| Loop | Owner | Runs as | Default |
|---|---|---|---|
| qBittorrent `sync/maindata` poll | Acquisition | ticker | 2 s with active downloads, 15 s idle |
| Pending (delay) releases | Acquisition | ticker | 1 min |
| Seeding goals | Acquisition | ticker | 5 min |
| RSS sync per indexer | Acquisition | backlite periodic | 15 min |
| Prowlarr indexer list | Acquisition | backlite periodic | 1 h + on demand |
| Missing backlog sweep | Acquisition | backlite periodic | daily, spread across the day |
| Cutoff-unmet sweep | Acquisition | backlite periodic | weekly, spread |
| TRaSH sync | Acquisition | backlite periodic | daily |
| TMDB refresh | Catalog | backlite periodic | continuing series daily; ended series and movies weekly; images before 5 months |
| Release-date check (`WantedReleased`) | Catalog | backlite periodic | hourly |
| Jellyfin full reconcile | Library | backlite periodic | daily |
| Jellyfin change stream | Library | WebSocket client | live |
| Subtitle upgrades | Subtitles | backlite periodic | 6 h, within the 7-day window and daily quota |
| Agent heartbeat / lease | Transcode | agent WebSocket | 10 s heartbeat, 60 s lease |
| Activity pruning | Activity | backlite periodic | daily |
| DB backup | platform | backlite periodic | daily 03:00 |

- Tickers only poll and enqueue. The real work runs as backlite jobs with retries.
- One-off jobs: search, grab, import, subtitle fetch, transcode enqueue.

## Live updates to browsers

[Ticket 12](../../.scratch/v1-spec/issues/12-progressive-search-ux.md). One app-wide WebSocket per browser tab: `GET /api/live` (upgrade, session cookie).

```go
// Server → client
type LiveMessage struct {
    Seq   uint64          // monotonically increasing per server process
    Epoch string          // server start ULID; a new epoch means "refetch everything"
    Type  string          // event type, e.g. "acquisition.DownloadProgressed", or "live.Heartbeat"
    Topic string          // "" for broadcast, or "search:<SearchID>"
    Data  json.RawMessage // event payload, same structs as api/ packages
}

// Client → server
type LiveClientMessage struct {
    Op     string // "resume" | "subscribe" | "unsubscribe" | "pong"
    Epoch  string // resume: last seen epoch
    Seq    uint64 // resume: last seen seq
    Topics []string
}
```

- **Gateway:** `platform/live` subscribes to every durable and ephemeral event on the bus and keeps a 2-minute ring buffer of messages.
- **Resume:** on connect the client sends `resume{epoch, seq}`.
  - Same epoch and seq still in the buffer: the gateway replays everything after seq.
  - Otherwise it sends `live.Reset` and the client refetches its views over REST.
  - A first connect sends an empty epoch, so it always gets `live.Reset`. `live.Reset` carries the current epoch and latest seq. The client resumes from the last event or reset it received, never from a heartbeat.
  - JSON field names are camelCase: `seq`, `epoch`, `type`, `topic`, `data`; `op`, `epoch`, `seq`, `topics`.
- **Topics:** search results (`acquisition.SearchResultsReceived`, `SearchCompleted`) only go to sockets subscribed to `search:<SearchID>`. Everything else is broadcast, then filtered.
- **Role filter:**
  - Requesters receive only events about their own requests and those requests' titles, plus their own notifications.
  - Requesters with `ViewOthersRequests` also receive every request's events and every requested title's events, but never other users' notifications.
  - Admins receive everything. Disabled users receive nothing.
  - The role and toggles come from `identity.Reader`, not the session's principal, so a change applies to open sockets.
  - The filter reads `requests.Reader` and `identity.Reader`.
- **Heartbeat:** server sends `live.Heartbeat` every 20 s; client replies `pong`. Missing two pongs closes the socket. The client reconnects with jittered backoff (1 s → 30 s).
- **Actions are never sent over the socket.** All mutations are REST calls.
- **Reverse proxy:** must allow WebSocket upgrade on `/api/live` and on `/api/transcode/agents/ws` (agents).

## External systems

| System | Version / auth | Used by | Notes |
|---|---|---|---|
| qBittorrent | ≥ 5.2, API key | Acquisition | No push; poll `sync/maindata`. `stopCondition=MetadataReceived` / `fetchMetadata` for blocked-file checks |
| Prowlarr | API key | Acquisition | `/api/v1/indexer` list; per-indexer Torznab `/{id}/api`. Never the aggregate search |
| Jellyfin | 12.x; header `Authorization: MediaBrowser Token="…"` only (`X-Emby-Token` and `api_key` are disabled in 12) | Identity (login), Library (refresh, availability, WebSocket) | `/QuickConnect/Initiate` is POST-only in 12 |
| TMDB | User-supplied key (setup wizard) | Catalog | Non-commercial terms, attribution on About page, cache ≤ 6 months |
| OpenSubtitles.com | Magpie app key shipped in the binary + each install's user login | Subtitles | 20 downloads/day free, 1000 VIP; 5 req/s per IP |
| TRaSH Guides | Public JSON | Acquisition | Daily sync |

Adapters live in `internal/adapters/<system>`. They are the only code that speaks these protocols. Contract tests for each adapter run against recorded fixtures.

- **Connection config is platform-owned:** TMDB key, qBittorrent URL/key, Prowlarr URL/key, Jellyfin URL/key and OpenSubtitles login live under `platform.connections.*` in `platform_settings`. Adapters read them; contexts never store credentials. One Jellyfin connection serves both Identity and Library.
- **Health:** `platform/health` probes each connection every minute and on settings change. It publishes `platform.ServiceHealthChanged{Service, Healthy, Error}` when the state flips. The struct lives in `internal/platform/api`, so Activity can import it without depending on the prober.

## Packaging and deployment

[Ticket 26](../../.scratch/v1-spec/issues/26-packaging.md).

- **Server image** (`ghcr.io/bwees/magpie`, amd64 + arm64):
  - Go binary, SPA build, ffmpeg/ffprobe.
  - Never transcodes.
- **Agent image** (`ghcr.io/bwees/magpie-agent`, amd64):
  - jellyfin-ffmpeg plus whisper.cpp (Vulkan backend).
  - One tag covers NVIDIA (Container Toolkit, `NVIDIA_DRIVER_CAPABILITIES` must include `video`), Intel and AMD (`/dev/dri` plus the host `render` GID).
  - Linux only in v1.
- **Example compose:** server, one agent, qBittorrent ≥ 5.2, Prowlarr.
- **Env bootstrap only:**

| Var | Purpose |
|---|---|
| `MAGPIE_PORT` | HTTP port (default 3000) |
| `MAGPIE_DATA_DIR` | SQLite, backups, image cache (local disk) |
| `MAGPIE_BASE_URL` | Public URL for links and agent downloads |
| `MAGPIE_AGENT_SECRET` | Shared secret agents present |

- **First-run setup wizard** (admin only, until complete):
  1. Local admin.
  2. TMDB key.
  3. qBittorrent.
  4. Prowlarr.
  5. Jellyfin.
  6. Library root folders.
  7. Quality presets.
  8. Wanted languages.
  9. OpenSubtitles login.
  10. Transcode target.
  11. Optional Sonarr/Radarr import.
- **Backups:** daily `VACUUM INTO data/backups/magpie-<date>.db`, keeping 7, plus one before every migration. Restore from Settings → General; the server restarts.
- **Upgrades:** goose migrations run at startup after the pre-migration backup. A failed migration aborts startup and leaves the backup in place.

## Visual style

- Dark mode only (`class="dark"` on `<html>`, `color-scheme: dark`).
- Grayscale surfaces with teal as the only accent: `--primary: oklch(0.76 0.12 195)`.
- Tokens live in `web/src/routes/app.css`, taken from the prototype `.scratch/v1-spec/prototypes/ui/src/routes/app.css`.
