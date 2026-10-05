package githubapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/netutil"
)

func loopbackClient(t *testing.T, baseURL, token string) *Client {
	t.Helper()
	policy, err := netutil.NewOutboundPolicy([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(baseURL, token, NewHTTPClient(policy))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRepoPath(t *testing.T) {
	tests := []struct {
		repo, want string
	}{
		{"owner/repo", "/repos/owner/repo"},
		{"org/my-project", "/repos/org/my-project"},
		{"Org_1/my.project_2", "/repos/Org_1/my.project_2"},
		{"o/.github", "/repos/o/.github"},
		{"", ""},
		{"noslash", ""},
		{"/repo", ""},
		{"owner/", ""},
		{"a/b/c", ""},
		{"owner/..", ""},
		{"../repo", ""},
		{"owner/.", ""},
		{"owner/re po", ""},
		{"owner/repo?x=1", ""},
		{"owner/repo#1", ""},
		{"owner/%2e%2e", ""},
		{"owner/repo\n", ""},
		{"owner/" + strings.Repeat("r", 101), ""},
	}
	for _, tt := range tests {
		got, err := RepoPath(tt.repo)
		if tt.want == "" {
			if !errors.Is(err, domain.ErrValidation) {
				t.Errorf("RepoPath(%q) = %q, %v; want ErrValidation", tt.repo, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("RepoPath(%q) = %q, %v; want %q", tt.repo, got, err, tt.want)
		}
	}
}

func TestNewClient_BaseURL(t *testing.T) {
	for _, base := range []string{"", "https://api.github.com", "https://ghe.example.com/api/v3/", "http://127.0.0.1:8080"} {
		if _, err := NewClient(base, "", http.DefaultClient); err != nil {
			t.Errorf("NewClient(%q): %v", base, err)
		}
	}
	for _, base := range []string{"ftp://api.github.com", "api.github.com", "https://user:pw@api.github.com", "https://api.github.com?x=1", "https://"} {
		if _, err := NewClient(base, "", http.DefaultClient); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("NewClient(%q) = %v, want ErrValidation", base, err)
		}
	}
}

// The token goes only to the API's origin: a redirect elsewhere is not
// followed, and the error says so.
func TestDo_RedirectToAnotherOriginIsNotFollowed(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization") != ""
	}))
	defer other.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/steal", http.StatusFound)
	}))
	defer api.Close()

	_, err := loopbackClient(t, api.URL, "ghp_secret").Do(t.Context(), http.MethodGet, "/repos/o/r", nil)
	if err == nil || leaked {
		t.Fatalf("err %v, token sent elsewhere %v", err, leaked)
	}
	if !strings.Contains(err.Error(), "302") || strings.Contains(err.Error(), "ghp_secret") {
		t.Fatalf("error %q", err)
	}
}

func TestDo_RefusedAddress(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("the public-only client connected to a loopback address")
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()
	c, err := NewClient(api.URL, "ghp_secret", PublicHTTPClient())
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Do(t.Context(), http.MethodGet, "/repos/o/r", nil)
	if !errors.Is(err, netutil.ErrAddressRefused) {
		t.Fatalf("err %v, want ErrAddressRefused", err)
	}
}

func TestDo_LargeAnswerAndLongMessage(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"message":"`+strings.Repeat("m", 2000)+`"}`)
	}))
	defer api.Close()
	c := loopbackClient(t, api.URL, "")

	if _, err := c.Do(t.Context(), http.MethodGet, "/large", nil); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("large answer: %v", err)
	}
	_, err := c.Do(t.Context(), http.MethodPost, "/long", []byte(`{}`))
	if !errors.Is(err, domain.ErrValidation) || len(err.Error()) > maxMessageRunes+200 {
		t.Fatalf("long message: %d bytes: %v", len(err.Error()), err)
	}
}

func TestNextLink(t *testing.T) {
	c, err := NewClient("https://api.github.com", "", http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		header, want string
	}{
		{`<https://api.github.com/repositories/1/issues?page=2>; rel="next", <https://api.github.com/repositories/1/issues?page=5>; rel="last"`,
			"https://api.github.com/repositories/1/issues?page=2"},
		{`<https://api.github.com/x?page=1>; rel="prev", <https://API.github.com/x?page=3>; rel="next"`, "https://API.github.com/x?page=3"},
		{`<https://api.github.com/x?page=5>; rel="last"`, ""},
		{`<https://evil.example/x?page=2>; rel="next"`, ""},
		{`<http://api.github.com/x?page=2>; rel="next"`, ""},
		{`<https://u:p@api.github.com/x?page=2>; rel="next"`, ""},
		{`https://api.github.com/x?page=2; rel="next"`, ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := c.nextLink(tt.header); got != tt.want {
			t.Errorf("nextLink(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
