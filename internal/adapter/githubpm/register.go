package githubpm

import "github.com/Strob0t/CodeForge/internal/port/pmprovider"

func init() {
	pmprovider.Register(providerName, func(cfg map[string]string) (pmprovider.Provider, error) {
		p := newProvider()
		p.token = cfg["token"]
		return p, nil
	})
}
