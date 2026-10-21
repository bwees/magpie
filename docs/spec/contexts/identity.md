# Identity

Who the user is and what they may do. Sign-in goes through Jellyfin, with one local admin account as a fallback.

- **Depends on (Readers):** none.
- **Publishes:** `identity.UserCreated`.
- **Consumes:** nothing.
- **Read by:** Requests, Activity, and the platform live gateway (role filter).
- **Decided in:** [ticket 15](../../../.scratch/v1-spec/issues/15-users-and-auth.md).

## Owns / Must not

| Owns | Must not |
|---|---|
| Users, roles, per-user permission toggles, default permissions for new users | Know about titles, requests or media |
| Sessions and the session cookie | Store Jellyfin access tokens beyond the login call |
| The single local admin account and its password | Grant admin from Jellyfin's `IsAdministrator` |
| Sign-in flows (Jellyfin password, Jellyfin Quick Connect, local admin) | Call other contexts |

## Data model

| Table | Key columns |
|---|---|
| `identity_users` | `id` (UserID), `username` (unique, case-insensitive), `display_name`, `role` (`ADMIN` \| `REQUESTER`), `jellyfin_user_id` (unique, nullable), `is_local_admin` (bool, at most one row true), `password_hash` (argon2id; local admin only), `auto_approve_movies`, `auto_approve_tv`, `view_others_requests`, `report_issues`, `disabled`, `created_at`, `last_login_at` |
| `identity_sessions` | `token_hash` (PK, SHA-256 of the cookie value), `user_id`, `created_at`, `expires_at`, `last_seen_at`, `user_agent` |
| `platform_settings` keys | `identity.defaults` → `Permissions` for new users; `identity.signIn` → `{AllowJellyfin}` (default true) |

## Query port

```go
package api

type Role string // ADMIN | REQUESTER

type Permissions struct {
    AutoApproveMovies  bool
    AutoApproveTV      bool
    ViewOthersRequests bool
    ReportIssues       bool
    ManageFiles        bool
}

type User struct {
    ID             domain.UserID
    Username       string
    DisplayName    string
    Role           Role
    Permissions    Permissions // admins: all true regardless of stored toggles
    JellyfinUserID *string
    LocalAdmin     bool
    Disabled       bool
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

var ErrNotFound = errors.New("identity: user not found")

type Reader interface {
    // User returns one user, including disabled users. ErrNotFound if the ID is unknown.
    User(ctx context.Context, id domain.UserID) (User, error)
    // Users returns users by ID; unknown IDs are omitted (no error). Order follows the input.
    Users(ctx context.Context, ids []domain.UserID) ([]User, error)
    // Admins returns all enabled admins ordered by username. Never empty after setup.
    Admins(ctx context.Context) ([]User, error)
    // All returns every user ordered by username, for admin screens.
    All(ctx context.Context) ([]User, error)
}
```

Request principal (set by Identity's HTTP middleware, read by every controller):

```go
// in internal/domain
type Principal struct {
    User        UserID
    Admin       bool
    Permissions struct{ AutoApproveMovies, AutoApproveTV, ViewOthersRequests, ReportIssues, ManageFiles bool }
}
func PrincipalFrom(ctx context.Context) (Principal, bool)
```

## Events published

```go
// Durable. Emitted once when a user row is first created (setup, first Jellyfin sign-in).
type UserCreated struct {
    User   domain.UserID
    Role   Role
    Source string // "LOCAL" | "JELLYFIN"
}
```

## Events and commands consumed

None.

## External ports

```go
type JellyfinAuth interface {
    // AuthenticateByName calls POST /Users/AuthenticateByName.
    // ErrInvalidCredentials on 401, ErrUnavailable on network/5xx.
    AuthenticateByName(ctx context.Context, username, password string) (JellyfinUser, error)
    // QuickConnectEnabled calls GET /QuickConnect/Enabled.
    QuickConnectEnabled(ctx context.Context) (bool, error)
    // QuickConnectInitiate calls POST /QuickConnect/Initiate. Code is 6 digits, valid 10 minutes.
    QuickConnectInitiate(ctx context.Context) (QuickConnect, error)
    // QuickConnectPoll calls GET /QuickConnect/Connect?secret=. ErrExpired once Jellyfin forgets the secret.
    QuickConnectPoll(ctx context.Context, secret string) (authenticated bool, err error)
    // AuthenticateWithQuickConnect calls POST /Users/AuthenticateWithQuickConnect.
    AuthenticateWithQuickConnect(ctx context.Context, secret string) (JellyfinUser, error)
}

type QuickConnect struct{ Code, Secret string; ExpiresAt time.Time }
type JellyfinUser struct{ ID, Name string; IsAdministrator, IsDisabled bool }

var ErrInvalidCredentials, ErrUnavailable, ErrExpired error
```

- The adapter sends `Authorization: MediaBrowser Client="Magpie", Device=…, DeviceId=…, Version=…`. `DeviceId` is fixed, so Jellyfin keeps one device entry per user.
- Jellyfin answers `403` for a disabled account; the adapter returns that as `JellyfinUser{IsDisabled: true}`. A `404` on the Quick Connect poll or finish is `ErrExpired`. A missing Jellyfin URL is `ErrUnavailable`.
- `QuickConnect.ExpiresAt` is timed from Magpie's clock (initiate time + 10 minutes), so clock skew can't expire a code early.
- The Jellyfin access token returned at login is discarded. Magpie talks to Jellyfin with the API key in platform connection config (`platform.connections.jellyfin`), shared by Identity's and Library's adapters.

## HTTP surface

| Route | Who | Purpose |
|---|---|---|
| `POST /api/identity/setup/admin` | anyone, only while no admin exists | Create the local admin; returns a session |
| `POST /api/identity/login/local` | anyone | Local admin sign-in |
| `GET /api/identity/login/methods` | anyone | What the sign-in page offers: `{jellyfin, quickConnect, jellyfinReachable}` |
| `POST /api/identity/login/jellyfin` | anyone | Username/password via Jellyfin |
| `POST /api/identity/login/quick-connect` | anyone | Start Quick Connect; returns code + opaque handle |
| `GET /api/identity/login/quick-connect/{handle}` | anyone | Poll: `202 {status: PENDING}`; `200 {status: APPROVED, user}` plus the session cookie; `410` once expired |
| `POST /api/identity/logout` | signed in | Delete the session |
| `GET /api/identity/me` | signed in | Current user + permissions |
| `GET /api/identity/users` | admin | List users |
| `PATCH /api/identity/users/{id}` | admin | Role, toggles, disabled |
| `GET/PUT /api/identity/defaults` | admin | Default permissions for new users |
| `GET/PUT /api/identity/sign-in-settings` | admin | Allow Jellyfin sign-in (`{allowJellyfin}`) |
| `PUT /api/identity/local-admin/password` | local admin | Change password |

## Behaviour

- **Setup:** while no admin exists, every route except `setup/admin`, `GET /api/health` (container health checks) and static assets returns `409` with problem type `urn:magpie:problem:setup-required`. Setup creates the local admin (`is_local_admin`, `ADMIN`, password ≥ 12 characters, argon2id) and publishes `UserCreated{Source: LOCAL}`. A second setup returns `409 setup-complete`.
- **Middleware:** Identity provides the `httpserver.Middleware` that wraps every `/api` route. It applies the setup gate, resolves the session cookie into `domain.Principal`, and returns `401` on any route except `setup/admin` and `login/*` when there is no valid session. Admin-only routes check `httpserver.RequireAdmin` and return `403` to requesters.
- **Jellyfin sign-in:**
  1. Reject with `403 jellyfin-sign-in-off` when `identity.signIn.AllowJellyfin` is false.
  2. Authenticate with Jellyfin. A wrong password is `401 invalid-credentials` and counts towards the rate limit.
  3. Reject if `IsDisabled` (`403 jellyfin-user-disabled`).
  4. Look up the user by `jellyfin_user_id`. If there is none, create them as `REQUESTER` with `identity.defaults` permissions and publish `UserCreated{Source: JELLYFIN}`. The username is the Jellyfin name, or `<name>-<jellyfin id>` when a Magpie user already has that name. Users are never linked by name.
  5. If the Magpie user is `disabled`, reject with `403 user-disabled`.
  6. Update `display_name` and `last_login_at`, then issue a session.
- **Quick Connect:**
  - Start checks `QuickConnectEnabled` (`403 quick-connect-off` when Jellyfin has it off), then initiates.
  - The handle is a random token mapped in memory to Jellyfin's secret, which never reaches the browser. Pending handles are lost on restart, and at most 1000 are kept (`429` beyond that).
  - Polling an unknown or expired handle, or one Jellyfin forgot, returns `410 quick-connect-expired`. Once approved, the poll runs steps 3–6 above.
- **Jellyfin `IsAdministrator` is ignored.** Only a Magpie admin can promote a user.
- **Jellyfin down:** Jellyfin sign-ins return `503 jellyfin-unavailable`. Local admin sign-in always works. Existing sessions keep working. `login/methods` reports `jellyfinReachable: false` so the page can point to the local admin.
- **Sessions:**
  - Random 32-byte token in an HttpOnly, SameSite=Lax cookie; only its SHA-256 is stored.
  - 30-day sliding expiry: `last_seen_at` refresh at most every 5 minutes.
  - Logout deletes the row.
  - Disabling a user deletes all their sessions.
  - Changing the local admin password deletes the admin's other sessions.
  - Cookie `magpie_session`; `Secure` when the request is HTTPS or `X-Forwarded-Proto: https`.
- **Roles:**
  - `ADMIN` can do everything; their toggles read as all true.
  - `REQUESTER` sees Discover/search, their own requests (or all requests when `ViewOthersRequests` is set), and issue reporting (when `ReportIssues` is set). With `ManageFiles` they also manage the media files of the titles whose requests they see: view and delete files, search and grab replacements (no override of rejected releases), remove those titles' downloads and fetch or remove subtitles. The platform `access.FileGate` enforces it.
  - Requesters have no access to settings, indexers, activity internals or other users' notification URLs.
- **Invariants** (checked in this order inside the write transaction):
  - At least one enabled admin always exists. Demoting or disabling the last admin returns `409 last-admin`.
  - The local admin can't be demoted, disabled or deleted: any `PATCH` of it returns `409 local-admin-locked`.
  - Users can't change their own role or toggles: requesters get `403` from the admin check, admins `403 own-account`.
- **Defaults** (`identity.defaults`): `AutoApproveMovies=false`, `AutoApproveTV=false`, `ViewOthersRequests=false`, `ReportIssues=true`, `ManageFiles=false`.
- **No quotas in v1.**
- **Rate limit:** 10 failed sign-ins per IP per 10 minutes return `429` (`urn:magpie:problem:too-many-sign-ins`), even for correct credentials. The IP is the connecting address; behind a reverse proxy all clients share it. Failures are kept in memory.

## Test seeds

1. **Given** no admin exists, **when** `POST setup/admin`, **then** a local admin is created, a session cookie is set, `UserCreated{Source: LOCAL}` is in the outbox, and a second call returns `409`.
2. **Given** setup is incomplete, **when** any other `/api/*` route is called, **then** `409 setup_required`.
3. **Given** a new Jellyfin user with valid credentials, **when** they sign in, **then** a `REQUESTER` is created with `identity.defaults` permissions and exactly one `UserCreated{Source: JELLYFIN}` is published.
4. **Given** that user signs in again, **then** no new row or event is created and `last_login_at` updates.
5. **Given** a Jellyfin user with `IsAdministrator=true`, **when** they first sign in, **then** their role is `REQUESTER`.
6. **Given** `JellyfinAuth` returns `ErrUnavailable`, **when** a Jellyfin sign-in is attempted, **then** `503 jellyfin_unavailable`; local admin sign-in still succeeds.
7. **Given** Jellyfin reports `IsDisabled=true`, **then** sign-in returns `403` and no user is created.
8. **Given** a Quick Connect handle, **when** it's polled before approval, **then** `202 pending`; after approval, a session is issued; after expiry, `410`.
9. **Given** one admin, **when** they're demoted or disabled, **then** `409 last_admin` and nothing changes.
10. **Given** a requester, **when** they `PATCH` their own toggles, **then** `403`.
11. **Given** an admin user, **then** `Reader.User` returns `Permissions` all true even if the stored toggles are false.
12. **Given** `Reader.Users` is called with known and unknown IDs, **then** only known users return, in input order, with no error.
13. **Given** a user is disabled, **then** all their sessions are deleted and their next request returns `401`.
14. **Given** 10 failed sign-ins from one IP in 10 minutes, **then** the 11th returns `429`.
15. **Given** a session older than 30 days without use, **then** it's rejected with `401`.
16. **Contract (Reader fake vs real):** suite 11–12 plus `ErrNotFound` for unknown `User`, and `Admins` ordered by username excluding disabled admins.
