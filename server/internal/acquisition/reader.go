package acquisition

import (
	"context"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// Reader is Acquisition's api.Reader. Profiles come from the quality tables. Downloads, the blocklist and
// file scores aren't stored yet, so those answer empty until the grab and cutoff slices build them.
type Reader struct {
	*QualityRepository
}

var _ api.Reader = Reader{}

func NewReader(q *QualityRepository) Reader {
	return Reader{QualityRepository: q}
}

func (Reader) ActiveDownloads(context.Context) ([]api.Download, error) {
	return []api.Download{}, nil
}

func (Reader) DownloadsForTitle(context.Context, domain.TitleID) ([]api.Download, error) {
	return []api.Download{}, nil
}

func (Reader) Blocklist(context.Context, domain.TitleID) ([]api.BlocklistEntry, error) {
	return []api.BlocklistEntry{}, nil
}

func (Reader) CutoffStatus(context.Context, domain.MediaFileID) (api.CutoffStatus, error) {
	return api.CutoffStatus{}, api.ErrNotFound
}
