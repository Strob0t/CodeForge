package github

import "github.com/Strob0t/CodeForge/internal/port/gitprovider"

func init() {
	gitprovider.Register("github-api", func(cfg map[string]string) (gitprovider.Provider, error) {
		return NewProvider(cfg["token"], cfg["base_url"]), nil
	})
}
