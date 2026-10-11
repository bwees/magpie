package prowlarr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"magpie/internal/acquisition"
)

const (
	pathIndexers      = "api/v1/indexer"
	pathIndexerStatus = "api/v1/indexerstatus"
	pathIndexerStats  = "api/v1/indexerstats"

	protocolTorrent = "torrent"

	disabledTimeLayout = "2006-01-02 15:04 MST"
)

var privacies = map[string]acquisition.IndexerPrivacy{
	"public":      acquisition.PrivacyPublic,
	"semiPrivate": acquisition.PrivacySemiPrivate,
	"private":     acquisition.PrivacyPrivate,
}

type indexerResource struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Protocol string `json:"protocol"`
	Privacy  string `json:"privacy"`
	Priority int    `json:"priority"`
}

// indexerStatusResource exists only for indexers Prowlarr has seen fail.
type indexerStatusResource struct {
	IndexerID         int        `json:"indexerId"`
	DisabledTill      *time.Time `json:"disabledTill"`
	MostRecentFailure *time.Time `json:"mostRecentFailure"`
	InitialFailure    *time.Time `json:"initialFailure"`
}

type indexerStatsResource struct {
	Indexers []struct {
		IndexerID           int   `json:"indexerId"`
		AverageResponseTime int64 `json:"averageResponseTime"` // milliseconds
	} `json:"indexers"`
}

// Indexers reads Prowlarr's indexer list, failure status and response stats, then each indexer's caps.
// Disabled and usenet indexers are left out: Magpie only downloads torrents.
func (c *Client) Indexers(ctx context.Context) ([]acquisition.ProwlarrIndexer, error) {
	var resources []indexerResource
	if err := c.getJSON(ctx, pathIndexers, &resources); err != nil {
		return nil, err
	}
	var statuses []indexerStatusResource
	if err := c.getJSON(ctx, pathIndexerStatus, &statuses); err != nil {
		return nil, err
	}
	var stats indexerStatsResource
	if err := c.getJSON(ctx, pathIndexerStats, &stats); err != nil {
		return nil, err
	}

	statusByID := make(map[int]indexerStatusResource, len(statuses))
	for _, s := range statuses {
		statusByID[s.IndexerID] = s
	}
	responseByID := make(map[int]time.Duration, len(stats.Indexers))
	for _, s := range stats.Indexers {
		responseByID[s.IndexerID] = time.Duration(s.AverageResponseTime) * time.Millisecond
	}

	indexers := make([]acquisition.ProwlarrIndexer, 0, len(resources))
	for _, r := range resources {
		if !r.Enable || r.Protocol != protocolTorrent {
			continue
		}
		ix := acquisition.ProwlarrIndexer{
			ID:       r.ID,
			Name:     r.Name,
			Privacy:  privacy(r.Privacy),
			Priority: r.Priority,
			Health:   c.health(statusByID[r.ID], responseByID[r.ID]),
		}
		caps, err := c.caps(ctx, r.ID)
		var indexerErr *acquisition.ErrIndexerFailed
		switch {
		case err == nil:
			ix.Caps = &caps
		case errors.As(err, &indexerErr):
			ix.Health.Error = indexerErr.Message
		default:
			return nil, fmt.Errorf("caps of indexer %d: %w", r.ID, err)
		}
		indexers = append(indexers, ix)
	}
	return indexers, nil
}

func (c *Client) health(status indexerStatusResource, averageResponse time.Duration) acquisition.ProwlarrHealth {
	h := acquisition.ProwlarrHealth{AverageResponse: averageResponse}
	if status.DisabledTill == nil || !status.DisabledTill.After(c.now()) {
		return h
	}
	h.Error = "Prowlarr disabled this indexer until " + status.DisabledTill.UTC().Format(disabledTimeLayout) + " after repeated failures"
	h.FailingSince = status.InitialFailure
	return h
}

func privacy(prowlarr string) acquisition.IndexerPrivacy {
	if p, ok := privacies[prowlarr]; ok {
		return p
	}
	return acquisition.PrivacyPrivate
}
