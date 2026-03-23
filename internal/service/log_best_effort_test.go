package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
)

func TestLogBestEffort_NilError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	logBestEffort(context.Background(), nil, "UpdateAgentStatus")

	if buf.Len() != 0 {
		t.Errorf("expected no log output for nil error, got: %s", buf.String())
	}
}

func TestLogBestEffort_NonNilError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	old := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(old)

	logBestEffort(context.Background(), errors.New("connection refused"), "UpdateAgentStatus",
		slog.String("agent_id", "a1"))

	out := buf.String()
	if out == "" {
		t.Fatal("expected log output for non-nil error")
	}
	for _, want := range []string{"best-effort", "UpdateAgentStatus", "connection refused", "agent_id", "a1"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("expected log to contain %q, got: %s", want, out)
		}
	}
}
