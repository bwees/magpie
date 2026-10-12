package acquisition

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// RegexPatternPrefix marks a blocked-file pattern as a regular expression instead of a glob.
const RegexPatternPrefix = "regex:"

const caseInsensitive = "(?i)"

var ErrInvalidPattern = errors.New("acquisition: invalid blocked-file pattern")

// DefaultBlockedPatterns are the Blocked file patterns a new install starts with.
var DefaultBlockedPatterns = []string{
	"*.exe", "*.msi", "*.bat", "*.cmd", "*.com", "*.scr", "*.lnk", "*.ps1", "*.vbs", "*.js", "*.jar",
	"*.apk", "*.dmg", "*.iso", "*.rar", "*.r[0-9][0-9]", "*.zip", "*.7z",
}

// blockedFiles matches file names, never folder names, against the user's patterns, ignoring case.
type blockedFiles struct {
	matchers []func(name string) bool
}

func compileBlockedPatterns(patterns []string) (blockedFiles, error) {
	var b blockedFiles
	var errs []error
	for _, pattern := range patterns {
		matcher, err := compilePattern(pattern)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		b.matchers = append(b.matchers, matcher)
	}
	if len(errs) > 0 {
		return blockedFiles{}, errors.Join(errs...)
	}
	return b, nil
}

func compilePattern(pattern string) (func(string) bool, error) {
	if expr, isRegex := strings.CutPrefix(pattern, RegexPatternPrefix); isRegex {
		re, err := regexp.Compile(caseInsensitive + expr)
		if err != nil {
			return nil, fmt.Errorf("%w %q: %w", ErrInvalidPattern, pattern, err)
		}
		return re.MatchString, nil
	}
	glob := strings.ToLower(strings.TrimSpace(pattern))
	if _, err := path.Match(glob, ""); err != nil || glob == "" || strings.Contains(glob, "/") {
		return nil, fmt.Errorf("%w %q: not a file-name glob", ErrInvalidPattern, pattern)
	}
	return func(name string) bool {
		matched, _ := path.Match(glob, strings.ToLower(name))
		return matched
	}, nil
}

// firstMatch returns the first path whose file name is blocked.
func (b blockedFiles) firstMatch(paths []string) (string, bool) {
	for _, p := range paths {
		name := path.Base(strings.ReplaceAll(p, `\`, "/"))
		for _, matches := range b.matchers {
			if matches(name) {
				return p, true
			}
		}
	}
	return "", false
}
