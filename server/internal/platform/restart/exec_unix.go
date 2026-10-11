//go:build unix

package restart

import (
	"os"
	"syscall"
)

// Exec replaces the current process with a fresh copy of the same binary and arguments.
func Exec() error {
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(binary, os.Args, os.Environ())
}
