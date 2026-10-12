package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"magpie/internal/library"
	"magpie/internal/platform/settings"
)

const (
	pathMediaUpdated = "/Library/Media/Updated"

	// Jellyfin rescans whatever lies under a reported path, whatever the update type says.
	updateModified = "Modified"

	// requestTimeout bounds one HTTP call.
	requestTimeout = 30 * time.Second
)

var errNoLibrary = fmt.Errorf("%w: no Jellyfin connection is set up", library.ErrJellyfinUnavailable)

type mediaUpdate struct {
	Path       string `json:"Path"`
	UpdateType string `json:"UpdateType"`
}

type mediaUpdates struct {
	Updates []mediaUpdate `json:"Updates"`
}

// Library tells Jellyfin about new files.
type Library struct {
	store  *settings.Store
	client *http.Client
}

var _ library.JellyfinLibrary = (*Library)(nil)

func NewLibrary(store *settings.Store) *Library {
	return &Library{store: store, client: &http.Client{Timeout: requestTimeout}}
}

func (l *Library) RefreshPath(ctx context.Context, path string) error {
	conn, err := settings.Jellyfin.Get(ctx, l.store)
	if err != nil {
		return err
	}
	target, err := l.target(conn, pathMediaUpdated)
	if err != nil {
		return err
	}
	body, err := json.Marshal(mediaUpdates{Updates: []mediaUpdate{{Path: path, UpdateType: updateModified}}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set(authorizationHeader, fmt.Sprintf(tokenAuthorization, conn.APIKey))
	req.Header.Set(contentTypeHeader, jsonContent)
	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", library.ErrJellyfinUnavailable, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: POST %s answered %s", library.ErrJellyfinUnavailable, pathMediaUpdated, resp.Status)
	}
	return nil
}

func (l *Library) target(conn settings.JellyfinConnection, path string) (string, error) {
	if conn.URL == "" || conn.APIKey == "" {
		return "", errNoLibrary
	}
	target, err := url.Parse(conn.URL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid Jellyfin URL: %w", library.ErrJellyfinUnavailable, err)
	}
	return target.JoinPath(path).String(), nil
}
