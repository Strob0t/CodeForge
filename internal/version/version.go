package version

import (
	"os"
	"path/filepath"
	"strings"
)

// Version is the application version. It can be overridden at build time via:
//
//	go build -ldflags "-X github.com/Strob0t/CodeForge/internal/version.Version=$(cat VERSION)"
var Version = ""

func init() {
	if Version != "" {
		return
	}
	// Dev fallback: read VERSION file from working directory or project root.
	for _, rel := range []string{"VERSION", "../VERSION", "../../VERSION"} {
		p := filepath.Clean(rel)
		b, err := os.ReadFile(p) //nolint:gosec // paths are hardcoded constants
		if err == nil {
			Version = strings.TrimSpace(string(b))
			return
		}
	}
	Version = "dev"
}
