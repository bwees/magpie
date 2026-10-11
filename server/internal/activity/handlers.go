package activity

import (
	"slices"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"
	subtitlesapi "magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"
)

const (
	handlerTimeline  = "activity.timeline"
	handlerHistory   = "activity.history"
	handlerPipeline  = "activity.pipeline"
	handlerAttention = "activity.attention"
)

// DurableEventTypes are the events Activity records: every durable event other contexts and the platform
// publish. Commands have one owner, and Activity's own events only restate what it recorded.
func DurableEventTypes() []string {
	var types []string
	for _, m := range slices.Concat(
		platformapi.Messages(),
		identityapi.Messages(),
		catalogapi.Messages(),
		requestsapi.Messages(),
		libraryapi.Messages(),
		acquisitionapi.Messages(),
		subtitlesapi.Messages(),
		transcodeapi.Messages(),
	) {
		if m.Durable() && !isCommand(m) {
			types = append(types, m.Type())
		}
	}
	return types
}

func isCommand(m domain.Message) bool {
	return m.Type() == transcodeapi.TypeRunAgentJob
}

func NewTimelineHandler(s *Service) bus.Handler {
	return bus.Handler{Name: handlerTimeline, EventTypes: DurableEventTypes(), Handle: s.RecordTimeline}
}

func NewHistoryHandler(s *Service) bus.Handler {
	return bus.Handler{Name: handlerHistory, EventTypes: DurableEventTypes(), Handle: s.RecordHistory}
}

func NewPipelineHandler(s *Service) bus.Handler {
	return bus.Handler{Name: handlerPipeline, EventTypes: DurableEventTypes(), Handle: s.UpdatePipeline}
}

func NewAttentionHandler(s *Service) bus.Handler {
	return bus.Handler{Name: handlerAttention, EventTypes: DurableEventTypes(), Handle: s.UpdateAttention}
}
