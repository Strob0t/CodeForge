package gitlocal

import "github.com/Strob0t/CodeForge/internal/port/gitprovider"

func init() {
	factory := func(_ map[string]string) (gitprovider.Provider, error) {
		return &Provider{}, nil
	}

	// All Git-based platforms use the same local git CLI for clone/pull/status.
	// Platform-specific API features (PRs, issues) are handled by pmprovider adapters.
	for _, name := range []string{providerName, "github", "gitlab", "gitea"} {
		gitprovider.Register(name, factory)
	}
}
