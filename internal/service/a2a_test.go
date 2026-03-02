package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// mockStoreForA2A extends mockStore with A2A-specific behaviour.
type mockStoreForA2A struct {
	mockStore

	remoteAgents   []a2adomain.RemoteAgent
	createdAgents  []*a2adomain.RemoteAgent
	a2aTasks       []a2adomain.A2ATask
	createAgentErr error
	getAgentErr    error
	deleteAgentErr error
}

func (m *mockStoreForA2A) CreateRemoteAgent(_ context.Context, a *a2adomain.RemoteAgent) error {
	if m.createAgentErr != nil {
		return m.createAgentErr
	}
	a.ID = "ra-1"
	m.createdAgents = append(m.createdAgents, a)
	return nil
}

func (m *mockStoreForA2A) GetRemoteAgent(_ context.Context, id string) (*a2adomain.RemoteAgent, error) {
	if m.getAgentErr != nil {
		return nil, m.getAgentErr
	}
	for i := range m.remoteAgents {
		if m.remoteAgents[i].ID == id {
			return &m.remoteAgents[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockStoreForA2A) ListRemoteAgents(_ context.Context, _ string, _ bool) ([]a2adomain.RemoteAgent, error) {
	return m.remoteAgents, nil
}

func (m *mockStoreForA2A) UpdateRemoteAgent(_ context.Context, _ *a2adomain.RemoteAgent) error {
	return nil
}

func (m *mockStoreForA2A) DeleteRemoteAgent(_ context.Context, id string) error {
	if m.deleteAgentErr != nil {
		return m.deleteAgentErr
	}
	for i := range m.remoteAgents {
		if m.remoteAgents[i].ID == id {
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (m *mockStoreForA2A) ListA2ATasks(_ context.Context, _ *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	return m.a2aTasks, len(m.a2aTasks), nil
}

// newMockAgentCardServer returns a test HTTP server that serves a minimal AgentCard.
func newMockAgentCardServer() *httptest.Server {
	card := map[string]any{
		"name":        "TestAgent",
		"description": "A test agent",
		"url":         "http://localhost:9999",
		"version":     "0.1.0",
		"skills": []map[string]any{
			{"id": "code", "name": "Code", "description": "Write code"},
		},
		"defaultInputModes":  []string{"text"},
		"defaultOutputModes": []string{"text"},
		"capabilities":       map[string]any{},
	}
	data, _ := json.Marshal(card)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-card.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	return httptest.NewServer(mux)
}

func TestA2AService_RegisterRemoteAgent(t *testing.T) {
	srv := newMockAgentCardServer()
	defer srv.Close()

	ms := &mockStoreForA2A{}
	svc := NewA2AService(ms, nil)

	ra, err := svc.RegisterRemoteAgent(context.Background(), "test-agent", srv.URL, "partial")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ra.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", ra.Name)
	}
	if ra.TrustLevel != "partial" {
		t.Errorf("expected trust_level partial, got %s", ra.TrustLevel)
	}
	if len(ms.createdAgents) != 1 {
		t.Fatalf("expected 1 created agent, got %d", len(ms.createdAgents))
	}
}

func TestA2AService_RegisterRemoteAgent_InvalidURL(t *testing.T) {
	ms := &mockStoreForA2A{}
	svc := NewA2AService(ms, nil)

	_, err := svc.RegisterRemoteAgent(context.Background(), "test", "", "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestA2AService_RegisterRemoteAgent_MissingName(t *testing.T) {
	ms := &mockStoreForA2A{}
	svc := NewA2AService(ms, nil)

	_, err := svc.RegisterRemoteAgent(context.Background(), "", "http://example.com", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestA2AService_RefreshAgent(t *testing.T) {
	srv := newMockAgentCardServer()
	defer srv.Close()

	ms := &mockStoreForA2A{
		remoteAgents: []a2adomain.RemoteAgent{
			{ID: "ra-1", Name: "test", URL: srv.URL, Skills: []string{}},
		},
	}
	svc := NewA2AService(ms, nil)

	ra, err := svc.RefreshAgent(context.Background(), "ra-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ra.LastSeen == nil {
		t.Error("expected LastSeen to be set")
	}
}

func TestA2AService_RefreshAgent_NotFound(t *testing.T) {
	ms := &mockStoreForA2A{getAgentErr: fmt.Errorf("not found")}
	svc := NewA2AService(ms, nil)

	_, err := svc.RefreshAgent(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestA2AService_ListRemoteAgents(t *testing.T) {
	ms := &mockStoreForA2A{
		remoteAgents: []a2adomain.RemoteAgent{
			{ID: "ra-1", Name: "agent1"},
			{ID: "ra-2", Name: "agent2"},
		},
	}
	svc := NewA2AService(ms, nil)

	agents, err := svc.ListRemoteAgents(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestA2AService_DeleteRemoteAgent(t *testing.T) {
	ms := &mockStoreForA2A{
		remoteAgents: []a2adomain.RemoteAgent{
			{ID: "ra-1", Name: "agent1"},
		},
	}
	svc := NewA2AService(ms, nil)

	if err := svc.DeleteRemoteAgent(context.Background(), "ra-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestA2AService_DeleteRemoteAgent_NotFound(t *testing.T) {
	ms := &mockStoreForA2A{deleteAgentErr: fmt.Errorf("not found")}
	svc := NewA2AService(ms, nil)

	err := svc.DeleteRemoteAgent(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestA2AService_DiscoverAgent(t *testing.T) {
	srv := newMockAgentCardServer()
	defer srv.Close()

	svc := NewA2AService(&mockStoreForA2A{}, nil)
	card, err := svc.DiscoverAgent(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if card.Name != "TestAgent" {
		t.Errorf("expected name TestAgent, got %s", card.Name)
	}
}
