package project

import (
	"net/url"
	"strings"
)

// RepoHostPath returns the host (lower case, without port) and the full
// repository path (owner/name, GitLab subgroups included, without a .git
// suffix) of a repository URL: https://host/path, http://, ssh:// or
// git@host:path. ok is false for anything else (local paths).
func RepoHostPath(repoURL string) (host, path string, ok bool) {
	if rest, found := strings.CutPrefix(repoURL, "git@"); found {
		h, p, found := strings.Cut(rest, ":")
		if !found {
			return "", "", false
		}
		host, path = h, p
	} else {
		u, err := url.Parse(repoURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh") {
			return "", "", false
		}
		host, path = u.Hostname(), u.Path
	}
	host = strings.ToLower(host)
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return "", "", false
	}
	return host, path, true
}
