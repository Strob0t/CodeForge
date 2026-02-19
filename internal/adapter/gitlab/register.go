package gitlab

import "github.com/Strob0t/CodeForge/internal/port/pmprovider"

func init() {
	pmprovider.Register(providerName, func(cfg map[string]string) (pmprovider.Provider, error) {
		return NewProvider(cfg["base_url"], cfg["token"]), nil
	})
}
