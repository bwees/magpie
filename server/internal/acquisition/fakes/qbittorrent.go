package fakes

import (
	"cmp"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/platform/scripted"
)

const (
	QBittorrentVersion        scripted.Method = "QBittorrent.Version"
	QBittorrentAddTorrent     scripted.Method = "QBittorrent.AddTorrent"
	QBittorrentStart          scripted.Method = "QBittorrent.Start"
	QBittorrentDelete         scripted.Method = "QBittorrent.Delete"
	QBittorrentFiles          scripted.Method = "QBittorrent.Files"
	QBittorrentSync           scripted.Method = "QBittorrent.Sync"
	QBittorrentSetShareLimits scripted.Method = "QBittorrent.SetShareLimits"
	QBittorrentFreeSpace      scripted.Method = "QBittorrent.FreeSpace"
)

// Deletion is one hash passed to Delete.
type Deletion struct {
	Hash        string
	DeleteFiles bool
}

type torrent struct {
	state   acquisition.Torrent
	added   acquisition.AddRequest
	started bool
	limits  *acquisition.ShareLimits
}

// QBittorrent holds torrents in memory. A torrent's hash is HashOf its AddRequest, so tests can set its
// files before or after adding it; a torrent has metadata once its files are set. Tests move a torrent
// along with Update.
type QBittorrent struct {
	scripted.Script

	mu        sync.Mutex
	now       func() time.Time
	version   acquisition.WebAPIVersion
	torrents  map[string]*torrent
	files     map[string][]acquisition.TorrentFile
	deletions []Deletion
	// freeSpace is nil until SetFreeSpace; FreeSpace then answers ErrUnavailable, as an unreachable client does.
	freeSpace *int64
}

var _ acquisition.QBittorrent = (*QBittorrent)(nil)

// NewQBittorrent reports MinWebAPIVersion and holds no torrents.
func NewQBittorrent() *QBittorrent {
	return &QBittorrent{
		now:      time.Now,
		version:  acquisition.MinWebAPIVersion,
		torrents: map[string]*torrent{},
		files:    map[string][]acquisition.TorrentFile{},
	}
}

// HashOf is the info hash the fake gives a torrent added with r.
func HashOf(r acquisition.AddRequest) string {
	payload := r.Torrent
	if len(payload) == 0 {
		payload = []byte(r.Magnet)
	}
	sum := sha1.Sum(payload)
	return hex.EncodeToString(sum[:])
}

func (q *QBittorrent) SetVersion(v acquisition.WebAPIVersion) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version = v
}

// SetFiles sets a torrent's file list, which also marks its metadata as received.
func (q *QBittorrent) SetFiles(hash string, files ...acquisition.TorrentFile) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.files[hash] = files
	if t, ok := q.torrents[hash]; ok {
		t.state.HasMetadata = true
	}
}

// Update changes a torrent's state, e.g. progress, status, ratio or content path.
func (q *QBittorrent) Update(hash string, fn func(*acquisition.Torrent)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if t, ok := q.torrents[hash]; ok {
		fn(&t.state)
	}
}

// Torrent returns a torrent's state and whether it exists.
func (q *QBittorrent) Torrent(hash string) (acquisition.Torrent, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.torrents[hash]
	if !ok {
		return acquisition.Torrent{}, false
	}
	return t.state, true
}

// Added returns the request a torrent was added with.
func (q *QBittorrent) Added(hash string) (acquisition.AddRequest, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.torrents[hash]
	if !ok {
		return acquisition.AddRequest{}, false
	}
	return t.added, true
}

// Started reports whether Start was called for a torrent.
func (q *QBittorrent) Started(hash string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.torrents[hash]
	return ok && t.started
}

// ShareLimits returns the last limits set on a torrent.
func (q *QBittorrent) ShareLimits(hash string) (acquisition.ShareLimits, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.torrents[hash]
	if !ok || t.limits == nil {
		return acquisition.ShareLimits{}, false
	}
	return *t.limits, true
}

// Deletions returns every Delete, one entry per hash, in call order.
func (q *QBittorrent) Deletions() []Deletion {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.deletions)
}

func (q *QBittorrent) Version(ctx context.Context) (acquisition.WebAPIVersion, error) {
	if err := q.Enter(ctx, QBittorrentVersion); err != nil {
		return acquisition.WebAPIVersion{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if compareVersions(q.version, acquisition.MinWebAPIVersion) < 0 {
		return q.version, acquisition.ErrUnsupportedVersion
	}
	return q.version, nil
}

func (q *QBittorrent) AddTorrent(ctx context.Context, r acquisition.AddRequest) (string, error) {
	if err := q.Enter(ctx, QBittorrentAddTorrent); err != nil {
		return "", err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	hash := HashOf(r)
	_, hasMetadata := q.files[hash]
	status := acquisition.TorrentMetaDL
	if hasMetadata || len(r.Torrent) > 0 {
		hasMetadata = true
		status = acquisition.TorrentStoppedDL
	}
	q.torrents[hash] = &torrent{
		added: r,
		state: acquisition.Torrent{
			Hash:        hash,
			Name:        hash,
			Category:    r.Category,
			Status:      status,
			HasMetadata: hasMetadata,
			AddedAt:     q.now(),
		},
	}
	return hash, nil
}

func (q *QBittorrent) Start(ctx context.Context, hashes ...string) error {
	if err := q.Enter(ctx, QBittorrentStart); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, hash := range hashes {
		t, ok := q.torrents[hash]
		if !ok {
			return acquisition.ErrTorrentNotFound
		}
		t.started = true
		t.state.Status = acquisition.TorrentDownloading
	}
	return nil
}

// Delete removes the torrents; hashes that are already gone are ignored, as qBittorrent does.
func (q *QBittorrent) Delete(ctx context.Context, deleteFiles bool, hashes ...string) error {
	if err := q.Enter(ctx, QBittorrentDelete); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, hash := range hashes {
		delete(q.torrents, hash)
		q.deletions = append(q.deletions, Deletion{Hash: hash, DeleteFiles: deleteFiles})
	}
	return nil
}

func (q *QBittorrent) Files(ctx context.Context, hash string) ([]acquisition.TorrentFile, error) {
	if err := q.Enter(ctx, QBittorrentFiles); err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.torrents[hash]; !ok {
		return nil, acquisition.ErrTorrentNotFound
	}
	return slices.Clone(q.files[hash]), nil
}

// Sync returns the full state every time, with RID one past the caller's.
func (q *QBittorrent) Sync(ctx context.Context, rid int64) (acquisition.SyncState, error) {
	if err := q.Enter(ctx, QBittorrentSync); err != nil {
		return acquisition.SyncState{}, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	state := acquisition.SyncState{RID: rid + 1, Torrents: map[string]acquisition.Torrent{}}
	for hash, t := range q.torrents {
		state.Torrents[hash] = t.state
	}
	return state, nil
}

func (q *QBittorrent) SetShareLimits(ctx context.Context, hash string, l acquisition.ShareLimits) error {
	if err := q.Enter(ctx, QBittorrentSetShareLimits); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	t, ok := q.torrents[hash]
	if !ok {
		return acquisition.ErrTorrentNotFound
	}
	t.limits = &l
	return nil
}

// SetFreeSpace sets what FreeSpace reports for the download disk.
func (q *QBittorrent) SetFreeSpace(bytes int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.freeSpace = &bytes
}

func (q *QBittorrent) FreeSpace(ctx context.Context) (int64, error) {
	if err := q.Enter(ctx, QBittorrentFreeSpace); err != nil {
		return 0, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.freeSpace == nil {
		return 0, acquisition.ErrUnavailable
	}
	return *q.freeSpace, nil
}

func compareVersions(a, b acquisition.WebAPIVersion) int {
	return cmp.Or(cmp.Compare(a.Major, b.Major), cmp.Compare(a.Minor, b.Minor), cmp.Compare(a.Patch, b.Patch))
}
