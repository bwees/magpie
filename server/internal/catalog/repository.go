package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ExternalIDSourceTMDB   = "TMDB"
	ExternalIDSourceImport = "IMPORT"

	orderByName    = "name COLLATE NOCASE, year, id"
	orderByAdded   = "added_at DESC, id"
	orderByYear    = "year DESC, name COLLATE NOCASE, id"
	orderEpisodes  = "season_number, episode_number"
	likeWildcard   = "%"
	likeEscape     = `\`
	emptyJSONArray = "[]"
)

type titleRow struct {
	ID               string `gorm:"primaryKey"`
	Kind             string
	TmdbID           int64
	TvdbID           *string
	ImdbID           *string
	ExternalIDSource string
	Name             string
	OriginalName     string
	Aliases          string
	// ImportedAliases are a migration's extra names as JSON; "" when there are none.
	ImportedAliases      string
	Year                 int
	Overview             string
	OriginalLanguage     string
	RuntimeMinutes       int
	SeriesStatus         string
	ProfileID            string
	Monitored            bool
	MonitorFutureSeasons bool
	EpisodeGroupID       string
	LanguagesOverride    *string
	Origin               string
	RequestedBy          *string
	RequestID            *string
	AddedAt              time.Time
	MetadataRefreshedAt  time.Time
	ReleaseTheatricalAt  *time.Time
	ReleaseDigitalAt     *time.Time
	ReleasePhysicalAt    *time.Time
	Released             bool
	ReleasedAt           *time.Time
}

func (titleRow) TableName() string { return "catalog_titles" }

type seasonRow struct {
	ID        string `gorm:"primaryKey"`
	TitleID   string
	Number    int
	Monitored bool
}

func (seasonRow) TableName() string { return "catalog_seasons" }

type episodeRow struct {
	ID             string `gorm:"primaryKey"`
	TitleID        string
	TmdbEpisodeID  int64
	SeasonNumber   int
	EpisodeNumber  int
	AiredSeason    int
	AiredNumber    int
	Name           string
	AirDate        *time.Time
	RuntimeMinutes int
	Monitored      bool
	Released       bool
	ReleasedAt     *time.Time
}

func (episodeRow) TableName() string { return "catalog_episodes" }

type imageRow struct {
	TitleID   string `gorm:"primaryKey"`
	Kind      string `gorm:"primaryKey"`
	TmdbPath  string
	LocalPath string
	FetchedAt time.Time
}

func (imageRow) TableName() string { return "catalog_images" }

type languageRow struct {
	Position        int `gorm:"primaryKey;autoIncrement:false"`
	Language        string
	Forced          bool
	HearingImpaired bool
}

func (languageRow) TableName() string { return "catalog_languages" }

// Repository stores Catalog's tables and implements api.Reader. Reads take a connection so the service can
// read inside its own write transaction.
type Repository struct {
	db *db.DB
}

var _ api.Reader = (*Repository)(nil)

func NewRepository(d *db.DB) *Repository {
	return &Repository{db: d}
}

func (r *Repository) Title(ctx context.Context, id domain.TitleID) (api.Title, error) {
	row, err := r.title(r.db.Read(ctx), id)
	if err != nil {
		return api.Title{}, err
	}
	return row.toAPI()
}

func (r *Repository) TitleByTmdb(ctx context.Context, kind domain.MediaKind, id domain.TmdbID) (api.Title, error) {
	row, err := r.titleByTmdb(r.db.Read(ctx), kind, id)
	if err != nil {
		return api.Title{}, err
	}
	return row.toAPI()
}

func (r *Repository) Titles(ctx context.Context, f api.TitleFilter) ([]api.Title, error) {
	rows, err := r.titleRows(r.db.Read(ctx), TitleQuery{Filter: f})
	if err != nil {
		return nil, err
	}
	titles := make([]api.Title, 0, len(rows))
	for _, row := range rows {
		t, err := row.toAPI()
		if err != nil {
			return nil, err
		}
		titles = append(titles, t)
	}
	return titles, nil
}

func (r *Repository) Episodes(ctx context.Context, title domain.TitleID) ([]api.Episode, error) {
	conn := r.db.Read(ctx)
	if _, err := r.title(conn, title); err != nil {
		return nil, err
	}
	rows, err := r.episodeRows(conn.Where("title_id = ?", title))
	if err != nil {
		return nil, err
	}
	return episodesToAPI(rows), nil
}

func (r *Repository) Episode(ctx context.Context, id domain.EpisodeID) (api.Episode, error) {
	var row episodeRow
	if err := first(r.db.Read(ctx).Where("id = ?", id), &row); err != nil {
		return api.Episode{}, err
	}
	return row.toAPI(), nil
}

func (r *Repository) EpisodeByNumber(ctx context.Context, title domain.TitleID, season, number int) (api.Episode, error) {
	var row episodeRow
	query := r.db.Read(ctx).Where("title_id = ? AND season_number = ? AND episode_number = ?", title, season, number)
	if err := first(query, &row); err != nil {
		return api.Episode{}, err
	}
	return row.toAPI(), nil
}

func (r *Repository) Wanted(ctx context.Context, title domain.TitleID) ([]api.Episode, bool, error) {
	conn := r.db.Read(ctx)
	row, err := r.title(conn, title)
	if err != nil {
		return nil, false, err
	}
	if domain.MediaKind(row.Kind) == domain.MediaKindMovie {
		return []api.Episode{}, row.Monitored && row.Released, nil
	}
	if !row.Monitored {
		return []api.Episode{}, false, nil
	}
	rows, err := r.episodeRows(conn.Where("title_id = ? AND monitored AND released", title))
	if err != nil {
		return nil, false, err
	}
	return episodesToAPI(rows), len(rows) > 0, nil
}

func (r *Repository) ImagePath(ctx context.Context, title domain.TitleID, kind string) (string, error) {
	var row imageRow
	if err := first(r.db.Read(ctx).Where("title_id = ? AND kind = ?", title, kind), &row); err != nil {
		return "", err
	}
	return row.LocalPath, nil
}

func (r *Repository) WantedLanguages(ctx context.Context, title domain.TitleID) ([]api.WantedLanguage, error) {
	conn := r.db.Read(ctx)
	row, err := r.title(conn, title)
	if err != nil {
		return nil, err
	}
	override, err := row.languagesOverride()
	if err != nil || override != nil {
		return override, err
	}
	return r.globalLanguages(conn)
}

// languagesOverride is nil when the Title uses the global list.
func (row titleRow) languagesOverride() ([]api.WantedLanguage, error) {
	if row.LanguagesOverride == nil {
		return nil, nil
	}
	override := []api.WantedLanguage{}
	if err := json.Unmarshal([]byte(*row.LanguagesOverride), &override); err != nil {
		return nil, fmt.Errorf("title %s languages override: %w", row.ID, err)
	}
	return override, nil
}

func (r *Repository) GlobalLanguages(ctx context.Context) ([]api.WantedLanguage, error) {
	return r.globalLanguages(r.db.Read(ctx))
}

func (r *Repository) globalLanguages(conn *gorm.DB) ([]api.WantedLanguage, error) {
	var rows []languageRow
	if err := conn.Order("position").Find(&rows).Error; err != nil {
		return nil, err
	}
	langs := make([]api.WantedLanguage, 0, len(rows))
	for _, row := range rows {
		langs = append(langs, api.WantedLanguage{Language: domain.Language(row.Language), Forced: row.Forced, HearingImpaired: row.HearingImpaired})
	}
	return langs, nil
}

// TitleSort orders title lists.
type TitleSort string

const (
	TitleSortName  TitleSort = "NAME"
	TitleSortAdded TitleSort = "ADDED"
	TitleSortYear  TitleSort = "YEAR"
)

// SeriesSummary is what a series row on the TV page shows besides the Title itself.
type SeriesSummary struct {
	// Seasons counts the numbered seasons, without specials.
	Seasons int
	// WantedEpisodes counts the monitored, released episodes of a monitored series.
	WantedEpisodes int
	// Next is the first announced episode not yet released, nil when none is.
	Next *api.Episode
}

// seriesSummaries summarises the listed series. Titles without episodes are absent.
func (r *Repository) seriesSummaries(conn *gorm.DB, titles []domain.TitleID) (map[domain.TitleID]SeriesSummary, error) {
	summaries := map[domain.TitleID]SeriesSummary{}
	if len(titles) == 0 {
		return summaries, nil
	}
	var counts []struct {
		TitleID domain.TitleID
		Seasons int
		Wanted  int
	}
	err := conn.Model(&episodeRow{}).
		Select("title_id, COUNT(DISTINCT CASE WHEN season_number <> ? THEN season_number END) AS seasons, "+
			"SUM(CASE WHEN monitored AND released THEN 1 ELSE 0 END) AS wanted", specialsSeason).
		Where("title_id IN ?", titles).Group("title_id").Scan(&counts).Error
	if err != nil {
		return nil, err
	}
	for _, c := range counts {
		summaries[c.TitleID] = SeriesSummary{Seasons: c.Seasons, WantedEpisodes: c.Wanted}
	}
	var upcoming []episodeRow
	err = conn.Where("title_id IN ? AND NOT released AND air_date IS NOT NULL", titles).
		Order("title_id, air_date, " + orderEpisodes).Find(&upcoming).Error
	if err != nil {
		return nil, err
	}
	for _, row := range upcoming {
		title := domain.TitleID(row.TitleID)
		summary := summaries[title]
		if summary.Next == nil {
			next := row.toAPI()
			summary.Next = &next
			summaries[title] = summary
		}
	}
	return summaries, nil
}

// TitleQuery is the HTTP list's filter: the Reader filter plus a name search and an order.
type TitleQuery struct {
	Filter api.TitleFilter
	// Search matches the name, original name or an alias, case-insensitively.
	Search string
	Sort   TitleSort
	// SeriesStatus keeps series with this api.SeriesStatus*; "" keeps every Title.
	SeriesStatus string
}

func (r *Repository) title(conn *gorm.DB, id domain.TitleID) (titleRow, error) {
	var row titleRow
	err := first(conn.Where("id = ?", id), &row)
	return row, err
}

func (r *Repository) titleByTmdb(conn *gorm.DB, kind domain.MediaKind, id domain.TmdbID) (titleRow, error) {
	var row titleRow
	err := first(conn.Where("kind = ? AND tmdb_id = ?", kind, id), &row)
	return row, err
}

func (r *Repository) titleRows(conn *gorm.DB, q TitleQuery) ([]titleRow, error) {
	query := conn.Model(&titleRow{})
	if q.Filter.Kind != nil {
		query = query.Where("kind = ?", *q.Filter.Kind)
	}
	if q.Filter.Monitored != nil {
		query = query.Where("monitored = ?", *q.Filter.Monitored)
	}
	if q.Filter.Released != nil {
		query = query.Where("released = ?", *q.Filter.Released)
	}
	if len(q.Filter.IDs) > 0 {
		query = query.Where("id IN ?", q.Filter.IDs)
	}
	if q.SeriesStatus != "" {
		query = query.Where("series_status = ?", q.SeriesStatus)
	}
	if search := strings.TrimSpace(q.Search); search != "" {
		pattern := likeWildcard + escapeLike(search) + likeWildcard
		query = query.Where(
			"name LIKE ? ESCAPE ? OR original_name LIKE ? ESCAPE ? OR aliases LIKE ? ESCAPE ?",
			pattern, likeEscape, pattern, likeEscape, pattern, likeEscape,
		)
	}
	order := orderByName
	switch q.Sort {
	case TitleSortAdded:
		order = orderByAdded
	case TitleSortYear:
		order = orderByYear
	}
	var rows []titleRow
	err := query.Order(order).Find(&rows).Error
	return rows, err
}

func (r *Repository) episodeRows(query *gorm.DB) ([]episodeRow, error) {
	var rows []episodeRow
	err := query.Order(orderEpisodes).Find(&rows).Error
	return rows, err
}

func (r *Repository) titleEpisodes(conn *gorm.DB, title domain.TitleID) ([]episodeRow, error) {
	return r.episodeRows(conn.Where("title_id = ?", title))
}

func (r *Repository) episode(conn *gorm.DB, title domain.TitleID, id domain.EpisodeID) (episodeRow, error) {
	var row episodeRow
	err := first(conn.Where("title_id = ? AND id = ?", title, id), &row)
	return row, err
}

// airedUnreleased lists episodes with an air date that the release clock hasn't released yet.
func (r *Repository) airedUnreleased(conn *gorm.DB) ([]episodeRow, error) {
	var rows []episodeRow
	err := conn.Where("NOT released AND air_date IS NOT NULL").Order("title_id, " + orderEpisodes).Find(&rows).Error
	return rows, err
}

func (r *Repository) saveEpisodes(tx *gorm.DB, rows []episodeRow) error {
	for _, row := range rows {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) seasonRows(conn *gorm.DB, title domain.TitleID) ([]seasonRow, error) {
	var rows []seasonRow
	err := conn.Where("title_id = ?", title).Order("number").Find(&rows).Error
	return rows, err
}

func (r *Repository) season(conn *gorm.DB, title domain.TitleID, number int) (seasonRow, error) {
	var row seasonRow
	err := first(conn.Where("title_id = ? AND number = ?", title, number), &row)
	return row, err
}

func (r *Repository) saveSeason(tx *gorm.DB, row seasonRow) error {
	return tx.Save(&row).Error
}

// replaceSeasons swaps a Title's season rows for rows.
func (r *Repository) replaceSeasons(tx *gorm.DB, title domain.TitleID, rows []seasonRow) error {
	if err := tx.Where("title_id = ?", title).Delete(&seasonRow{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// replaceGlobalLanguages stores langs as the global list, in order.
func (r *Repository) replaceGlobalLanguages(tx *gorm.DB, langs []api.WantedLanguage) error {
	if err := tx.Where("1 = 1").Delete(&languageRow{}).Error; err != nil {
		return err
	}
	rows := make([]languageRow, 0, len(langs))
	for position, lang := range langs {
		rows = append(rows, languageRow{Position: position, Language: string(lang.Language), Forced: lang.Forced, HearingImpaired: lang.HearingImpaired})
	}
	return tx.Create(&rows).Error
}

func (r *Repository) imageRows(conn *gorm.DB, title domain.TitleID) ([]imageRow, error) {
	var rows []imageRow
	err := conn.Where("title_id = ?", title).Find(&rows).Error
	return rows, err
}

// tracked finds which TMDB items are already Titles.
func (r *Repository) tracked(conn *gorm.DB, kind domain.MediaKind, ids []domain.TmdbID) (map[domain.TmdbID]domain.TitleID, error) {
	found := map[domain.TmdbID]domain.TitleID{}
	if len(ids) == 0 {
		return found, nil
	}
	var rows []titleRow
	if err := conn.Select("id", "tmdb_id").Where("kind = ? AND tmdb_id IN ?", kind, ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		found[domain.TmdbID(row.TmdbID)] = domain.TitleID(row.ID)
	}
	return found, nil
}

func (r *Repository) createTitle(tx *gorm.DB, row titleRow) error {
	return tx.Create(&row).Error
}

func (r *Repository) titlesOnProfile(conn *gorm.DB, profile domain.ProfileID) ([]domain.TitleID, error) {
	var ids []domain.TitleID
	err := conn.Model(&titleRow{}).Where("profile_id = ?", profile).Order("id").Pluck("id", &ids).Error
	return ids, err
}

func (r *Repository) saveTitle(tx *gorm.DB, row titleRow) error {
	return tx.Save(&row).Error
}

func (r *Repository) deleteTitle(tx *gorm.DB, id domain.TitleID) error {
	return tx.Where("id = ?", id).Delete(&titleRow{}).Error
}

func (r *Repository) saveImage(tx *gorm.DB, row imageRow) error {
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func (r *Repository) deleteImage(tx *gorm.DB, title domain.TitleID, kind string) error {
	return tx.Where("title_id = ? AND kind = ?", title, kind).Delete(&imageRow{}).Error
}

func first(query *gorm.DB, dest any) error {
	err := query.Take(dest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return api.ErrNotFound
	}
	return err
}

func escapeLike(s string) string {
	replacer := strings.NewReplacer(likeEscape, likeEscape+likeEscape, likeWildcard, likeEscape+likeWildcard, "_", likeEscape+"_")
	return replacer.Replace(s)
}

func (row titleRow) toAPI() (api.Title, error) {
	var aliases []string
	if err := json.Unmarshal([]byte(row.Aliases), &aliases); err != nil {
		return api.Title{}, fmt.Errorf("title %s aliases: %w", row.ID, err)
	}
	if row.ImportedAliases != "" {
		var imported []string
		if err := json.Unmarshal([]byte(row.ImportedAliases), &imported); err != nil {
			return api.Title{}, fmt.Errorf("title %s imported aliases: %w", row.ID, err)
		}
		for _, alias := range imported {
			if !slices.ContainsFunc(aliases, func(a string) bool { return strings.EqualFold(a, alias) }) {
				aliases = append(aliases, alias)
			}
		}
	}
	t := api.Title{
		ID:                   domain.TitleID(row.ID),
		Kind:                 domain.MediaKind(row.Kind),
		External:             domain.ExternalIDs{Tmdb: domain.TmdbID(row.TmdbID), Tvdb: row.TvdbID, Imdb: row.ImdbID},
		Name:                 row.Name,
		OriginalName:         row.OriginalName,
		Aliases:              aliases,
		Year:                 row.Year,
		RuntimeMinutes:       row.RuntimeMinutes,
		OriginalLanguage:     domain.Language(row.OriginalLanguage),
		SeriesStatus:         row.SeriesStatus,
		Profile:              domain.ProfileID(row.ProfileID),
		Monitored:            row.Monitored,
		MonitorFutureSeasons: row.MonitorFutureSeasons,
		EpisodeGroupID:       row.EpisodeGroupID,
		Dates:                row.releaseDates(),
		Released:             row.Released,
		ReleasedAt:           utc(row.ReleasedAt),
		Origin:               domain.Origin(row.Origin),
	}
	if row.RequestedBy != nil {
		user := domain.UserID(*row.RequestedBy)
		t.RequestedBy = &user
	}
	return t, nil
}

func (row titleRow) releaseDates() api.ReleaseDates {
	return api.ReleaseDates{
		Theatrical: utc(row.ReleaseTheatricalAt),
		Digital:    utc(row.ReleaseDigitalAt),
		Physical:   utc(row.ReleasePhysicalAt),
	}
}

func (row episodeRow) toAPI() api.Episode {
	return api.Episode{
		ID:             domain.EpisodeID(row.ID),
		Title:          domain.TitleID(row.TitleID),
		Season:         row.SeasonNumber,
		Number:         row.EpisodeNumber,
		AiredSeason:    row.AiredSeason,
		AiredNumber:    row.AiredNumber,
		Name:           row.Name,
		AirDate:        utc(row.AirDate),
		RuntimeMinutes: row.RuntimeMinutes,
		Monitored:      row.Monitored,
		Released:       row.Released,
	}
}

func episodesToAPI(rows []episodeRow) []api.Episode {
	episodes := make([]api.Episode, 0, len(rows))
	for _, row := range rows {
		episodes = append(episodes, row.toAPI())
	}
	return episodes
}

func encodeAliases(aliases []string) (string, error) {
	if len(aliases) == 0 {
		return emptyJSONArray, nil
	}
	encoded, err := json.Marshal(aliases)
	return string(encoded), err
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
