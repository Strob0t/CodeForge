package markdownspec

import "github.com/Strob0t/CodeForge/internal/port/specprovider"

func init() {
	specprovider.Register(providerName, func(_ map[string]string) (specprovider.Provider, error) {
		return &Provider{}, nil
	})
}
