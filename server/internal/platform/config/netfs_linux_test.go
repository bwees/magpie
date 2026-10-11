//go:build linux

package config

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestClassifyMagic(t *testing.T) {
	cases := []struct {
		magic   int64
		network bool
	}{
		{unix.NFS_SUPER_MAGIC, true},
		{unix.SMB2_SUPER_MAGIC, true},
		{unix.CIFS_SUPER_MAGIC, true},
		{unix.EXT4_SUPER_MAGIC, false},
		{unix.XFS_SUPER_MAGIC, false},
		{unix.OVERLAYFS_SUPER_MAGIC, false},
	}
	for _, tc := range cases {
		name, network := classifyMagic(tc.magic)
		if network != tc.network {
			t.Errorf("classify %#x (%s): network = %v, want %v", tc.magic, name, network, tc.network)
		}
	}
}
