package acquisition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/db"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/parse"
	"magpie/internal/platform/settings"

	"github.com/dlclark/regexp2/v2"
	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

// BlockTermScore is TRaSH's "never grab" score, which the block-term action gives its format.
const BlockTermScore = -10000

const (
	blockTermNamePrefix  = "Block: "
	trashDefaultScoreSet = "default"
)

var (
	ErrInvalidProfile  = errors.New("acquisition: invalid quality profile")
	ErrNameTaken       = errors.New("acquisition: name already in use")
	ErrDefaultProfile  = errors.New("acquisition: the default profile can't be deleted")
	ErrInvalidSettings = errors.New("acquisition: invalid settings")
)

var propersModes = []api.PropersMode{api.PropersPreferAndUpgrade, api.PropersDoNotUpgrade, api.PropersDoNotPrefer}

// QualityService owns quality profiles, custom formats, the first-run presets and the quality settings.
// A change that can alter file scores queues a rescore of the files it touches.
type QualityService struct {
	db    *db.DB
	repo  *QualityRepository
	store *settings.Store
	jobs  jobs.Enqueuer
	now   func() time.Time
}

func NewQualityService(d *db.DB, repo *QualityRepository, store *settings.Store, q jobs.Enqueuer) *QualityService {
	return &QualityService{db: d, repo: repo, store: store, jobs: q, now: time.Now}
}

// rescoreEveryFile follows a custom format change, which can change scores under every profile.
var rescoreEveryFile = rescoreTask{}

// Qualities is Radarr's quality catalogue, which Magpie uses for movies and TV.
func (s *QualityService) Qualities() []domain.Quality {
	catalogue := parse.Qualities()
	qualities := make([]domain.Quality, 0, len(catalogue))
	for _, q := range catalogue {
		qualities = append(qualities, q.Quality(0))
	}
	return qualities
}

func (s *QualityService) Profiles(ctx context.Context) ([]api.ProfileSummary, error) {
	return s.repo.Profiles(ctx)
}

func (s *QualityService) Profile(ctx context.Context, id domain.ProfileID) (api.Profile, error) {
	return s.repo.Profile(ctx, id)
}

// ScoringProfile loads a profile with every custom format compiled, ready for Evaluate.
func (s *QualityService) ScoringProfile(ctx context.Context, id domain.ProfileID) (ScoringProfile, error) {
	return s.repo.ScoringProfile(ctx, id)
}

func (s *QualityService) CreateProfile(ctx context.Context, p api.Profile) (api.Profile, error) {
	p.ID = domain.NewID[domain.ProfileID]()
	return s.writeProfile(ctx, p)
}

// UpdateProfile replaces a profile; its preset and TRaSH link stay as stored. A format score that
// differs from the format's TRaSH score is a user override, which TRaSH sync leaves alone; setting
// it back to TRaSH's value clears that.
func (s *QualityService) UpdateProfile(ctx context.Context, p api.Profile) (api.Profile, error) {
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		stored, err := loadProfile(tx, p.ID)
		if err != nil {
			return err
		}
		p.Preset, p.TrashProfileID, p.TrashScoreSet = stored.Preset, stored.TrashProfileID, stored.TrashScoreSet
		if err := s.saveValidProfile(tx, &p); err != nil {
			return err
		}
		return s.jobs.EnqueueTx(tx, rescoreTask{Profile: p.ID})
	})
	if err != nil {
		return api.Profile{}, err
	}
	return s.repo.Profile(ctx, p.ID)
}

func (s *QualityService) writeProfile(ctx context.Context, p api.Profile) (api.Profile, error) {
	if err := s.db.Write(ctx, func(tx *gorm.DB) error { return s.saveValidProfile(tx, &p) }); err != nil {
		return api.Profile{}, err
	}
	return s.repo.Profile(ctx, p.ID)
}

func (s *QualityService) saveValidProfile(tx *gorm.DB, p *api.Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	if err := validateProfile(*p); err != nil {
		return err
	}
	taken, err := profileNameTaken(tx, p.Name, p.ID)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("%w: profile %q", ErrNameTaken, p.Name)
	}
	formats, err := loadFormats(tx)
	if err != nil {
		return err
	}
	if err := applyOverrides(p, formatsByID(formats)); err != nil {
		return err
	}
	return saveProfile(tx, *p, s.now())
}

func validateProfile(p api.Profile) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidProfile, fmt.Sprintf(format, args...))
	}
	if p.Name == "" {
		return invalid("name is empty")
	}
	if len(p.Items) == 0 {
		return invalid("no qualities")
	}
	if !slices.Contains(propersModes, p.PropersMode) {
		return invalid("unknown propers mode %q", p.PropersMode)
	}
	seen := map[int]bool{}
	for i, item := range p.Items {
		if item.Group == nil && len(item.Qualities) != 1 {
			return invalid("item %d: a single quality item holds exactly one quality", i)
		}
		if item.Group != nil && (strings.TrimSpace(item.Group.Name) == "" || len(item.Qualities) == 0) {
			return invalid("item %d: a group needs a name and at least one quality", i)
		}
		for _, q := range item.Qualities {
			if _, ok := parse.QualityByID(parse.QualityID(q.ID)); !ok {
				return invalid("item %d: unknown quality %d", i, q.ID)
			}
			if seen[q.ID] {
				return invalid("quality %d is listed twice", q.ID)
			}
			seen[q.ID] = true
		}
		sizes := item.Sizes
		if sizes.MinMBPerMin < 0 || sizes.MinMBPerMin > sizes.PreferredMBPerMin || sizes.PreferredMBPerMin > sizes.MaxMBPerMin {
			return invalid("item %d: sizes must be 0 ≤ min ≤ preferred ≤ max", i)
		}
	}
	if p.Cutoff < 0 || p.Cutoff >= len(p.Items) || !p.Items[p.Cutoff].Allowed {
		return invalid("the cutoff must be an allowed item")
	}
	return nil
}

func formatsByID(formats []CustomFormat) map[api.CustomFormatID]CustomFormat {
	byID := make(map[api.CustomFormatID]CustomFormat, len(formats))
	for _, f := range formats {
		byID[f.ID] = f
	}
	return byID
}

// applyOverrides checks the scored formats exist and marks the scores that differ from TRaSH's.
func applyOverrides(p *api.Profile, formats map[api.CustomFormatID]CustomFormat) error {
	seen := map[api.CustomFormatID]bool{}
	for i, score := range p.FormatScores {
		f, ok := formats[score.Format]
		if !ok {
			return fmt.Errorf("%w: unknown custom format %q", ErrInvalidProfile, score.Format)
		}
		if seen[score.Format] {
			return fmt.Errorf("%w: format %q is scored twice", ErrInvalidProfile, f.Name)
		}
		seen[score.Format] = true
		trash, linked := trashScore(f, p.TrashScoreSet)
		p.FormatScores[i].Name = f.Name
		p.FormatScores[i].UserOverride = linked && score.Score != trash
	}
	return nil
}

// trashScore is a format's TRaSH score in a score set, falling back to TRaSH's default set.
func trashScore(f CustomFormat, scoreSet string) (int, bool) {
	if score, ok := f.TrashScores[scoreSet]; ok && scoreSet != "" {
		return score, true
	}
	score, ok := f.TrashScores[trashDefaultScoreSet]
	return score, ok
}

func (s *QualityService) DeleteProfile(ctx context.Context, id domain.ProfileID) error {
	defaultProfile, err := DefaultProfileKey.Get(ctx, s.store)
	if err != nil {
		return err
	}
	if id == defaultProfile {
		return ErrDefaultProfile
	}
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := deleteProfile(tx, id); err != nil {
			return err
		}
		return s.jobs.EnqueueTx(tx, rescoreTask{Profile: id})
	})
}

// CreateFromPreset builds a resolution preset and attaches the TRaSH scores of its formats that
// have been synced so far. TRaSH sync attaches the rest later.
func (s *QualityService) CreateFromPreset(ctx context.Context, preset api.QualityPreset) (api.Profile, error) {
	var id domain.ProfileID
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		p, err := s.createPreset(tx, preset)
		id = p.ID
		return err
	})
	if err != nil {
		return api.Profile{}, err
	}
	return s.repo.Profile(ctx, id)
}

func (s *QualityService) createPreset(tx *gorm.DB, preset api.QualityPreset) (api.Profile, error) {
	if !knownPreset(preset) {
		return api.Profile{}, fmt.Errorf("%w: unknown preset %q", ErrInvalidProfile, preset)
	}
	p := PresetProfile(preset)
	p.ID = domain.NewID[domain.ProfileID]()
	formats, err := loadFormats(tx)
	if err != nil {
		return api.Profile{}, err
	}
	p.FormatScores = presetScores(preset, formats, p.TrashScoreSet)
	return p, s.saveValidProfile(tx, &p)
}

// presetScores scores every synced format the preset names, at TRaSH's score.
func presetScores(preset api.QualityPreset, formats []CustomFormat, scoreSet string) []api.FormatScore {
	wanted := map[string]bool{}
	for _, name := range presetFormatNames(preset) {
		wanted[trashNameKey(name)] = true
	}
	scores := []api.FormatScore{}
	for _, f := range formats {
		if f.TrashID == "" || !wanted[trashNameKey(f.Name)] {
			continue
		}
		if score, ok := trashScore(f, scoreSet); ok {
			scores = append(scores, api.FormatScore{Format: f.ID, Score: score})
		}
	}
	return scores
}

// SeedPresets creates the resolution presets on the first start and makes 1080p the default.
// Later starts do nothing, so presets the user deleted stay deleted.
func (s *QualityService) SeedPresets(ctx context.Context) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		seeded, err := presetsSeededKey.GetTx(tx)
		if err != nil || seeded {
			return err
		}
		for _, preset := range FirstRunPresets {
			p, err := s.createPreset(tx, preset)
			if errors.Is(err, ErrNameTaken) {
				continue
			}
			if err != nil {
				return err
			}
			if preset == DefaultPreset {
				if err := DefaultProfileKey.SetTx(tx, p.ID); err != nil {
					return err
				}
			}
		}
		return presetsSeededKey.SetTx(tx, true)
	})
}

func (s *QualityService) Formats(ctx context.Context) ([]CustomFormat, error) {
	return s.repo.Formats(ctx)
}

func (s *QualityService) CreateFormat(ctx context.Context, f CustomFormat) (CustomFormat, error) {
	f.ID = domain.NewID[api.CustomFormatID]()
	return s.writeFormat(ctx, f)
}

// UpdateFormat replaces a format's name and specs. Its TRaSH link and scores stay as stored.
func (s *QualityService) UpdateFormat(ctx context.Context, f CustomFormat) (CustomFormat, error) {
	stored, err := s.repo.Format(ctx, f.ID)
	if err != nil {
		return CustomFormat{}, err
	}
	f.TrashID, f.TrashScores = stored.TrashID, stored.TrashScores
	return s.writeFormat(ctx, f, rescoreEveryFile)
}

// writeFormat saves a valid format and queues followUps in the same transaction.
func (s *QualityService) writeFormat(ctx context.Context, f CustomFormat, followUps ...backlite.Task) (CustomFormat, error) {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return CustomFormat{}, fmt.Errorf("%w: name is empty", ErrInvalidFormat)
	}
	if _, err := CompileFormat(f); err != nil {
		return CustomFormat{}, err
	}
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		taken, err := formatNameTaken(tx, f.Name, f.ID)
		if err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("%w: custom format %q", ErrNameTaken, f.Name)
		}
		if err := saveFormat(tx, f, s.now()); err != nil {
			return err
		}
		for _, task := range followUps {
			if err := s.jobs.EnqueueTx(tx, task); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return CustomFormat{}, err
	}
	return s.repo.Format(ctx, f.ID)
}

func (s *QualityService) DeleteFormat(ctx context.Context, id api.CustomFormatID) error {
	return s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := deleteFormat(tx, id); err != nil {
			return err
		}
		return s.jobs.EnqueueTx(tx, rescoreEveryFile)
	})
}

// BlockTerm creates a format matching release names that contain term, literally and ignoring case,
// and scores it BlockTermScore in every profile.
func (s *QualityService) BlockTerm(ctx context.Context, term string) (CustomFormat, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return CustomFormat{}, fmt.Errorf("%w: the term is empty", ErrInvalidFormat)
	}
	f := CustomFormat{
		ID:    domain.NewID[api.CustomFormatID](),
		Name:  blockTermNamePrefix + term,
		Specs: []api.FormatSpec{{Kind: api.SpecReleaseTitle, Name: term, Value: regexp2.Escape(term)}},
	}
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		taken, err := formatNameTaken(tx, f.Name, f.ID)
		if err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("%w: %q is already blocked", ErrNameTaken, term)
		}
		if err := saveFormat(tx, f, s.now()); err != nil {
			return err
		}
		ids, err := profileIDs(tx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := upsertFormatScore(tx, formatScoreRow{ProfileID: id, FormatID: f.ID, Score: BlockTermScore}); err != nil {
				return err
			}
		}
		return s.jobs.EnqueueTx(tx, rescoreEveryFile)
	})
	if err != nil {
		return CustomFormat{}, err
	}
	return s.repo.Format(ctx, f.ID)
}

func (s *QualityService) Settings(ctx context.Context) (QualitySettings, error) {
	var cfg QualitySettings
	var err error
	var delay, settle settings.Duration
	if cfg.DefaultProfile, err = DefaultProfileKey.Get(ctx, s.store); err != nil {
		return cfg, err
	}
	if delay, err = DefaultDelayKey.Get(ctx, s.store); err != nil {
		return cfg, err
	}
	if settle, err = SettleWindowKey.Get(ctx, s.store); err != nil {
		return cfg, err
	}
	if cfg.TrashSyncEnabled, err = TrashSyncEnabledKey.Get(ctx, s.store); err != nil {
		return cfg, err
	}
	cfg.DefaultDelay, cfg.SettleWindow = delay.Std(), settle.Std()
	return cfg, nil
}

func (s *QualityService) UpdateSettings(ctx context.Context, cfg QualitySettings) (QualitySettings, error) {
	if cfg.DefaultDelay < MinDefaultDelay || cfg.DefaultDelay > MaxDefaultDelay {
		return QualitySettings{}, fmt.Errorf("%w: the delay must be 0-%v", ErrInvalidSettings, MaxDefaultDelay)
	}
	if cfg.SettleWindow < MinSettleWindow || cfg.SettleWindow > MaxSettleWindow {
		return QualitySettings{}, fmt.Errorf("%w: the settle window must be %v-%v", ErrInvalidSettings, MinSettleWindow, MaxSettleWindow)
	}
	err := s.db.Write(ctx, func(tx *gorm.DB) error {
		found, err := profileExists(tx, cfg.DefaultProfile)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: default profile %q doesn't exist", ErrInvalidSettings, cfg.DefaultProfile)
		}
		return errors.Join(
			DefaultProfileKey.SetTx(tx, cfg.DefaultProfile),
			DefaultDelayKey.SetTx(tx, settings.Duration(cfg.DefaultDelay)),
			SettleWindowKey.SetTx(tx, settings.Duration(cfg.SettleWindow)),
			TrashSyncEnabledKey.SetTx(tx, cfg.TrashSyncEnabled),
		)
	})
	if err != nil {
		return QualitySettings{}, err
	}
	return s.Settings(ctx)
}
