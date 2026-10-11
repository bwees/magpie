package activity

import (
	"slices"

	"magpie/internal/activity/api"
)

// KindScope says which role may subscribe to a notification kind.
type KindScope string

const (
	ScopeRequester KindScope = "REQUESTER"
	ScopeAdmin     KindScope = "ADMIN"
)

// KindSpec describes one notification kind. Opt-in kinds go only to URLs that subscribe to them, never in-app.
type KindSpec struct {
	Kind  string
	Label string
	Scope KindScope
	OptIn bool
}

var kindSpecs = []KindSpec{
	{Kind: api.KindRequestApproved, Label: "Request approved", Scope: ScopeRequester},
	{Kind: api.KindRequestDeclined, Label: "Request declined", Scope: ScopeRequester},
	{Kind: api.KindRequestAvailable, Label: "Request available", Scope: ScopeRequester},
	{Kind: api.KindIssueReply, Label: "Issue reply", Scope: ScopeRequester},
	{Kind: api.KindIssueResolved, Label: "Issue resolved", Scope: ScopeRequester},
	{Kind: api.KindRequestPending, Label: "Request waiting for approval", Scope: ScopeAdmin},
	{Kind: api.KindIssueReported, Label: "Issue reported", Scope: ScopeAdmin},
	{Kind: api.KindImportFailed, Label: "Import failed", Scope: ScopeAdmin},
	{Kind: api.KindDownloadFailed, Label: "Download failed", Scope: ScopeAdmin},
	{Kind: api.KindTranscodeFailed, Label: "Transcode failed", Scope: ScopeAdmin},
	{Kind: api.KindServiceUnreachable, Label: "Service unreachable", Scope: ScopeAdmin},
	{Kind: api.KindAgentOffline, Label: "Agent offline", Scope: ScopeAdmin},
	{Kind: api.KindGrabbed, Label: "Release grabbed", Scope: ScopeAdmin, OptIn: true},
	{Kind: api.KindImported, Label: "File imported", Scope: ScopeAdmin, OptIn: true},
	{Kind: api.KindTranscoded, Label: "File transcoded", Scope: ScopeAdmin, OptIn: true},
	{Kind: api.KindBlockedFileRejected, Label: "Release rejected for a blocked file", Scope: ScopeAdmin, OptIn: true},
}

// KindsFor lists the kinds a role may subscribe its URLs to.
func KindsFor(admin bool) []KindSpec {
	if admin {
		return slices.Clone(kindSpecs)
	}
	return slices.DeleteFunc(slices.Clone(kindSpecs), func(k KindSpec) bool { return k.Scope != ScopeRequester })
}

func kindSpecOf(kind string) (KindSpec, bool) {
	i := slices.IndexFunc(kindSpecs, func(k KindSpec) bool { return k.Kind == kind })
	if i < 0 {
		return KindSpec{}, false
	}
	return kindSpecs[i], true
}

// mayHold reports whether a user of this role may subscribe to kind.
func mayHold(admin bool, kind string) bool {
	spec, ok := kindSpecOf(kind)
	return ok && (admin || spec.Scope == ScopeRequester)
}
