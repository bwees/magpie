package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"magpie/internal/domain"

	"gorm.io/gorm"
)

var (
	ErrInvalidRootFolder = errors.New("library: invalid root folder")
	ErrRootFolderExists  = errors.New("library: that folder is already a root folder")
	ErrRootFolderInUse   = errors.New("library: the root folder still holds media files")
)

// writeProbeName is created and removed in a new root folder to check that Magpie can write there.
const writeProbeName = ".magpie-write-check"

// LinkMode is how imports reach a root folder from qBittorrent's download folder.
type LinkMode string

const (
	// LinkHardlink: both are on one filesystem, so seeding imports cost no space.
	LinkHardlink LinkMode = "HARDLINK"
	// LinkCopy: different filesystems, so seeding imports are copied.
	LinkCopy LinkMode = "COPY"
	// LinkUnknown: the download folder isn't known or isn't visible to Magpie.
	LinkUnknown LinkMode = "UNKNOWN"
)

// FolderLink is one root folder's result in the hardlink check.
type FolderLink struct {
	RootFolder
	Link LinkMode
}

// HardlinkCheck says whether imports from qBittorrent's download folder can be hardlinks.
type HardlinkCheck struct {
	DownloadPath string
	Folders      []FolderLink
	// Problem says why the links are unknown.
	Problem string
}

var mediaKinds = []domain.MediaKind{domain.MediaKindMovie, domain.MediaKindSeries}

// AddRootFolder stores a folder imports of one kind go to. It must exist and be writable.
func (s *Service) AddRootFolder(ctx context.Context, kind domain.MediaKind, path string) (FolderLink, error) {
	if !slices.Contains(mediaKinds, kind) {
		return FolderLink{}, fmt.Errorf("%w: unknown kind %q", ErrInvalidRootFolder, kind)
	}
	if !filepath.IsAbs(path) {
		return FolderLink{}, fmt.Errorf("%w: the path must be absolute", ErrInvalidRootFolder)
	}
	path = filepath.Clean(path)
	info, err := s.deps.Files.Stat(path)
	if err != nil {
		return FolderLink{}, fmt.Errorf("%w: %s doesn't exist or can't be read", ErrInvalidRootFolder, path)
	}
	if !info.IsDir() {
		return FolderLink{}, fmt.Errorf("%w: %s isn't a folder", ErrInvalidRootFolder, path)
	}
	probe := filepath.Join(path, writeProbeName)
	if err := s.deps.Files.MkdirAll(probe); err != nil {
		return FolderLink{}, fmt.Errorf("%w: Magpie can't write to %s", ErrInvalidRootFolder, path)
	}
	if err := s.deps.Files.Remove(probe); err != nil {
		return FolderLink{}, err
	}
	folders, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return FolderLink{}, err
	}
	if slices.ContainsFunc(folders, func(f RootFolder) bool { return f.Path == path }) {
		return FolderLink{}, ErrRootFolderExists
	}
	folder := RootFolder{ID: domain.NewID[string](), Kind: kind, Path: path, Device: info.Device}
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return insertRootFolder(tx, folder, s.now().UTC()) })
	if err != nil {
		return FolderLink{}, err
	}
	check := s.linksTo(ctx, []RootFolder{folder})
	return check.Folders[0], nil
}

// RemoveRootFolder forgets an empty root folder; its files on disk are left alone.
func (s *Service) RemoveRootFolder(ctx context.Context, id string) error {
	return s.deps.DB.Write(ctx, func(tx *gorm.DB) error {
		var files int64
		if err := tx.Model(&mediaFileRow{}).Where("root_folder_id = ?", id).Count(&files).Error; err != nil {
			return err
		}
		if files > 0 {
			return ErrRootFolderInUse
		}
		result := tx.Where("id = ?", id).Delete(&rootFolderRow{})
		if result.Error == nil && result.RowsAffected == 0 {
			return ErrRootFolderNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		return tx.Where("root_folder_id = ?", id).Delete(&scanFolderRow{}).Error
	})
}

// HardlinkCheck compares qBittorrent's download folder with every root folder.
func (s *Service) HardlinkCheck(ctx context.Context) (HardlinkCheck, error) {
	folders, err := s.deps.Repo.RootFolders(ctx)
	if err != nil {
		return HardlinkCheck{}, err
	}
	return s.linksTo(ctx, folders), nil
}

func (s *Service) linksTo(ctx context.Context, folders []RootFolder) HardlinkCheck {
	check := HardlinkCheck{Folders: make([]FolderLink, 0, len(folders))}
	unknown := func(problem string) HardlinkCheck {
		check.Problem = problem
		for _, f := range folders {
			check.Folders = append(check.Folders, FolderLink{RootFolder: f, Link: LinkUnknown})
		}
		return check
	}
	path, err := s.deps.Downloads.SavePath(ctx)
	if err != nil {
		return unknown(fmt.Sprintf("Couldn't ask qBittorrent where it saves downloads: %v", err))
	}
	if path == "" {
		return unknown("qBittorrent hasn't said where it saves downloads; check the connection in Settings → Connections")
	}
	check.DownloadPath = path
	if _, err := s.deps.Files.Stat(path); err != nil {
		return unknown(fmt.Sprintf("Magpie can't see %s; set the path mapping in Settings → Connections", path))
	}
	for _, f := range folders {
		link := LinkUnknown
		if same, err := s.deps.Files.SameDevice(path, f.Path); err == nil && same {
			link = LinkHardlink
		} else if err == nil {
			link = LinkCopy
		}
		check.Folders = append(check.Folders, FolderLink{RootFolder: f, Link: link})
	}
	return check
}
