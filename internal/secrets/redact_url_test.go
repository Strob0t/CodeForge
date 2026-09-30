package secrets_test

import (
	"testing"

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
		{"at sign in path is treated as userinfo (never under-redact)", "https://host/path@v1", "https://[REDACTED]@v1"},
		{"at sign only in query", "https://host/x?mail=a@b.c", "https://host/x?mail=a@b.c"},
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
