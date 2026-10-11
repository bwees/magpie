package config

import "fmt"

var detectFilesystem = filesystemType

// SQLite's WAL mode relies on shared memory and locks that NFS and SMB do not provide.
func ensureLocalFilesystem(dir string) error {
	fsType, network, err := detectFilesystem(dir)
	if err != nil {
		return fmt.Errorf("checking filesystem: %w", err)
	}
	if network {
		return fmt.Errorf("%w (%s)", ErrNetworkFilesystem, fsType)
	}
	return nil
}
