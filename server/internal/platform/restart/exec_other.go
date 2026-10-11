//go:build !unix

package restart

import "errors"

// Exec is unsupported off Unix; the process exits and its supervisor must start it again.
func Exec() error {
	return errors.New("restart: re-exec is only supported on Unix")
}
