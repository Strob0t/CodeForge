package agentbackend_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
)

type testBackend struct {
	name string
}

func (b *testBackend) Name() string { return b.name }
func (b *testBackend) Capabilities() agentbackend.Capabilities {
	return agentbackend.Capabilities{Edit: true}
}
func (b *testBackend) Execute(_ context.Context, _ *task.Task) (*task.Result, error) { return nil, nil }
func (b *testBackend) Stop(_ context.Context, _ string) error                        { return nil }

func TestRegisterAndNew(t *testing.T) {
	agentbackend.Register("test-agent", func(_ map[string]string) (agentbackend.Backend, error) {
		return &testBackend{name: "test-agent"}, nil
	})

	b, err := agentbackend.New("test-agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name() != "test-agent" {
		t.Fatalf("expected test-agent, got %s", b.Name())
	}
}

func TestNewUnknownBackend(t *testing.T) {
	_, err := agentbackend.New("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestAvailable(t *testing.T) {
	names := agentbackend.Available()
	found := false
	for _, n := range names {
		if n == "test-agent" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected test-agent in available backends")
	}
}
