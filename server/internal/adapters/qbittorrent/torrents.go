package qbittorrent

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"magpie/internal/acquisition"
)

// Form fields and values of the WebAPI, as qBittorrent spells them.
const (
	formTorrents                 = "torrents"
	formURLs                     = "urls"
	formStopped                  = "stopped"
	formStopCondition            = "stopCondition"
	formHashes                   = "hashes"
	formHash                     = "hash"
	formDeleteFiles              = "deleteFiles"
	formRatioLimit               = "ratioLimit"
	formSeedingTimeLimit         = "seedingTimeLimit"
	formInactiveSeedingTimeLimit = "inactiveSeedingTimeLimit"
	formShareLimitAction         = "shareLimitAction"
	formShareLimitsMode          = "shareLimitsMode"
	torrentFileName              = "release.torrent"
	torrentContentType           = "application/x-bittorrent"
	stopNone                     = "None"
	stopMetadataReceived         = "MetadataReceived"
	formFalse                    = "false"

	// Share limit values qBittorrent reads as "the global limit" and "no limit".
	limitGlobal    = -2
	limitUnlimited = -1
	// limitsMatchAny ends seeding at whichever goal is met first; limitsDefault defers to the global setting.
	limitsMatchAny = "MatchAny"
	limitsDefault  = "Default"
)

var shareActions = map[acquisition.ShareLimitAction]string{
	acquisition.ShareStop:              "Stop",
	acquisition.ShareRemove:            "Remove",
	acquisition.ShareRemoveWithContent: "RemoveWithContent",
	acquisition.ShareSuperSeeding:      "EnableSuperSeeding",
}

func stopCondition(c acquisition.StopCondition) string {
	if c == acquisition.StopMetadataReceived {
		return stopMetadataReceived
	}
	return stopNone
}

func (c *Client) Version(ctx context.Context) (acquisition.WebAPIVersion, error) {
	body, err := c.call(ctx, request{path: pathWebAPIVersion})
	if err != nil {
		return acquisition.WebAPIVersion{}, err
	}
	v, err := parseVersion(strings.TrimSpace(string(body)))
	if err != nil {
		return acquisition.WebAPIVersion{}, err
	}
	if compareVersions(v, acquisition.MinWebAPIVersion) < 0 {
		return v, acquisition.ErrUnsupportedVersion
	}
	return v, nil
}

func parseVersion(text string) (acquisition.WebAPIVersion, error) {
	parts := strings.Split(text, ".")
	numbers := make([]int, 3)
	for i := 0; i < len(parts) && i < len(numbers); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return acquisition.WebAPIVersion{}, fmt.Errorf("%w: unreadable WebAPI version %q", acquisition.ErrUnavailable, text)
		}
		numbers[i] = n
	}
	return acquisition.WebAPIVersion{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}, nil
}

func compareVersions(a, b acquisition.WebAPIVersion) int {
	return cmp.Or(cmp.Compare(a.Major, b.Major), cmp.Compare(a.Minor, b.Minor), cmp.Compare(a.Patch, b.Patch))
}

type addResponse struct {
	SuccessCount    int      `json:"success_count"`
	PendingCount    int      `json:"pending_count"`
	FailureCount    int      `json:"failure_count"`
	AddedTorrentIDs []string `json:"added_torrent_ids"`
}

// AddTorrent uploads a .torrent or sends a magnet. The info hash comes from the release itself, since
// qBittorrent leaves added_torrent_ids empty while a magnet is still pending.
func (c *Client) AddTorrent(ctx context.Context, t acquisition.AddRequest) (string, error) {
	hash, err := infoHash(t)
	if err != nil {
		return "", err
	}
	conn, err := c.conn(ctx)
	if err != nil {
		return "", err
	}
	body, contentType, err := addForm(t, cmp.Or(t.Category, conn.Category))
	if err != nil {
		return "", err
	}
	a, err := c.do(ctx, conn, request{path: pathAdd, body: body, contentType: contentType})
	if err != nil {
		return "", err
	}
	if a.status == http.StatusConflict {
		return "", fmt.Errorf("%w: qBittorrent didn't add the torrent", acquisition.ErrUnavailable)
	}
	if a.status != http.StatusOK && a.status != http.StatusAccepted {
		return "", fmt.Errorf("%s: %w: %w", pathAdd, acquisition.ErrUnavailable, httpStatus(a.status))
	}
	var added addResponse
	if err := decode(pathAdd, a.body, &added); err != nil {
		return "", err
	}
	if len(added.AddedTorrentIDs) > 0 {
		return added.AddedTorrentIDs[0], nil
	}
	return hash, nil
}

func infoHash(t acquisition.AddRequest) (string, error) {
	if len(t.Torrent) > 0 {
		meta, err := acquisition.ParseTorrent(t.Torrent)
		return meta.InfoHash, err
	}
	return acquisition.MagnetInfoHash(t.Magnet)
}

func addForm(t acquisition.AddRequest, category string) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	fields := [][2]string{{formCategory, category}}
	if t.SavePath != "" {
		fields = append(fields, [2]string{formSavePath, t.SavePath})
	}
	fields = append(fields, [2]string{formStopped, formFalse}, [2]string{formStopCondition, stopCondition(t.StopCondition)})
	if len(t.Torrent) == 0 {
		fields = append(fields, [2]string{formURLs, t.Magnet})
	}
	for _, f := range fields {
		if err := form.WriteField(f[0], f[1]); err != nil {
			return nil, "", err
		}
	}
	if len(t.Torrent) > 0 {
		part, err := form.CreatePart(map[string][]string{
			"Content-Disposition": {fmt.Sprintf(`form-data; name=%q; filename=%q`, formTorrents, torrentFileName)},
			contentTypeHeader:     {torrentContentType},
		})
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(t.Torrent); err != nil {
			return nil, "", err
		}
	}
	if err := form.Close(); err != nil {
		return nil, "", err
	}
	return &body, form.FormDataContentType(), nil
}

func (c *Client) Start(ctx context.Context, hashes ...string) error {
	_, err := c.call(ctx, formRequest(pathStart, url.Values{formHashes: {joinHashes(hashes)}}))
	return err
}

// Delete removes the torrents; qBittorrent ignores hashes it doesn't have.
func (c *Client) Delete(ctx context.Context, deleteFiles bool, hashes ...string) error {
	form := url.Values{formHashes: {joinHashes(hashes)}, formDeleteFiles: {strconv.FormatBool(deleteFiles)}}
	_, err := c.call(ctx, formRequest(pathDelete, form))
	return err
}

type fileEntry struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority int     `json:"priority"`
}

// Files is empty until the torrent has metadata. Names are relative to the content root.
func (c *Client) Files(ctx context.Context, hash string) ([]acquisition.TorrentFile, error) {
	body, err := c.call(ctx, request{path: pathFiles, query: url.Values{formHash: {hash}}})
	if err != nil {
		return nil, err
	}
	var entries []fileEntry
	if err := decode(pathFiles, body, &entries); err != nil {
		return nil, err
	}
	files := make([]acquisition.TorrentFile, 0, len(entries))
	for _, e := range entries {
		files = append(files, acquisition.TorrentFile{Index: e.Index, Path: e.Name, Size: e.Size, Progress: e.Progress, Priority: e.Priority})
	}
	return files, nil
}

// SetShareLimits always sends every limit: WebAPI 2.15 rejects a call that leaves one out.
func (c *Client) SetShareLimits(ctx context.Context, hash string, l acquisition.ShareLimits) error {
	ratio, seeding, inactive, mode := shareLimitValues(l)
	action, ok := shareActions[l.Action]
	if !ok {
		return fmt.Errorf("unknown share limit action %q", l.Action)
	}
	form := url.Values{
		formHashes:                   {hash},
		formRatioLimit:               {ratio},
		formSeedingTimeLimit:         {seeding},
		formInactiveSeedingTimeLimit: {inactive},
		formShareLimitAction:         {action},
		formShareLimitsMode:          {mode},
	}
	_, err := c.call(ctx, formRequest(pathSetShareLimits, form))
	return err
}

func shareLimitValues(l acquisition.ShareLimits) (ratio, seeding, inactive, mode string) {
	switch l.Mode {
	case acquisition.ShareLimitsUnlimited:
		unlimited := strconv.Itoa(limitUnlimited)
		return unlimited, unlimited, unlimited, limitsDefault
	case acquisition.ShareLimitsCustom:
		return strconv.FormatFloat(l.Ratio, 'f', -1, 64), minutes(l.SeedingTime), minutes(l.InactiveSeedingTime), limitsMatchAny
	}
	global := strconv.Itoa(limitGlobal)
	return global, global, global, limitsDefault
}

// minutes is a limit in qBittorrent's unit; zero means no limit.
func minutes(d time.Duration) string {
	if d <= 0 {
		return strconv.Itoa(limitUnlimited)
	}
	return strconv.FormatInt(int64(d/time.Minute), 10)
}

type serverState struct {
	FreeSpaceOnDisk *int64 `json:"free_space_on_disk"`
}

// FreeSpace asks for a full sync/maindata and reads the download disk's free space from it.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	body, err := c.call(ctx, request{path: pathMainData, query: url.Values{queryRID: {"0"}}})
	if err != nil {
		return 0, err
	}
	var data struct {
		ServerState serverState `json:"server_state"`
	}
	if err := decode(pathMainData, body, &data); err != nil {
		return 0, err
	}
	if data.ServerState.FreeSpaceOnDisk == nil {
		return 0, fmt.Errorf("%w: qBittorrent didn't report free space", acquisition.ErrUnavailable)
	}
	return *data.ServerState.FreeSpaceOnDisk, nil
}
