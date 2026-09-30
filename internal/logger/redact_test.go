package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func logThroughRedactHandler(t *testing.T, msg string, attrs ...any) string {
	t.Helper()
	var buf bytes.Buffer
	l := slog.New(NewRedactHandler(slog.NewJSONHandler(&buf, nil)))
	l.Info(msg, attrs...)
	return buf.String()
}

func TestRedactHandler_URLUserinfo(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		attrs    []any
		want     string
		mustMiss []string
	}{
		{
			name:     "nats url attribute keeps the host",
			msg:      "nats connected",
			attrs:    []any{"url", "nats://codeforge-1a2b:hexpass123@nats:4222"},
			want:     "nats://[REDACTED]@nats:4222",
			mustMiss: []string{"hexpass123", "codeforge-1a2b"},
		},
		{
			name:     "dsn in the message",
			msg:      "connecting to postgresql://codeforge:pw-123@postgres:5432/codeforge",
			want:     "postgresql://[REDACTED]@postgres:5432/codeforge",
			mustMiss: []string{"pw-123"},
		},
		{
			name:     "password with characters outside the email pattern",
			msg:      "x",
			attrs:    []any{"url", "nats://u:p/w=x@nats:4222"},
			want:     "",
			mustMiss: []string{"p/w=x", "u:p"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := logThroughRedactHandler(t, tt.msg, tt.attrs...)
			if tt.want != "" && !strings.Contains(out, tt.want) {
				t.Fatalf("output %s does not contain %q", out, tt.want)
			}
			for _, s := range tt.mustMiss {
				if strings.Contains(out, s) {
					t.Fatalf("output %s leaks %q", out, s)
				}
			}
		})
	}
}

func TestRedactHandler_EmailStillRedacted(t *testing.T) {
	out := logThroughRedactHandler(t, "login", "who", "alice@example.com")
	if strings.Contains(out, "alice@example.com") {
		t.Fatalf("email not redacted: %s", out)
	}
}
