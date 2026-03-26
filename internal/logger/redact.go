package logger

import (
	"context"
	"log/slog"
	"regexp"
)

// sensitivePatterns matches common secret and PII formats in log output.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9_-]{20,})`),            // API keys (OpenAI, LiteLLM)
	regexp.MustCompile(`(?i)(sk-ant-[a-zA-Z0-9_-]{20,})`),        // Anthropic API keys
	regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36})`),              // GitHub personal access tokens
	regexp.MustCompile(`(?i)(github_pat_[a-zA-Z0-9_]{36,})`),     // GitHub fine-grained PATs
	regexp.MustCompile(`(?i)(gsk_[a-zA-Z0-9]{20,})`),             // Groq API keys
	regexp.MustCompile(`(?i)(hf_[a-zA-Z0-9]{20,})`),              // HuggingFace tokens
	regexp.MustCompile(`(?i)(AIza[a-zA-Z0-9_-]{30,})`),           // Google API keys
	regexp.MustCompile(`(?i)(password|passwd|secret)=\S+`),       // key=value secrets
	regexp.MustCompile(`(?i)([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+)`), // Email addresses
}

const redacted = "[REDACTED]"

// RedactHandler wraps an inner slog.Handler and redacts sensitive patterns
// from log messages and string attributes before forwarding.
type RedactHandler struct {
	inner slog.Handler
}

// NewRedactHandler creates a handler that redacts PII/secrets from log output.
func NewRedactHandler(inner slog.Handler) *RedactHandler {
	return &RedactHandler{inner: inner}
}

func (h *RedactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	r.Message = redactString(r.Message)

	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, redactAttr(a))
		return true
	})

	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	for _, a := range attrs {
		newRecord.AddAttrs(a)
	}
	return h.inner.Handle(ctx, newRecord)
}

func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = redactAttr(a)
	}
	return &RedactHandler{inner: h.inner.WithAttrs(redacted)}
}

func (h *RedactHandler) WithGroup(name string) slog.Handler {
	return &RedactHandler{inner: h.inner.WithGroup(name)}
}

// redactString replaces sensitive patterns in a string with [REDACTED].
func redactString(s string) string {
	for _, p := range sensitivePatterns {
		s = p.ReplaceAllString(s, redacted)
	}
	return s
}

// redactAttr redacts string-valued attributes; other types pass through.
func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		a.Value = slog.StringValue(redactString(a.Value.String()))
	}
	if a.Value.Kind() == slog.KindGroup {
		groupAttrs := a.Value.Group()
		redactedGroup := make([]slog.Attr, len(groupAttrs))
		for i, ga := range groupAttrs {
			redactedGroup[i] = redactAttr(ga)
		}
		a.Value = slog.GroupValue(redactedGroup...)
	}
	return a
}
