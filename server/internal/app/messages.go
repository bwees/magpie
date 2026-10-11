package app

import (
	"slices"

	acquisitionapi "magpie/internal/acquisition/api"
	activityapi "magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/httpserver"
	requestsapi "magpie/internal/requests/api"
	subtitlesapi "magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"
)

// Messages lists every event and command the platform and the contexts publish: the event catalog in code.
func Messages() []domain.Message {
	return slices.Concat(
		platformapi.Messages(),
		identityapi.Messages(),
		catalogapi.Messages(),
		requestsapi.Messages(),
		libraryapi.Messages(),
		acquisitionapi.Messages(),
		subtitlesapi.Messages(),
		transcodeapi.Messages(),
		activityapi.Messages(),
	)
}

// messageSchemas puts every message payload in the OpenAPI components, so the web client gets live-event types.
func messageSchemas() httpserver.Schemas {
	schemas := httpserver.Schemas{}
	for _, m := range Messages() {
		schemas = append(schemas, m)
	}
	return schemas
}
