//go:build !linux && !darwin

package config

const unknownFilesystem = "unknown"

// The server image is Linux-only; other platforms skip the check.
func filesystemType(string) (string, bool, error) {
	return unknownFilesystem, false, nil
}
