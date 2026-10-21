package acquisition

import (
	"context"
	"errors"

	"magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
)

// DownloadView is a download for the Downloads tab, with its title's name and the last polled progress.
type DownloadView struct {
	download
	TitleName string
	// Live is nil until the poll has seen the torrent.
	Live *api.DownloadProgressed
}

// BlocklistView is a blocklist entry with its title's name.
type BlocklistView struct {
	api.BlocklistEntry
	TitleName string
}

// Downloads lists downloads still in qBittorrent, newest first.
func (s *DownloadService) Downloads(ctx context.Context) ([]DownloadView, error) {
	downloads, err := s.deps.Repo.activeDownloads(ctx)
	if err != nil {
		return nil, err
	}
	names := s.titleNames()
	views := make([]DownloadView, 0, len(downloads))
	for _, d := range downloads {
		views = append(views, s.view(ctx, d, names))
	}
	return views, nil
}

func (s *DownloadService) DownloadTitle(ctx context.Context, id domain.DownloadID) (domain.TitleID, error) {
	d, err := s.deps.Repo.Download(ctx, id)
	return d.Title, err
}

// Remove takes a download out of qBittorrent, with its data when deleteData is set. Nothing is blocklisted.
func (s *DownloadService) Remove(ctx context.Context, id domain.DownloadID, deleteData bool) (DownloadView, error) {
	d, err := s.deps.Repo.Download(ctx, id)
	if err != nil {
		return DownloadView{}, err
	}
	if d.RemovedAt == nil {
		if err := s.deps.QBittorrent.Delete(ctx, deleteData, d.QbitHash); ignoreGone(err) != nil {
			return DownloadView{}, err
		}
		if err := s.markRemoved(ctx, d); err != nil {
			return DownloadView{}, err
		}
		if d, err = s.deps.Repo.Download(ctx, id); err != nil {
			return DownloadView{}, err
		}
	}
	return s.view(ctx, d, s.titleNames()), nil
}

func (s *DownloadService) view(ctx context.Context, d download, names titleNamer) DownloadView {
	v := DownloadView{download: d, TitleName: names(ctx, d.Title)}
	s.mu.Lock()
	if live, ok := s.live[d.ID]; ok {
		v.Live = &live
	}
	s.mu.Unlock()
	return v
}

// Blocklist lists every entry, newest first, expired temporary entries included.
func (s *DownloadService) Blocklist(ctx context.Context) ([]BlocklistView, error) {
	entries, err := s.deps.Repo.Blocklist(ctx, "")
	if err != nil {
		return nil, err
	}
	names := s.titleNames()
	views := make([]BlocklistView, 0, len(entries))
	for _, e := range entries {
		views = append(views, BlocklistView{BlocklistEntry: e, TitleName: names(ctx, e.Title)})
	}
	return views, nil
}

// Unblock removes a blocklist entry, so its release can be grabbed again.
func (s *DownloadService) Unblock(ctx context.Context, id string) (BlocklistView, error) {
	e, err := s.deps.Repo.RemoveFromBlocklist(ctx, id)
	if err != nil {
		return BlocklistView{}, err
	}
	return BlocklistView{BlocklistEntry: e, TitleName: s.titleNames()(ctx, e.Title)}, nil
}

type titleNamer func(context.Context, domain.TitleID) string

// titleNames looks titles up once each; a title Catalog no longer has gets no name.
func (s *DownloadService) titleNames() titleNamer {
	known := map[domain.TitleID]string{}
	return func(ctx context.Context, id domain.TitleID) string {
		if name, ok := known[id]; ok {
			return name
		}
		title, err := s.deps.Catalog.Title(ctx, id)
		if err != nil && !errors.Is(err, catalogapi.ErrNotFound) {
			return ""
		}
		known[id] = title.Name
		return title.Name
	}
}

func (s *DownloadService) Options(ctx context.Context) (DownloadOptions, error) {
	return DownloadOptionsKey.Get(ctx, s.deps.Store)
}

func (s *DownloadService) SetOptions(ctx context.Context, o DownloadOptions) (DownloadOptions, error) {
	if err := o.Validate(); err != nil {
		return DownloadOptions{}, err
	}
	err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return DownloadOptionsKey.SetTx(tx, o) })
	if err != nil {
		return DownloadOptions{}, err
	}
	return s.Options(ctx)
}
