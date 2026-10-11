package identity

import (
	"magpie/internal/identity/api"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"
)

// SignInSettings controls who may sign in. The local admin can always sign in.
type SignInSettings struct {
	AllowJellyfin bool `json:"allowJellyfin"`
}

var (
	defaultsKey       = settings.NewKey(owner.Identity, "defaults", api.Permissions{ReportIssues: true})
	signInSettingsKey = settings.NewKey(owner.Identity, "signIn", SignInSettings{AllowJellyfin: true})
)
