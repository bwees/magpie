package disk

import (
	"os"

	"magpie/internal/transcode"
)

// FileStat implements Transcode's FileStat port on the server's own disks.
type FileStat struct{}

var _ transcode.FileStat = FileStat{}

func NewFileStat() FileStat { return FileStat{} }

func (FileStat) Stat(path string) (transcode.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return transcode.FileInfo{}, err
	}
	info := infoOf(path, fi)
	return transcode.FileInfo{Size: info.Size, ModTime: info.ModTime, Inode: info.Inode, Device: info.Device}, nil
}
