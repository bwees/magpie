package acquisition

import (
	"errors"
	"net/http"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	pathProfileID = "id"
	pathFormatID  = "id"

	TypeProfileNotFound problem.Type = "urn:magpie:problem:profile-not-found"
	TypeFormatNotFound  problem.Type = "urn:magpie:problem:custom-format-not-found"
	TypeNameTaken       problem.Type = "urn:magpie:problem:name-taken"
	TypeDefaultProfile  problem.Type = "urn:magpie:problem:default-profile"
)

type QualityResponse struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	Resolution int    `json:"resolution"`
}

type ProfileSummaryResponse struct {
	ID             domain.ProfileID  `json:"id"`
	Name           string            `json:"name"`
	UpgradeAllowed bool              `json:"upgradeAllowed"`
	Preset         api.QualityPreset `json:"preset,omitempty"`
	TrashProfileID string            `json:"trashProfileId,omitempty"`
}

type QualityGroupResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ProfileItemResponse struct {
	Group             *QualityGroupResponse `json:"group,omitempty"`
	Qualities         []QualityResponse     `json:"qualities" validate:"required"`
	Allowed           bool                  `json:"allowed"`
	MinMBPerMin       float64               `json:"minMbPerMin"`
	PreferredMBPerMin float64               `json:"preferredMbPerMin"`
	MaxMBPerMin       float64               `json:"maxMbPerMin"`
}

type FormatScoreResponse struct {
	FormatID     api.CustomFormatID `json:"formatId"`
	Name         string             `json:"name"`
	Score        int                `json:"score"`
	UserOverride bool               `json:"userOverride"`
}

type ProfileResponse struct {
	ID             domain.ProfileID      `json:"id"`
	Name           string                `json:"name"`
	UpgradeAllowed bool                  `json:"upgradeAllowed"`
	Preset         api.QualityPreset     `json:"preset,omitempty"`
	Items          []ProfileItemResponse `json:"items" validate:"required"`
	// Cutoff is the index into items of the cutoff quality or group.
	Cutoff                int                   `json:"cutoff"`
	MinFormatScore        int                   `json:"minFormatScore"`
	CutoffFormatScore     int                   `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int                   `json:"minUpgradeFormatScore"`
	PropersMode           api.PropersMode       `json:"propersMode"`
	FormatScores          []FormatScoreResponse `json:"formatScores" validate:"required"`
	TrashProfileID        string                `json:"trashProfileId,omitempty"`
	TrashScoreSet         string                `json:"trashScoreSet,omitempty"`
}

// ProfileItemRequest is one quality, or a group of equally ranked qualities when groupName is set.
type ProfileItemRequest struct {
	GroupName         string  `json:"groupName,omitempty"`
	QualityIDs        []int   `json:"qualityIds" validate:"required"`
	Allowed           bool    `json:"allowed"`
	MinMBPerMin       float64 `json:"minMbPerMin"`
	PreferredMBPerMin float64 `json:"preferredMbPerMin"`
	MaxMBPerMin       float64 `json:"maxMbPerMin"`
}

type FormatScoreRequest struct {
	FormatID api.CustomFormatID `json:"formatId"`
	Score    int                `json:"score"`
}

// ProfileRequest creates or replaces a profile. Items are most preferred first.
type ProfileRequest struct {
	Name                  string               `json:"name"`
	UpgradeAllowed        bool                 `json:"upgradeAllowed"`
	Items                 []ProfileItemRequest `json:"items" validate:"required"`
	Cutoff                int                  `json:"cutoff"`
	MinFormatScore        int                  `json:"minFormatScore"`
	CutoffFormatScore     int                  `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int                  `json:"minUpgradeFormatScore"`
	PropersMode           api.PropersMode      `json:"propersMode"`
	FormatScores          []FormatScoreRequest `json:"formatScores" validate:"required"`
}

type FromPresetRequest struct {
	Preset api.QualityPreset `json:"preset"`
}

type FormatSpecDTO struct {
	Kind     api.FormatSpecKind `json:"kind"`
	Name     string             `json:"name"`
	Negate   bool               `json:"negate"`
	Required bool               `json:"required"`
	Value    string             `json:"value"`
	Min      float64            `json:"min"`
	Max      float64            `json:"max"`
}

type CustomFormatResponse struct {
	ID                  api.CustomFormatID `json:"id"`
	Name                string             `json:"name"`
	TrashID             string             `json:"trashId,omitempty"`
	IncludeWhenRenaming bool               `json:"includeWhenRenaming"`
	Specs               []FormatSpecDTO    `json:"specs" validate:"required"`
}

type CustomFormatRequest struct {
	Name                string          `json:"name"`
	IncludeWhenRenaming bool            `json:"includeWhenRenaming"`
	Specs               []FormatSpecDTO `json:"specs" validate:"required"`
}

type BlockTermRequest struct {
	Term string `json:"term"`
}

type TrashStatusResponse struct {
	Enabled       bool       `json:"enabled"`
	LastAttemptAt *time.Time `json:"lastAttemptAt,omitempty"`
	LastSyncedAt  *time.Time `json:"lastSyncedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

type TrashProfileResponse struct {
	TrashID     string    `json:"trashId"`
	App         TrashApp  `json:"app"`
	Name        string    `json:"name"`
	ScoreSet    string    `json:"scoreSet"`
	Cutoff      string    `json:"cutoff"`
	Qualities   []string  `json:"qualities" validate:"required"` // allowed items, most preferred first
	FormatCount int       `json:"formatCount"`
	SyncedAt    time.Time `json:"syncedAt"`
}

type AcquisitionSettingsResponse struct {
	DefaultProfileID    domain.ProfileID `json:"defaultProfileId"`
	DefaultDelayMinutes int              `json:"defaultDelayMinutes"`
	SettleWindowSeconds int              `json:"settleWindowSeconds"`
	TrashSyncEnabled    bool             `json:"trashSyncEnabled"`
}

type AcquisitionSettingsRequest struct {
	DefaultProfileID    domain.ProfileID `json:"defaultProfileId"`
	DefaultDelayMinutes int              `json:"defaultDelayMinutes"`
	SettleWindowSeconds int              `json:"settleWindowSeconds"`
	TrashSyncEnabled    bool             `json:"trashSyncEnabled"`
}

// QualityController serves Settings → Quality and the profile pickers.
type QualityController struct {
	quality *QualityService
	trash   *TrashSyncService
	jobs    jobs.Enqueuer
}

func NewQualityController(q *QualityService, t *TrashSyncService, j jobs.Enqueuer) *QualityController {
	return &QualityController{quality: q, trash: t, jobs: j}
}

func (c *QualityController) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagAcquisition)
	profilePath := fuego.OptionPath(pathProfileID, "Profile ID")
	formatPath := fuego.OptionPath(pathFormatID, "Custom format ID")

	fuego.Get(s, "/acquisition/qualities", c.qualities, tags,
		fuego.OptionOperationID("listQualities"), fuego.OptionSummary("Radarr's 30 qualities, used for movies and TV"))
	fuego.Get(s, "/acquisition/profiles", c.listProfiles, tags,
		fuego.OptionOperationID("listProfiles"), fuego.OptionSummary("Quality profiles ordered by name, for profile pickers"))
	fuego.Get(s, "/acquisition/profiles/{id}", c.getProfile, tags, profilePath,
		fuego.OptionOperationID("getProfile"), fuego.OptionSummary("A profile with items, groups, sizes and format scores (admin)"))
	fuego.Post(s, "/acquisition/profiles", c.createProfile, tags,
		fuego.OptionOperationID("createProfile"), fuego.OptionSummary("Create a quality profile (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Post(s, "/acquisition/profiles/from-preset", c.createFromPreset, tags,
		fuego.OptionOperationID("createProfileFromPreset"), fuego.OptionSummary("Create a 720P, 1080P or 2160P resolution preset (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Put(s, "/acquisition/profiles/{id}", c.updateProfile, tags, profilePath,
		fuego.OptionOperationID("updateProfile"), fuego.OptionSummary("Replace a quality profile (admin)"))
	fuego.Delete(s, "/acquisition/profiles/{id}", c.deleteProfile, tags, profilePath,
		fuego.OptionOperationID("deleteProfile"), fuego.OptionSummary("Delete a quality profile other than the default (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))

	fuego.Get(s, "/acquisition/custom-formats", c.listFormats, tags,
		fuego.OptionOperationID("listCustomFormats"), fuego.OptionSummary("Custom formats ordered by name (admin)"))
	fuego.Post(s, "/acquisition/custom-formats", c.createFormat, tags,
		fuego.OptionOperationID("createCustomFormat"), fuego.OptionSummary("Create a custom format (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Post(s, "/acquisition/custom-formats/block-term", c.blockTerm, tags,
		fuego.OptionOperationID("blockTerm"), fuego.OptionSummary("Create a format for a release-name term scored -10000 in every profile (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusCreated))
	fuego.Put(s, "/acquisition/custom-formats/{id}", c.updateFormat, tags, formatPath,
		fuego.OptionOperationID("updateCustomFormat"), fuego.OptionSummary("Replace a custom format's name and specs (admin)"))
	fuego.Delete(s, "/acquisition/custom-formats/{id}", c.deleteFormat, tags, formatPath,
		fuego.OptionOperationID("deleteCustomFormat"), fuego.OptionSummary("Delete a custom format and its scores (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))

	fuego.Get(s, "/acquisition/trash/status", c.trashStatus, tags,
		fuego.OptionOperationID("getTrashStatus"), fuego.OptionSummary("TRaSH sync setting and last attempt (admin)"))
	fuego.Post(s, "/acquisition/trash/sync", c.syncTrash, tags,
		fuego.OptionOperationID("syncTrash"), fuego.OptionSummary("Queue a TRaSH Guides sync now (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
	fuego.Get(s, "/acquisition/trash/profiles", c.trashProfiles, tags,
		fuego.OptionOperationID("listTrashProfiles"), fuego.OptionSummary("TRaSH guide profiles as of the last sync (admin)"))

	fuego.Get(s, "/acquisition/settings", c.settings, tags,
		fuego.OptionOperationID("getAcquisitionSettings"), fuego.OptionSummary("Default profile, delay, settle window and TRaSH sync (admin)"))
	fuego.Put(s, "/acquisition/settings", c.updateSettings, tags,
		fuego.OptionOperationID("updateAcquisitionSettings"), fuego.OptionSummary("Change the default profile, delay, settle window and TRaSH sync (admin)"))
}

func (c *QualityController) qualities(ctx fuego.ContextNoBody) ([]QualityResponse, error) {
	qualities := c.quality.Qualities()
	out := make([]QualityResponse, 0, len(qualities))
	for _, q := range qualities {
		out = append(out, qualityResponse(q))
	}
	return out, nil
}

// listProfiles is open to every signed-in user: request dialogs and the title page pick from it.
func (c *QualityController) listProfiles(ctx fuego.ContextNoBody) ([]ProfileSummaryResponse, error) {
	profiles, err := c.quality.Profiles(ctx.Context())
	if err != nil {
		return nil, err
	}
	out := make([]ProfileSummaryResponse, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, ProfileSummaryResponse(p))
	}
	return out, nil
}

func (c *QualityController) getProfile(ctx fuego.ContextNoBody) (ProfileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ProfileResponse{}, err
	}
	p, err := c.quality.Profile(ctx.Context(), domain.ProfileID(ctx.PathParam(pathProfileID)))
	if err != nil {
		return ProfileResponse{}, qualityProblem(err)
	}
	return profileResponse(p), nil
}

func (c *QualityController) createProfile(ctx fuego.ContextWithBody[ProfileRequest]) (ProfileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ProfileResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ProfileResponse{}, err
	}
	p, err := c.quality.CreateProfile(ctx.Context(), body.toProfile(""))
	if err != nil {
		return ProfileResponse{}, qualityProblem(err)
	}
	return profileResponse(p), nil
}

func (c *QualityController) createFromPreset(ctx fuego.ContextWithBody[FromPresetRequest]) (ProfileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ProfileResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ProfileResponse{}, err
	}
	p, err := c.quality.CreateFromPreset(ctx.Context(), body.Preset)
	if err != nil {
		return ProfileResponse{}, qualityProblem(err)
	}
	return profileResponse(p), nil
}

func (c *QualityController) updateProfile(ctx fuego.ContextWithBody[ProfileRequest]) (ProfileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return ProfileResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return ProfileResponse{}, err
	}
	p, err := c.quality.UpdateProfile(ctx.Context(), body.toProfile(domain.ProfileID(ctx.PathParam(pathProfileID))))
	if err != nil {
		return ProfileResponse{}, qualityProblem(err)
	}
	return profileResponse(p), nil
}

func (c *QualityController) deleteProfile(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, qualityProblem(c.quality.DeleteProfile(ctx.Context(), domain.ProfileID(ctx.PathParam(pathProfileID))))
}

func (c *QualityController) listFormats(ctx fuego.ContextNoBody) ([]CustomFormatResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	formats, err := c.quality.Formats(ctx.Context())
	if err != nil {
		return nil, err
	}
	out := make([]CustomFormatResponse, 0, len(formats))
	for _, f := range formats {
		out = append(out, formatResponse(f))
	}
	return out, nil
}

func (c *QualityController) createFormat(ctx fuego.ContextWithBody[CustomFormatRequest]) (CustomFormatResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return CustomFormatResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return CustomFormatResponse{}, err
	}
	f, err := c.quality.CreateFormat(ctx.Context(), body.toFormat(""))
	if err != nil {
		return CustomFormatResponse{}, qualityProblem(err)
	}
	return formatResponse(f), nil
}

func (c *QualityController) updateFormat(ctx fuego.ContextWithBody[CustomFormatRequest]) (CustomFormatResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return CustomFormatResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return CustomFormatResponse{}, err
	}
	f, err := c.quality.UpdateFormat(ctx.Context(), body.toFormat(api.CustomFormatID(ctx.PathParam(pathFormatID))))
	if err != nil {
		return CustomFormatResponse{}, formatProblem(err)
	}
	return formatResponse(f), nil
}

func (c *QualityController) deleteFormat(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, formatProblem(c.quality.DeleteFormat(ctx.Context(), api.CustomFormatID(ctx.PathParam(pathFormatID))))
}

func (c *QualityController) blockTerm(ctx fuego.ContextWithBody[BlockTermRequest]) (CustomFormatResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return CustomFormatResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return CustomFormatResponse{}, err
	}
	f, err := c.quality.BlockTerm(ctx.Context(), body.Term)
	if err != nil {
		return CustomFormatResponse{}, qualityProblem(err)
	}
	return formatResponse(f), nil
}

func (c *QualityController) trashStatus(ctx fuego.ContextNoBody) (TrashStatusResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TrashStatusResponse{}, err
	}
	return c.trashStatusResponse(ctx)
}

func (c *QualityController) syncTrash(ctx fuego.ContextNoBody) (TrashStatusResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return TrashStatusResponse{}, err
	}
	if err := RequestTrashSync(ctx.Context(), c.jobs); err != nil {
		return TrashStatusResponse{}, err
	}
	return c.trashStatusResponse(ctx)
}

func (c *QualityController) trashStatusResponse(ctx fuego.ContextNoBody) (TrashStatusResponse, error) {
	state, err := c.trash.Status(ctx.Context())
	if err != nil {
		return TrashStatusResponse{}, err
	}
	cfg, err := c.quality.Settings(ctx.Context())
	if err != nil {
		return TrashStatusResponse{}, err
	}
	return TrashStatusResponse{Enabled: cfg.TrashSyncEnabled, LastAttemptAt: state.LastAttemptAt, LastSyncedAt: state.LastSyncedAt, LastError: state.LastError}, nil
}

func (c *QualityController) trashProfiles(ctx fuego.ContextNoBody) ([]TrashProfileResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	profiles, err := c.trash.GuideProfiles(ctx.Context())
	if err != nil {
		return nil, err
	}
	out := make([]TrashProfileResponse, 0, len(profiles))
	for _, g := range profiles {
		allowed := []string{}
		for _, item := range g.Profile.Items {
			if item.Allowed {
				allowed = append(allowed, item.Name)
			}
		}
		out = append(out, TrashProfileResponse{
			TrashID: g.Profile.TrashID, App: g.App, Name: g.Profile.Name, ScoreSet: g.Profile.ScoreSet,
			Cutoff: g.Profile.Cutoff, Qualities: allowed, FormatCount: len(g.Profile.Formats), SyncedAt: g.SyncedAt,
		})
	}
	return out, nil
}

func (c *QualityController) settings(ctx fuego.ContextNoBody) (AcquisitionSettingsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return AcquisitionSettingsResponse{}, err
	}
	cfg, err := c.quality.Settings(ctx.Context())
	if err != nil {
		return AcquisitionSettingsResponse{}, err
	}
	return settingsResponse(cfg), nil
}

func (c *QualityController) updateSettings(ctx fuego.ContextWithBody[AcquisitionSettingsRequest]) (AcquisitionSettingsResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return AcquisitionSettingsResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return AcquisitionSettingsResponse{}, err
	}
	cfg, err := c.quality.UpdateSettings(ctx.Context(), QualitySettings{
		DefaultProfile:   body.DefaultProfileID,
		DefaultDelay:     time.Duration(body.DefaultDelayMinutes) * time.Minute,
		SettleWindow:     time.Duration(body.SettleWindowSeconds) * time.Second,
		TrashSyncEnabled: body.TrashSyncEnabled,
	})
	if err != nil {
		return AcquisitionSettingsResponse{}, qualityProblem(err)
	}
	return settingsResponse(cfg), nil
}

// qualityProblem maps quality errors to problems; a not-found is a profile.
func qualityProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, api.ErrNotFound):
		return problem.New(TypeProfileNotFound, http.StatusNotFound, "no such quality profile")
	}
	return commonProblem(err)
}

func formatProblem(err error) error {
	if errors.Is(err, api.ErrNotFound) {
		return problem.New(TypeFormatNotFound, http.StatusNotFound, "no such custom format")
	}
	return commonProblem(err)
}

func commonProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNameTaken):
		return problem.New(TypeNameTaken, http.StatusConflict, err.Error())
	case errors.Is(err, ErrDefaultProfile):
		return problem.New(TypeDefaultProfile, http.StatusConflict, "pick another default profile before deleting this one")
	case errors.Is(err, ErrInvalidProfile), errors.Is(err, ErrInvalidFormat), errors.Is(err, ErrInvalidSettings):
		return problem.New(problem.TypeBadRequest, http.StatusBadRequest, err.Error())
	}
	return err
}

func qualityResponse(q domain.Quality) QualityResponse {
	return QualityResponse{ID: q.ID, Name: q.Name, Source: q.Source, Resolution: q.Resolution}
}

func profileResponse(p api.Profile) ProfileResponse {
	out := ProfileResponse{
		ID: p.ID, Name: p.Name, UpgradeAllowed: p.UpgradeAllowed, Preset: p.Preset,
		Items: make([]ProfileItemResponse, 0, len(p.Items)), Cutoff: p.Cutoff,
		MinFormatScore: p.MinFormatScore, CutoffFormatScore: p.CutoffFormatScore, MinUpgradeFormatScore: p.MinUpgradeFormatScore,
		PropersMode: p.PropersMode, FormatScores: make([]FormatScoreResponse, 0, len(p.FormatScores)),
		TrashProfileID: p.TrashProfileID, TrashScoreSet: p.TrashScoreSet,
	}
	for _, item := range p.Items {
		resp := ProfileItemResponse{
			Qualities: make([]QualityResponse, 0, len(item.Qualities)), Allowed: item.Allowed,
			MinMBPerMin: item.Sizes.MinMBPerMin, PreferredMBPerMin: item.Sizes.PreferredMBPerMin, MaxMBPerMin: item.Sizes.MaxMBPerMin,
		}
		if item.Group != nil {
			resp.Group = &QualityGroupResponse{ID: item.Group.ID, Name: item.Group.Name}
		}
		for _, q := range item.Qualities {
			resp.Qualities = append(resp.Qualities, qualityResponse(q))
		}
		out.Items = append(out.Items, resp)
	}
	for _, s := range p.FormatScores {
		out.FormatScores = append(out.FormatScores, FormatScoreResponse{FormatID: s.Format, Name: s.Name, Score: s.Score, UserOverride: s.UserOverride})
	}
	return out
}

func (r ProfileRequest) toProfile(id domain.ProfileID) api.Profile {
	p := api.Profile{
		ID: id, Name: r.Name, UpgradeAllowed: r.UpgradeAllowed, Items: make([]api.ProfileItem, 0, len(r.Items)),
		Cutoff: r.Cutoff, MinFormatScore: r.MinFormatScore, CutoffFormatScore: r.CutoffFormatScore,
		MinUpgradeFormatScore: r.MinUpgradeFormatScore, PropersMode: r.PropersMode,
		FormatScores: make([]api.FormatScore, 0, len(r.FormatScores)),
	}
	for _, item := range r.Items {
		profileItem := api.ProfileItem{
			Allowed: item.Allowed,
			Sizes:   api.SizeLimits{MinMBPerMin: item.MinMBPerMin, PreferredMBPerMin: item.PreferredMBPerMin, MaxMBPerMin: item.MaxMBPerMin},
		}
		if item.GroupName != "" {
			profileItem.Group = &api.QualityGroup{Name: item.GroupName}
		}
		for _, id := range item.QualityIDs {
			profileItem.Qualities = append(profileItem.Qualities, domain.Quality{ID: id})
		}
		p.Items = append(p.Items, profileItem)
	}
	for _, s := range r.FormatScores {
		p.FormatScores = append(p.FormatScores, api.FormatScore{Format: s.FormatID, Score: s.Score})
	}
	return p
}

func formatResponse(f CustomFormat) CustomFormatResponse {
	out := CustomFormatResponse{ID: f.ID, Name: f.Name, TrashID: f.TrashID, IncludeWhenRenaming: f.IncludeWhenRenaming, Specs: make([]FormatSpecDTO, 0, len(f.Specs))}
	for _, s := range f.Specs {
		out.Specs = append(out.Specs, FormatSpecDTO(s))
	}
	return out
}

func (r CustomFormatRequest) toFormat(id api.CustomFormatID) CustomFormat {
	f := CustomFormat{ID: id, Name: r.Name, IncludeWhenRenaming: r.IncludeWhenRenaming, Specs: make([]api.FormatSpec, 0, len(r.Specs))}
	for _, s := range r.Specs {
		f.Specs = append(f.Specs, api.FormatSpec(s))
	}
	return f
}

func settingsResponse(cfg QualitySettings) AcquisitionSettingsResponse {
	return AcquisitionSettingsResponse{
		DefaultProfileID:    cfg.DefaultProfile,
		DefaultDelayMinutes: int(cfg.DefaultDelay / time.Minute),
		SettleWindowSeconds: int(cfg.SettleWindow / time.Second),
		TrashSyncEnabled:    cfg.TrashSyncEnabled,
	}
}
