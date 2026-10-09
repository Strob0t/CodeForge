package svn

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// KI-189 (R3-14): local (file://) repositories are allowed by the operator
// (svn.allow_file_urls, SetAllowFileURLs), never by the project config,
// which project editors set: such a URL may come from the agent-writable
// wc.db and read other repositories on the Go Core host.
func TestFileURLsOnlyByOperator(t *testing.T) {
	t.Cleanup(func() { SetAllowFileURLs(false) })
	newProvider := func(cfg map[string]string) *Provider {
		t.Helper()
		gp, err := gitprovider.New(providerName, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return gp.(*Provider)
	}
	const local = "file:///srv/svn/repo"

	if err := newProvider(map[string]string{"allow_file_urls": "true"}).checkRepositoryURL(local); err == nil {
		t.Fatal("the project config key allow_file_urls allowed a file:// repository")
	}
	SetAllowFileURLs(true)
	if err := newProvider(nil).checkRepositoryURL(local); err != nil {
		t.Fatalf("svn.allow_file_urls on: %v", err)
	}
	SetAllowFileURLs(false)
	if err := newProvider(map[string]string{"allow_file_urls": "true"}).checkRepositoryURL(local); err == nil {
		t.Fatal("svn.allow_file_urls off: a file:// repository was allowed")
	}
}
