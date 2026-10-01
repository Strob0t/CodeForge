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
		// The project's repository URL (set by the project service, never by
		// the project config): svn contacts nothing outside it.
		p.repoURL = cfg["repo_url"]
		return p, nil
	})
}
