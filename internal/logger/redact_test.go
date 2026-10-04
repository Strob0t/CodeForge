package logger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
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

type urlStringer struct{ u string }

func (s urlStringer) String() string { return "endpoint " + s.u }

type urlValuer struct{ u string }

func (v urlValuer) LogValue() slog.Value { return slog.StringValue(v.u) }

func TestRedactHandler_NonStringValues(t *testing.T) {
	tokenURL := "https://ghp_tok123@github.com/org/repo.git"
	tests := []struct {
		name  string
		attrs []any
	}{
		{"error value", []any{"error", &url.Error{Op: "Get", URL: tokenURL, Err: io.EOF}}},
		{"wrapped error", []any{"error", fmt.Errorf("clone failed: %w", &url.Error{Op: "Get", URL: tokenURL, Err: io.EOF})}},
		{"fmt.Stringer", []any{"target", urlStringer{u: tokenURL}}},
		{"LogValuer", []any{"target", urlValuer{u: tokenURL}}},
		{"error inside a group", []any{slog.Group("req", slog.Any("error", errors.New("dial "+tokenURL)))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := logThroughRedactHandler(t, "x", tt.attrs...)
			if strings.Contains(out, "ghp_tok123") {
				t.Fatalf("output leaks the token: %s", out)
			}
			if !strings.Contains(out, "[REDACTED]@github.com") {
				t.Fatalf("output lost the host: %s", out)
			}
		})
	}
}

type ptrStringer struct{ u string }

func (p *ptrStringer) String() string { return p.u }

func TestRedactHandler_PanickingStringerIsLeftToTheInnerHandler(t *testing.T) {
	var nilStringer *ptrStringer
	out := logThroughRedactHandler(t, "x", "target", nilStringer)
	if !strings.Contains(out, `"msg":"x"`) {
		t.Fatalf("record was not written: %s", out)
	}
}

func TestRedactHandler_OtherValuesUnchanged(t *testing.T) {
	out := logThroughRedactHandler(t, "x", "count", 42, "ids", []int{1, 2}, "ok", true)
	for _, want := range []string{`"count":42`, `"ids":[1,2]`, `"ok":true`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %s lacks %s", out, want)
		}
	}
}
