package acquisition

import (
	"context"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

const (
	DefaultSettleWindow = 30 * time.Second
	MinSettleWindow     = 5 * time.Second
	MaxSettleWindow     = 120 * time.Second
	MinDefaultDelay     = 0
	MaxDefaultDelay     = 24 * time.Hour
)

// Quality settings. Search, RSS and delay slices read DefaultDelayKey and SettleWindowKey.
var (
	DefaultProfileKey   = settings.NewKey(owner.Acquisition, "defaultProfile", domain.ProfileID(""))
	DefaultDelayKey     = settings.NewKey(owner.Acquisition, "defaultDelay", settings.Duration(MinDefaultDelay))
	SettleWindowKey     = settings.NewKey(owner.Acquisition, "settleWindow", settings.Duration(DefaultSettleWindow))
	TrashSyncEnabledKey = settings.NewKey(owner.Acquisition, "trashSyncEnabled", true)
	UpgradesEnabledKey  = settings.NewKey(owner.Acquisition, "upgradesEnabled", true)

	trashSyncStateKey = settings.NewKey(owner.Acquisition, "trashSyncState", TrashSyncState{})
	presetsSeededKey  = settings.NewKey(owner.Acquisition, "presetsSeeded", false)
)

// QualitySettings are the Settings → Quality values besides profiles and formats.
type QualitySettings struct {
	DefaultProfile   domain.ProfileID
	DefaultDelay     time.Duration
	SettleWindow     time.Duration
	TrashSyncEnabled bool
	// UpgradesEnabled off stops every automatic upgrade, except for titles that allow them.
	UpgradesEnabled bool
}

// titleUpgradesEnabled reports whether automatic upgrades may replace the title's files.
func titleUpgradesEnabled(ctx context.Context, store *settings.Store, title catalogapi.Title) (bool, error) {
	if title.AllowUpgrades {
		return true, nil
	}
	return UpgradesEnabledKey.Get(ctx, store)
}

// TrashSyncState records the last TRaSH sync attempt.
type TrashSyncState struct {
	LastAttemptAt *time.Time `json:"lastAttemptAt,omitempty"`
	LastSyncedAt  *time.Time `json:"lastSyncedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}
