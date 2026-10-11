package activity

import (
	"magpie/internal/activity/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"
	transcodeapi "magpie/internal/transcode/api"
)

// attentionChange opens or clears the item of (Kind, Ref). File finds the title of a file-keyed item.
type attentionChange struct {
	Kind   string
	Ref    string
	Open   bool
	Title  *domain.TitleID
	File   domain.MediaFileID
	Text   string
	Action string
}

var attentionChanges = map[string]func(bus.Event) ([]attentionChange, error){}

// onAttention registers which attention items events of one payload type open or clear.
func onAttention[T domain.Message](fn func(T) []attentionChange) {
	var zero T
	attentionChanges[zero.Type()] = func(ev bus.Event) ([]attentionChange, error) {
		payload, err := bus.Decode[T](ev)
		if err != nil {
			return nil, err
		}
		return fn(payload), nil
	}
}

func clearItem(kind, ref string) []attentionChange {
	return []attentionChange{{Kind: kind, Ref: ref}}
}

func init() {
	onAttention(func(e requestsapi.RequestCreated) []attentionChange {
		return []attentionChange{{
			Kind: api.AttentionRequestPending, Ref: string(e.Request), Open: true,
			Text: capitalize(humanize(e.Kind)) + " request waiting for approval", Action: api.ActionApprove,
		}}
	})
	onAttention(func(e requestsapi.RequestApproved) []attentionChange {
		return clearItem(api.AttentionRequestPending, string(e.Request))
	})
	onAttention(func(e requestsapi.RequestDeclined) []attentionChange {
		return clearItem(api.AttentionRequestPending, string(e.Request))
	})
	onAttention(func(e requestsapi.IssueReported) []attentionChange {
		return []attentionChange{{
			Kind: api.AttentionIssueOpen, Ref: string(e.Issue), Open: true, Title: &e.Title,
			Text: capitalize(humanize(e.Kind) + " issue reported"), Action: api.ActionRegrab,
		}}
	})
	onAttention(func(e requestsapi.IssueResolved) []attentionChange {
		return clearItem(api.AttentionIssueOpen, string(e.Issue))
	})

	onAttention(func(e transcodeapi.JobFailed) []attentionChange {
		if e.Kind != transcodeapi.JobKindTranscode || !e.Final {
			return nil
		}
		return []attentionChange{{
			Kind: api.AttentionTranscodeFailed, Ref: string(e.MediaFile), Open: true, File: e.MediaFile,
			Text: "Transcode failed: " + e.Reason, Action: api.ActionRetry,
		}}
	})
	onAttention(func(e transcodeapi.JobQueued) []attentionChange {
		return clearItem(api.AttentionTranscodeFailed, string(e.MediaFile))
	})
	onAttention(func(e libraryapi.FileTranscoded) []attentionChange {
		return clearItem(api.AttentionTranscodeFailed, string(e.MediaFile))
	})
	onAttention(func(e transcodeapi.AgentOffline) []attentionChange {
		return []attentionChange{{
			Kind: api.AttentionAgentOffline, Ref: string(e.Agent), Open: true,
			Text: "Agent offline", Action: api.ActionViewAgent,
		}}
	})
	onAttention(func(e transcodeapi.AgentOnline) []attentionChange {
		return clearItem(api.AttentionAgentOffline, string(e.Agent))
	})

	onAttention(func(e platformapi.ServiceHealthChanged) []attentionChange {
		if e.Healthy {
			return clearItem(api.AttentionServiceDown, string(e.Service))
		}
		return []attentionChange{{
			Kind: api.AttentionServiceDown, Ref: string(e.Service), Open: true,
			Text: string(e.Service) + " unreachable", Action: api.ActionOpenSettings,
		}}
	})
}

// pipelineAttention is the PIPELINE_FAILED change for a title, given its first failed row (nil if none).
func pipelineAttention(title domain.TitleID, failed *pipelineRow) attentionChange {
	change := attentionChange{Kind: api.AttentionPipelineFailed, Ref: string(title), Title: &title}
	if failed == nil {
		return change
	}
	change.Open, change.Text, change.Action = true, failed.Detail, api.ActionOpenTitle
	if failed.Stage == api.StageSearching {
		change.Action = api.ActionSearchAgain
	}
	return change
}
