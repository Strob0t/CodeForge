package svn

import (
	"sync/atomic"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// allowFileURLs is the operator setting svn.allow_file_urls.
var allowFileURLs atomic.Bool

// SetAllowFileURLs sets whether working copies of local (file://)
// repositories may be used (svn.allow_file_urls, KI-189).
func SetAllowFileURLs(allow bool) { allowFileURLs.Store(allow) }

func init() {
	gitprovider.Register(providerName, func(cfg map[string]string) (gitprovider.Provider, error) {
		p := NewProvider(nil)
		if u, ok := cfg["username"]; ok {
			p.username = u
		}
		if pw, ok := cfg["password"]; ok {
			p.password = pw
		}
		// Operators hosting local repositories allow file:// URLs
		// (svn.allow_file_urls); the project config cannot (KI-189).
		p.allowFileURLs = allowFileURLs.Load()
		// The project's repository URL (set by the project service, never by
		// the project config): svn contacts nothing outside it.
		p.repoURL = cfg["repo_url"]
		return p, nil
	})
}
