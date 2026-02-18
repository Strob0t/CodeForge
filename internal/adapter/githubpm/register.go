package githubpm

import "github.com/Strob0t/CodeForge/internal/port/pmprovider"

func init() {
	pmprovider.Register(providerName, func(_ map[string]string) (pmprovider.Provider, error) {
		return newProvider(), nil
	})
}
