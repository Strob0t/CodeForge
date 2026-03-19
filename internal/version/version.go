package version

import (
	"os"
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
	for _, path := range []string{"VERSION", "../VERSION", "../../VERSION"} {
		if b, err := os.ReadFile(path); err == nil {
			Version = strings.TrimSpace(string(b))
			return
		}
	}
	Version = "dev"
}
