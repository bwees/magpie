package health

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	versionSeparator = "."
	versionPrefix    = "v"
)

// VersionAtLeast compares a dotted version such as "v5.2.1" or "2.15" with minimum, part by part.
// Missing parts count as zero.
func VersionAtLeast(version string, minimum ...int) (bool, error) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), versionPrefix), versionSeparator)
	for i, want := range minimum {
		have := 0
		if i < len(parts) {
			n, err := strconv.Atoi(parts[i])
			if err != nil {
				return false, fmt.Errorf("unreadable version %q", version)
			}
			have = n
		}
		if have != want {
			return have > want, nil
		}
	}
	return true, nil
}
