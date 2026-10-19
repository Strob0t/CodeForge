package secrets_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/secrets"
)

// TestRedactURLField (KI-97 security review): one URL value (the url of an
// MCP server) is redacted as net/url reads it. The free-text scanner of
// RedactURL stops at quotes, brackets and spaces, which net/url accepts in
// userinfo and query values, and it lets an "@" in the path or query move
// the host that is shown.
func TestRedactURLField(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"user and password", "https://svc:pw@mcp.example/sse", "https://***@mcp.example/sse"},
		{"parentheses in the password", "https://svc:p(w)d@mcp.example/sse", "https://***@mcp.example/sse"},
		{"quote in the password", "https://svc:p'wd@mcp.example/sse", "https://***@mcp.example/sse"},
		{"space in the password", "https://svc:p wd@mcp.example/sse", "https://***@mcp.example/sse"},
		{"brackets and braces in the password", "https://svc:p[w]{d}<x>@mcp.example/sse", "https://***@mcp.example/sse"},
		{"encoded @ in the password", "https://svc:p%40wd@mcp.example/sse", "https://***@mcp.example/sse"},
		{"raw @ in the password", "https://svc:p@wd@mcp.example:8443/sse", "https://***@mcp.example:8443/sse"},
		{"token as the user", "https://ghp_tok@mcp.example/sse", "https://***@mcp.example/sse"},
		{"empty userinfo", "https://@mcp.example/sse", "https://***@mcp.example/sse"},
		{"query credential", "https://mcp.example/sse?api_key=sk-1&x=1", "https://mcp.example/sse?api_key=***&x=1"},
		{"parentheses in a query value", "https://mcp.example/sse?api_key=sk-ab(cd)ef", "https://mcp.example/sse?api_key=***"},
		{"quote and space in a query value", "https://mcp.example/sse?token=a'b c&x=1", "https://mcp.example/sse?token=***&x=1"},
		{"repeated keys", "https://mcp.example/sse?token=a&token=b&token=", "https://mcp.example/sse?token=***&token=***&token="},
		{"encoded credential name", "https://mcp.example/sse?api%5Fkey=sk-1", "https://mcp.example/sse?api%5Fkey=***"},
		{"other parameters byte-identical", "https://mcp.example/sse?q=a%20b&max_tokens=5&flag", "https://mcp.example/sse?q=a%20b&max_tokens=5&flag"},
		{"fragment with a credential pair", "https://mcp.example/sse#access_token=t&state=s", "https://mcp.example/sse#access_token=***&state=s"},
		{"fragment without pairs", "https://mcp.example/sse#section-2", "https://mcp.example/sse#section-2"},
		{"no userinfo, no query", "http://mcp.example:6280/sse", "http://mcp.example:6280/sse"},
		{"ipv6 host", "http://u:p@[fd00::1]:6280/sse", "http://***@[fd00::1]:6280/sse"},
		{"unparsable url", "https://user:p%zz@mcp.example/sse?token=t", "https://***@mcp.example/sse?token=***"},
		{"no scheme", "user:pw@mcp.example/sse", "***@mcp.example/sse"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := secrets.RedactURLField(tt.in, "***")
			if got != tt.want {
				t.Fatalf("RedactURLField(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if again := secrets.RedactURLField(got, "***"); again != got {
				t.Fatalf("not stable: %q, then %q", got, again)
			}
		})
	}
}

// TestRedactURLField_ShowsTheHostConnectedTo (KI-97 security review): an
// "@" in the path, query or fragment never makes another host appear.
func TestRedactURLField_ShowsTheHostConnectedTo(t *testing.T) {
	for _, in := range []string{
		"https://evil.example/x?r=a@trusted.corp",
		"https://evil.example/a@trusted.corp/sse",
		"https://evil.example/sse#a@trusted.corp",
		"https://u:p@evil.example/x?r=a@trusted.corp#b@other.corp",
	} {
		got := secrets.RedactURLField(in, "***")
		want, err := url.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		shown, err := url.Parse(got)
		if err != nil {
			t.Fatalf("RedactURLField(%q) = %q does not parse: %v", in, got, err)
		}
		if shown.Host != want.Host || strings.Contains(shown.Host, "trusted") {
			t.Errorf("RedactURLField(%q) = %q shows host %q, want %q", in, got, shown.Host, want.Host)
		}
	}
}

func TestURLFieldSecrets(t *testing.T) {
	got := secrets.URLFieldSecrets("https://svc:p%40wd@mcp.example/sse?api_key=sk-a%2Bb&x=1&token=t#access_token=frag")
	for _, want := range []string{"svc:p%40wd", "p%40wd", "p@wd", "sk-a%2Bb", "sk-a+b", "t", "frag"} {
		if !slices.Contains(got, want) {
			t.Errorf("URLFieldSecrets misses %q: %q", want, got)
		}
	}
	for _, public := range []string{"mcp.example", "1", "svc"} {
		if slices.Contains(got, public) {
			t.Errorf("URLFieldSecrets lists the public part %q: %q", public, got)
		}
	}
}
