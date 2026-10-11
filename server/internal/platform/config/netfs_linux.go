//go:build linux

package config

import (
	"fmt"

	"golang.org/x/sys/unix"
)

var networkFilesystems = map[int64]string{
	unix.NFS_SUPER_MAGIC:  "nfs",
	unix.SMB_SUPER_MAGIC:  "smb",
	unix.SMB2_SUPER_MAGIC: "smb2",
	unix.CIFS_SUPER_MAGIC: "cifs",
	unix.AFS_SUPER_MAGIC:  "afs",
	unix.AFS_FS_MAGIC:     "afs",
	unix.CEPH_SUPER_MAGIC: "ceph",
	unix.CODA_SUPER_MAGIC: "coda",
}

func filesystemType(dir string) (string, bool, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return "", false, err
	}
	name, network := classifyMagic(int64(st.Type))
	return name, network, nil
}

func classifyMagic(magic int64) (string, bool) {
	if name, ok := networkFilesystems[magic]; ok {
		return name, true
	}
	return fmt.Sprintf("%#x", magic), false
}
