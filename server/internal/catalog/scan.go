package catalog

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"

	"gorm.io/gorm"
)

// ScanReviewState is where a library folder Catalog couldn't match with certainty stands.
type ScanReviewState string

const (
	ScanReviewPending ScanReviewState = "PENDING"
	ScanReviewMatched ScanReviewState = "MATCHED"
	ScanReviewIgnored ScanReviewState = "IGNORED"
)

const (
	// MatchThreshold is the score a lone candidate needs to match a folder without the admin.
	MatchThreshold = 0.9
	// reviewCandidates is how many TMDB results a review row offers.
	reviewCandidates = 5
	// A year off by one is common (a festival premiere, a late release); further off is likely another film.
	nearYearFactor    = 0.95
	unknownYearFactor = 0.95
	wrongYearFactor   = 0.6

	candidatePosterSize = "w154"
	andWord             = " and "
)

var (
	ErrScanReviewNotFound = errors.New("catalog: no such scan review")
	ErrScanReviewResolved = errors.New("catalog: the folder was already matched or ignored")
	errMatchedTitleGone   = errors.New("catalog: the matched title was removed meanwhile")
)

// ScanCandidate is one TMDB result a folder might be.
type ScanCandidate struct {
	Tmdb       domain.TmdbID    `json:"tmdbId"`
	Kind       domain.MediaKind `json:"kind"`
	Name       string           `json:"name"`
	Year       int              `json:"year"`
	Overview   string           `json:"overview"`
	PosterPath string           `json:"posterPath"`
	Score      float64          `json:"score"`
	// PosterURL is filled in when the row is read.
	PosterURL string `json:"-"`
}

// ScanReview is a library folder and what it might be.
type ScanReview struct {
	ID          string
	Folder      string
	Kind        domain.MediaKind
	ParsedTitle string
	ParsedYear  *int
	TmdbHint    *domain.TmdbID
	Candidates  []ScanCandidate
	State       ScanReviewState
	Title       *domain.TitleID
	CreatedAt   time.Time
}

// MatchFolder matches a folder Library found: its TMDB tag first, else a lone search result scoring at least
// MatchThreshold. A match creates the Title if it's new and publishes ScanFolderMatched; anything else waits in
// the review queue. A folder Catalog has seen before is left alone.
func (s *Service) MatchFolder(ctx context.Context, found libraryapi.FolderFound) error {
	if _, err := s.repo.scanReviewByPath(s.db.Read(ctx), found.Path); !errors.Is(err, errScanReviewMissing) {
		return err
	}
	matched := func(tx *gorm.DB, title domain.TitleID) error {
		return s.repo.insertScanReview(tx, s.newScanReview(found, ScanReviewMatched, &title, nil))
	}
	if found.TmdbHint != nil {
		err := s.matchFolder(ctx, found.Path, found.Kind, *found.TmdbHint, matched)
		if !errors.Is(err, ErrTmdbNotFound) {
			return err
		}
		slog.WarnContext(ctx, "catalog: TMDB doesn't know the folder's tag; searching instead", "folder", found.Path, "tmdb", *found.TmdbHint)
	}
	candidates, err := s.candidates(ctx, found)
	if err != nil {
		return err
	}
	if lone, ok := certain(candidates); ok {
		return s.matchFolder(ctx, found.Path, found.Kind, lone.Tmdb, matched)
	}
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		return s.repo.insertScanReview(tx, s.newScanReview(found, ScanReviewPending, nil, candidates))
	})
}

func (s *Service) newScanReview(found libraryapi.FolderFound, state ScanReviewState, title *domain.TitleID, candidates []ScanCandidate) scanReviewRow {
	if candidates == nil {
		candidates = []ScanCandidate{}
	}
	encoded, _ := json.Marshal(candidates)
	now := s.now().UTC()
	return scanReviewRow{
		ID: domain.NewID[string](), FolderPath: found.Path, Kind: found.Kind, ParsedTitle: found.ParsedTitle,
		ParsedYear: found.Year, TmdbHint: found.TmdbHint, Candidates: string(encoded), State: state, TitleID: title,
		CreatedAt: now, UpdatedAt: now,
	}
}

// candidates searches TMDB for the folder's title and keeps the best results of its kind, best first.
func (s *Service) candidates(ctx context.Context, found libraryapi.FolderFound) ([]ScanCandidate, error) {
	if strings.TrimSpace(found.ParsedTitle) == "" {
		return nil, nil
	}
	page, err := s.tmdb.Search(ctx, found.ParsedTitle, 1)
	if err != nil {
		return nil, err
	}
	var candidates []ScanCandidate
	for _, item := range page.Items {
		if item.Kind != found.Kind {
			continue
		}
		candidates = append(candidates, ScanCandidate{
			Tmdb: item.ID, Kind: item.Kind, Name: item.Name, Year: item.Year, Overview: item.Overview,
			PosterPath: item.PosterPath, Score: matchScore(found, item),
		})
	}
	slices.SortStableFunc(candidates, func(a, b ScanCandidate) int { return cmp.Compare(b.Score, a.Score) })
	return candidates[:min(len(candidates), reviewCandidates)], nil
}

// certain returns the only candidate scoring at least MatchThreshold.
func certain(candidates []ScanCandidate) (ScanCandidate, bool) {
	sure := slices.DeleteFunc(slices.Clone(candidates), func(c ScanCandidate) bool { return c.Score < MatchThreshold })
	if len(sure) != 1 {
		return ScanCandidate{}, false
	}
	return sure[0], true
}

// matchScore is how alike the folder's title is to the result's name, lowered when the years disagree.
func matchScore(found libraryapi.FolderFound, item TmdbSearchItem) float64 {
	score := max(similarity(found.ParsedTitle, item.Name), similarity(found.ParsedTitle, item.OriginalName))
	if found.Year == nil {
		return score
	}
	switch diff := *found.Year - item.Year; {
	case item.Year == 0:
		return score * unknownYearFactor
	case diff == 0:
		return score
	case diff == 1 || diff == -1:
		return score * nearYearFactor
	}
	return score * wrongYearFactor
}

// similarity is 1 minus the edit distance between the normalised names, over the longer one's length.
func similarity(a, b string) float64 {
	x, y := []rune(normaliseName(a)), []rune(normaliseName(b))
	longest := max(len(x), len(y))
	if longest == 0 {
		return 0
	}
	return 1 - float64(editDistance(x, y))/float64(longest)
}

// normaliseName lowercases a name, spells out "&" and turns punctuation into single spaces.
func normaliseName(name string) string {
	name = strings.ReplaceAll(strings.ToLower(name), "&", andWord)
	return strings.Join(strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// editDistance is the Levenshtein distance between two rune slices.
func editDistance(a, b []rune) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := range a {
		current[0] = i + 1
		for j := range b {
			substitution := previous[j]
			if a[i] != b[j] {
				substitution++
			}
			current[j+1] = min(previous[j+1]+1, current[j]+1, substitution)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

// matchFolder links a folder to a TMDB title: it fetches the title unless it's tracked, then in one
// transaction creates it if it's new, lets record note the match, and publishes ScanFolderMatched.
func (s *Service) matchFolder(ctx context.Context, folder string, kind domain.MediaKind, tmdb domain.TmdbID, record func(tx *gorm.DB, title domain.TitleID) error) error {
	create, images, err := s.scannedTitle(ctx, folder, kind, tmdb)
	if err != nil {
		return err
	}
	var added *domain.TitleID
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.titleByTmdb(tx, kind, tmdb)
		switch {
		case errors.Is(err, api.ErrNotFound) && create != nil:
			if row, err = create(tx); err != nil {
				return err
			}
			added = ptr(domain.TitleID(row.ID))
		case errors.Is(err, api.ErrNotFound):
			return errMatchedTitleGone
		case err != nil:
			return err
		}
		title := domain.TitleID(row.ID)
		if err := record(tx, title); err != nil {
			return err
		}
		return s.publish(tx, api.ScanFolderMatched{Folder: folder, Title: title})
	})
	if err != nil || added == nil {
		return err
	}
	if err := s.cacheImages(ctx, *added, images); err != nil {
		slog.WarnContext(ctx, "caching title images failed; the refresh job retries", "title", *added, "error", err)
	}
	return nil
}

// scannedTitle fetches an untracked title from TMDB and returns how to store it; create is nil for a tracked one.
func (s *Service) scannedTitle(ctx context.Context, folder string, kind domain.MediaKind, tmdb domain.TmdbID) (create func(tx *gorm.DB) (titleRow, error), images TmdbImages, err error) {
	_, err = s.repo.titleByTmdb(s.db.Read(ctx), kind, tmdb)
	if !errors.Is(err, api.ErrNotFound) {
		return nil, TmdbImages{}, err
	}
	meta, err := metadataSetting.Get(ctx, s.settings)
	if err != nil {
		return nil, TmdbImages{}, err
	}
	if kind == domain.MediaKindSeries {
		snap, err := s.fetchSeries(ctx, tmdb, "", meta.Language)
		if err != nil {
			return nil, TmdbImages{}, err
		}
		return func(tx *gorm.DB) (titleRow, error) { return s.createScannedSeries(tx, snap, folder) }, snap.series.Images, nil
	}
	movie, err := s.tmdb.Movie(ctx, tmdb, meta.Language)
	if err != nil {
		return nil, TmdbImages{}, err
	}
	return func(tx *gorm.DB) (titleRow, error) {
		return s.createScannedMovie(tx, movie, meta.ReleaseRegion, folder)
	}, movie.Images, nil
}

// createScannedMovie stores a monitored movie on the default profile and publishes TitleAdded{LIBRARY_SCAN}.
func (s *Service) createScannedMovie(tx *gorm.DB, movie TmdbMovie, region, folder string) (titleRow, error) {
	row, err := s.movieRow(movie, region)
	if err != nil {
		return titleRow{}, err
	}
	row.Monitored = true
	row.Origin = string(domain.OriginLibraryScan)
	if err := s.repo.createTitle(tx, row); err != nil {
		return titleRow{}, err
	}
	return row, s.publish(tx, api.TitleAdded{
		Title:      domain.TitleID(row.ID),
		Kind:       domain.MediaKindMovie,
		External:   domain.ExternalIDs{Tmdb: movie.ID, Tvdb: row.TvdbID, Imdb: row.ImdbID},
		Monitored:  []domain.EpisodeID{},
		Released:   row.Released,
		Origin:     domain.OriginLibraryScan,
		ScanFolder: folder,
	})
}

// createScannedSeries stores a series on the default profile with every regular season and future seasons
// monitored, as a whole-series request would, and publishes TitleAdded{LIBRARY_SCAN}.
func (s *Service) createScannedSeries(tx *gorm.DB, snap seriesSnapshot, folder string) (titleRow, error) {
	row := s.newSeriesRow(snap.series.ID)
	row.Monitored = true
	row.MonitorFutureSeasons = true
	row.Origin = string(domain.OriginLibraryScan)
	row, changes, err := s.createSeries(tx, row, snap, regularSeason)
	if err != nil {
		return titleRow{}, err
	}
	return row, s.publish(tx, api.TitleAdded{
		Title:      domain.TitleID(row.ID),
		Kind:       domain.MediaKindSeries,
		External:   domain.ExternalIDs{Tmdb: snap.series.ID, Tvdb: row.TvdbID, Imdb: row.ImdbID},
		Monitored:  changes.monitored(),
		Released:   row.Released,
		Origin:     domain.OriginLibraryScan,
		ScanFolder: folder,
	})
}

// ScanReviews lists the folders waiting for the admin, oldest first.
func (s *Service) ScanReviews(ctx context.Context) ([]ScanReview, error) {
	rows, err := s.repo.scanReviews(s.db.Read(ctx), ScanReviewPending)
	if err != nil {
		return nil, err
	}
	reviews := make([]ScanReview, 0, len(rows))
	for _, row := range rows {
		review, err := s.scanReviewView(row)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func (s *Service) scanReviewView(row scanReviewRow) (ScanReview, error) {
	review, err := row.review()
	if err != nil {
		return ScanReview{}, err
	}
	for i, c := range review.Candidates {
		if c.PosterPath != "" {
			review.Candidates[i].PosterURL = s.tmdb.ImageURL(c.PosterPath, candidatePosterSize)
		}
	}
	return review, nil
}

// MatchScanReview matches a pending folder to the TMDB title the admin picked.
func (s *Service) MatchScanReview(ctx context.Context, id string, tmdb domain.TmdbID) (ScanReview, error) {
	row, err := s.pendingReview(ctx, id)
	if err != nil {
		return ScanReview{}, err
	}
	err = s.matchFolder(ctx, row.FolderPath, row.Kind, tmdb, func(tx *gorm.DB, title domain.TitleID) error {
		return s.resolveReview(tx, id, ScanReviewMatched, &title)
	})
	if err != nil {
		return ScanReview{}, err
	}
	return s.reviewByID(ctx, id)
}

// IgnoreScanReview leaves a pending folder out of the library; later scans don't ask again.
func (s *Service) IgnoreScanReview(ctx context.Context, id string) (ScanReview, error) {
	if _, err := s.pendingReview(ctx, id); err != nil {
		return ScanReview{}, err
	}
	err := s.db.Write(ctx, func(tx *gorm.DB) error { return s.resolveReview(tx, id, ScanReviewIgnored, nil) })
	if err != nil {
		return ScanReview{}, err
	}
	return s.reviewByID(ctx, id)
}

func (s *Service) pendingReview(ctx context.Context, id string) (scanReviewRow, error) {
	row, err := s.repo.scanReview(s.db.Read(ctx), id)
	if err != nil {
		return scanReviewRow{}, err
	}
	if row.State != ScanReviewPending {
		return scanReviewRow{}, ErrScanReviewResolved
	}
	return row, nil
}

func (s *Service) resolveReview(tx *gorm.DB, id string, state ScanReviewState, title *domain.TitleID) error {
	moved, err := s.repo.resolveScanReview(tx, id, state, title, s.now().UTC())
	if err == nil && !moved {
		return ErrScanReviewResolved
	}
	return err
}

func (s *Service) reviewByID(ctx context.Context, id string) (ScanReview, error) {
	row, err := s.repo.scanReview(s.db.Read(ctx), id)
	if err != nil {
		return ScanReview{}, err
	}
	return s.scanReviewView(row)
}
