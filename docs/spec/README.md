# Magpie v1 spec

Magpie is a self-hosted app that finds, downloads, organises, subtitles and transcodes movies and TV for a Jellyfin library. It replaces Sonarr, Radarr, Bazarr, Seerr and Tdarr. qBittorrent (downloads) and Prowlarr (indexers) stay as external services. GPU work runs on agent containers spread across machines.

This spec comes out of the wayfinding map at [`.scratch/v1-spec/map.md`](../../.scratch/v1-spec/map.md). Every rule links back to the ticket that decided it.

## How to read it

| File | What it covers |
|---|---|
| [00-context-map.md](00-context-map.md) | Contexts, ownership, allowed dependencies, messaging contract, **event catalog**, key flows, contract-test strategy. Read first. |
| [01-architecture.md](01-architecture.md) | Stack, packages, database, queue, scheduling, live WebSocket gateway, external systems, packaging |
| [contexts/identity.md](contexts/identity.md) | Users, Jellyfin sign-in, roles |
| [contexts/catalog.md](contexts/catalog.md) | TMDB titles, episodes, episode groups, monitoring |
| [contexts/requests.md](contexts/requests.md) | Requests, approvals, issues |
| [contexts/acquisition.md](contexts/acquisition.md) | Prowlarr search, quality model, decisions, grabs, blocked files, qBittorrent, seeding, cutoff |
| [contexts/library.md](contexts/library.md) | Import, naming, files, library scan, Jellyfin availability |
| [contexts/subtitles.md](contexts/subtitles.md) | Languages, OpenSubtitles, sync, Whisper requests |
| [contexts/transcode.md](contexts/transcode.md) | Transcode policy, agents, agent protocol, verification |
| [contexts/activity.md](contexts/activity.md) | Timeline, history, pipeline status, notifications |
| [10-ux.md](10-ux.md) | Screens, flows, mobile, visual style, which context feeds each block |
| [11-migration.md](11-migration.md) | One-time Sonarr/Radarr import |
| [12-settings.md](12-settings.md) | Every setting, its owner and default |
| [ADR 0001](../adr/0001-context-boundaries-and-contracts.md) | Why contexts talk only through published contracts |

Each context file has the same sections: owns / must not, data model, query port, events published, events consumed, external ports, HTTP surface, behaviour, test seeds.

## v1 scope

- Movies and TV; TMDB is the only metadata source.
- Multi-user requests with approval; Jellyfin sign-in plus one local admin.
- Indexers through Prowlarr, queried per indexer so results stream in.
- Torrents through qBittorrent ≥ 5.2; blocked-file rejection before any data downloads.
- Sonarr-compatible quality model (profiles, custom formats, TRaSH sync) shared by movies and TV.
- Import with hardlink or move, fixed Jellyfin naming, availability confirmed by Jellyfin.
- Subtitles in this order: embedded track, then OpenSubtitles, then Whisper on agents. Auto-sync.
- Transcoding to HEVC 10-bit or AV1 on Linux GPU agents (NVIDIA, Intel, AMD). Media moves over the agent connection, with no shared mount.
- In-app and per-user shoutrrr notifications.
- One-time Sonarr/Radarr import.
- Dark-only UI, desktop and mobile.

## Out of scope for v1

Usenet · music, books, audiobooks · anime as first-class (absolute numbering, XEM) · Plex and Emby · agents on the open internet · macOS and Windows agents · splitting one file across agents · Postgres · replacing Prowlarr · release push API (autobrr) · automatic adds from lists (TMDB lists, collections, favourites) · single-episode requests · request quotas · watchlists · OCR of image subtitles · DV/HDR10+ transcoding · scene numbering (XEM).

## Building it in parallel

1. **Platform first (one stream):**
   - `internal/domain` IDs and value types.
   - `internal/platform`: bus, outbox dispatcher, backlite wiring, live gateway, DB and migrations, fx app.
   - An in-memory fake for every external adapter port.
2. **Contracts as code (one short stream):** copy every `api/` interface, event struct and command struct from the context files into `internal/<ctx>/api`. Add an in-memory fake for each `Reader` and the empty contract-test suites.
3. **Contexts (eight streams in parallel):** each context is built against the other contexts' `Reader` fakes and recorded events, starting from the test seeds in its file. No context waits on another's implementation.
4. **Adapters (parallel with 3):** TMDB, qBittorrent, Prowlarr, Jellyfin, OpenSubtitles clients against their port interfaces, plus the agent binary against the agent protocol in [contexts/transcode.md](contexts/transcode.md).
5. **Frontend (parallel with 3):** screens from [10-ux.md](10-ux.md) against the OpenAPI client and the live event types. Fake data until the endpoints land.
6. **Integration:** flow tests 1–7 from [00-context-map.md](00-context-map.md#key-flows) with real contexts and fake adapters, then a compose-based end-to-end smoke test.
