package project

import "testing"

// S3-F review C3: webhooks find their project by the exact repository
// (host and full path, GitLab subgroups included), not by a substring.
func TestRepoHostPath(t *testing.T) {
	tests := []struct {
		url, host, path string
		ok              bool
	}{
		{"https://github.com/acme/app", "github.com", "acme/app", true},
		{"https://github.com/acme/app.git", "github.com", "acme/app", true},
		{"https://GitHub.com/acme/app/", "github.com", "acme/app", true},
		{"http://gitlab.example.com:8080/group/sub/app.git", "gitlab.example.com", "group/sub/app", true},
		{"git@github.com:acme/app.git", "github.com", "acme/app", true},
		{"ssh://git@gitlab.example.com:2222/group/app.git", "gitlab.example.com", "group/app", true},
		{"https://github.com/", "", "", false},
		{"/srv/repos/app", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range tests {
		host, path, ok := RepoHostPath(tc.url)
		if host != tc.host || path != tc.path || ok != tc.ok {
			t.Errorf("RepoHostPath(%q) = %q, %q, %v; want %q, %q, %v", tc.url, host, path, ok, tc.host, tc.path, tc.ok)
		}
	}
}
