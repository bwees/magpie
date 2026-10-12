package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"magpie/internal/library"
	"magpie/internal/platform/settings"

	"github.com/coder/websocket"
)

const (
	pathMediaUpdated = "/Library/Media/Updated"
	pathItems        = "/Items"
	pathSocket       = "/socket"

	queryIDs                  = "ids"
	queryFields               = "fields"
	queryIncludeItemTypes     = "includeItemTypes"
	queryExcludeLocationTypes = "excludeLocationTypes"
	queryRecursive            = "recursive"
	queryMinDateLastSaved     = "minDateLastSaved"
	queryStartIndex           = "startIndex"
	queryLimit                = "limit"
	queryEnableImages         = "enableImages"
	queryEnableUserData       = "enableUserData"

	fieldPath           = "Path"
	itemTypeMovie       = "Movie"
	itemTypeEpisode     = "Episode"
	locationTypeVirtual = "Virtual"
	listSeparator       = ","
	queryTrue           = "true"
	queryFalse          = "false"

	// Jellyfin rescans whatever lies under a reported path, whatever the update type says.
	updateModified = "Modified"

	messageForceKeepAlive = "ForceKeepAlive"
	messageKeepAlive      = "KeepAlive"
	messageLibraryChanged = "LibraryChanged"

	// itemsPageSize bounds one /Items answer while reconciling a whole library.
	itemsPageSize = 500
	// idsPerRequest keeps an /Items?ids= URL well under common proxy limits.
	idsPerRequest = 100
	// requestTimeout bounds one HTTP call; the WebSocket only uses it to connect.
	requestTimeout = 30 * time.Second
	// maxItemsBodyBytes fits a full page of items.
	maxItemsBodyBytes = 16 << 20
	// maxSocketMessageBytes fits a LibraryChanged message from a full library scan.
	maxSocketMessageBytes = 4 << 20
	// keepAliveDivisor: Jellyfin wants a KeepAlive at least every half of the timeout it announces.
	keepAliveDivisor = 2
)

var (
	videoItemTypes = strings.Join([]string{itemTypeMovie, itemTypeEpisode}, listSeparator)
	errNoLibrary   = fmt.Errorf("%w: no Jellyfin connection is set up", library.ErrJellyfinUnavailable)
)

type mediaUpdate struct {
	Path       string `json:"Path"`
	UpdateType string `json:"UpdateType"`
}

type mediaUpdates struct {
	Updates []mediaUpdate `json:"Updates"`
}

type baseItem struct {
	ID   string `json:"Id"`
	Path string `json:"Path"`
	Type string `json:"Type"`
}

type itemsResult struct {
	Items            []baseItem `json:"Items"`
	TotalRecordCount int        `json:"TotalRecordCount"`
}

type socketMessage struct {
	MessageType string          `json:"MessageType"`
	Data        json.RawMessage `json:"Data,omitempty"`
}

type libraryUpdateInfo struct {
	ItemsAdded   []string `json:"ItemsAdded"`
	ItemsUpdated []string `json:"ItemsUpdated"`
	ItemsRemoved []string `json:"ItemsRemoved"`
}

// Library tells Jellyfin about new files and reads back what it has, for Library's availability.
type Library struct {
	store    *settings.Store
	client   *http.Client
	pageSize int
}

var _ library.JellyfinLibrary = (*Library)(nil)

func NewLibrary(store *settings.Store) *Library {
	return &Library{store: store, client: &http.Client{Timeout: requestTimeout}, pageSize: itemsPageSize}
}

func (l *Library) RefreshPath(ctx context.Context, path string) error {
	body := mediaUpdates{Updates: []mediaUpdate{{Path: path, UpdateType: updateModified}}}
	return l.call(ctx, http.MethodPost, pathMediaUpdated, nil, body, nil)
}

func (l *Library) Items(ctx context.Context, ids []string) ([]library.JellyfinItem, error) {
	items := []library.JellyfinItem{}
	for start := 0; start < len(ids); start += idsPerRequest {
		query := videoItemsQuery()
		query.Set(queryIDs, strings.Join(ids[start:min(start+idsPerRequest, len(ids))], listSeparator))
		var result itemsResult
		if err := l.call(ctx, http.MethodGet, pathItems, query, nil, &result); err != nil {
			return nil, err
		}
		items = append(items, result.items()...)
	}
	return items, nil
}

func (l *Library) ItemsSince(ctx context.Context, t time.Time) ([]library.JellyfinItem, error) {
	items := []library.JellyfinItem{}
	for start := 0; ; start += l.pageSize {
		query := videoItemsQuery()
		if !t.IsZero() {
			query.Set(queryMinDateLastSaved, t.UTC().Format(time.RFC3339))
		}
		query.Set(queryStartIndex, strconv.Itoa(start))
		query.Set(queryLimit, strconv.Itoa(l.pageSize))
		var result itemsResult
		if err := l.call(ctx, http.MethodGet, pathItems, query, nil, &result); err != nil {
			return nil, err
		}
		items = append(items, result.items()...)
		if len(result.Items) < l.pageSize || start+l.pageSize >= result.TotalRecordCount {
			return items, nil
		}
	}
}

// videoItemsQuery asks for movies and episodes that have a file, with its path.
func videoItemsQuery() url.Values {
	return url.Values{
		queryRecursive:            {queryTrue},
		queryIncludeItemTypes:     {videoItemTypes},
		queryExcludeLocationTypes: {locationTypeVirtual},
		queryFields:               {fieldPath},
		queryEnableImages:         {queryFalse},
		queryEnableUserData:       {queryFalse},
	}
}

func (r itemsResult) items() []library.JellyfinItem {
	items := make([]library.JellyfinItem, 0, len(r.Items))
	for _, item := range r.Items {
		if item.Path != "" {
			items = append(items, library.JellyfinItem(item))
		}
	}
	return items
}

// Changes connects to Jellyfin's WebSocket and streams its LibraryChanged messages. It answers
// ForceKeepAlive and keeps the socket alive until ctx ends or Jellyfin drops it; then the channel closes.
func (l *Library) Changes(ctx context.Context) (<-chan library.LibraryChange, error) {
	conn, err := settings.Jellyfin.Get(ctx, l.store)
	if err != nil {
		return nil, err
	}
	target, err := l.target(conn, pathSocket, nil)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set(authorizationHeader, fmt.Sprintf(tokenAuthorization, conn.APIKey))
	socket, resp, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPClient: l.client, HTTPHeader: header})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: connecting to %s: %w", library.ErrJellyfinUnavailable, pathSocket, err)
	}
	socket.SetReadLimit(maxSocketMessageBytes)
	changes := make(chan library.LibraryChange)
	go listen(ctx, socket, changes)
	return changes, nil
}

// listen reads the socket until it fails or ctx ends, then closes changes.
func listen(ctx context.Context, socket *websocket.Conn, changes chan<- library.LibraryChange) {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
		_ = socket.CloseNow()
		close(changes)
	}()
	intervals := make(chan time.Duration, 1)
	wg.Go(func() { keepAlive(ctx, socket, intervals) })
	for {
		_, data, err := socket.Read(ctx)
		if err != nil {
			return
		}
		var msg socketMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.MessageType {
		case messageForceKeepAlive:
			var seconds int
			if err := json.Unmarshal(msg.Data, &seconds); err == nil && seconds > 0 {
				select {
				case intervals <- time.Duration(seconds) * time.Second / keepAliveDivisor:
				default:
				}
			}
		case messageLibraryChanged:
			var info libraryUpdateInfo
			if err := json.Unmarshal(msg.Data, &info); err != nil {
				continue
			}
			select {
			case changes <- library.LibraryChange{Added: info.ItemsAdded, Updated: info.ItemsUpdated, Removed: info.ItemsRemoved}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// keepAlive answers each ForceKeepAlive at once, then sends a KeepAlive on the interval it set.
func keepAlive(ctx context.Context, socket *websocket.Conn, intervals <-chan time.Duration) {
	message, err := json.Marshal(socketMessage{MessageType: messageKeepAlive})
	if err != nil {
		return
	}
	var ticker *time.Ticker
	var tick <-chan time.Time
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case interval := <-intervals:
			if ticker == nil {
				ticker = time.NewTicker(interval)
			} else {
				ticker.Reset(interval)
			}
			tick = ticker.C
		case <-tick:
		}
		if err := socket.Write(ctx, websocket.MessageText, message); err != nil {
			return
		}
	}
}

// call sends one request with the API key and decodes a JSON answer into out, when out is set.
func (l *Library) call(ctx context.Context, method, path string, query url.Values, body, out any) error {
	conn, err := settings.Jellyfin.Get(ctx, l.store)
	if err != nil {
		return err
	}
	target, err := l.target(conn, path, query)
	if err != nil {
		return err
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return err
	}
	req.Header.Set(authorizationHeader, fmt.Sprintf(tokenAuthorization, conn.APIKey))
	if body != nil {
		req.Header.Set(contentTypeHeader, jsonContent)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", library.ErrJellyfinUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s %s answered %s", library.ErrJellyfinUnavailable, method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxItemsBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("%w: unreadable answer from %s: %w", library.ErrJellyfinUnavailable, path, err)
	}
	return nil
}

func (l *Library) target(conn settings.JellyfinConnection, path string, query url.Values) (string, error) {
	if conn.URL == "" || conn.APIKey == "" {
		return "", errNoLibrary
	}
	target, err := url.Parse(conn.URL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid Jellyfin URL: %w", library.ErrJellyfinUnavailable, err)
	}
	target = target.JoinPath(path)
	target.RawQuery = query.Encode()
	return target.String(), nil
}
