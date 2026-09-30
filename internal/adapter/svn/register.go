package svn

import "github.com/Strob0t/CodeForge/internal/port/gitprovider"

func init() {
	gitprovider.Register(providerName, func(cfg map[string]string) (gitprovider.Provider, error) {
		p := NewProvider(nil)
		if u, ok := cfg["username"]; ok {
			p.username = u
		}
		if pw, ok := cfg["password"]; ok {
			p.password = pw
		}
		// Operators hosting local repositories allow file:// URLs.
		p.allowFileURLs = cfg["allow_file_urls"] == "true"
		return p, nil
	})
}
