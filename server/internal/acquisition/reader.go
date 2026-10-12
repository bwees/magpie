package acquisition

import (
	"context"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
)

// Reader is Acquisition's api.Reader. Profiles come from the quality tables; downloads, the blocklist and
// file scores from the repository.
type Reader struct {
	*QualityRepository
	repo *Repository
}

var _ api.Reader = Reader{}

func NewReader(q *QualityRepository, r *Repository) Reader {
	return Reader{QualityRepository: q, repo: r}
}

func (r Reader) ActiveDownloads(ctx context.Context) ([]api.Download, error) {
	return r.repo.ActiveDownloads(ctx)
}

func (r Reader) DownloadsForTitle(ctx context.Context, title domain.TitleID) ([]api.Download, error) {
	return r.repo.DownloadsForTitle(ctx, title)
}

func (r Reader) Blocklist(ctx context.Context, title domain.TitleID) ([]api.BlocklistEntry, error) {
	return r.repo.Blocklist(ctx, title)
}

func (r Reader) CutoffStatus(ctx context.Context, file domain.MediaFileID) (api.CutoffStatus, error) {
	return r.repo.CutoffStatus(ctx, file)
}
