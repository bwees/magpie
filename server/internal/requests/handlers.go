package requests

import (
	"context"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

const (
	handlerTitleAdded     = "requests.link-on-title-added"
	handlerMonitoring     = "requests.link-on-monitoring-changed"
	handlerEpisodes       = "requests.extend-on-new-episodes"
	handlerTitleAddFailed = "requests.fail-on-title-add-failed"
	handlerItemAvailable  = "requests.mark-available-on-item-available"
)

// newTitleAddedHandler links the request a Title was added for.
func newTitleAddedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerTitleAdded,
		EventTypes: []string{catalogapi.TypeTitleAdded},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			added, err := bus.Decode[catalogapi.TitleAdded](ev)
			if err != nil || added.RequestID == nil {
				return err
			}
			return s.LinkTitle(ctx, tx, *added.RequestID, added.Title)
		},
	}
}

// newMonitoringHandler links a request to a Title that already existed, and otherwise picks up episodes
// an admin started monitoring.
func newMonitoringHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerMonitoring,
		EventTypes: []string{catalogapi.TypeMonitoringChanged},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			changed, err := bus.Decode[catalogapi.MonitoringChanged](ev)
			if err != nil {
				return err
			}
			if changed.RequestID != nil {
				return s.LinkTitle(ctx, tx, *changed.RequestID, changed.Title)
			}
			return s.ExtendItems(ctx, tx, changed.Title)
		},
	}
}

// newEpisodesHandler adds episodes that became wanted to the requests on their Title.
func newEpisodesHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerEpisodes,
		EventTypes: []string{catalogapi.TypeWantedReleased, catalogapi.TypeEpisodesAnnounced},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			title, err := episodesTitle(ev)
			if err != nil {
				return err
			}
			return s.ExtendItems(ctx, tx, title)
		},
	}
}

func episodesTitle(ev bus.Event) (domain.TitleID, error) {
	if ev.Type == catalogapi.TypeWantedReleased {
		released, err := bus.Decode[catalogapi.WantedReleased](ev)
		return released.Title, err
	}
	announced, err := bus.Decode[catalogapi.EpisodesAnnounced](ev)
	return announced.Title, err
}

// newItemAvailableHandler counts a movie or episodes Jellyfin confirmed towards the requests on the Title.
func newItemAvailableHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerItemAvailable,
		EventTypes: []string{libraryapi.TypeItemAvailable},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			available, err := bus.Decode[libraryapi.ItemAvailable](ev)
			if err != nil {
				return err
			}
			return s.MarkAvailable(tx, available.Title, available.Episodes)
		},
	}
}

func newTitleAddFailedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerTitleAddFailed,
		EventTypes: []string{catalogapi.TypeTitleAddFailed},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			failed, err := bus.Decode[catalogapi.TitleAddFailed](ev)
			if err != nil || failed.RequestID == nil {
				return err
			}
			return s.Fail(tx, *failed.RequestID, failed.Reason)
		},
	}
}
