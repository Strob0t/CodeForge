package policy

import (
	"path/filepath"
	"strings"
)

// matchesAnyGlob reports whether value matches one of the path glob
// patterns. Deny matching ignores case and treats a malformed pattern as a
// match, so that it errs on the side of denying; allow matching is exact.
func matchesAnyGlob(patterns []string, value string, deny bool) bool {
	if deny {
		value = strings.ToLower(value)
	}
	for _, pattern := range patterns {
		if deny {
			pattern = strings.ToLower(pattern)
		}
		matched, err := matchGlob(pattern, value)
		if err != nil {
			if deny {
				return true
			}
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// matchGlob matches a slash-separated path against a glob pattern:
// filepath.Match semantics within a path segment ("*" does not cross "/")
// and "**" as a whole segment for zero or more segments.
func matchGlob(pattern, value string) (bool, error) {
	value = filepath.Clean(value)
	if !strings.Contains(pattern, "**") {
		return filepath.Match(pattern, value)
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(value, "/"))
}

// matchSegments recursively matches pattern segments against value segments.
func matchSegments(pat, val []string) (bool, error) {
	for len(pat) > 0 && len(val) > 0 {
		if pat[0] == "**" {
			pat = pat[1:]
			if len(pat) == 0 {
				return true, nil // trailing ** matches everything
			}
			for i := 0; i <= len(val); i++ {
				matched, err := matchSegments(pat, val[i:])
				if err != nil || matched {
					return matched, err
				}
			}
			return false, nil
		}
		matched, err := filepath.Match(pat[0], val[0])
		if err != nil || !matched {
			return false, err
		}
		pat = pat[1:]
		val = val[1:]
	}
	for _, p := range pat {
		if p != "**" {
			return false, nil
		}
	}
	return len(val) == 0, nil
}

// validateGlob reports a malformed path glob pattern.
func validateGlob(pattern string) error {
	for _, seg := range strings.Split(pattern, "/") {
		if _, err := filepath.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}

// matchWildcard matches s against a pattern in which "*" matches any run of
// characters (including "/" and spaces) and "?" matches one character.
func matchWildcard(pattern, s string, foldCase bool) bool {
	if foldCase {
		pattern, s = strings.ToLower(pattern), strings.ToLower(s)
	}
	p, i := 0, 0
	star, retry := -1, 0
	for i < len(s) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, retry = p, i
			p++
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == s[i]):
			p++
			i++
		case star >= 0:
			retry++
			p, i = star+1, retry
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

func joinWords(words []string) string {
	return strings.Join(words, " ")
}
