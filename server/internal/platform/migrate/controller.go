package migrate

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	tagMigration = "migration"
	routeRoot    = "/platform/migration"
	queryKind    = "kind"
	queryQuery   = "q"

	TypeUnsupportedVersion problem.Type = "urn:magpie:problem:unsupported-version"
	TypeArrUnreachable     problem.Type = "urn:magpie:problem:arr-unreachable"
	TypeMigrationBusy      problem.Type = "urn:magpie:problem:migration-busy"
	TypeMigrationNotReady  problem.Type = "urn:magpie:problem:migration-not-ready"
	TypeInvalidDecision    problem.Type = "urn:magpie:problem:invalid-decision"
)

type MigrationSource struct {
	App         App      `json:"app" validate:"required"`
	URL         string   `json:"url" validate:"required"`
	Version     string   `json:"version" validate:"required"`
	Skipped     bool     `json:"skipped" validate:"required"`
	RootFolders []string `json:"rootFolders" validate:"required"`
}

type MigrationPath struct {
	App    App        `json:"app" validate:"required"`
	From   string     `json:"from" validate:"required"`
	To     string     `json:"to" validate:"required"`
	Access PathAccess `json:"access" validate:"required"`
}

type MigrationProgress struct {
	Step  string `json:"step" validate:"required"`
	Done  int    `json:"done" validate:"required"`
	Total int    `json:"total" validate:"required"`
}

type MigrationOptions struct {
	RenameFolders bool `json:"renameFolders" validate:"required"`
	ImportAliases bool `json:"importAliases" validate:"required"`
}

// MigrationAppReport counts one app's titles by dry-run outcome.
type MigrationAppReport struct {
	App     App `json:"app" validate:"required"`
	Matched int `json:"matched" validate:"required"`
	Review  int `json:"review" validate:"required"`
	Skipped int `json:"skipped" validate:"required"`
}

type MigrationReport struct {
	Apps []MigrationAppReport `json:"apps" validate:"required"`
	// NumberingMismatches are series whose seasons differ from TMDB's aired order.
	NumberingMismatches int `json:"numberingMismatches" validate:"required"`
	UnmappedEpisodes    int `json:"unmappedEpisodes" validate:"required"`
	Formats             int `json:"formats" validate:"required"`
	FormatClashes       int `json:"formatClashes" validate:"required"`
	UntranslatedSpecs   int `json:"untranslatedSpecs" validate:"required"`
	// ReleaseProfileFormats are the formats release profiles became.
	ReleaseProfileFormats int      `json:"releaseProfileFormats" validate:"required"`
	Profiles              int      `json:"profiles" validate:"required"`
	ProfileClashes        int      `json:"profileClashes" validate:"required"`
	Files                 int      `json:"files" validate:"required"`
	MissingFiles          int      `json:"missingFiles" validate:"required"`
	BlocklistWithHash     int      `json:"blocklistWithHash" validate:"required"`
	BlocklistWithoutHash  int      `json:"blocklistWithoutHash" validate:"required"`
	History               int      `json:"history" validate:"required"`
	Warnings              []string `json:"warnings" validate:"required"`
}

type MigrationCandidate struct {
	Tmdb domain.TmdbID    `json:"tmdb" validate:"required"`
	Kind domain.MediaKind `json:"kind" validate:"required"`
	Name string           `json:"name" validate:"required"`
	Year int              `json:"year" validate:"required"`
}

type MigrationTitleReview struct {
	ID         string               `json:"id" validate:"required"`
	App        App                  `json:"app" validate:"required"`
	Kind       domain.MediaKind     `json:"kind" validate:"required"`
	Name       string               `json:"name" validate:"required"`
	Year       int                  `json:"year" validate:"required"`
	Candidates []MigrationCandidate `json:"candidates" validate:"required"`
	Decision   TitleDecision        `json:"decision" validate:"required"`
	Tmdb       []domain.TmdbID      `json:"tmdb" validate:"required"`
}

type MigrationSeasonCount struct {
	Season   int `json:"season" validate:"required"`
	Episodes int `json:"episodes" validate:"required"`
}

type MigrationGroupOption struct {
	ID    string  `json:"id" validate:"required"`
	Name  string  `json:"name" validate:"required"`
	Type  int     `json:"type" validate:"required"`
	Score float64 `json:"score" validate:"required"`
}

type MigrationNumberingReview struct {
	ID            string                 `json:"id" validate:"required"`
	Name          string                 `json:"name" validate:"required"`
	SonarrSeasons []MigrationSeasonCount `json:"sonarrSeasons" validate:"required"`
	TmdbSeasons   []MigrationSeasonCount `json:"tmdbSeasons" validate:"required"`
	Groups        []MigrationGroupOption `json:"groups" validate:"required"`
	Suggested     string                 `json:"suggested" validate:"required"`
	Decision      NumberingDecision      `json:"decision" validate:"required"`
	Group         string                 `json:"group" validate:"required"`
}

type MigrationClash struct {
	ID     string      `json:"id" validate:"required"`
	Kind   ClashKind   `json:"kind" validate:"required"`
	Name   string      `json:"name" validate:"required"`
	Sonarr []string    `json:"sonarr" validate:"required"`
	Radarr []string    `json:"radarr" validate:"required"`
	Choice ClashChoice `json:"choice" validate:"required"`
}

type MigrationSpecReview struct {
	ID         string       `json:"id" validate:"required"`
	FormatName string       `json:"formatName" validate:"required"`
	App        App          `json:"app" validate:"required"`
	SpecName   string       `json:"specName" validate:"required"`
	Kind       string       `json:"kind" validate:"required"`
	RawValue   string       `json:"rawValue" validate:"required"`
	Reason     string       `json:"reason" validate:"required"`
	Options    []string     `json:"options" validate:"required"`
	Decision   SpecDecision `json:"decision" validate:"required"`
	Value      string       `json:"value" validate:"required"`
}

type MigrationSeriesTypeReview struct {
	ID       string             `json:"id" validate:"required"`
	Name     string             `json:"name" validate:"required"`
	Type     string             `json:"type" validate:"required"`
	Decision SeriesTypeDecision `json:"decision" validate:"required"`
}

type MigrationQueueCount struct {
	Queue   ReviewQueue `json:"queue" validate:"required"`
	Total   int         `json:"total" validate:"required"`
	Pending int         `json:"pending" validate:"required"`
}

type MigrationReviews struct {
	Queues      []MigrationQueueCount       `json:"queues" validate:"required"`
	Titles      []MigrationTitleReview      `json:"titles" validate:"required"`
	Numbering   []MigrationNumberingReview  `json:"numbering" validate:"required"`
	Clashes     []MigrationClash            `json:"clashes" validate:"required"`
	Specs       []MigrationSpecReview       `json:"specs" validate:"required"`
	SeriesTypes []MigrationSeriesTypeReview `json:"seriesTypes" validate:"required"`
}

type MigrationSummaryStep struct {
	Step     CommitStep `json:"step" validate:"required"`
	Imported int        `json:"imported" validate:"required"`
}

type MigrationSummary struct {
	Steps         []MigrationSummaryStep `json:"steps" validate:"required"`
	SkippedTitles []string               `json:"skippedTitles" validate:"required"`
	MissingFiles  []string               `json:"missingFiles" validate:"required"`
	UnmappedFiles []string               `json:"unmappedFiles" validate:"required"`
}

// MigrationResponse is the whole Sonarr/Radarr import as the wizard shows it. API keys are never sent back.
type MigrationResponse struct {
	Phase            Phase             `json:"phase" validate:"required"`
	Sources          []MigrationSource `json:"sources" validate:"required"`
	Paths            []MigrationPath   `json:"paths" validate:"required"`
	CutoverConfirmed bool              `json:"cutoverConfirmed" validate:"required"`
	Options          MigrationOptions  `json:"options" validate:"required"`
	Progress         MigrationProgress `json:"progress" validate:"required"`
	Error            string            `json:"error" validate:"required"`
	FailedStep       string            `json:"failedStep" validate:"required"`
	CanCommit        bool              `json:"canCommit" validate:"required"`
	// Report and Reviews are set once a dry run finished; Summary once the commit did.
	Report  *MigrationReport  `json:"report,omitempty"`
	Reviews *MigrationReviews `json:"reviews,omitempty"`
	Summary *MigrationSummary `json:"summary,omitempty"`
}

type MigrationConnectRequest struct {
	App    App    `json:"app" validate:"required"`
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
	// Skip leaves the app out of the import.
	Skip bool `json:"skip"`
}

type MigrationPathRequest struct {
	App  App    `json:"app" validate:"required"`
	From string `json:"from" validate:"required"`
	To   string `json:"to"`
}

type MigrationPathsRequest struct {
	Paths []MigrationPathRequest `json:"paths" validate:"required"`
}

type MigrationPathCheckRequest struct {
	Path string `json:"path"`
}

type MigrationPathCheck struct {
	Path   string     `json:"path" validate:"required"`
	Access PathAccess `json:"access" validate:"required"`
}

type MigrationCutoverRequest struct {
	Confirmed bool `json:"confirmed"`
}

type MigrationOptionsRequest struct {
	RenameFolders bool `json:"renameFolders"`
	ImportAliases bool `json:"importAliases"`
}

// MigrationDecisionRequest settles one review item; only the fields of its queue are read.
type MigrationDecisionRequest struct {
	Queue      ReviewQueue        `json:"queue" validate:"required"`
	ID         string             `json:"id" validate:"required"`
	Title      TitleDecision      `json:"title,omitempty"`
	Tmdb       []domain.TmdbID    `json:"tmdb,omitempty"`
	Numbering  NumberingDecision  `json:"numbering,omitempty"`
	Group      string             `json:"group,omitempty"`
	Clash      ClashChoice        `json:"clash,omitempty"`
	Spec       SpecDecision       `json:"spec,omitempty"`
	Value      string             `json:"value,omitempty"`
	SeriesType SeriesTypeDecision `json:"seriesType,omitempty"`
}

type MigrationSkipQueueRequest struct {
	Queue ReviewQueue `json:"queue" validate:"required"`
}

type Controller struct {
	service *Service
}

func NewController(s *Service) *Controller {
	return &Controller{service: s}
}

func (c *Controller) Register(s *fuego.Server) {
	tags := fuego.OptionTags(tagMigration)
	fuego.Get(s, routeRoot, c.get, tags,
		fuego.OptionOperationID("getMigration"),
		fuego.OptionSummary("The Sonarr/Radarr import: connections, paths, dry-run report, review queues and summary (admin)"))
	fuego.Delete(s, routeRoot, c.reset, tags,
		fuego.OptionOperationID("resetMigration"),
		fuego.OptionSummary("Forget the import to start over; anything committed stays (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusNoContent))
	fuego.Put(s, routeRoot+"/connections", c.connect, tags,
		fuego.OptionOperationID("connectMigrationSource"),
		fuego.OptionSummary("Connect or skip Sonarr or Radarr; checks the version (Sonarr ≥ 4.0) and reads its root folders (admin)"))
	fuego.Put(s, routeRoot+"/paths", c.setPaths, tags,
		fuego.OptionOperationID("setMigrationPaths"),
		fuego.OptionSummary("Set where each app's root folder is as the server sees it (admin)"))
	fuego.Post(s, routeRoot+"/paths/check", c.checkPath, tags,
		fuego.OptionOperationID("checkMigrationPath"),
		fuego.OptionSummary("Check that the server can read and write a folder (admin)"))
	fuego.Put(s, routeRoot+"/cutover", c.setCutover, tags,
		fuego.OptionOperationID("setMigrationCutover"),
		fuego.OptionSummary("Confirm the manual cutover checklist; Commit needs it (admin)"))
	fuego.Put(s, routeRoot+"/options", c.setOptions, tags,
		fuego.OptionOperationID("setMigrationOptions"),
		fuego.OptionSummary("Choose folder renames and alias import (admin)"))
	fuego.Post(s, routeRoot+"/dry-run", c.dryRun, tags,
		fuego.OptionOperationID("startMigrationDryRun"),
		fuego.OptionSummary("Read both apps and plan the import without writing anything; runs in the background (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
	fuego.Post(s, routeRoot+"/reviews", c.decide, tags,
		fuego.OptionOperationID("decideMigrationReview"),
		fuego.OptionSummary("Settle one review item (admin)"))
	fuego.Post(s, routeRoot+"/reviews/skip", c.skip, tags,
		fuego.OptionOperationID("skipMigrationReviewQueue"),
		fuego.OptionSummary("Settle every open item of a queue with its default (admin)"))
	fuego.Get(s, routeRoot+"/search", c.search, tags,
		fuego.OptionOperationID("searchMigrationTitle"),
		fuego.OptionSummary("Search TMDB for a title to match an unmatched one to (admin)"),
		fuego.OptionQuery(queryKind, "MOVIE or SERIES", fuego.ParamRequired()),
		fuego.OptionQuery(queryQuery, "Search text", fuego.ParamRequired()))
	fuego.Post(s, routeRoot+"/commit", c.commit, tags,
		fuego.OptionOperationID("startMigrationCommit"),
		fuego.OptionSummary("Import the plan, or resume a failed commit; runs in the background (admin)"),
		fuego.OptionDefaultStatusCode(http.StatusAccepted))
}

func (c *Controller) get(ctx fuego.ContextNoBody) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.State(ctx.Context()))
}

func (c *Controller) reset(ctx fuego.ContextNoBody) (any, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	return nil, migrationProblem(c.service.Reset(ctx.Context()))
}

func (c *Controller) connect(ctx fuego.ContextWithBody[MigrationConnectRequest]) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationResponse{}, err
	}
	if !slices.Contains(Apps, body.App) {
		return MigrationResponse{}, problem.New(problem.TypeBadRequest, http.StatusBadRequest, "app must be SONARR or RADARR")
	}
	req := ConnectRequest{App: body.App, Connection: Connection{URL: body.URL, APIKey: body.APIKey}, Skip: body.Skip}
	return c.respond(c.service.Connect(ctx.Context(), req))
}

func (c *Controller) setPaths(ctx fuego.ContextWithBody[MigrationPathsRequest]) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationResponse{}, err
	}
	paths := make([]PathRewrite, 0, len(body.Paths))
	for _, p := range body.Paths {
		paths = append(paths, PathRewrite(p))
	}
	return c.respond(c.service.SetPaths(ctx.Context(), paths))
}

func (c *Controller) checkPath(ctx fuego.ContextWithBody[MigrationPathCheckRequest]) (MigrationPathCheck, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationPathCheck{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationPathCheck{}, err
	}
	path := strings.TrimSpace(body.Path)
	return MigrationPathCheck{Path: path, Access: c.service.CheckPath(path)}, nil
}

func (c *Controller) setCutover(ctx fuego.ContextWithBody[MigrationCutoverRequest]) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.SetCutover(ctx.Context(), body.Confirmed))
}

func (c *Controller) setOptions(ctx fuego.ContextWithBody[MigrationOptionsRequest]) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.SetOptions(ctx.Context(), Options(body)))
}

func (c *Controller) dryRun(ctx fuego.ContextNoBody) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.StartDryRun(ctx.Context()))
}

func (c *Controller) decide(ctx fuego.ContextWithBody[MigrationDecisionRequest]) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.Decide(ctx.Context(), Decision(body)))
}

func (c *Controller) skip(ctx fuego.ContextWithBody[MigrationSkipQueueRequest]) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.SkipQueue(ctx.Context(), body.Queue))
}

func (c *Controller) search(ctx fuego.ContextNoBody) ([]MigrationCandidate, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return nil, err
	}
	found, err := c.service.Search(ctx.Context(), domain.MediaKind(ctx.QueryParam(queryKind)), ctx.QueryParam(queryQuery))
	if err != nil {
		return nil, migrationProblem(err)
	}
	return candidates(found), nil
}

func (c *Controller) commit(ctx fuego.ContextNoBody) (MigrationResponse, error) {
	if err := httpserver.RequireAdmin(ctx.Context()); err != nil {
		return MigrationResponse{}, err
	}
	return c.respond(c.service.StartCommit(ctx.Context()))
}

func (c *Controller) respond(s Session, err error) (MigrationResponse, error) {
	if err != nil {
		return MigrationResponse{}, migrationProblem(err)
	}
	return c.response(s), nil
}

func migrationProblem(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrUnsupportedVersion):
		return problem.New(TypeUnsupportedVersion, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrArrUnauthorized), errors.Is(err, ErrArrUnreachable), errors.Is(err, ErrArrWrongApp):
		return problem.New(TypeArrUnreachable, http.StatusBadGateway, err.Error())
	case errors.Is(err, ErrBusy):
		return problem.New(TypeMigrationBusy, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotReady), errors.Is(err, ErrNoSource):
		return problem.New(TypeMigrationNotReady, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidDecision):
		return problem.New(TypeInvalidDecision, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUnknownReview):
		return problem.New(problem.TypeNotFound, http.StatusNotFound, err.Error())
	}
	return err
}

func (c *Controller) response(s Session) MigrationResponse {
	r := MigrationResponse{
		Phase: s.Phase, Sources: []MigrationSource{}, Paths: []MigrationPath{}, CutoverConfirmed: s.CutoverConfirmed,
		Options:  MigrationOptions(s.Options),
		Progress: MigrationProgress(s.Progress), Error: s.Error, FailedStep: string(s.FailedStep), CanCommit: s.CanCommit(),
	}
	for _, src := range s.Sources {
		r.Sources = append(r.Sources, MigrationSource{App: src.App, URL: src.Connection.URL, Version: src.Version, Skipped: src.Skipped, RootFolders: nonNil(src.RootFolders)})
	}
	for _, p := range s.Paths {
		r.Paths = append(r.Paths, MigrationPath{App: p.App, From: p.From, To: p.To, Access: c.service.CheckPath(p.To)})
	}
	if s.Plan != nil {
		report := reportOf(s.Plan)
		reviews := reviewsOf(s.Plan.Reviews)
		r.Report, r.Reviews = &report, &reviews
	}
	if s.Summary != nil {
		summary := summaryOf(*s.Summary)
		r.Summary = &summary
	}
	return r
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func reportOf(p *Plan) MigrationReport {
	r := MigrationReport{Apps: []MigrationAppReport{}, Warnings: nonNil(p.Warnings), Formats: len(p.Formats), Profiles: len(p.Profiles), Files: len(p.Files), History: len(p.History)}
	for _, app := range Apps {
		row := MigrationAppReport{App: app}
		for _, t := range p.Titles {
			if t.App != app || t.SplitFrom != "" {
				continue
			}
			switch t.Status {
			case TitleMatched, TitleSplit:
				row.Matched++
			case TitleInReview:
				row.Review++
			case TitleSkipped:
				row.Skipped++
			}
		}
		r.Apps = append(r.Apps, row)
	}
	r.NumberingMismatches = len(p.Reviews.Numbering)
	for _, t := range p.Titles {
		if t.Status != TitleMatched {
			continue
		}
		for _, e := range t.Episodes {
			if !e.Mapped {
				r.UnmappedEpisodes++
			}
		}
	}
	for _, c := range p.Reviews.Clashes {
		if c.Kind == ClashFormat {
			r.FormatClashes++
		} else {
			r.ProfileClashes++
		}
	}
	r.UntranslatedSpecs = len(p.Reviews.Specs)
	for _, f := range p.Formats {
		if strings.Contains(string(f.Source), keySeparator+recordReleaseProfile+keySeparator) {
			r.ReleaseProfileFormats++
		}
	}
	for _, f := range p.Files {
		if f.Missing {
			r.MissingFiles++
		}
	}
	for _, b := range p.Blocklist {
		if b.InfoHash != "" {
			r.BlocklistWithHash++
		} else {
			r.BlocklistWithoutHash++
		}
	}
	return r
}

func candidates(found []TmdbCandidate) []MigrationCandidate {
	out := make([]MigrationCandidate, 0, len(found))
	for _, c := range found {
		out = append(out, MigrationCandidate(c))
	}
	return out
}

func seasonRows(counts map[int]int) []MigrationSeasonCount {
	out := make([]MigrationSeasonCount, 0, len(counts))
	for season, episodes := range counts {
		out = append(out, MigrationSeasonCount{Season: season, Episodes: episodes})
	}
	slices.SortFunc(out, func(a, b MigrationSeasonCount) int { return a.Season - b.Season })
	return out
}

func reviewsOf(r Reviews) MigrationReviews {
	out := MigrationReviews{
		Queues: []MigrationQueueCount{}, Titles: []MigrationTitleReview{}, Numbering: []MigrationNumberingReview{},
		Clashes: []MigrationClash{}, Specs: []MigrationSpecReview{}, SeriesTypes: []MigrationSeriesTypeReview{},
	}
	pending := r.Pending()
	totals := map[ReviewQueue]int{
		QueueTitles: len(r.Titles), QueueNumbering: len(r.Numbering), QueueClashes: len(r.Clashes),
		QueueSpecs: len(r.Specs), QueueSeriesTypes: len(r.SeriesTypes),
	}
	for _, q := range ReviewQueues {
		out.Queues = append(out.Queues, MigrationQueueCount{Queue: q, Total: totals[q], Pending: pending[q]})
	}
	for _, t := range r.Titles {
		out.Titles = append(out.Titles, MigrationTitleReview{
			ID: t.ID, App: t.App, Kind: t.Kind, Name: t.Name, Year: t.Year, Candidates: candidates(t.Candidates),
			Decision: t.Decision, Tmdb: nonNil(t.Tmdb),
		})
	}
	for _, n := range r.Numbering {
		groups := make([]MigrationGroupOption, 0, len(n.Groups))
		for _, g := range n.Groups {
			groups = append(groups, MigrationGroupOption(g))
		}
		out.Numbering = append(out.Numbering, MigrationNumberingReview{
			ID: n.ID, Name: n.Name, SonarrSeasons: seasonRows(n.SonarrSeasons), TmdbSeasons: seasonRows(n.TmdbSeasons),
			Groups: groups, Suggested: n.Suggested, Decision: n.Decision, Group: n.Group,
		})
	}
	for _, c := range r.Clashes {
		out.Clashes = append(out.Clashes, MigrationClash{ID: c.ID, Kind: c.Kind, Name: c.Name, Sonarr: nonNil(c.Sonarr), Radarr: nonNil(c.Radarr), Choice: c.Choice})
	}
	for _, s := range r.Specs {
		out.Specs = append(out.Specs, MigrationSpecReview{
			ID: s.ID, FormatName: s.FormatName, App: s.App, SpecName: s.SpecName, Kind: string(s.Kind), RawValue: s.RawValue,
			Reason: s.Reason, Options: nonNil(s.Options), Decision: s.Decision, Value: s.Value,
		})
	}
	for _, s := range r.SeriesTypes {
		out.SeriesTypes = append(out.SeriesTypes, MigrationSeriesTypeReview(s))
	}
	return out
}

func summaryOf(s Summary) MigrationSummary {
	out := MigrationSummary{Steps: []MigrationSummaryStep{}, SkippedTitles: nonNil(s.SkippedTitles), MissingFiles: nonNil(s.MissingFiles), UnmappedFiles: nonNil(s.UnmappedFiles)}
	for _, step := range CommitSteps {
		out.Steps = append(out.Steps, MigrationSummaryStep{Step: step, Imported: s.Imported[step]})
	}
	return out
}
