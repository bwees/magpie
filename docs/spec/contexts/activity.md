# Activity

What happened, and telling the right people. Activity records every durable event onto title timelines and the global history, keeps the per-movie/episode pipeline status the UI shows, and delivers notifications in-app and to each user's external URLs.

- **Depends on (Readers):** `catalog.Reader`, `identity.Reader`, `requests.Reader`; `acquisition.Reader` and `transcode.Reader` for the pipeline repair only.
- **Publishes:** `activity.NotificationCreated`, `activity.AttentionChanged`, and the ephemeral `activity.ProjectionUpdated`.
- **Consumes:** every durable event in the [event catalog](../00-context-map.md#event-catalog), including `platform.ServiceHealthChanged`.
- **Importer entrypoint:** `api.Importer` (migration job only): Sonarr/Radarr grab and import history onto timelines.
- **Decided in:**
  - [ticket 25](../../../.scratch/v1-spec/issues/25-notifications-activity.md), including the shoutrrr amendment.
  - [ticket 23](../../../.scratch/v1-spec/issues/23-module-boundaries.md): the pipeline status projection.
  - [ticket 17](../../../.scratch/v1-spec/issues/17-pipeline-ia.md): the timeline and "Needs you" UI.

## Owns / Must not

| Owns | Must not |
|---|---|
| Title timelines, global history, pipeline-status projection | Change any other context's state |
| The attention list ("Needs you" and the Home badge) | Act on attention items (actions call the owning context's HTTP API) |
| In-app notifications and read state | Decide *whether* something failed; it only reports events |
| Each user's notification URLs and their event subscriptions | Send notifications for events the user's role may not see |
| The shoutrrr sender adapter port | Store progress ticks (ephemeral events) |

## Data model

| Table | Key columns |
|---|---|
| `activity_timeline` | `id`, `event_id` (unique), `title_id`, `episode_ids` JSON, `type`, `severity` (`INFO` \| `WARN` \| `ERROR`), `text`, `detail`, `at` |
| `activity_history` | `id`, `event_id` (unique), `type`, `severity`, `title_id` (nullable), `title_name` (the name when recorded, kept after removal), `episode_ids` JSON, `summary`, `detail`, `at` |
| `activity_pipeline` | PK (`title_id`, `episode_id`; `episode_id` = `""` for movies), `season`, `number` (sort order, from `catalog.Reader` and `EpisodeGroupChanged`), facet columns `wanted`, `search`, `download`, `file`, `media_file_id`, `jellyfin`, `note` (the latest event's own detail), `stage`, `failed`, `detail`, `subs_chip` JSON, `transcode_chip` JSON, `last_event_id`, `updated_at` |
| `activity_attention` | `id`, `kind` (`REQUEST_PENDING` \| `ISSUE_OPEN` \| `PIPELINE_FAILED` \| `AGENT_OFFLINE` \| `SERVICE_DOWN` \| `TRANSCODE_FAILED`), `ref` (request/issue/title/media file/agent/service ID), `title_id?`, `text`, `action` (`APPROVE` \| `REGRAB` \| `SEARCH_AGAIN` \| `RETRY` \| `VIEW_AGENT` \| `OPEN_TITLE` \| `OPEN_SETTINGS`; the UI picks the label and the owner's endpoint), `opened_at`, `opened_event_id`, `cleared_at?`, `cleared_event_id?`; unique open (`kind`, `ref`); unique (`kind`, `opened_event_id`) and (`kind`, `cleared_event_id`) so a redelivered event changes nothing |
| `activity_refs` | PK (`kind` `MEDIA_FILE` \| `DOWNLOAD` \| `SEARCH`, `ref`), `title_id`, `episode_ids` JSON. Written by every handler from `FileImported`, `ReleaseGrabbed`/`DownloadCompleted` and `SearchStarted`, so events carrying only a file, download or search ID find their title |
| `activity_removed_titles` | `title_id` PK, `removed_at`. Events arriving after `catalog.TitleRemoved` (e.g. `FileDeleted{TITLE_REMOVED}`) add nothing to that title's timeline or pipeline |
| `activity_notifications` | `id`, `user_id`, `event_id`, `kind`, `title`, `body`, `link`, `created_at`, `read_at`; unique (`user_id`, `event_id`, `kind`) |
| `activity_notification_urls` | `id`, `user_id`, `label`, `url` (secret; never returned unmasked), `kinds` JSON, `created_at`, `last_sent_at`, `last_error` |

## Query port

```go
package api

// Main stages only. Subtitles and transcode are separate chips beside the bar.
type Stage string // REQUESTED | SEARCHING | DOWNLOADING | IMPORTING | AVAILABLE

type PipelineRow struct {
    Title     domain.TitleID
    Episode   *domain.EpisodeID // nil for movies
    Season, Number int          // the episode's current numbering; 0 for movies
    Stage     Stage
    Failed    bool
    Detail    string // human text, e.g. "Waiting for Jellyfin", "No accepted release"
    Download  *domain.DownloadID // set while download=ACTIVE: the UI overlays live % from ephemeral DownloadProgressed
    Subs      *SubsChip      // nil until a file exists
    Transcode *TranscodeChip // nil until the file is eligible (CutoffMet) or skipped
    Updated   time.Time
}

type SubsChip struct {
    State     string            // PENDING | SETTLED
    Satisfied []domain.Language // e.g. [en] → "Subs ✓ en"
    Missing   []domain.Language
}

type TranscodeChip struct {
    State   string // QUEUED | RUNNING | DONE | SKIPPED | FAILED
    Saved   *int   // percent saved, when DONE
    Reason  string // skip/failure reason
    Job     *domain.JobID // RUNNING: the UI overlays live % from ephemeral transcode.JobProgressed
}

type AttentionItem struct {
    ID, Kind, Ref, Text, Action string
    Title    *domain.TitleID
    OpenedAt time.Time
}

type TimelineEntry struct {
    EventID   string
    Type      string
    Severity  string
    Title     *domain.TitleID // nil for history entries about no title
    TitleName string          // history: the title's name when recorded
    Episodes  []domain.EpisodeID
    Text     string
    Detail   string
    At       time.Time
}

type Notification struct {
    ID, Kind, Title, Body, Link string
    CreatedAt                   time.Time
    Read                        bool
}

// Page selects entries strictly older than Before (nil = from the newest).
// Limit ≤ 0 means DefaultPageLimit (50); it is capped at MaxPageLimit (200). Page.EffectiveLimit() applies both.
type Page struct{ Before *time.Time; Limit int }

type Reader interface {
    // Timeline returns a title's entries, newest first. Unknown title → empty slice, no error.
    Timeline(ctx context.Context, title domain.TitleID, p Page) ([]TimelineEntry, error)
    // History returns the global feed (90-day retention), newest first. minSeverity keeps entries at
    // that severity or above (INFO < WARN < ERROR); "" keeps all.
    History(ctx context.Context, minSeverity string, p Page) ([]TimelineEntry, error)
    // Pipeline returns one row per movie or per episode of the title, episodes in season/number order.
    // A title with no events yet returns rows at REQUESTED once catalog.TitleAdded is processed; unknown → empty.
    Pipeline(ctx context.Context, title domain.TitleID) ([]PipelineRow, error)
    // TitleStages returns the summary stage per title: the least-advanced row (first in season/number
    // order on ties), for library grids. Unknown titles are absent from the map.
    TitleStages(ctx context.Context, titles []domain.TitleID) (map[domain.TitleID]PipelineRow, error)
    // Notifications returns a user's in-app notifications, newest first.
    Notifications(ctx context.Context, user domain.UserID, unreadOnly bool, p Page) ([]Notification, error)
    UnreadCount(ctx context.Context, user domain.UserID) (int, error)
    // Attention returns open attention items, oldest first. Admin view; requesters get none.
    Attention(ctx context.Context) ([]AttentionItem, error)
}
```

No other context depends on this Reader in v1. The UI uses it through HTTP.

The api package also exports the string constants for severities, chip states, attention kinds and notification kinds.

```go
// Importer is the migration entrypoint (platform migration job only). Idempotent on Source.
type Importer interface {
    ImportTimeline(ctx context.Context, in []ImportTimelineEvent) error
}
type ImportTimelineEvent struct {
    Source      domain.SourceKey
    Title       domain.TitleID
    Episodes    []domain.EpisodeID // empty for movies
    Kind        ImportKind         // GRABBED | IMPORTED | DOWNLOAD_FAILED | FILE_DELETED
    ReleaseName string
    Reason      string             // failure or deletion reason
    At          time.Time          // original timestamp
}
```

## Events published

```go
// Durable. One per recipient per notification, so the live gateway can push it to that user only:
// it implements domain.Personal with Recipient() = User, which hides it from admins too.
type NotificationCreated struct {
    Notification string // ID
    User         domain.UserID
    Kind         string
    Title, Link  string
}

// Durable. An attention item opened or cleared. Admin-only on the live gateway.
type AttentionChanged struct {
    Item   string // attention item ID
    Kind   string
    Ref    string
    Open   bool   // false = cleared
    Count  int    // open items after this change (Home badge)
}

// Ephemeral. A handler committed a change to a projection; views showing it refetch.
// Views can't refetch on the source event: the projection may not have handled it yet.
type ProjectionUpdated struct {
    Projection string          // TIMELINE | HISTORY | PIPELINE
    Title      *domain.TitleID // nil for HISTORY
}
```

- Aggregate ID: the user for `NotificationCreated`, the attention item for `AttentionChanged`, the title (or `HISTORY`) for `ProjectionUpdated`.

## Events and commands consumed

Every durable event other contexts and the platform publish is handled by the handlers below; Activity's own events and the `transcode.RunAgentJob` command are not. Each dedupes on (`handler`, `event_id`) and on the unique `event_id` columns. After each change a handler emits `ProjectionUpdated` once its transaction commits.

| Handler | Reaction | Failure |
|---|---|---|
| `timeline` | Events about a title (directly, or through `activity_refs`) write one `activity_timeline` row with rendered text. `catalog.TitleRemoved` deletes that title's timeline. | Retry via the dispatcher. Render errors fall back to the raw event type as text. |
| `history` | Every durable event writes one `activity_history` row, with the title's name from `catalog.Reader`. | Same |
| `pipeline` | Updates facets for the affected movie/episode rows, then recomputes `stage` and the chips (tables below). Ignores events older than `last_event_id` for the row (ULID order). `catalog.TitleRemoved` deletes the title's rows. Opens and clears `PIPELINE_FAILED` items, since only it knows when a row's `failed` changes. | Same |
| `attention` | Opens and clears the other `activity_attention` items (table below) and publishes `AttentionChanged` for each change. | Same |
| `notify` | Maps the event to notification kinds and recipients (table below), writes in-app rows, publishes `NotificationCreated`, and enqueues one backlite `activity.send` job per matching external URL. | `send` retries 3× with backoff, then stores `last_error`. It never blocks the dispatcher. |

### Pipeline facets

| Event | Facet change (for the payload's title/episodes) |
|---|---|
| `catalog.TitleAdded`, `MonitoringChanged` (+) | Create rows: `wanted=true`, everything else empty |
| `catalog.MonitoringChanged` (−) | `wanted=false` (row kept, shown as unmonitored) |
| `acquisition.SearchStarted` | `search=RUNNING` |
| `acquisition.SearchCompleted` | `Outcome = NOTHING_ACCEPTED` → `search=NOTHING_ACCEPTED`; `GRABBED`, `PENDING_DELAY`, `RELEASES_FOUND` or `NO_RELEASES` → `search` cleared (detail "Waiting for a better release" when pending, "No releases found" when none came back) |
| `acquisition.ReleaseRejected` | Rows downloading its `Download`: `download=` cleared, detail "Rejected: <reason>". Other searching or downloading rows: detail only; search stays `RUNNING` |
| `acquisition.ReleaseGrabbed` | `download=ACTIVE` |
| `acquisition.DownloadFailed` | `download=FAILED` on the rows still on that download (a fallback grab keeps its rows) |
| `acquisition.DownloadRemoved` | `download=` none on the rows still on that download |
| `acquisition.DownloadCompleted` | `download=DONE` |
| `library.FileImported` | `file=PRESENT`, `jellyfin=false`, `subtitles=PENDING`, `download=` cleared |
| `library.ImportFailed` | `download=FAILED`, detail = reason |
| `subtitles.CoverageSettled` | `subtitles=SETTLED` with satisfied/missing languages |
| `transcode.JobQueued` (transcode kind) | `transcode=QUEUED` |
| `transcode.JobStarted` (transcode kind) | `transcode=RUNNING`, job ID stored |
| `transcode.TranscodeSkipped` | `transcode=SKIPPED` with reason |
| `transcode.JobFailed` (transcode kind, `Final`) | `transcode=FAILED` |
| `transcode.JobFailed` (transcode kind, not `Final`) | `transcode=QUEUED`: the job waits for its next attempt, whose `JobStarted` sets `RUNNING` again |
| `transcode.JobCancelled` | no `transcode` chip |
| `library.FileTranscoded` | `transcode=DONE`, saved % from sizes |
| `library.ItemAvailable` | `jellyfin=true` |
| `library.FileDeleted` | `REMOVED` / `TITLE_REMOVED`: `file=NONE`, transcode/subtitles/jellyfin cleared. `UPGRADED`: no change, because the row already reflects the replacing file from its `FileImported`. |

Which rows an event changes:
- Events with episodes change those episodes' rows; events with a title and no episodes change every row of the title (a movie has one).
- `SearchCompleted` and `DownloadFailed` use the episodes their `SearchStarted` / `ReleaseGrabbed` named; `ImportFailed` finds its title through the download.
- File events (`CoverageSettled`, `Job…`, `TranscodeSkipped`, `FileTranscoded`, `FileDeleted`) change the rows whose `media_file_id` is that file, so late events for a replaced file change nothing.
- `ReleaseRejected` with a `Download` changes the rows whose current download is that ID, whatever its order against the fallback's `ReleaseGrabbed`; rows the fallback covers are moved by that `ReleaseGrabbed`. Its detail then goes on the title's other rows that are searching or downloading. Without a `Download` (rejected before the grab, or stored before the field existed), only the detail changes.
- `FileImported` also stores `media_file_id` and clears the transcode chip: the new file isn't eligible yet.
- The detail is the latest event's note ("Rejected: …", "No accepted release", a failure reason) unless a stage rule names one.

### Stage derivation

Main stages: Requested → Searching → Downloading → Importing → Available, where Available means the file is in the library. Evaluated top to bottom; the first match wins.

1. `download=ACTIVE` → `DOWNLOADING` (detail "Upgrade" when `file=PRESENT`).
2. `file=NONE` and `download=DONE` → `IMPORTING`.
3. `file=NONE` and `download=FAILED` → `DOWNLOADING`, `failed=true`.
4. `file=NONE` and `search=RUNNING` → `SEARCHING`.
5. `file=NONE` and `search=NOTHING_ACCEPTED` → `SEARCHING`, `failed=true`.
6. `file=NONE` → `REQUESTED`.
7. `jellyfin=false` → `IMPORTING`, detail "Waiting for Jellyfin". Library publishes `ItemAvailable` with every `FileImported`, so this only lasts until that event is handled.
8. Otherwise → `AVAILABLE`.

### Chips

Shown beside the bar; they never change the main stage.

| Chip | Facet | Text |
|---|---|---|
| Subs | `subtitles=PENDING` | "Subs …" |
| | `subtitles=SETTLED`, none missing | "Subs ✓ en, es" |
| | `subtitles=SETTLED`, some missing | "Subs ✓ en · es missing" |
| Transcode | `QUEUED` | "Transcode queued" |
| | `RUNNING` | "Transcode 41%" (live % from ephemeral `JobProgressed`, never stored) |
| | `DONE` | "Transcoded −62%" |
| | `SKIPPED` | "Original kept" (reason on hover) |
| | `FAILED` | "Transcode failed" (failed styling) |

### Attention items

| Opens on | Clears on | Kind |
|---|---|---|
| `requests.RequestCreated` | `RequestApproved` / `RequestDeclined` for that request | `REQUEST_PENDING` |
| `requests.IssueReported` | `requests.IssueResolved` | `ISSUE_OPEN` |
| pipeline row becomes `failed=true` | row no longer failed | `PIPELINE_FAILED` (one per title) |
| `transcode.JobFailed{Final}` (transcode kind) | a later `JobQueued` or `FileTranscoded` for the file | `TRANSCODE_FAILED` (ref = media file) |
| `transcode.AgentOffline` | `transcode.AgentOnline` | `AGENT_OFFLINE` |
| `platform.ServiceHealthChanged{Healthy: false}` | same with `Healthy: true` | `SERVICE_DOWN` |

An admin can also dismiss every open item with `POST /api/activity/attention/clear`. A dismissed item gets `cleared_at` and no `cleared_event_id`, and Activity publishes `AttentionChanged{Open: false}` for it. Its clearing event then changes nothing, and its next opening event opens a new item.

### Notification kinds and recipients

| Kind | From event | Recipients | Role scope |
|---|---|---|---|
| `REQUEST_PENDING` | `requests.RequestCreated` (only published when not auto-approved) | Admins | Admin |
| `REQUEST_APPROVED` | `requests.RequestApproved` | Request's user | Requester |
| `REQUEST_DECLINED` | `requests.RequestDeclined` | Request's user | Requester |
| `REQUEST_AVAILABLE` | `requests.RequestAvailable` | Request's user | Requester |
| `ISSUE_REPORTED` | `requests.IssueReported` | Admins | Admin |
| `ISSUE_REPLY` / `ISSUE_RESOLVED` | `requests.IssueCommented` / `IssueResolved` | Reporter (and admins for replies by the reporter) | Requester |
| `IMPORT_FAILED` | `library.ImportFailed` | Admins | Admin |
| `DOWNLOAD_FAILED` | `acquisition.DownloadFailed` | Admins | Admin |
| `TRANSCODE_FAILED` | `transcode.JobFailed` (transcode kind, `Final = true`) | Admins | Admin |
| `SERVICE_UNREACHABLE` | `platform.ServiceHealthChanged{Healthy: false}` | Admins | Admin |
| `AGENT_OFFLINE` | `transcode.AgentOffline` | Admins | Admin |
| `GRABBED` | `acquisition.ReleaseGrabbed` | Opt-in only | Admin |
| `IMPORTED` | `library.FileImported` | Opt-in only | Admin |
| `TRANSCODED` | `library.FileTranscoded` | Opt-in only | Admin |
| `BLOCKED_FILE_REJECTED` | `acquisition.ReleaseRejected` with `blockedFile` | Opt-in only | Admin |

- **In-app:** created for every recipient of the non-opt-in kinds. Opt-in kinds go only to URLs that subscribe to them, never in-app.
- **Admin scope for requester kinds:** admins may subscribe their URLs to requester-scoped kinds for *any* user.
- **Requester limits:** requesters may subscribe only to requester-scoped kinds for their own requests. Saving any other kind returns `422`; at send time it's also skipped.
- **Issue links:** issue notifications link to the issue's thread, `/requests?issue={id}`.
- **Request lookups:** recipients and titles come from `requests.Reader` (request → user) and `catalog.Reader` (title name for message text).
- **Recipients:** "Admins" means the enabled admins (`identity.Reader.Admins`); a disabled user gets nothing, in-app or by URL. Failures that carry only a download or file ID find their title through `activity_refs`.
- **Links:** in-app links are app paths: the title (`/titles/{id}`) when known, else `/requests`, `/settings/connections` (service unreachable), `/activity/agents` (agent offline) or `/activity`. External messages carry the same path joined to `MAGPIE_BASE_URL`, or no link when it is unset.
- **Sending:** one `activity.send` backlite job per subscribed URL, queued in the handler's transaction. The job names the URL by ID, so the secret stays out of the queue. At send time it is dropped if the URL was deleted, no longer subscribes to the kind, or its owner may no longer hold the kind (demoted or disabled).
  - `ErrUnavailable`: tried 3 times in all, waiting 1 min then 5 min, then `last_error` is stored.
  - `ErrRejected` or an invalid URL: not retried; `last_error` is stored at once.
  - A delivery sets `last_sent_at` and clears `last_error`.

## External ports

```go
type NotificationSender interface {
    // Validate parses a shoutrrr URL (discord://, telegram://, ntfy://, pushover://, smtp://, …).
    // ErrInvalidURL for unknown schemes or malformed tokens. No network call.
    Validate(url string) error
    // Send delivers one message. Returns ErrRejected (4xx from the service) or ErrUnavailable (network/5xx).
    Send(ctx context.Context, url string, m Message) error
}

type Message struct{ Title, Body, Link string }
```

Implemented in `internal/adapters/shoutrrr` with the nicholas-fedor fork of shoutrrr (MIT).

- **Schemes:** `discord`, `gotify`, `ntfy`, `pushover`, `slack`, `smtp`, `telegram`. These parse without a network call; other shoutrrr services (matrix signs in on parse, for one) are `ErrInvalidURL`.
- **Errors:** the adapter reads the service's last HTTP exchange, because some services (telegram) swallow failed responses. A 4xx other than 408/429 is `ErrRejected`; network errors, 5xx, 408 and 429 are `ErrUnavailable`; any other failure (e.g. SMTP) is `ErrUnavailable`.

## HTTP surface

| Route | Who | Purpose |
|---|---|---|
| `GET /api/activity/timeline?title=&before=&limit=` | signed in (requesters: own requested titles, without details such as release names) | Title timeline |
| `GET /api/activity/pipeline?title=` | same | Pipeline rows (stage + chips) for one title |
| `GET /api/activity/pipeline?titles=…&titles=…` | signed in (requesters: own titles; at most 500 titles) | Summary stage per title (library grids) |
| `GET /api/activity/pipeline?active=true` | signed in (requesters: own titles) | Rows currently searching, downloading, importing or transcoding ("Live now"), at most 200 |
| `GET /api/activity/pipeline?recent=true` | signed in (requesters: own titles, found among the latest 500) | Summary row of the 24 titles whose pipeline changed most recently, newest first (Home "Recently active"; Catalog can't sort by activity) |
| `GET /api/activity/history?severity=&before=&limit=` | admin | Global feed |
| `GET /api/activity/attention` | admin | Open attention items ("Needs you") |
| `POST /api/activity/attention/clear` | admin | Dismiss every open attention item (`204`) |
| `GET /api/activity/notifications?unread=&before=&limit=` | signed in | Own in-app notifications, newest first, plus `unreadCount` |
| `POST /api/activity/notifications/{id}/read`, `POST …/read-all` | signed in | Mark read (`204`; someone else's notification is `404`) |
| `GET /api/activity/notification-kinds` | signed in | Kinds the caller's role may subscribe to: `kind`, `label`, `scope` (`REQUESTER` \| `ADMIN`), `optIn` |
| `GET/POST /api/activity/notification-urls` | signed in | Own URLs (masked) / add `{label, url, kinds}` |
| `PATCH/DELETE /api/activity/notification-urls/{id}` | owner | Edit `{label?, kinds?}` (null or left out = unchanged), delete |
| `POST /api/activity/notification-urls/{id}/test` | owner | Send a test message now: `204`, `422` when the service refuses it, `502` when it can't be reached |

- Pipeline takes exactly one of `title`, `titles`, `active=true` or `recent=true`; anything else is `400`. A title a requester may not see is `404`.
- `before` is an RFC 3339 time; `limit` follows `Page` (default 50, max 200).
- Requesters may see a title when `requests.Reader.Requesters` lists them, or any requester with `ViewOthersRequests`.

## Behaviour

- **Retention:**
  - Timeline rows live as long as their title and are deleted on `catalog.TitleRemoved`.
  - History rows older than 90 days, and in-app notifications older than 90 days, are pruned daily.
- **Severity:** `*Failed`, `ReleaseRejected`, `AgentOffline`, `SearchCompleted` with nothing accepted → `WARN` or `ERROR`. The rest are `INFO`.
  - `ERROR`: `DownloadFailed`, `ImportFailed`, `TitleAddFailed`, final `JobFailed`, `ServiceHealthChanged{Healthy: false}`.
  - `WARN`: `ReleaseRejected`, `TranscodeSwapRejected`, `AgentOffline`, `SearchCompleted{NOTHING_ACCEPTED}`, non-final `JobFailed`.
- **Pipeline repair:** once per boot, a backlite job (`activity.repair-pipeline`) checks the rows still waiting on a download or a running transcode against their owners. Earlier releases left such rows behind, because `ReleaseRejected` carried no download and a non-final `JobFailed` changed nothing.
  - `download=ACTIVE` whose download isn't in `acquisition.Reader.ActiveDownloads` → `download=` cleared.
  - `transcode=RUNNING` whose job (`transcode.Reader.Job`) is `QUEUED` or `OFFERED` → `QUEUED`; cancelled or unknown → chip cleared. Other states keep the chip.
  - A row an event changed since the job read it is left alone. `last_event_id` is kept, so events still queued apply after the repair. Changed titles get their `PIPELINE_FAILED` item opened or cleared and a `ProjectionUpdated`.
- **"Needs you"** (home screen) is the attention list Activity owns (table above). The UI reads `GET /api/activity/attention`, updates it from `AttentionChanged`, and calls the owning context's API for each action (approve → Requests, search again → Acquisition, view agent → Transcode).
- **Limits:** at most 10 notification URLs per user. A URL is validated on save; invalid → `422`. The 11th URL, and a kind the caller's role may not hold, are also `422`, each with its own problem type (`invalid-notification-url`, `notification-url-limit`, `notification-kind-not-allowed`).
- **Labels:** trimmed, at most 60 characters; a blank label becomes the service name (`Ntfy`). Kinds are deduped and stored in the table's order.
- **Secrets:** URLs are never returned in full. Responses show scheme and host plus `…`, and only the owner can edit them. Another user's URL is `404`.
- **Settings owned:** `activity.historyRetentionDays` (90), `activity.notificationRetentionDays` (90).

## Test seeds

1. **Given** `catalog.TitleAdded` for a movie, **then** one pipeline row exists at `REQUESTED` and one timeline row is written.
2. **Given** the same event is delivered twice, **then** exactly one timeline row, one history row and one pipeline update exist.
3. **Given** a movie at `SEARCHING`, **when** `ReleaseGrabbed` then `DownloadCompleted` then `FileImported` arrive, **then** the stage moves `DOWNLOADING` → `IMPORTING` → `IMPORTING` ("Waiting for Jellyfin") until `ItemAvailable` and the Subs chip is "Subs …".
4. **Given** a file imported, **when** `ItemAvailable` arrives, **then** the stage is `AVAILABLE` even while subtitles are still pending; after `CoverageSettled{Satisfied: [en]}` the Subs chip is "Subs ✓ en".
5. **Given** an available file, **when** `JobQueued` then `JobStarted` then `FileTranscoded` arrive, **then** the stage stays `AVAILABLE` and the Transcode chip goes "Transcode queued" → RUNNING (with job ID) → "Transcoded −N%".
6. **Given** an available file, **when** an upgrade `ReleaseGrabbed` arrives, **then** `DOWNLOADING` with detail "Upgrade".
7. **Given** `SearchCompleted{Outcome: NOTHING_ACCEPTED}`, **then** `SEARCHING` with `failed=true`, and `TitleStages` returns that row for the title.
8. **Given** events for one episode arrive out of order (older ULID after newer), **then** the older event doesn't change that row's facets.
9. **Given** `catalog.TitleRemoved`, **then** the title's timeline and pipeline rows are deleted and history rows stay.
10. **Given** `requests.RequestApproved` for user U, **then** U gets one in-app `REQUEST_APPROVED` notification and one `NotificationCreated` event; admins get nothing.
11. **Given** `library.ImportFailed`, **then** every enabled admin gets an in-app `IMPORT_FAILED`, and each admin URL subscribed to it gets one `activity.send` job.
12. **Given** a requester saves a URL subscribed to `IMPORT_FAILED`, **then** `422`.
13. **Given** `ReleaseGrabbed` and no URL subscribed to `GRABBED`, **then** no in-app notification and no send job.
14. **Given** `NotificationSender.Send` returns `ErrUnavailable` three times, **then** the job stops, `last_error` is set, and the in-app notification still exists.
15. **Given** an invalid shoutrrr URL, **when** saved, **then** `422` and nothing is stored.
16. **Given** a stored URL, **when** listed by its owner, **then** the token part is masked; another user gets `404`.
17. **Given** history rows older than 90 days, **when** the prune job runs, **then** they're deleted and newer rows remain.
18. **Given** `RequestCreated` for request R, **then** one `REQUEST_PENDING` attention item opens and `AttentionChanged{Open: true, Count: n+1}` is published; **when** `RequestApproved` for R arrives, **then** it clears with `AttentionChanged{Open: false}`; redelivery of either changes nothing.
19. **Given** `AgentOffline` for agent A delivered twice, **then** one `AGENT_OFFLINE` item exists; `AgentOnline` clears it.
20. **Given** `transcode.JobFailed{Final: false}`, **then** no notification and no attention item; with `Final: true`, both.
21. **Given** a running transcode, **when** `JobFailed{Final: false}` arrives, **then** the Transcode chip is `QUEUED` and the row leaves "Live now"; the next attempt's `JobStarted` sets `RUNNING` again.
22. **Given** a season pack D downloading two episodes and a fallback grab covering the first, **when** `ReleaseRejected{Download: D, Next: fallback}` arrives, **then** the first episode keeps downloading the fallback and the second has no download, with detail "Rejected: <reason>".
23. **Given** a row downloading a download that left qBittorrent, or running a job back in `QUEUED`, **when** the pipeline repair runs, **then** the download is cleared or the chip is `QUEUED`; an event read before the repair still applies after it.
24. **Given** open `AGENT_OFFLINE` and `REQUEST_PENDING` items, **when** an admin clears attention, **then** both close with `AttentionChanged{Open: false, Count: 0}`; a later `AgentOnline` changes nothing and a later `AgentOffline` opens a new item. A requester gets `403`.
25. **Contract (Reader fake vs real):**
    - Ordering: newest first.
    - Unknown title → empty slice, no error.
    - `Page.Limit` is capped at 200.
    - `Pipeline` orders episodes by season, then number.
