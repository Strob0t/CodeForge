package plane

import "github.com/Strob0t/CodeForge/internal/port/pmprovider"

func init() {
	pmprovider.Register(providerName, func(config map[string]string) (pmprovider.Provider, error) {
		return newProvider(config)
	})
}
