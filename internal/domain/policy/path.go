package policy

import (
	"path/filepath"
	"strings"
)

// NormalizePath maps a tool-call path to a clean, slash-separated path
// relative to the workspace root. Absolute paths inside the workspace become
// relative. ok is false when the path leaves the workspace: an absolute path
// outside it, a relative path that climbs above it with "..", or an absolute
// path that cannot be checked because the workspace is unknown or relative.
// An empty path stays empty.
func NormalizePath(workspace, p string) (rel string, ok bool) {
	if p == "" {
		return "", true
	}
	if filepath.IsAbs(p) {
		if workspace == "" || !filepath.IsAbs(workspace) {
			return "", false
		}
		r, err := filepath.Rel(filepath.Clean(workspace), filepath.Clean(p))
		if err != nil {
			return "", false
		}
		p = r
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return filepath.ToSlash(clean), true
}
