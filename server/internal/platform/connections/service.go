// Package connections stores the URLs and credentials of external services and tests them on demand.
package connections

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/db"
	"magpie/internal/platform/health"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"gorm.io/gorm"
)

// SecretMask stands in for a stored secret in responses. Sending it back keeps the stored value.
const SecretMask = "••••••••"

const urlPathSeparator = "/"

var (
	ErrInvalidConnection = errors.New("invalid connection settings")
	categoryPattern      = regexp.MustCompile(`^[a-z0-9-]+$`)
	urlSchemes           = map[string]bool{"http": true, "https": true}
)

// Connections is every service's stored config.
type Connections struct {
	TMDB          settings.TMDBConnection
	QBittorrent   settings.QBittorrentConnection
	Prowlarr      settings.ProwlarrConnection
	Jellyfin      settings.JellyfinConnection
	OpenSubtitles settings.OpenSubtitlesConnection
}

// Update changes only the services that are set.
type Update struct {
	TMDB          *settings.TMDBConnection
	QBittorrent   *settings.QBittorrentConnection
	Prowlarr      *settings.ProwlarrConnection
	Jellyfin      *settings.JellyfinConnection
	OpenSubtitles *settings.OpenSubtitlesConnection
}

type Service struct {
	db      *db.DB
	store   *settings.Store
	queue   jobs.Enqueuer
	checker *health.Checker
}

func NewService(d *db.DB, store *settings.Store, queue jobs.Enqueuer, checker *health.Checker) *Service {
	return &Service{db: d, store: store, queue: queue, checker: checker}
}

// Get returns the stored config with secrets masked.
func (s *Service) Get(ctx context.Context) (Connections, error) {
	stored, err := s.stored(ctx)
	if err != nil {
		return Connections{}, err
	}
	return masked(stored), nil
}

// Save validates and stores the changed services, then queues a health check for each in the same transaction.
func (s *Service) Save(ctx context.Context, update Update) (Connections, error) {
	stored, err := s.stored(ctx)
	if err != nil {
		return Connections{}, err
	}
	next := stored
	var changed []platformapi.Service

	if update.TMDB != nil {
		next.TMDB = settings.TMDBConnection{APIKey: keepSecret(update.TMDB.APIKey, stored.TMDB.APIKey)}
		changed = append(changed, platformapi.ServiceTMDB)
	}
	if update.QBittorrent != nil {
		qb := *update.QBittorrent
		qb.APIKey = keepSecret(qb.APIKey, stored.QBittorrent.APIKey)
		if qb.Category == "" {
			qb.Category = settings.DefaultQBittorrentCategory
		}
		if !categoryPattern.MatchString(qb.Category) {
			return Connections{}, fmt.Errorf("%w: qBittorrent category may use only a-z, 0-9 and -", ErrInvalidConnection)
		}
		qb.RemotePath, qb.LocalPath = strings.TrimSpace(qb.RemotePath), strings.TrimSpace(qb.LocalPath)
		if (qb.RemotePath == "") != (qb.LocalPath == "") {
			return Connections{}, fmt.Errorf("%w: set both qBittorrent paths, or neither", ErrInvalidConnection)
		}
		if qb.URL, err = normaliseURL(qb.URL); err != nil {
			return Connections{}, fmt.Errorf("%w: qBittorrent %w", ErrInvalidConnection, err)
		}
		next.QBittorrent = qb
		changed = append(changed, platformapi.ServiceQBittorrent)
	}
	if update.Prowlarr != nil {
		prowlarr := settings.ProwlarrConnection{APIKey: keepSecret(update.Prowlarr.APIKey, stored.Prowlarr.APIKey)}
		if prowlarr.URL, err = normaliseURL(update.Prowlarr.URL); err != nil {
			return Connections{}, fmt.Errorf("%w: Prowlarr %w", ErrInvalidConnection, err)
		}
		next.Prowlarr = prowlarr
		changed = append(changed, platformapi.ServiceProwlarr)
	}
	if update.Jellyfin != nil {
		jellyfin := settings.JellyfinConnection{APIKey: keepSecret(update.Jellyfin.APIKey, stored.Jellyfin.APIKey)}
		if jellyfin.URL, err = normaliseURL(update.Jellyfin.URL); err != nil {
			return Connections{}, fmt.Errorf("%w: Jellyfin %w", ErrInvalidConnection, err)
		}
		next.Jellyfin = jellyfin
		changed = append(changed, platformapi.ServiceJellyfin)
	}
	if update.OpenSubtitles != nil {
		next.OpenSubtitles = settings.OpenSubtitlesConnection{
			Username: strings.TrimSpace(update.OpenSubtitles.Username),
			Password: keepSecret(update.OpenSubtitles.Password, stored.OpenSubtitles.Password),
		}
		changed = append(changed, platformapi.ServiceOpenSubtitles)
	}

	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		if err := errors.Join(
			settings.TMDB.SetTx(tx, next.TMDB),
			settings.QBittorrent.SetTx(tx, next.QBittorrent),
			settings.Prowlarr.SetTx(tx, next.Prowlarr),
			settings.Jellyfin.SetTx(tx, next.Jellyfin),
			settings.OpenSubtitles.SetTx(tx, next.OpenSubtitles),
		); err != nil {
			return err
		}
		for _, service := range changed {
			if err := s.queue.EnqueueTx(tx, health.CheckTask{Service: service}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Connections{}, err
	}
	return masked(next), nil
}

// Test probes one service now and records the result like a scheduled check.
func (s *Service) Test(ctx context.Context, service platformapi.Service) (health.Check, error) {
	return s.checker.Check(ctx, service)
}

func (s *Service) Health(ctx context.Context) ([]health.Check, error) {
	return s.checker.Latest(ctx)
}

func (s *Service) stored(ctx context.Context) (Connections, error) {
	tmdb, tmdbErr := settings.TMDB.Get(ctx, s.store)
	qbittorrent, qbittorrentErr := settings.QBittorrent.Get(ctx, s.store)
	prowlarr, prowlarrErr := settings.Prowlarr.Get(ctx, s.store)
	jellyfin, jellyfinErr := settings.Jellyfin.Get(ctx, s.store)
	openSubtitles, openSubtitlesErr := settings.OpenSubtitles.Get(ctx, s.store)
	c := Connections{TMDB: tmdb, QBittorrent: qbittorrent, Prowlarr: prowlarr, Jellyfin: jellyfin, OpenSubtitles: openSubtitles}
	return c, errors.Join(tmdbErr, qbittorrentErr, prowlarrErr, jellyfinErr, openSubtitlesErr)
}

func masked(c Connections) Connections {
	c.TMDB.APIKey = mask(c.TMDB.APIKey)
	c.QBittorrent.APIKey = mask(c.QBittorrent.APIKey)
	c.Prowlarr.APIKey = mask(c.Prowlarr.APIKey)
	c.Jellyfin.APIKey = mask(c.Jellyfin.APIKey)
	c.OpenSubtitles.Password = mask(c.OpenSubtitles.Password)
	return c
}

func mask(secret string) string {
	if secret == "" {
		return ""
	}
	return SecretMask
}

func keepSecret(incoming, stored string) string {
	if incoming == SecretMask {
		return stored
	}
	return strings.TrimSpace(incoming)
}

// normaliseURL accepts an empty URL (not configured) or an absolute http(s) URL, without a trailing slash.
func normaliseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || !urlSchemes[u.Scheme] {
		return "", fmt.Errorf("address %q must be an absolute http or https URL", raw)
	}
	return strings.TrimRight(raw, urlPathSeparator), nil
}
