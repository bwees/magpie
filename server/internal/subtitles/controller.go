package subtitles

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/access"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/parse"
	"magpie/internal/platform/problem"
	"magpie/internal/platform/settings"
	"magpie/internal/subtitles/api"

	"github.com/go-fuego/fuego"
)

const (
	tagSubtitles = "subtitles"

	pathMediaFileID = "mediaFileId"
	pathSubtitleID  = "subtitleId"
	queryLanguage   = "language"
	queryForced     = "forced"

	TypeFileNotFound           problem.Type = "urn:magpie:problem:subtitles-file-not-found"
	TypeSubtitleNotFound       problem.Type = "urn:magpie:problem:subtitle-not-found"
	TypeEmbeddedSubtitle       problem.Type = "urn:magpie:problem:embedded-subtitle"
	TypeCandidateNotFound      problem.Type = "urn:magpie:problem:subtitle-candidate-not-found"
	TypeQuotaExhausted         problem.Type = "urn:magpie:problem:subtitle-quota-exhausted"
	TypeOpenSubtitlesPaused    problem.Type = "urn:magpie:problem:opensubtitles-paused"
	TypeOpenSubtitlesFailed    problem.Type = "urn:magpie:problem:opensubtitles-unavailable"
	TypeInvalidSettings        problem.Type = "urn:magpie:problem:invalid-subtitle-settings"
	TypeInvalidSubtitleRequest problem.Type = "urn:magpie:problem:invalid-subtitle-request"
)

// SubtitleResponse is one subtitle of a file: an embedded track or a sidecar.
type SubtitleResponse struct {
	ID       SubtitleID      `json:"id"`
	Language domain.Language `json:"language"`
	Forced   bool            `json:"forced"`
	SDH      bool            `json:"sdh"`
	Source   api.Source      `json:"source"`
	// Score is the percent of the best possible score, for provider subtitles.
	Score        *int       `json:"score,omitempty"`
	Synced       bool       `json:"synced"`
	SyncOffsetMs *int64     `json:"syncOffsetMs,omitempty"`
	Path         string     `json:"path"`
	ReleaseName  string     `json:"releaseName"`
	AddedAt      time.Time  `json:"addedAt"`
	UpgradeUntil *time.Time `json:"upgradeUntil,omitempty"`
}

// CoverageResponse is one wanted track of a file, in wanted-language order.
type CoverageResponse struct {
	Language domain.Language    `json:"language"`
	Forced   bool               `json:"forced"`
	Status   api.CoverageStatus `json:"status"`
	Source   *api.Source        `json:"source,omitempty"`
	// Reason says why a missing track is missing.
	Reason        api.MissingReason `json:"reason,omitempty"`
	NextAttemptAt *time.Time        `json:"nextAttemptAt,omitempty"`
	// WaitingForQuota is true when a subtitle was found and waits for tomorrow's downloads.
	WaitingForQuota bool `json:"waitingForQuota"`
	// WaitingForWhisper is true while an agent generates the track with Whisper.
	WaitingForWhisper bool `json:"waitingForWhisper"`
}

type FileSubtitlesResponse struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Subtitles []SubtitleResponse `json:"subtitles" validate:"required"`
	Coverage  []CoverageResponse `json:"coverage" validate:"required"`
}

// CandidateResponse is one OpenSubtitles result for the candidate picker, best first.
type CandidateResponse struct {
	FileID          int64           `json:"fileId"`
	Language        domain.Language `json:"language"`
	ReleaseName     string          `json:"releaseName"`
	Score           int             `json:"score"`
	Points          int             `json:"points"`
	MaxPoints       int             `json:"maxPoints"`
	HashMatch       bool            `json:"hashMatch"`
	HearingImpaired bool            `json:"hearingImpaired"`
	Forced          bool            `json:"forced"`
	AITranslated    bool            `json:"aiTranslated"`
	// Acceptable is whether the automatic search would take it.
	Acceptable bool `json:"acceptable"`
}

type DownloadSubtitleRequest struct {
	FileID   int64           `json:"fileId"`
	Language domain.Language `json:"language"`
	Forced   bool            `json:"forced"`
}

type SubtitleSettingsBody struct {
	WhisperEnabled bool `json:"whisperEnabled"`
	// WhisperModel is large-v3-turbo, large-v3, medium or small.
	WhisperModel      string `json:"whisperModel"`
	AutoSync          bool   `json:"autoSync"`
	MinScoreEpisode   int    `json:"minScoreEpisode"`
	MinScoreMovie     int    `json:"minScoreMovie"`
	UpgradeWindowDays int    `json:"upgradeWindowDays"`
	UpgradeCheckHours int    `json:"upgradeCheckHours"`
}

// QuotaResponse is the OpenSubtitles quota badge.
type QuotaResponse struct {
	// Remaining is nil until OpenSubtitles first reports the quota.
	Remaining   *int       `json:"remaining"`
	ResetAt     *time.Time `json:"resetAt"`
	VIP         bool       `json:"vip"`
	PausedUntil *time.Time `json:"pausedUntil"`
	Problem     string     `json:"problem"`
}

type Controller struct {
	subtitles *Service
	gate      *access.FileGate
}

func NewController(s *Service, g *access.FileGate) *Controller {
	return &Controller{subtitles: s, gate: g}
}

// requireFile returns a problem unless the principal may manage the file's title.
func (c *Controller) requireFile(ctx context.Context, file domain.MediaFileID) error {
	if err := access.RequireManager(ctx); err != nil {
		return err
	}
	title, err := c.subtitles.FileTitle(ctx, file)
	if err != nil {
		return subtitlesProblem(err)
	}
	return c.gate.Require(ctx, title)
}

func (c *Controller) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagSubtitles)
	filePath := fuego.OptionPath(pathMediaFileID, "Media file ID")

	fuego.Get(s, "/subtitles/files/{mediaFileId}", c.file, tags, filePath,
		fuego.OptionOperationID("getFileSubtitles"),
		fuego.OptionSummary("A media file's subtitles and wanted-language coverage (file managers)"))
	fuego.Post(s, "/subtitles/files/{mediaFileId}/search", c.search, tags, filePath,
		fuego.OptionOperationID("searchFileSubtitles"),
		fuego.OptionSummary("Search the file's missing languages now, skipping the backoff (file managers)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
	fuego.Get(s, "/subtitles/files/{mediaFileId}/candidates", c.candidates, tags, filePath,
		fuego.OptionOperationID("listSubtitleCandidates"),
		fuego.OptionSummary("Scored OpenSubtitles results for one language of the file, best first (file managers)"),
		fuego.OptionQuery(queryLanguage, "ISO 639-1 language", fuego.ParamRequired()),
		fuego.OptionQueryBool(queryForced, "Forced (foreign parts only) subtitles"))
	fuego.Post(s, "/subtitles/files/{mediaFileId}/download", c.download, tags, filePath,
		fuego.OptionOperationID("downloadSubtitle"),
		fuego.OptionSummary("Download a candidate, sync it and store it as the file's sidecar (file managers)"))
	fuego.Delete(s, "/subtitles/{subtitleId}", c.remove, tags,
		fuego.OptionPath(pathSubtitleID, "Subtitle ID"),
		fuego.OptionOperationID("deleteSubtitle"),
		fuego.OptionSummary("Remove a sidecar subtitle (file managers)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Get(s, "/subtitles/settings", c.settings, tags,
		fuego.OptionOperationID("getSubtitleSettings"),
		fuego.OptionSummary("Whisper, auto-sync, score thresholds and the upgrade window (admin)"))
	fuego.Put(s, "/subtitles/settings", c.setSettings, tags,
		fuego.OptionOperationID("setSubtitleSettings"),
		fuego.OptionSummary("Replace the subtitle settings (admin)"))
	fuego.Get(s, "/subtitles/quota", c.quota, tags,
		fuego.OptionOperationID("getSubtitleQuota"),
		fuego.OptionSummary("OpenSubtitles downloads left today (admin)"))
}

func (c *Controller) file(ctx fuego.ContextNoBody) (FileSubtitlesResponse, error) {
	id := domain.MediaFileID(ctx.PathParam(pathMediaFileID))
	if err := c.requireFile(ctx.Context(), id); err != nil {
		return FileSubtitlesResponse{}, err
	}
	view, err := c.subtitles.File(ctx.Context(), id)
	if err != nil {
		return FileSubtitlesResponse{}, subtitlesProblem(err)
	}
	return fileResponse(id, view), nil
}

func (c *Controller) search(ctx fuego.ContextNoBody) (FileSubtitlesResponse, error) {
	id := domain.MediaFileID(ctx.PathParam(pathMediaFileID))
	if err := c.requireFile(ctx.Context(), id); err != nil {
		return FileSubtitlesResponse{}, err
	}
	view, err := c.subtitles.SearchNow(ctx.Context(), id)
	if err != nil {
		return FileSubtitlesResponse{}, subtitlesProblem(err)
	}
	return fileResponse(id, view), nil
}

func (c *Controller) candidates(ctx fuego.ContextNoBody) ([]CandidateResponse, error) {
	id := domain.MediaFileID(ctx.PathParam(pathMediaFileID))
	if err := c.requireFile(ctx.Context(), id); err != nil {
		return nil, err
	}
	track, err := trackFrom(ctx.QueryParam(queryLanguage), ctx.QueryParam(queryForced))
	if err != nil {
		return nil, err
	}
	views, err := c.subtitles.Candidates(ctx.Context(), id, track.Language, track.Forced)
	if err != nil {
		return nil, subtitlesProblem(err)
	}
	out := make([]CandidateResponse, 0, len(views))
	for _, v := range views {
		out = append(out, CandidateResponse{
			FileID: v.FileID, Language: v.Language, ReleaseName: v.ReleaseName, Score: v.Score.Percent(),
			Points: v.Score.Points, MaxPoints: v.Score.Max, HashMatch: v.HashMatch, HearingImpaired: v.HearingImpaired,
			Forced: v.Forced, AITranslated: v.AITranslated, Acceptable: v.Acceptable,
		})
	}
	return out, nil
}

func (c *Controller) download(ctx fuego.ContextWithBody[DownloadSubtitleRequest]) (FileSubtitlesResponse, error) {
	id := domain.MediaFileID(ctx.PathParam(pathMediaFileID))
	if err := c.requireFile(ctx.Context(), id); err != nil {
		return FileSubtitlesResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return FileSubtitlesResponse{}, err
	}
	track, err := trackFrom(string(body.Language), strconv.FormatBool(body.Forced))
	if err != nil {
		return FileSubtitlesResponse{}, err
	}
	view, err := c.subtitles.Download(ctx.Context(), id, track.Language, track.Forced, body.FileID)
	if err != nil {
		return FileSubtitlesResponse{}, subtitlesProblem(err)
	}
	return fileResponse(id, view), nil
}

func (c *Controller) remove(ctx fuego.ContextNoBody) (any, error) {
	if err := access.RequireManager(ctx.Context()); err != nil {
		return nil, err
	}
	id := SubtitleID(ctx.PathParam(pathSubtitleID))
	title, err := c.subtitles.SubtitleTitle(ctx.Context(), id)
	if err != nil {
		return nil, subtitlesProblem(err)
	}
	if err := c.gate.Require(ctx.Context(), title); err != nil {
		return nil, err
	}
	return nil, subtitlesProblem(c.subtitles.DeleteSubtitle(ctx.Context(), id))
}

func (c *Controller) settings(ctx fuego.ContextNoBody) (SubtitleSettingsBody, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SubtitleSettingsBody{}, err
	}
	cfg, err := c.subtitles.Settings(ctx.Context())
	if err != nil {
		return SubtitleSettingsBody{}, err
	}
	return settingsBody(cfg), nil
}

func (c *Controller) setSettings(ctx fuego.ContextWithBody[SubtitleSettingsBody]) (SubtitleSettingsBody, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return SubtitleSettingsBody{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return SubtitleSettingsBody{}, err
	}
	cfg, err := c.subtitles.SetSettings(ctx.Context(), Settings{
		WhisperEnabled: body.WhisperEnabled, WhisperModel: body.WhisperModel, AutoSync: body.AutoSync,
		MinScoreEpisode: body.MinScoreEpisode, MinScoreMovie: body.MinScoreMovie,
		UpgradeWindowDays:    body.UpgradeWindowDays,
		UpgradeCheckInterval: settings.Duration(time.Duration(body.UpgradeCheckHours) * time.Hour),
	})
	if err != nil {
		return SubtitleSettingsBody{}, subtitlesProblem(err)
	}
	return settingsBody(cfg), nil
}

func (c *Controller) quota(ctx fuego.ContextNoBody) (QuotaResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return QuotaResponse{}, err
	}
	q, err := c.subtitles.Quota(ctx.Context())
	if err != nil {
		return QuotaResponse{}, err
	}
	return QuotaResponse(q), nil
}

func trackFrom(language, forced string) (wantedTrack, error) {
	lang, ok := parse.LanguageCode(language)
	if !ok {
		return wantedTrack{}, problem.New(TypeInvalidSubtitleRequest, http.StatusBadRequest, "Pass an ISO 639-1 language")
	}
	isForced := false
	if forced != "" {
		var err error
		if isForced, err = strconv.ParseBool(forced); err != nil {
			return wantedTrack{}, problem.New(TypeInvalidSubtitleRequest, http.StatusBadRequest, "forced must be true or false")
		}
	}
	return wantedTrack{Language: lang, Forced: isForced}, nil
}

func settingsBody(cfg Settings) SubtitleSettingsBody {
	return SubtitleSettingsBody{
		WhisperEnabled: cfg.WhisperEnabled, WhisperModel: cfg.WhisperModel, AutoSync: cfg.AutoSync,
		MinScoreEpisode: cfg.MinScoreEpisode, MinScoreMovie: cfg.MinScoreMovie,
		UpgradeWindowDays: cfg.UpgradeWindowDays,
		UpgradeCheckHours: int(cfg.UpgradeCheckInterval.Std() / time.Hour),
	}
}

func fileResponse(id domain.MediaFileID, view FileView) FileSubtitlesResponse {
	out := FileSubtitlesResponse{MediaFile: id, Subtitles: []SubtitleResponse{}, Coverage: []CoverageResponse{}}
	for _, sub := range view.Subtitles {
		r := SubtitleResponse{
			ID: sub.ID, Language: sub.Language, Forced: sub.Forced, SDH: sub.SDH, Source: sub.Source,
			Synced: sub.Synced, SyncOffsetMs: sub.SyncOffsetMs, Path: sub.Path, ReleaseName: sub.ReleaseName,
			AddedAt: sub.AddedAt, UpgradeUntil: sub.UpgradeUntil,
		}
		if score, ok := sub.score(); ok {
			pct := score.Percent()
			r.Score = &pct
		}
		out.Subtitles = append(out.Subtitles, r)
	}
	for _, row := range view.Wanted {
		out.Coverage = append(out.Coverage, CoverageResponse{
			Language: row.Language, Forced: row.Forced, Status: row.Status.Coverage(), Source: row.Source,
			Reason: row.Reason, NextAttemptAt: row.NextAttemptAt, WaitingForQuota: row.Candidate != nil,
			WaitingForWhisper: row.Status == StatusWaitingWhisper,
		})
	}
	return out
}

func subtitlesProblem(err error) error {
	var exhausted *QuotaExhaustedError
	var paused *pausedError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return problem.New(TypeFileNotFound, http.StatusNotFound, "Subtitles knows no such media file")
	case errors.Is(err, ErrSubtitleNotFound):
		return problem.New(TypeSubtitleNotFound, http.StatusNotFound, "No such subtitle")
	case errors.Is(err, ErrEmbedded):
		return problem.New(TypeEmbeddedSubtitle, http.StatusConflict, "An embedded track is part of the video and can't be removed")
	case errors.Is(err, ErrCandidateNotFound):
		return problem.New(TypeCandidateNotFound, http.StatusNotFound, "OpenSubtitles no longer offers that subtitle. Search again.")
	case errors.As(err, &exhausted):
		return problem.New(TypeQuotaExhausted, http.StatusTooManyRequests,
			"No OpenSubtitles downloads left today. They reset at "+exhausted.ResetAt.UTC().Format(time.RFC3339)+".")
	case errors.As(err, &paused):
		return problem.New(TypeOpenSubtitlesPaused, http.StatusServiceUnavailable, authProblem)
	case errors.Is(err, ErrRateLimited), errors.Is(err, ErrUnavailable):
		return problem.New(TypeOpenSubtitlesFailed, http.StatusBadGateway, "OpenSubtitles didn't answer. Try again in a minute.")
	case errors.Is(err, ErrInvalidSettings):
		return problem.New(TypeInvalidSettings, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrInvalidSettings.Error()+": "))
	}
	return err
}
