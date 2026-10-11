# Requests

Handles users asking for Titles, admin approval, tracking each request until Jellyfin has it, and user-reported Issues.

- **Depends on (Readers):** `catalog.Reader`, `identity.Reader`.
- **Publishes:** `RequestCreated`, `RequestApproved`, `RequestDeclined`, `RequestAvailable`, `IssueReported`, `IssueCommented`, `IssueActionRequested`, `IssueResolved`.
- **Consumes:** `catalog.TitleAdded`, `catalog.MonitoringChanged`, `catalog.TitleAddFailed`, `catalog.WantedReleased`, `catalog.EpisodesAnnounced`, `catalog.TitleRemoved`, `library.ItemAvailable`, `library.FileImported`, `subtitles.SubtitleAdded`.
- **Read by:** the platform live gateway (role filter) and Activity.

## Owns / Must not

| Owns | Must not |
|---|---|
| Requests, their scope, status and per-item availability | Add Titles or change monitoring itself (it publishes `RequestApproved`; Catalog acts) |
| Approval decisions and decline reasons | Choose releases, profiles' contents or files |
| Issues, Issue comments, Issue actions and resolution | Blocklist or re-run subtitles itself (it publishes `IssueActionRequested`) |
| | Store user permissions (it reads them from Identity) |

## Data model

| Table | Key columns |
|---|---|
| `requests` | `id`, `userId`, `kind` (`MOVIE` \| `SERIES`), `tmdbId`, `titleId?`, `scope` (`MOVIE` \| `ALL_SEASONS` \| `SEASONS`), `seasons` (JSON int[]), `profileId`, `status`, `createdAt`, `decidedBy?`, `decidedAt?`, `declineReason?`, `failureReason?`, `availableAt?` |
| `request_items` | `requestId`, `episodeId?` (null for a movie), `availableAt?` — one row per requested movie or released episode in scope |
| `issues` | `id`, `userId`, `titleId`, `episodeId?`, `kind` (`VIDEO` \| `AUDIO` \| `SUBTITLE` \| `OTHER`), `text`, `status` (`OPEN` \| `ACTION_PENDING` \| `RESOLVED`), `pendingAction?`, `createdAt`, `resolvedAt?`, `resolvedBy?` |
| `issue_comments` | `id`, `issueId`, `userId`, `body`, `createdAt` |

## Query port

```go
package api // internal/requests/api

var ErrNotFound = errors.New("requests: not found")

type Status string // PENDING | APPROVED | PROCESSING | PARTIALLY_AVAILABLE | AVAILABLE | DECLINED | FAILED

type Request struct {
    ID       domain.RequestID
    User     domain.UserID
    Kind     domain.MediaKind
    Tmdb     domain.TmdbID
    Title    *domain.TitleID // set once Catalog has the Title
    Scope    string          // MOVIE | ALL_SEASONS | SEASONS
    Seasons  []int
    Profile  domain.ProfileID
    Status   Status
    CreatedAt time.Time
    AvailableItems, TotalItems int
}

type Issue struct {
    ID      domain.IssueID
    User    domain.UserID
    Title   domain.TitleID
    Episode *domain.EpisodeID
    Kind    string
    Text    string
    Status  string // OPEN | ACTION_PENDING | RESOLVED
    Comments int
    CreatedAt time.Time
}

type Reader interface {
    // Request returns one request. ErrNotFound if unknown.
    Request(ctx context.Context, id domain.RequestID) (Request, error)
    // RequestsForTitle returns all requests linked to a Title, newest first. Empty slice if none.
    RequestsForTitle(ctx context.Context, title domain.TitleID) ([]Request, error)
    // Requesters returns the distinct users with a non-declined request for the Title.
    // Activity uses it to route "available" notifications.
    Requesters(ctx context.Context, title domain.TitleID) ([]domain.UserID, error)
    // OpenIssues returns OPEN and ACTION_PENDING issues, newest first. A nil title means all Titles.
    OpenIssues(ctx context.Context, title *domain.TitleID) ([]Issue, error)
    // Issue returns one issue. ErrNotFound if unknown.
    Issue(ctx context.Context, id domain.IssueID) (Issue, error)
}
```

## Events published

All durable.

```go
// RequestCreated: a request is waiting for approval. Not published for auto-approved requests.
type RequestCreated struct {
    Request domain.RequestID
    User    domain.UserID
    Kind    domain.MediaKind
    Tmdb    domain.TmdbID
    Scope   string
    Seasons []int
}

// RequestApproved: Catalog must make the scope tracked and monitored.
type RequestApproved struct {
    Request  domain.RequestID
    User     domain.UserID
    Kind     domain.MediaKind
    Tmdb     domain.TmdbID
    Scope    string // MOVIE | ALL_SEASONS | SEASONS
    Seasons  []int  // set for SEASONS
    Profile  domain.ProfileID
    Auto     bool   // approved by the user's auto-approve toggle or admin role
    ApprovedBy domain.UserID
}

type RequestDeclined struct {
    Request domain.RequestID
    User    domain.UserID
    Reason  string
}

// RequestAvailable: every item in scope is confirmed in Jellyfin. Published once per request.
type RequestAvailable struct {
    Request domain.RequestID
    User    domain.UserID
    Title   domain.TitleID
}

type IssueReported struct {
    Issue   domain.IssueID
    Title   domain.TitleID
    Episode *domain.EpisodeID
    Kind    string
    User    domain.UserID
}

// IssueCommented: someone replied on an Issue.
type IssueCommented struct {
    Issue  domain.IssueID
    Author domain.UserID
    Reporter domain.UserID
}

// IssueActionRequested: an admin chose a fix. Carries Title + Episode, not a MediaFile:
// Requests can't read Library. Acquisition and Subtitles resolve the file through library.Reader.
type IssueActionRequested struct {
    Issue   domain.IssueID
    Title   domain.TitleID
    Episode *domain.EpisodeID
    Action  string // REGRAB | RERUN_SUBTITLES
}

// IssueResolved: the Issue was closed, by a completed action or by a person.
type IssueResolved struct {
    Issue    domain.IssueID
    Reporter domain.UserID
    By       string // "ACTION" | user ID
}
```

## Events and commands consumed

| Message | From | Reaction | Idempotency | Failure |
|---|---|---|---|---|
| `catalog.TitleAdded{RequestID}` | Catalog | Link `titleId`; status → `PROCESSING`; build `request_items` from `catalog.Reader.Wanted` limited to the scope | Event ID; item rows keyed by `(requestId, episodeId)` | Unknown `RequestID` → ignore |
| `catalog.MonitoringChanged{RequestID}` | Catalog | Same as `TitleAdded` for a Title that already existed | Same | Same |
| `catalog.MonitoringChanged` (no RequestID), `catalog.WantedReleased`, `catalog.EpisodesAnnounced` | Catalog | For non-terminal requests on that Title, add `request_items` for newly released episodes in scope | Keyed rows | — |
| `catalog.TitleAddFailed{RequestID}` | Catalog | Status → `FAILED` with the reason. Admin can retry. | Event ID | — |
| `catalog.TitleRemoved` | Catalog | Requests on that Title stay as history (`titleId` kept); open Issues → `RESOLVED` | Event ID | — |
| `library.ItemAvailable` | Library | Set `availableAt` on matching items, then recompute status: some → `PARTIALLY_AVAILABLE`, all → `AVAILABLE` + `RequestAvailable` (first time only) | Event ID; `availableAt` set once | — |
| `library.FileImported` (`Origin = DOWNLOAD`) | Library | Issues in `ACTION_PENDING` with `REGRAB` for that Title/episode → `RESOLVED` + `IssueResolved` | Event ID | — |
| `subtitles.SubtitleAdded` | Subtitles | Issues in `ACTION_PENDING` with `RERUN_SUBTITLES` whose file matches (via the Title/episode in the payload) → `RESOLVED` + `IssueResolved` | Event ID | — |

## External ports

None. Requests reads Identity and Catalog only through their Readers.

## HTTP surface

Routes under `/api/requests`: requests at the root (`POST /api/requests`), issues under `/api/requests/issues`.

- `POST /api/requests` create `{kind, tmdbId, scope, seasons?}`. The profile defaults to the admin-set default profile (`1080p` on a fresh install).
- `GET /api/requests` list. Requesters see their own unless they have the "view others' requests" toggle. Filters: `status`, `userId`; `count` returns only the count.
- `GET /api/requests/{id}` one request with item progress.
- `POST /api/requests/{id}/approve` `{profileId?}` (admin).
- `POST /api/requests/{id}/decline` `{reason?}` (admin).
- `POST /api/requests/{id}/retry` re-publish `RequestApproved` for a `FAILED` request (admin).
- `DELETE /api/requests/{id}` cancel your own `PENDING` request.
- `POST /api/requests/issues` `{titleId, episodeId?, kind, text}` (needs the "report issues" toggle).
- `GET /api/requests/issues` list; `GET /api/requests/issues/{id}`.
- `POST /api/requests/issues/{id}/comments` `{body}` (reporter or admin).
- `POST /api/requests/issues/{id}/actions` `{action: REGRAB | RERUN_SUBTITLES}` (admin).
- `POST /api/requests/issues/{id}/resolve` (admin or reporter).

## Behaviour

**Scope** ([ticket 15](../../../.scratch/v1-spec/issues/15-users-and-auth.md))
- A request is for a movie, a whole series (`ALL_SEASONS`: every season plus future ones) or chosen seasons. Single-episode requests are not supported.
- No quotas. Approval is the only gate.

**Creation and approval**
- On `POST /requests`, Requests reads `identity.Reader` for the user's role and toggles:
  - Admin, or the toggle matching the kind (`autoApproveMovies` / `autoApproveTv`): approved at once, publishing `RequestApproved{Auto: true}`.
  - Otherwise status `PENDING` and `RequestCreated` is published (admins are notified).
- Duplicate guard: if a non-terminal request (not `DECLINED`/`FAILED`) already covers the same `(kind, tmdbId)` and every requested season, the API returns `409` with the existing request ID.
- Also rejected with `409`: a request whose scope is already fully monitored in Catalog (`catalog.Reader.TitleByTmdb` + monitoring).
- Approving lets the admin override the profile.

**State machine**

```
PENDING ──approve──▶ APPROVED ──TitleAdded/MonitoringChanged──▶ PROCESSING
   │                     │                                          │
 decline           TitleAddFailed                         ItemAvailable (some)
   ▼                     ▼                                          ▼
DECLINED              FAILED ──retry──▶ APPROVED          PARTIALLY_AVAILABLE ──all items──▶ AVAILABLE
```

- `PENDING` can also end by the user cancelling (row deleted).
- **Available** ([ticket 18](../../../.scratch/v1-spec/issues/18-import-and-library.md)): an item counts as available only when Jellyfin confirms it (`library.ItemAvailable`), not on import.
- Items are the movie, or every released episode in scope. Episodes that release later are added as items.
- If a new item makes an `AVAILABLE` request incomplete, it moves back to `PARTIALLY_AVAILABLE`. `RequestAvailable` is never published twice for one request.
- While items are imported but not yet confirmed, the UI shows "Imported, waiting for Jellyfin". That comes from the Activity pipeline projection, not from Requests' status.

**Issues** ([ticket 15](../../../.scratch/v1-spec/issues/15-users-and-auth.md))
- An Issue has a kind (video, audio, subtitle, other), text, and a comment thread. Reporting needs the "report issues" toggle.
- Admin actions:
  - `REGRAB`: Acquisition blocklists the current file's release and searches again. This works even when the file has been transcoded (a manual override of "upgrades stop after transcode").
  - `RERUN_SUBTITLES`: Subtitles redoes subtitles for the file.
  - Either action sets `ACTION_PENDING` and publishes `IssueActionRequested`. The Issue resolves automatically when the matching `FileImported` or `SubtitleAdded` arrives, or manually.
- Only one action can be pending per Issue. A second action returns `409` until the first resolves.

## Test seeds

1. **Given** a Requester without auto-approve, **when** they request a movie, **then** status is `PENDING`, `RequestCreated` is published, and `RequestApproved` is not.
2. **Given** a Requester with `autoApproveMovies`, **when** they request a movie, **then** status is `APPROVED` and `RequestApproved{Auto: true}` is published.
3. **Given** a Requester with `autoApproveMovies` only, **when** they request a series, **then** status is `PENDING`.
4. **Given** a `PENDING` request, **when** an admin approves with profile Q, **then** `RequestApproved{Profile: Q, ApprovedBy: admin}` is published.
5. **Given** a non-terminal request for TMDB 974576, **when** another user requests the same movie, **then** the API returns `409` with the existing request ID.
6. **Given** `RequestApproved`, **when** `TitleAdded{RequestID}` arrives, **then** the request is `PROCESSING` with `titleId` set and one item per released episode in scope.
7. **Given** the same `TitleAdded` delivered twice, **then** item rows are not duplicated.
8. **Given** a `SEASONS [2]` request in `PROCESSING` with 10 items, **when** `ItemAvailable` arrives for 4 episodes of season 2 and 3 of season 1, **then** 4 items are available and status is `PARTIALLY_AVAILABLE`.
9. **Given** all items available except one, **when** `ItemAvailable` covers it, **then** status is `AVAILABLE` and `RequestAvailable` is published once; a redelivered event publishes nothing.
10. **Given** an `AVAILABLE` whole-series request, **when** `WantedReleased` adds a new episode, **then** status is `PARTIALLY_AVAILABLE`; **when** it becomes available, **then** status is `AVAILABLE` and no second `RequestAvailable` is published.
11. **Given** `TitleAddFailed{RequestID}`, **then** status is `FAILED`; **when** an admin retries, **then** `RequestApproved` is published again and status is `APPROVED`.
12. **Given** a user without the "view others' requests" toggle, **when** they list requests, **then** only their own are returned.
13. **Given** a user without the "report issues" toggle, **when** they `POST /issues`, **then** the API returns `403`.
14. **Given** an `OPEN` issue on episode E, **when** an admin requests `REGRAB`, **then** status is `ACTION_PENDING` and `IssueActionRequested{Episode: E, Action: REGRAB}` is published; a second action returns `409`.
15. **Given** an `ACTION_PENDING` `REGRAB` issue on episode E, **when** `library.FileImported` for E arrives, **then** the issue is `RESOLVED` and `IssueResolved{By: "ACTION"}` is published once.
16. **Given** a comment by an admin, **then** `IssueCommented{Reporter}` is published.
