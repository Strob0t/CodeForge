package secrets_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/secrets"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"user and password", "nats://user:s3cret@nats:4222", "nats://[REDACTED]@nats:4222"},
		{"token only", "nats://t0ken@nats:4222", "nats://[REDACTED]@nats:4222"},
		{"postgres dsn with query", "postgresql://codeforge:pw@postgres:5432/codeforge?sslmode=require", "postgresql://[REDACTED]@postgres:5432/codeforge?sslmode=require"},
		{"password containing @ and :", "postgres://u:p@ss:w@rd@db:5432/x", "postgres://[REDACTED]@db:5432/x"},
		{"percent-encoded password", "postgres://u:p%2Fw%40d@db/x", "postgres://[REDACTED]@db/x"},
		{"https with token user", "https://ghp_abc@github.com/org/repo.git", "https://[REDACTED]@github.com/org/repo.git"},
		{"comma-separated server list", "nats://u:p@a:4222,nats://u:p@b:4222", "nats://[REDACTED]@a:4222,nats://[REDACTED]@b:4222"},
		{"embedded in a sentence", "dial nats://u:p@a:4222 failed", "dial nats://[REDACTED]@a:4222 failed"},
		{"no userinfo", "nats://nats:4222", "nats://nats:4222"},
		{"unencoded slash in password (base64)", "nats://u:ab/cd+ef==@nats:4222", "nats://[REDACTED]@nats:4222"},
		{"ipv6 host", "postgres://u:p@[::1]:5432/db", "postgres://[REDACTED]@[::1]:5432/db"},
		{"url at end of text", "server nats://u:p@nats", "server nats://[REDACTED]@nats"},
		{"go url.Error with quotes", `parse "nats://u:ab/cd+ef==@nats:4222": invalid port`, `parse "nats://[REDACTED]@nats:4222": invalid port`},
		{"single quotes", "dsn='postgres://u:p@db/x'", "dsn='postgres://[REDACTED]@db/x'"},
		{"followed by a paren", "(see nats://u:p@nats:4222)", "(see nats://[REDACTED]@nats:4222)"},
		{"followed by a full stop", "cannot reach nats://u:p@nats:4222.", "cannot reach nats://[REDACTED]@nats:4222."},
		{"followed by a semicolon", "a=nats://u:p@nats:4222;b=1", "a=nats://[REDACTED]@nats:4222;b=1"},
		{"inside brackets", "[nats://u:p@nats:4222]", "[nats://[REDACTED]@nats:4222]"},
		{"inside angle brackets", "<nats://u:p@nats:4222>", "<nats://[REDACTED]@nats:4222>"},
		{"host with underscore", "nats://u:p@nats_server:4222", "nats://[REDACTED]@nats_server:4222"},
		{"already redacted stays unchanged", "nats://[REDACTED]@nats:4222", "nats://[REDACTED]@nats:4222"},
		{"at sign in path is treated as userinfo (never under-redact)", "https://host/path@v1", "https://[REDACTED]@v1"},
		{"at sign in query is treated as userinfo (never under-redact)", "https://host/x?mail=a@b.c", "https://[REDACTED]@b.c"},
		{"scheme separator without scheme", "text ://u:p@host", "text ://u:p@host"},
		{"empty", "", ""},
		{"not a url", "just text", "just text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := secrets.RedactURL(tt.in); got != tt.want {
				t.Fatalf("RedactURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRedactURLWith: the MCP API shows redacted urls with "***" (KI-97
// review), which a url parser accepts in userinfo and query values.
func TestRedactURLWith(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://user:pw@mcp.example/sse", "https://***@mcp.example/sse"},
		{"https://tok@mcp.example/sse", "https://***@mcp.example/sse"},
		{"https://mcp.example/sse?api_key=sk-1&x=1", "https://mcp.example/sse?api_key=***&x=1"},
		{"https://u:p@mcp.example/sse?token=t&accessToken=a#frag", "https://***@mcp.example/sse?token=***&accessToken=***#frag"},
		{"https://mcp.example/sse?max_tokens=5", "https://mcp.example/sse?max_tokens=5"},
		{"https://mcp.example/sse?api_key=", "https://mcp.example/sse?api_key="},
		{"http://mcp.example/sse", "http://mcp.example/sse"},
	}
	for _, tt := range tests {
		if got := secrets.RedactURLWith(tt.in, "***"); got != tt.want {
			t.Errorf("RedactURLWith(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRedactURL_Stable: redacting a redacted string changes nothing, so a
// value read back can be compared with a fresh redaction.
func TestRedactURL_Stable(t *testing.T) {
	inputs := []string{
		"nats://user:s3cret@nats:4222", "https://ghp_abc@github.com/org/repo.git", "postgres://u:p@ss:w@rd@db:5432/x",
		"https://host/path@v1", "https://host/x?mail=a@b.c", "https://u:p@h/x?api_key=k&token=t&x=1#f",
		"nats://u:p@a:4222,nats://u:p@b:4222", "dsn='postgres://u:p@db/x'", "https://h/?token=", "just text", "",
	}
	for _, in := range inputs {
		once := secrets.RedactURL(in)
		if twice := secrets.RedactURL(once); twice != once {
			t.Errorf("RedactURL not stable for %q: %q, then %q", in, once, twice)
		}
		onceStars := secrets.RedactURLWith(in, "***")
		if twice := secrets.RedactURLWith(onceStars, "***"); twice != onceStars {
			t.Errorf("RedactURLWith not stable for %q: %q, then %q", in, onceStars, twice)
		}
	}
}

// TestRedactURL_LinearTime guards against pathological inputs: every log line
// passes through RedactURL.
func TestRedactURL_LinearTime(t *testing.T) {
	inputs := map[string]string{
		"scheme characters":          strings.Repeat("a", 100_000),
		"scheme characters with sep": strings.Repeat("a", 100_000) + "://",
		"many separators":            strings.Repeat("a://", 25_000),
		"long authority without at":  "x://" + strings.Repeat("b", 100_000),
		"many at signs":              "x://" + strings.Repeat("@", 100_000),
		"many urls":                  strings.Repeat("n://u:p@h ", 10_000),
	}
	for name, in := range inputs {
		start := time.Now()
		secrets.RedactURL(in)
		if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
			t.Errorf("%s: %v for %d bytes", name, elapsed, len(in))
		}
	}
}
