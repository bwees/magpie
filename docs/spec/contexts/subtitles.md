# Subtitles

Makes sure every media file has subtitles in the user's wanted languages: embedded tracks first, then OpenSubtitles, then Whisper on an agent for the original language.

- **Depends on (Readers):** `catalog.Reader` (title, original language, wanted languages), `library.Reader` (path, OSHash, release name, embedded streams)
- **Publishes:** `SubtitleAdded`, `SubtitleMissing`, `CoverageSettled`
- **Sends command:** `transcode.RunAgentJob{Kind: WHISPER}`
- **Consumes:** `library.FileImported`, `library.FileTranscoded`, `library.FileDeleted`, `requests.IssueActionRequested` (RERUN_SUBTITLES), `transcode.WhisperCompleted`, `transcode.JobFailed`, `catalog.LanguagesChanged`

Decisions: [07](../../../.scratch/v1-spec/issues/07-subtitles.md), [16](../../../.scratch/v1-spec/issues/16-subtitle-design.md), [24](../../../.scratch/v1-spec/issues/24-scheduling.md).

## Owns / Must not

| Owns | Must not |
|---|---|
| Subtitle records and sidecar files (`<video basename>.<lang>[.forced\|.sdh].srt`) | Write, remux or rename video files (Library) |
| OpenSubtitles token and daily quota counter | Run Whisper itself or talk to agents (Transcode) |
| The wanted queue (missing file × language) and upgrade window | Own the wanted-languages list (Catalog owns it; Transcode reads it too) or store the OpenSubtitles login (platform connection config) |
| Scoring and timing sync | OCR image subtitles (not in v1) |

## Data model

| Table | Key columns |
|---|---|
| `files` | media_file_id, title_id, settled (false while a wanted track is in progress), imported_at |
| `subtitles` | id, media_file_id, language, forced, sdh, source (EMBEDDED/RELEASE/PROVIDER/WHISPER), stream_index? (EMBEDDED), provider_file_id?, release_name, score?, max_score?, synced, sync_offset_ms?, path?, added_at, upgrade_until? |
| `wanted` | media_file_id, language, forced, hearing_impaired (HI ok), position (wanted order), status (PENDING/SEARCHING/WAITING_WHISPER/MISSING/SATISFIED), source? (when SATISFIED), reason? (when MISSING), priority (IMPORT/MANUAL/UPGRADE/BACKLOG), next_attempt_at?, candidate? (an accepted subtitle waiting for quota) |
| `blocklist` | media_file_id, provider_file_id (filled by RERUN_SUBTITLES and by deleting a provider subtitle) |
| `opensubtitles_session` | downloads_remaining, reset_at, vip, token_expires_at, paused_until, problem (the token itself stays in the adapter's memory; login credentials live in `platform.connections.opensubtitles`) |
| `settings` | one settings key: whisper_enabled, whisper_model (default `large-v3-turbo`), auto_sync, min_score_episode (90), min_score_movie (70), upgrade_window_days (7), upgrade_check_interval (6 h) |

- Tables are prefixed `subtitles_`.
- Every wanted track has a `wanted` row, satisfied ones included, so `Coverage` reads one table in wanted order. A wanted language with Forced on has two rows: the regular track and the forced one.
- SEARCHING isn't stored: a per-file lock in memory keeps two searches of one file apart, and the row stays PENDING until the search records its outcome.
- EMBEDDED rows have no path: they record that the track exists in the video, so Magpie doesn't search for it. Image tracks (PGS/VobSub) count as present.

## Query port

```go
package api // internal/subtitles/api

type Source string         // EMBEDDED | RELEASE | PROVIDER | WHISPER
type CoverageStatus string // SATISFIED | PENDING | MISSING (SEARCHING and WAITING_WHISPER read as PENDING)
type Quota struct { Remaining int; ResetAt time.Time }

type Reader interface {
	// ForFile returns every subtitle known for a media file, embedded tracks included, ordered by
	// language, then Forced, then SDH. An unknown file returns an empty slice, no error.
	ForFile(ctx context.Context, file domain.MediaFileID) ([]Subtitle, error)
	// Coverage returns per wanted language whether it's satisfied, pending or missing, for the title page,
	// in wanted-language order. An unknown file returns an empty slice, no error.
	Coverage(ctx context.Context, file domain.MediaFileID) ([]LanguageCoverage, error)
	// Quota returns OpenSubtitles downloads left today and when they reset.
	Quota(ctx context.Context) (Quota, error)
}

type Subtitle struct {
	Language domain.Language
	Forced   bool
	SDH      bool
	Source   Source  // EMBEDDED | RELEASE | PROVIDER | WHISPER
	Score    *int    // percent of max, PROVIDER only
	Synced   bool
	Path     string  // empty for EMBEDDED
}

type LanguageCoverage struct { Language domain.Language; Forced bool; Status CoverageStatus; Source *Source }
```

## Events published

```go
type SubtitleAdded struct {
	MediaFile domain.MediaFileID
	Language  domain.Language
	Forced    bool
	SDH       bool
	Source    Source
	Synced    bool
	Replaces  bool // an upgrade replaced an earlier provider subtitle
}

type SubtitleMissing struct {
	MediaFile domain.MediaFileID
	Language  domain.Language
	Forced    bool
	Reason    MissingReason // NO_MATCH | QUOTA_EXHAUSTED | WHISPER_FAILED | NOT_ORIGINAL_LANGUAGE
}

// CoverageSettled: no wanted language for the file is PENDING, SEARCHING or WAITING_WHISPER any more.
// Drives the "Subs" chip on the title page.
type CoverageSettled struct {
	MediaFile domain.MediaFileID
	Title     domain.TitleID
	Satisfied []domain.Language
	Missing   []domain.Language
}
```

- All are durable, keyed by media file. `SubtitleMissing` is published once per (file, language), when a language moves to MISSING, not on every retry.
- `CoverageSettled` is published each time the file's wanted set goes from "something in progress" to "nothing in progress" (after import, after a Whisper result, after an upgrade or a languages change).

## Events and commands consumed

| Message | Reaction | Idempotency key | Failure handling |
|---|---|---|---|
| `library.FileImported` | Record EMBEDDED tracks from `MediaInfo`, record `Sidecars` as RELEASE, and create `wanted` rows for uncovered languages (wanted list from `catalog.Reader.WantedLanguages`). Queue a search job. `Replaces` is ignored; the old file is cleaned up on its `FileDeleted`. | media file ID | Search errors leave rows PENDING with backoff. |
| `library.FileTranscoded` | Re-read embedded streams; drop EMBEDDED rows for tracks the transcode removed; create `wanted` rows if a wanted language is now uncovered | event ID | none |
| `library.FileDeleted` (any `Reason`) | Delete the file's subtitle rows and its sidecar files, except sidecar paths that now belong to another live media file (an upgrade that kept the same basename) | media file ID | A missing sidecar counts as deleted |
| `requests.IssueActionRequested{RERUN_SUBTITLES}` | Delete PROVIDER/WHISPER subtitles for the file and blocklist their provider file IDs for this file, then re-run the search | issue ID | none |
| `catalog.LanguagesChanged` | Recompute `wanted` for affected files (one title, or all titles if global) | event ID | Done in batches via a backlite job |
| `transcode.WhisperCompleted` | Move the staged SRT to the sidecar path; source WHISPER, not synced; publish `SubtitleAdded` | job ID | If the staged file is missing, treat as `JobFailed` |
| `transcode.JobFailed` (Kind WHISPER) | Mark the language MISSING `WHISPER_FAILED`; publish `SubtitleMissing` | job ID | Retried by the next sweep, at most 3 attempts |

## External ports

```go
type OpenSubtitles interface {
	// Login exchanges the login in platform connection config for a fresh token, which later calls use
	// (app API key is built in). ErrAuth on bad or missing credentials.
	Login(ctx context.Context) (Token, error)
	// Search queries by moviehash plus tmdb/imdb id, season, episode and language. Max 5 req/s.
	Search(ctx context.Context, q SearchQuery) ([]Candidate, error)
	// Download returns the subtitle content and the remaining daily quota. ErrAuth without a valid token.
	Download(ctx context.Context, fileID int64) (content []byte, remaining int, resetAt time.Time, err error)
}
// Errors: ErrAuth (re-login once, then pause and notify), ErrQuotaExceeded (pause until resetAt),
// ErrRateLimited (back off with Retry-After; returned as *RetryAfterError{Wait}), ErrUnavailable (retry with backoff).

type Syncer interface {
	// Sync aligns subtitle timing to the video's speech (Go port of ffsubsync: VAD + FFT offset search).
	// Returns the shifted subtitle and the applied offset; ErrNoSpeech if alignment is not confident.
	Sync(ctx context.Context, videoPath string, srt []byte) (synced []byte, offset time.Duration, err error)
}

var ErrNoSpeech error // declared in subtitles/ports.go; internal/adapters/subsync returns it

type SidecarFiles interface {
	// Write replaces the file at path with content.
	Write(path string, content []byte) error
	// Remove deletes path. A missing file is not an error.
	Remove(path string) error
}
```

- `Token` (value, expiry, VIP, daily allowed downloads), `SearchQuery` (comparable: moviehash, TMDB/IMDb ID, season, episode, one language; for an episode the IDs are the series') and `Candidate` (file ID, release name, hash match, HI, forced, machine/AI translated, identity fields for scoring) are in `ports.go`.
- Adapters: `internal/adapters/opensubtitles` (login from `platform.connections.opensubtitles`, the token kept in memory, one 5 req/s limiter for every call), `internal/adapters/subsync` (`Syncer`) and `internal/adapters/disk` (`SidecarFiles`, written through a temporary file and a rename).

## HTTP surface

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/subtitles/files/{mediaFileId}` | Subtitles plus coverage for the title page |
| POST | `/api/subtitles/files/{mediaFileId}/search` | Manual search now (skips the backoff and a login pause) |
| GET | `/api/subtitles/files/{mediaFileId}/candidates?language=&forced=` | Interactive list of scored candidates, best first, each flagged when it reaches the minimum score |
| POST | `/api/subtitles/files/{mediaFileId}/download` | Body: candidate file ID, language, forced; downloads, syncs and stores it. `429` when no quota is left |
| DELETE | `/api/subtitles/{subtitleId}` | Remove a sidecar (`409` for an embedded track). A provider file is blocklisted for the media file and the language is searched again |
| GET/PUT | `/api/subtitles/settings` | Whisper toggle/model, auto-sync, score thresholds, upgrade window and check interval (the OpenSubtitles login is platform connection config; wanted languages are `/api/catalog/languages`) |
| GET | `/api/subtitles/quota` | Downloads left today, the reset time, VIP, and a login problem while calls are paused |

All routes are admin only.

## Behaviour

**Per (file, wanted language)**, in order:
1. **Embedded:** a text or image track in that language (forced matching the `forced` flag) satisfies it. No search.
2. **Release sidecar:** a RELEASE subtitle satisfies it.
3. **OpenSubtitles:**
   - Search with the file's OSHash, TMDB/IMDb ID and season/episode.
   - Score each candidate Bazarr-style:
     - Episodes: hash 359, series 160, year 90, season 30, episode 30, source 25, release group 20, 1 each for audio/video codec, resolution, HI and streaming service.
     - Movies: hash 179, title 60, year 40, source 30, edition 30, release group 15, the same 1-point extras.
     - A hash only counts when the other identity fields also match: the movie, or the series, season and episode (TMDB aired numbering). A counted hash stands in for every other attribute but HI.
     - A matching TMDB or IMDb ID implies the year, as in Bazarr. Max is the sum of the non-hash weights (episodes 360, movies 180).
     - Release attributes come from parsing the stored `release_name`, not the file name.
   - Accept the best candidate scoring ≥ 90% (episodes) or 70% (movies) of max. Prefer non-HI unless the wanted entry allows HI.
   - Skip machine-translated results.
   - Download (uses quota), sync if `auto_sync` (on `ErrNoSpeech` keep it unsynced), write the sidecar, publish `SubtitleAdded`.
4. **Whisper:** only when step 3 finds nothing, `whisper_enabled` is on, and the language is the title's original language (from `catalog.Reader`). Send `RunAgentJob{Kind: WHISPER, MediaFile, Language, Model}` and set WAITING_WHISPER. Output is never synced.
5. Otherwise mark MISSING and publish `SubtitleMissing`. Retry on the 6-hourly sweep for 7 days, then daily.

**Quota:**
- Downloads are ordered: new imports first, then manual requests, then upgrades, then the missing backlog.
- When `downloads_remaining` hits 0 the queue pauses until `reset_at`. Searches still run (unlimited).
  - An accepted candidate found without quota stays on its `wanted` row (`candidate`, PENDING, `next_attempt_at = reset_at`). A downloads job at `reset_at`, and every sweep, downloads the waiting candidates in priority order.
  - The sweep's order is: waiting downloads, then upgrades, then searches for the due MISSING backlog.
- `ErrAuth` twice in a row (after one re-login) pauses every OpenSubtitles call for 6 h. The quota badge shows the problem; **Search now** on a file lifts the pause.
- `ErrRateLimited` and `ErrUnavailable` keep the row's status and book another search after Retry-After, or after 15 min.

**Upgrades:** a PROVIDER subtitle with score < max − 3 gets `upgrade_until = added_at + 7 days`. The 6-hourly sweep re-searches it and replaces it only with a candidate scoring ≥ old + 1 point. The replacement publishes `SubtitleAdded{Replaces: true}`.

**Sidecar naming:** `<video basename>.<iso639-1>.srt`, plus `.forced` or `.sdh` before the extension. Library moves sidecars when it renames videos; Subtitles writes and deletes them.

**Not in v1:** OCR of image subtitles, providers other than OpenSubtitles, translation.

## Test seeds

1. **Given** wanted = [en] and a file whose MediaInfo has an English text track, **when** `FileImported`, **then** an EMBEDDED row exists, no search runs, and no event is published.
2. **Given** a file with no English subtitle and a candidate whose hash and series/season/episode/source match, **when** the search runs, **then** that candidate is downloaded, synced, written as `<basename>.en.srt`, and `SubtitleAdded{Source: PROVIDER, Synced: true}` is published.
3. **Given** a candidate with a hash match but the wrong episode, **when** scored, **then** the hash weight is not counted.
4. **Given** the best candidate scores 85% for an episode, **when** the search runs, **then** nothing is downloaded and the next step (Whisper or MISSING) runs.
5. **Given** no match, Whisper on, and wanted language = original language, **when** the search ends, **then** `RunAgentJob{WHISPER}` is sent once and the row is WAITING_WHISPER.
6. **Given** no match and wanted language ≠ original language, **when** the search ends, **then** no Whisper job is sent and `SubtitleMissing{NOT_ORIGINAL_LANGUAGE}` is published.
7. **Given** `WhisperCompleted` is delivered twice, **when** handled, **then** one sidecar and one `SubtitleAdded` exist.
8. **Given** a quota of 0 with reset in 3 h, **when** a download is due, **then** no download call is made and the row retries after the reset.
9. **Given** the Syncer returns `ErrNoSpeech`, **when** storing the subtitle, **then** it is saved unsynced and `SubtitleAdded{Synced: false}` is published.
10. **Given** a PROVIDER subtitle at score 80/100 added 2 days ago, **when** the upgrade sweep finds an 82 candidate, **then** it replaces the sidecar and publishes `SubtitleAdded{Replaces: true}`; an 80 candidate is ignored.
11. **Given** the same subtitle added 8 days ago, **when** the sweep runs, **then** it is not re-searched.
12. **Given** a transcode dropped the Spanish audio and subtitle tracks, **when** `FileTranscoded` arrives and Spanish is wanted, **then** the EMBEDDED Spanish row is removed and a `wanted` row is created.
13. **Given** `FileImported{MediaFile: Y, Replaces: X}` followed by `FileDeleted{MediaFile: X, Reason: UPGRADED}`, **when** handled, **then** X's subtitle rows and sidecars are gone, except a sidecar path that Y's RELEASE subtitle now uses, which is kept; `FileImported` alone deletes nothing.
14. **Given** OpenSubtitles returns `ErrAuth` twice, **when** downloading, **then** the queue pauses and an admin failure is surfaced (via `SubtitleMissing` reason or a settings warning) without hammering the API.
15. **Given** wanted [en, es], **when** English is found on OpenSubtitles and Spanish ends MISSING, **then** `CoverageSettled{Satisfied: [en], Missing: [es]}` is published once; a later Whisper job for a new language publishes it again only when that job ends.
16. **Given** `LanguagesChanged{Title: T}` adding `ja`, **then** `wanted` rows for `ja` are created for T's files and searches are queued; files of other titles are untouched.
17. **Given** a Whisper job, **then** `RunAgentJob.Model` equals the configured model (default `large-v3-turbo`).
