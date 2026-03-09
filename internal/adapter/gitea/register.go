package gitea

import "github.com/Strob0t/CodeForge/internal/port/pmprovider"

func init() {
	factory := func(cfg map[string]string) (pmprovider.Provider, error) {
		return NewProviderFromConfig(cfg)
	}
	for _, name := range []string{"gitea", "forgejo", "codeberg"} {
		pmprovider.Register(name, factory)
	}
}
