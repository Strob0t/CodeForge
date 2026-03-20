package version

import (
	"os"
	"path/filepath"
	"strings"
)

// Version is the application version. Override at build time via:
//
//	go build -ldflags "-X github.com/Strob0t/CodeForge/internal/version.Version=$(cat VERSION) -X github.com/Strob0t/CodeForge/internal/version.GitSHA=$(git rev-parse --short HEAD)"
var Version = ""

// GitSHA is the git commit hash. Set at build time via ldflags.
var GitSHA = ""

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
