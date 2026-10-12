package migrate

import (
	"os"
	"path/filepath"
)

// writeProbe is created and removed to check that the server can write to a folder.
const (
	writeProbe     = ".magpie-migration-check"
	writeProbeMode = 0o755
)

// OSDisk checks paths on the server's own filesystem.
type OSDisk struct{}

func (OSDisk) CheckFolder(path string) PathAccess {
	if !filepath.IsAbs(path) {
		return PathNotAbsolute
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return PathMissing
	case !info.IsDir():
		return PathNotFolder
	}
	probe := filepath.Join(path, writeProbe)
	if err := os.Mkdir(probe, writeProbeMode); err != nil && !os.IsExist(err) {
		return PathNotWritable
	}
	if err := os.Remove(probe); err != nil {
		return PathNotWritable
	}
	return PathOK
}

func (OSDisk) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
