// Package api holds the events the platform publishes to contexts.
// Spec: docs/spec/01-architecture.md#external-systems and the event catalog in docs/spec/00-context-map.md.
package api

import "magpie/internal/domain"

const TypeServiceHealthChanged = "platform.ServiceHealthChanged"

// Service names one external connection the platform health check probes.
type Service string

const (
	ServiceTMDB          Service = "TMDB"
	ServiceQBittorrent   Service = "QBITTORRENT"
	ServiceProwlarr      Service = "PROWLARR"
	ServiceJellyfin      Service = "JELLYFIN"
	ServiceOpenSubtitles Service = "OPENSUBTITLES"
)

// ServiceHealthChanged is durable. Published when a connection's health check flips.
type ServiceHealthChanged struct {
	Service Service `json:"service"`
	Healthy bool    `json:"healthy"`
	Error   string  `json:"error"` // empty when healthy
}

func (ServiceHealthChanged) Type() string          { return TypeServiceHealthChanged }
func (e ServiceHealthChanged) AggregateID() string { return string(e.Service) }
func (ServiceHealthChanged) Durable() bool         { return true }

// Messages lists every event this package declares, for the bus registry and the live schemas.
func Messages() []domain.Message {
	return []domain.Message{ServiceHealthChanged{}}
}
