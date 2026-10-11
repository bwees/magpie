//go:build darwin

package config

import (
	"golang.org/x/sys/unix"
)

var networkFilesystems = map[string]bool{
	"nfs":    true,
	"smbfs":  true,
	"afpfs":  true,
	"webdav": true,
	"cifs":   true,
}

func filesystemType(dir string) (string, bool, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return "", false, err
	}
	name := unix.ByteSliceToString(st.Fstypename[:])
	return name, networkFilesystems[name], nil
}
