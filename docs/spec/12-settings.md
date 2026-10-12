# Settings catalogue

Every setting: owner, default, validation, and where it appears. Owners follow [00-context-map.md](00-context-map.md): a setting is stored and validated only by its owner, and others read it through the owner's `api.Reader`. The UI sections come from the [winning prototype](../../.scratch/v1-spec/issues/17-pipeline-ia.md) (Settings sub-nav). "Platform" means `internal/platform` or adapter configuration, not a context.

All defaults below are confirmed (decided in the wayfinding tickets or confirmed by the user during spec assembly, [ticket 29](../../.scratch/v1-spec/issues/29-assemble-spec.md)).

## Environment (bootstrap only)

Set on the container. Everything else lives in SQLite and is edited in the UI ([ticket 26](../../.scratch/v1-spec/issues/26-packaging.md)).

| Variable | Image | Default | Validation | Use |
|---|---|---|---|---|
| `MAGPIE_PORT` | server | `3000` | 1–65535 | HTTP listen port |
| `MAGPIE_DATA_DIR` | server | `/data` | writable dir | SQLite DB, backups, image cache |
| `MAGPIE_BASE_URL` | server | none | absolute URL | Links in notifications, agent docs |
| `MAGPIE_AGENT_SECRET` | server | none (required; the server refuses to start without it) | ≥ 32 chars | Shared secret agents present ([ticket 14](../../.scratch/v1-spec/issues/14-agent-protocol.md)). Env-only; rotate by changing it and restarting. |
| `MAGPIE_URL` | agent | none (required) | absolute URL | Server to dial |
| `MAGPIE_AGENT_SECRET` | agent | none (required) | must equal the server secret | Agent auth |
| `MAGPIE_AGENT_NAME` | agent | hostname | `[a-z0-9-]` | Name shown in the UI. The stable agent ID is a ULID the agent creates on first start in `<scratch>/.magpie-agent-id`. |
| `MAGPIE_SCRATCH_DIR` | agent | `/scratch` | writable dir | Source/output scratch space and the agent ID; mount it as a volume. Each job's scratch is `jobs/<job>/` (`job.json`, `source`, `output.mkv`); scratch that survives a restart is offered back as a `ResumableJob` and deleted after 10 minutes if no offer resumes it. |
| `MAGPIE_AGENT_CPU` | agent | `false` | `true` \| `false` | Offers the software encoders (libx265, libsvtav1) as device `cpu` with vendor `CPU`, for testing without a GPU. |

## General

| Setting | Owner | Default | Validation |
|---|---|---|---|
| Local admin password | Identity | set in the first-run wizard | ≥ 12 chars |
| Backups: daily snapshot | Platform | on, 03:00, keep 7 | keep 1–90 |
| Backup now / Restore | Platform | action | restore only from `data/backups` |
| Base URL (display) | Platform | from env | read-only |

### Schedules (advanced, collapsed)

All from [ticket 24](../../.scratch/v1-spec/issues/24-scheduling.md). Each row is owned by the context that runs the loop. Acquisition's RSS, pending and sweep rows are `GET · PUT /api/acquisition/schedules`; the Jellyfin full reconcile is `GET · PUT /api/library/schedules`; the sweeps' searches are queued 4 s apart.

| Loop | Owner | Default | Bounds |
|---|---|---|---|
| RSS sync per indexer | Acquisition | 15 min | 10–120 min |
| qBittorrent poll | Acquisition | 2 s active / 15 s idle | 1–10 s / 5–60 s |
| Pending (delay) queue check | Acquisition | 1 min | fixed |
| Seeding goal check | Acquisition | 5 min | 1–60 min |
| Missing backlog sweep | Acquisition | daily, spread | 1–7 days |
| Cutoff-unmet sweep | Acquisition | weekly, spread | 1–30 days |
| Prowlarr indexer list refresh | Acquisition | 1 h | 15 min–24 h |
| TRaSH sync | Acquisition | daily | daily, or off (the TRaSH Guides sync switch under Quality); a weekly option isn't built yet |
| TMDB refresh | Catalog | continuing series daily; ended series and movies weekly; images < 5 months | images must stay < 6 months (TMDB terms) |
| Subtitle upgrade check | Subtitles | 6 h | 1–24 h (edited under Settings → Subtitles; the same sweep retries missing languages) |
| Jellyfin full reconcile | Library | daily | 6 h–weekly |

## Connections

Owner: **Platform** (`platform.connections.*`). Each adapter is configured once and serves every context port it implements: the Jellyfin adapter serves both Identity's auth port and Library's library port. Contexts never store credentials. Each card has a status badge (live from `platform.ServiceHealthChanged`) and **Test**.

| Setting | Default | Validation / Test result |
|---|---|---|
| TMDB API key | none (required) | Test: key valid. The user supplies the key; none is bundled ([ticket 09](../../.scratch/v1-spec/issues/09-choose-metadata-source.md)). |
| qBittorrent URL, API key | none (required) | Test: version ≥ 5.2 ([ticket 18](../../.scratch/v1-spec/issues/18-import-and-library.md)), category `magpie` created |
| qBittorrent category | `magpie` | `[a-z0-9-]` |
| qBittorrent download path / same path in Magpie | none (paths match) | both or neither. The adapter rewrites qBittorrent's `content_path` from the first prefix to the second, whole folders only. |
| Prowlarr URL, API key | none (required) | Test: lists indexers |
| Jellyfin URL, API key | none (required) | Test: version ≥ 12, libraries listed, WebSocket connects. Auth header `Authorization: MediaBrowser Token=…` only. |
| OpenSubtitles app key | bundled with Magpie | not user-editable ([ticket 16](../../.scratch/v1-spec/issues/16-subtitle-design.md)) |
| OpenSubtitles username / password | none | Test login. Shown under Settings → Subtitles; stored as platform connection config. |

## Library

Owner: **Library** unless noted.

| Setting | Default | Validation |
|---|---|---|
| Root folders (Movies, TV) | set in the wizard | must exist, be writable, and be visible to Jellyfin |
| After download (seeding mode) | Seed until goals | `SEED_UNTIL_GOALS` \| `DONT_SEED`. Owner: Acquisition (`acquisition.seedingMode`, in `GET · PUT /api/acquisition/download-options`; also shown under Downloads); Library receives it in `DownloadCompleted.SeedingMode`. |
| Hardlink check | automatic | Shows the result per root folder against qBittorrent's save path for Magpie's category. Copy fallback when downloads and library are on different filesystems. |
| Naming | fixed Jellyfin convention | read-only: `Movies/Title (Year) [tmdbid-N]/Title (Year).mkv`, `TV/Show (Year) [tmdbid-N]/Season 01/Show S01E02.mkv` |
| Scan existing library | action | match by `[tmdbid-N]`, else fuzzy title+year → review. `POST /api/library/scan`; status and counts from `GET /api/library/scan`; review queue from Catalog's `/api/catalog/scan-review` |
| Rename matched folders on scan | off | `library.scan.options` `{renameMatched}`, `PUT /api/library/scan/options` |

## Quality

Owner: **Acquisition**. Decided in [ticket 10](../../.scratch/v1-spec/issues/10-quality-model.md).

| Setting | Default | Validation |
|---|---|---|
| Quality profiles | first-run presets `720p`, `1080p`, `2160p` (resolution-only: one group with every source at that resolution, which is also the cutoff; source preference from TRaSH CF scores) | name unique; cutoff must be an allowed quality; one shared pool for movies and TV |
| Default profile for new requests | `1080p` | must exist |
| Profile sizes (MB/min min/preferred/max per quality) | presets: the lowest TRaSH minimum of the group (720p 10, 1080p 12.5, 2160p 25), preferred 1999, max 2000 | 0 ≤ min ≤ preferred ≤ max |
| Profile scores: min CF score, cutoff CF score, upgrade-until | from preset | integers |
| Custom formats (Advanced) | TRaSH-linked set | regexes compile with `regexp2`, 100 ms match timeout |
| "Block term" quick action | none | creates a CF scored −10000 |
| TRaSH Guides sync | on, daily; **Sync now** queues one | user score overrides kept across syncs; the first-run quality step starts the first sync |
| Delay before grabbing (default delay profile) | 0 min | 0–1440. Bypassed when the release meets the cutoff or reaches the profile's maximum attainable CF score. |
| Search settle window | 30 s | 5–120 s |
| Movie availability rule | search only after TMDB digital/physical release date; with neither, theatrical + 90 days | fixed rule, owner Catalog ([ticket 31](../../.scratch/v1-spec/issues/31-import-wizard-and-model-gaps.md)) |

## Indexers

Owner: **Acquisition**. The list is read-only from Prowlarr ([ticket 22](../../.scratch/v1-spec/issues/22-indexer-engine.md)); FlareSolverr is set up in Prowlarr.

| Setting | Default | Validation |
|---|---|---|
| Per-indexer priority | 25 | 1–50 (lower is preferred) |
| Per-indexer minimum seeders | 1 | 0–100 |
| Per-indexer after-download override | Use global | `USE_GLOBAL` \| `SEED_UNTIL_GOALS` \| `DONT_SEED` |
| Per-indexer seeding goal | use global: ratio 2.0 or 14 days, whichever first | ratio 0–100, days 0–365; empty = global |
| Maximum release size | off | bytes in `acquisition.maximumSize`, 0 = off; no UI yet |
| Search timeout per indexer | 60 s | 10–120 s |
| Result cache | 30 min | 0–120 min (0 = off; a search's own results still stay 30 min) |
| Indexer list sync | 1 h | 15 min–24 h |

The per-indexer rows live on each indexer (`PUT /api/acquisition/indexers/{id}`); the last three are one setting, `acquisition.indexers` (`GET/PUT /api/acquisition/indexer-options`).

## Downloads

Owner: **Acquisition**. Decided in [ticket 11](../../.scratch/v1-spec/issues/11-blocked-file-policy.md). The first five rows and the after-download mode are `GET · PUT /api/acquisition/download-options`; patterns, the switch and the mode save on change, the timeouts and the seeding goal with Save.

| Setting | Default | Validation |
|---|---|---|
| Reject releases with blocked files | on | |
| Blocked file patterns | `*.exe *.msi *.bat *.cmd *.com *.scr *.lnk *.ps1 *.vbs *.js *.jar *.apk *.dmg *.iso *.rar *.r[0-9][0-9] *.zip *.7z` | glob or `regex:` prefix; regex must compile |
| Magnet metadata timeout | 5 min (then temporary blocklist for 24 h) | 1–60 min |
| Stalled download limit | 6 h (then `DownloadFailed`, temporary blocklist for 24 h, grab next) | 30 min–72 h |
| Global seeding goal | ratio 2.0 or 14 days, whichever first (`acquisition.seedingGoal`) | ratio 0–100, days 0–365 |
| Search when a title is added | on | `GET · PUT /api/acquisition/search-options`; saves on change. Requested and manually added titles only. |

## Subtitles

Owner: **Subtitles**, except where noted. Decided in [ticket 16](../../.scratch/v1-spec/issues/16-subtitle-design.md).

| Setting | Default | Validation |
|---|---|---|
| Wanted languages (ordered, with Forced / HI-ok flags) | English | Owner: **Catalog** (`catalog.Reader.WantedLanguages`). ≥ 1 language, ISO 639-1. Subtitles and Transcode both read it. |
| OpenSubtitles login | none | Platform connection config (see Connections). Shows today's quota (20 free / 1000 VIP). |
| Generate with Whisper when nothing is found | off | original language only; the admin turns it on once an agent passes `WHISPER_VULKAN` |
| Whisper model | `large-v3-turbo` | `large-v3-turbo` \| `large-v3` \| `medium` \| `small` |
| Auto-sync downloaded subtitles | on | never applied to Whisper output |
| Upgrade window | 7 days | 0–30 days |
| Minimum score | episodes 90%, movies 70% | 50–100 |

## Transcode

Owner: **Transcode**. Decided in [ticket 13](../../.scratch/v1-spec/issues/13-transcode-policy.md).

| Setting | Default | Validation |
|---|---|---|
| Enabled | on | |
| Target | HEVC 10-bit | `HEVC_10` \| `AV1`. AV1 runs only on agents that passed an AV1 test. Container is always MKV. |
| Quality preset | High | Archive \| High \| Balanced \| Small |
| Per-encoder overrides (Advanced) | none | per encoder: CQ/ICQ/QP value in that encoder's range |
| Max bitrate per resolution | 2160p 25, 1080p 10, 720p 5, SD 2 Mbps | 1–200 Mbps |
| Skip: already target codec and under cap | on | |
| Skip: source bitrate already low | on, below 50% of its resolution cap | 10–100% of cap |
| Skip: result not smaller by | 15% | 0–90% |
| Failed job retries | 3 attempts, backoff 1 / 5 / 30 min, different agent preferred | fixed |
| Skip Dolby Vision / HDR10+ | on (fixed in v1) | read-only |
| Backlog | action: Re-scan library | |

## Agents

Owner: **Transcode**. Decided in [ticket 14](../../.scratch/v1-spec/issues/14-agent-protocol.md).

| Setting | Default | Validation |
|---|---|---|
| Shared secret | from `MAGPIE_AGENT_SECRET` | shown masked with Copy; no rotate button. Rotating it means changing env and restarting. |
| Agent scratch requirement | ≥ 2.2 × source size (direct read ≈ 1.2 ×) | fixed |
| Concurrent transfers (server-wide) | 2 | 1–16 |
| Default jobs per GPU | 1 | 1–4 |
| Per agent: slots | 1 per GPU | 0 (drain) – 8 |
| Per agent: transfer mode | Download first | `DOWNLOAD_FIRST` \| `DIRECT_READ` |
| Heartbeat / lease | 10 s / 60 s | fixed |

Per-agent settings are edited on Activity → Agents.

## Users

Owner: **Identity**. Decided in [ticket 15](../../.scratch/v1-spec/issues/15-users-and-auth.md).

| Setting | Default | Validation |
|---|---|---|
| Role per user | Requester | `ADMIN` \| `REQUESTER`; at least one admin must remain |
| Auto-approve movies / TV per user | off | |
| View others' requests per user | off | |
| Report issues per user | on | |
| Default permissions for new Jellyfin users | Requester, auto-approve off | |
| Allow Jellyfin sign-in | on | the local admin is always available |

## Notifications

Owner: **Activity**. Per user, visible to every user ([ticket 25](../../.scratch/v1-spec/issues/25-notifications-activity.md)).

| Setting | Default | Validation |
|---|---|---|
| Notification URLs (label + shoutrrr URL) | none | at most 10 per user; URL parses with shoutrrr, schemes `discord`, `gotify`, `ntfy`, `pushover`, `slack`, `smtp`, `telegram`; Send test |
| Events per URL | none ticked | event must be in the user's role-scoped list |
| In-app notifications | always on | |
| Global activity retention | 90 days | 7–365 (admin only) |

Role-scoped event lists:
- **Requester:** their request approved/declined, available, issue reply/resolved.
- **Admin:** also request pending approval, issue reported, failures (import, transcode, service unreachable, agent offline), grab, import, transcode, blocked-file rejection.

## Per-title settings (title page → Settings tab)

| Setting | Owner | Default |
|---|---|---|
| Quality profile | Catalog (stores the `ProfileID`; profiles are owned by Acquisition) | profile from the request or add dialog |
| Episode order (TMDB episode group) | Catalog | aired order |
| Monitored (title / season / episode) | Catalog | on |
| Exclude from transcoding | Transcode (keyed by `TitleID`) | off |
| Subtitle language override | Catalog (keyed by `TitleID`; publishes `LanguagesChanged`) | use global |
