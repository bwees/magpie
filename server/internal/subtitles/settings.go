package subtitles

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
	transcodeapi "magpie/internal/transcode/api"
)

const day = 24 * time.Hour

// Defaults and ranges from 12-settings › Subtitles and › Schedules.
const (
	DefaultMinScoreEpisode = 90
	DefaultMinScoreMovie   = 70
	MinScoreFloor          = 50
	MinScoreCeiling        = 100

	DefaultUpgradeWindowDays = 7
	MaxUpgradeWindowDays     = 30

	DefaultUpgradeCheckInterval = 6 * time.Hour
	MinUpgradeCheckInterval     = time.Hour
	MaxUpgradeCheckInterval     = day
)

// WhisperModels are the models the Whisper setting accepts, as whisper.cpp names them.
var WhisperModels = []string{
	transcodeapi.WhisperModelLargeV3Turbo,
	transcodeapi.WhisperModelLargeV3,
	transcodeapi.WhisperModelMedium,
	transcodeapi.WhisperModelSmall,
}

var ErrInvalidSettings = errors.New("subtitles: invalid settings")

// Settings are Settings → Subtitles. The Whisper fields are stored for the Whisper fallback.
type Settings struct {
	WhisperEnabled       bool              `json:"whisperEnabled"`
	WhisperModel         string            `json:"whisperModel"`
	AutoSync             bool              `json:"autoSync"`
	MinScoreEpisode      int               `json:"minScoreEpisode"`
	MinScoreMovie        int               `json:"minScoreMovie"`
	UpgradeWindowDays    int               `json:"upgradeWindowDays"`
	UpgradeCheckInterval settings.Duration `json:"upgradeCheckInterval"`
}

var SettingsKey = settings.NewKey(owner.Subtitles, "settings", Settings{
	WhisperEnabled:       true,
	WhisperModel:         transcodeapi.WhisperModelLargeV3Turbo,
	AutoSync:             true,
	MinScoreEpisode:      DefaultMinScoreEpisode,
	MinScoreMovie:        DefaultMinScoreMovie,
	UpgradeWindowDays:    DefaultUpgradeWindowDays,
	UpgradeCheckInterval: settings.Duration(DefaultUpgradeCheckInterval),
})

func (s Settings) Validate() error {
	var errs []error
	if !slices.Contains(WhisperModels, s.WhisperModel) {
		errs = append(errs, fmt.Errorf("the Whisper model %q is not one of %v", s.WhisperModel, WhisperModels))
	}
	if !validMinScore(s.MinScoreEpisode) {
		errs = append(errs, fmt.Errorf("the minimum score for episodes must be %d-%d%%", MinScoreFloor, MinScoreCeiling))
	}
	if !validMinScore(s.MinScoreMovie) {
		errs = append(errs, fmt.Errorf("the minimum score for movies must be %d-%d%%", MinScoreFloor, MinScoreCeiling))
	}
	if s.UpgradeWindowDays < 0 || s.UpgradeWindowDays > MaxUpgradeWindowDays {
		errs = append(errs, fmt.Errorf("the upgrade window must be 0-%d days", MaxUpgradeWindowDays))
	}
	if interval := s.UpgradeCheckInterval.Std(); interval < MinUpgradeCheckInterval || interval > MaxUpgradeCheckInterval {
		errs = append(errs, fmt.Errorf("the upgrade check must run every %s-%s", MinUpgradeCheckInterval, MaxUpgradeCheckInterval))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidSettings, errors.Join(errs...))
	}
	return nil
}

func validMinScore(percent int) bool {
	return percent >= MinScoreFloor && percent <= MinScoreCeiling
}

// UpgradeWindow is how long a provider subtitle that isn't a near-perfect match keeps being re-searched.
func (s Settings) UpgradeWindow() time.Duration {
	return time.Duration(s.UpgradeWindowDays) * day
}
