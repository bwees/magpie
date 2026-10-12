package acquisition

import (
	"context"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

const (
	handlerSearchOnTitleAdded   = "acquisition.search-on-title-added"
	handlerSearchOnMonitoring   = "acquisition.search-on-monitoring-changed"
	handlerSearchOnReleased     = "acquisition.search-on-wanted-released"
	handlerForgetOnTitleRemoved = "acquisition.forget-on-title-removed"
)

// handle decodes an event's payload for one of the search service's reactions.
func handle[T any](name, eventType string, react func(context.Context, *gorm.DB, T) error) bus.Handler {
	return bus.Handler{
		Name:       name,
		EventTypes: []string{eventType},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			payload, err := bus.Decode[T](ev)
			if err != nil {
				return err
			}
			return react(ctx, tx, payload)
		},
	}
}

func newTitleAddedHandler(s *SearchService) bus.Handler {
	return handle(handlerSearchOnTitleAdded, catalogapi.TypeTitleAdded, s.SearchAdded)
}

func newMonitoringChangedHandler(s *SearchService) bus.Handler {
	return handle(handlerSearchOnMonitoring, catalogapi.TypeMonitoringChanged, s.SearchMonitored)
}

func newWantedReleasedHandler(s *SearchService) bus.Handler {
	return handle(handlerSearchOnReleased, catalogapi.TypeWantedReleased, s.SearchReleased)
}

func newTitleRemovedHandler(s *SearchService) bus.Handler {
	return handle(handlerForgetOnTitleRemoved, catalogapi.TypeTitleRemoved, s.ForgetTitle)
}
