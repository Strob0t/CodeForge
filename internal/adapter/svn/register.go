package svn

import "github.com/Strob0t/CodeForge/internal/port/gitprovider"

func init() {
	gitprovider.Register(providerName, func(_ map[string]string) (gitprovider.Provider, error) {
		return NewProvider(nil), nil
	})
}
