package qbittorrent

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/platform/settings"
)

const (
	queryRID = "rid"
	// etaUnknown is what qBittorrent reports as the ETA of a torrent that isn't moving.
	etaUnknown = 8640000
)

var statuses = map[string]acquisition.TorrentStatus{
	"error":              acquisition.TorrentError,
	"missingFiles":       acquisition.TorrentMissingFiles,
	"uploading":          acquisition.TorrentUploading,
	"stalledUP":          acquisition.TorrentStalledUp,
	"stoppedUP":          acquisition.TorrentStoppedUp,
	"queuedUP":           acquisition.TorrentQueuedUp,
	"forcedUP":           acquisition.TorrentForcedUp,
	"checkingUP":         acquisition.TorrentCheckingUp,
	"allocating":         acquisition.TorrentAllocating,
	"downloading":        acquisition.TorrentDownloading,
	"metaDL":             acquisition.TorrentMetaDL,
	"forcedMetaDL":       acquisition.TorrentMetaDL,
	"stoppedDL":          acquisition.TorrentStoppedDL,
	"queuedDL":           acquisition.TorrentQueuedDL,
	"stalledDL":          acquisition.TorrentStalledDL,
	"forcedDL":           acquisition.TorrentForcedDL,
	"checkingDL":         acquisition.TorrentCheckingDL,
	"checkingResumeData": acquisition.TorrentCheckingData,
	"moving":             acquisition.TorrentMoving,
}

type mainData struct {
	RID             int64                                 `json:"rid"`
	FullUpdate      bool                                  `json:"full_update"`
	Torrents        map[string]map[string]json.RawMessage `json:"torrents"`
	TorrentsRemoved []string                              `json:"torrents_removed"`
}

// torrentFields are the sync/maindata fields Magpie reads. Times are Unix seconds; durations seconds.
type torrentFields struct {
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	State        string  `json:"state"`
	HasMetadata  bool    `json:"has_metadata"`
	Progress     float64 `json:"progress"`
	Size         int64   `json:"size"`
	DLSpeed      int64   `json:"dlspeed"`
	ETA          int64   `json:"eta"`
	Ratio        float64 `json:"ratio"`
	SeedingTime  int64   `json:"seeding_time"`
	ContentPath  string  `json:"content_path"`
	AddedOn      int64   `json:"added_on"`
	CompletionOn int64   `json:"completion_on"`
	LastActivity int64   `json:"last_activity"`
}

// Sync asks for the changes since rid and merges them into the state it holds. A rid other than the one
// it last returned starts over from a full update. Only torrents in Magpie's category come back.
func (c *Client) Sync(ctx context.Context, rid int64) (acquisition.SyncState, error) {
	conn, err := c.conn(ctx)
	if err != nil {
		return acquisition.SyncState{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if rid != c.rid {
		rid = 0
	}
	body, err := c.call(ctx, request{path: pathMainData, query: url.Values{queryRID: {strconv.FormatInt(rid, 10)}}})
	if err != nil {
		return acquisition.SyncState{}, err
	}
	var data mainData
	if err := decode(pathMainData, body, &data); err != nil {
		return acquisition.SyncState{}, err
	}
	if data.FullUpdate || rid == 0 {
		c.merged = map[string]map[string]json.RawMessage{}
	}
	for hash, changed := range data.Torrents {
		fields, known := c.merged[hash]
		if !known {
			fields = map[string]json.RawMessage{}
			c.merged[hash] = fields
		}
		maps.Copy(fields, changed)
	}
	for _, hash := range data.TorrentsRemoved {
		delete(c.merged, hash)
	}
	c.rid = data.RID

	state := acquisition.SyncState{RID: data.RID, Torrents: map[string]acquisition.Torrent{}}
	for hash, raw := range c.merged {
		t, err := torrentOf(hash, raw)
		if err != nil {
			return acquisition.SyncState{}, err
		}
		if t.Category == conn.Category {
			state.Torrents[hash] = t
		}
	}
	return state, nil
}

func torrentOf(hash string, raw map[string]json.RawMessage) (acquisition.Torrent, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return acquisition.Torrent{}, err
	}
	var f torrentFields
	if err := decode(pathMainData, encoded, &f); err != nil {
		return acquisition.Torrent{}, err
	}
	status, ok := statuses[f.State]
	if !ok {
		status = acquisition.TorrentUnknownStatus
	}
	t := acquisition.Torrent{
		Hash:         hash,
		Name:         f.Name,
		Category:     f.Category,
		Status:       status,
		HasMetadata:  f.HasMetadata,
		Progress:     f.Progress,
		SizeBytes:    f.Size,
		DownloadBps:  f.DLSpeed,
		Ratio:        f.Ratio,
		SeedingTime:  time.Duration(f.SeedingTime) * time.Second,
		ContentPath:  f.ContentPath,
		AddedAt:      unix(f.AddedOn),
		LastActivity: unix(f.LastActivity),
	}
	if f.ETA < etaUnknown {
		t.ETA = time.Duration(f.ETA) * time.Second
	}
	if f.CompletionOn > 0 {
		completed := unix(f.CompletionOn)
		t.CompletedAt = &completed
	}
	return t, nil
}

// localPath maps a path under qBittorrent's RemotePath to the same place under Magpie's LocalPath.
func localPath(p string, conn settings.QBittorrentConnection) string {
	if conn.RemotePath == "" || p == "" {
		return p
	}
	remote := strings.TrimSuffix(conn.RemotePath, "/")
	rest, under := strings.CutPrefix(p, remote)
	if !under || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return p
	}
	return path.Join(conn.LocalPath, rest)
}

func unix(seconds int64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}
