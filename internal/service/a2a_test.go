package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// mockStoreForA2A extends mockStore with A2A-specific behaviour.
type mockStoreForA2A struct {
	mockStore

	remoteAgents    []a2adomain.RemoteAgent
	createdAgents   []*a2adomain.RemoteAgent
	a2aTasks        []a2adomain.A2ATask
	pushConfigs     []database.A2APushConfig
	createAgentErr  error
	getAgentErr     error
	deleteAgentErr  error
	pushConfigIDSeq int
	updatedTasks    []*a2adomain.A2ATask
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

func (m *mockStoreForA2A) CreateA2ATask(_ context.Context, t *a2adomain.A2ATask) error {
	m.a2aTasks = append(m.a2aTasks, *t)
	return nil
}

func (m *mockStoreForA2A) GetA2ATask(_ context.Context, id string) (*a2adomain.A2ATask, error) {
	for i := range m.a2aTasks {
		if m.a2aTasks[i].ID == id {
			return &m.a2aTasks[i], nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockStoreForA2A) UpdateA2ATask(_ context.Context, t *a2adomain.A2ATask) error {
	m.updatedTasks = append(m.updatedTasks, t)
	for i := range m.a2aTasks {
		if m.a2aTasks[i].ID == t.ID {
			m.a2aTasks[i] = *t
			return nil
		}
	}
	return nil
}

func (m *mockStoreForA2A) CreateA2APushConfig(_ context.Context, taskID, url, token string) (string, error) {
	m.pushConfigIDSeq++
	id := fmt.Sprintf("pc-%d", m.pushConfigIDSeq)
	m.pushConfigs = append(m.pushConfigs, database.A2APushConfig{
		ID: id, TaskID: taskID, URL: url, Token: token,
	})
	return id, nil
}

func (m *mockStoreForA2A) ListA2APushConfigs(_ context.Context, taskID string) ([]database.A2APushConfig, error) {
	var result []database.A2APushConfig
	for _, c := range m.pushConfigs {
		if c.TaskID == taskID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockStoreForA2A) DeleteA2APushConfig(_ context.Context, id string) error {
	for i, c := range m.pushConfigs {
		if c.ID == id {
			m.pushConfigs = append(m.pushConfigs[:i], m.pushConfigs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (m *mockStoreForA2A) GetA2APushConfig(_ context.Context, _ string) (_, _, _ string, _ error) {
	return "", "", "", nil
}

func (m *mockStoreForA2A) DeleteAllA2APushConfigs(_ context.Context, _ string) error { return nil }

// newMockAgentCardServer returns a test HTTP server that serves a minimal AgentCard.
func newMockAgentCardServer() *httptest.Server {
	card := map[string]any{
		"name":        "TestAgent",
		"description": "A test agent",
		"url":         "http://localhost:9999",
		"version":     "0.8.0",
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

// --- Push Config Service Tests (Phase 27O) ---

func TestA2AService_CreatePushConfig(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{
			*a2adomain.NewA2ATask("t-1"),
		},
	}
	svc := NewA2AService(ms, nil)

	id, err := svc.CreatePushConfig(context.Background(), "t-1", "https://example.com/hook", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty push config ID")
	}
	if len(ms.pushConfigs) != 1 {
		t.Errorf("expected 1 push config, got %d", len(ms.pushConfigs))
	}
}

func TestA2AService_CreatePushConfig_EmptyTaskID(t *testing.T) {
	svc := NewA2AService(&mockStoreForA2A{}, nil)
	_, err := svc.CreatePushConfig(context.Background(), "", "https://example.com", "")
	if err == nil {
		t.Fatal("expected error for empty task_id")
	}
}

func TestA2AService_CreatePushConfig_EmptyURL(t *testing.T) {
	svc := NewA2AService(&mockStoreForA2A{}, nil)
	_, err := svc.CreatePushConfig(context.Background(), "t-1", "", "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestA2AService_CreatePushConfig_TaskNotFound(t *testing.T) {
	svc := NewA2AService(&mockStoreForA2A{}, nil)
	_, err := svc.CreatePushConfig(context.Background(), "missing", "https://example.com", "")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestA2AService_ListPushConfigs(t *testing.T) {
	ms := &mockStoreForA2A{
		pushConfigs: []database.A2APushConfig{
			{ID: "pc-1", TaskID: "t-1", URL: "https://a.com"},
			{ID: "pc-2", TaskID: "t-1", URL: "https://b.com"},
		},
	}
	svc := NewA2AService(ms, nil)

	configs, err := svc.ListPushConfigs(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(configs))
	}
}

func TestA2AService_DeletePushConfig(t *testing.T) {
	ms := &mockStoreForA2A{
		pushConfigs: []database.A2APushConfig{
			{ID: "pc-1", TaskID: "t-1", URL: "https://a.com"},
		},
	}
	svc := NewA2AService(ms, nil)

	err := svc.DeletePushConfig(context.Background(), "pc-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ms.pushConfigs) != 0 {
		t.Errorf("expected 0 configs after delete, got %d", len(ms.pushConfigs))
	}
}

func TestA2AService_HandleTaskComplete(t *testing.T) {
	task := a2adomain.NewA2ATask("t-1")
	task.State = a2adomain.TaskStateWorking
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*task},
	}
	svc := NewA2AService(ms, nil)

	err := svc.HandleTaskComplete(context.Background(), "t-1", "completed", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ms.updatedTasks) == 0 {
		t.Fatal("expected task to be updated")
	}
	if ms.updatedTasks[0].State != a2adomain.TaskStateCompleted {
		t.Errorf("expected state completed, got %s", ms.updatedTasks[0].State)
	}
}

func TestA2AService_HandleTaskComplete_Failed(t *testing.T) {
	task := a2adomain.NewA2ATask("t-1")
	task.State = a2adomain.TaskStateWorking
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*task},
	}
	svc := NewA2AService(ms, nil)

	err := svc.HandleTaskComplete(context.Background(), "t-1", "failed", "timeout exceeded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ms.updatedTasks[0].ErrorMessage != "timeout exceeded" {
		t.Errorf("expected error message 'timeout exceeded', got %q", ms.updatedTasks[0].ErrorMessage)
	}
}

func TestA2AService_HandleTaskComplete_NotFound(t *testing.T) {
	svc := NewA2AService(&mockStoreForA2A{}, nil)
	err := svc.HandleTaskComplete(context.Background(), "missing", "completed", "")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestA2AService_DispatchPushNotifications_NoConfigs(t *testing.T) {
	task := a2adomain.NewA2ATask("t-1")
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*task},
	}
	svc := NewA2AService(ms, nil)

	// Should not panic with no push configs.
	svc.DispatchPushNotifications(context.Background(), "t-1")
}

// --- Security Hardening Tests (Phase 27P) ---

func TestA2AService_SendTask_PromptTooLong(t *testing.T) {
	ms := &mockStoreForA2A{
		remoteAgents: []a2adomain.RemoteAgent{
			{ID: "ra-1", Name: "test", URL: "http://localhost:9999"},
		},
	}
	svc := NewA2AService(ms, nil)

	longPrompt := make([]byte, MaxA2APromptLength+1)
	for i := range longPrompt {
		longPrompt[i] = 'a'
	}

	_, err := svc.SendTask(context.Background(), "ra-1", "code", string(longPrompt))
	if err == nil {
		t.Fatal("expected error for prompt exceeding max length")
	}
	if !strings.Contains(err.Error(), "prompt exceeds maximum length") {
		t.Errorf("expected prompt length error, got %v", err)
	}
}

func TestA2AService_CreatePushConfig_InvalidURL_NotHTTPS(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*a2adomain.NewA2ATask("t-1")},
	}
	svc := NewA2AService(ms, nil)

	_, err := svc.CreatePushConfig(context.Background(), "t-1", "http://external.example.com/hook", "")
	if err == nil {
		t.Fatal("expected error for non-https URL")
	}
	if !strings.Contains(err.Error(), "must use https") {
		t.Errorf("expected https error, got %v", err)
	}
}

func TestA2AService_CreatePushConfig_AllowHTTPLocalhost(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*a2adomain.NewA2ATask("t-1")},
	}
	svc := NewA2AService(ms, nil)

	id, err := svc.CreatePushConfig(context.Background(), "t-1", "http://localhost:8080/hook", "secret")
	if err != nil {
		t.Fatalf("expected http://localhost to be allowed, got error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty push config ID")
	}
}

func TestA2AService_CreatePushConfig_AllowHTTPLoopback(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*a2adomain.NewA2ATask("t-1")},
	}
	svc := NewA2AService(ms, nil)

	_, err := svc.CreatePushConfig(context.Background(), "t-1", "http://127.0.0.1:8080/hook", "secret")
	if err != nil {
		t.Fatalf("expected http://127.0.0.1 to be allowed, got error: %v", err)
	}
}

func TestA2AService_CreatePushConfig_BlockPrivateIP(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*a2adomain.NewA2ATask("t-1")},
	}
	svc := NewA2AService(ms, nil)

	privateURLs := []string{
		"https://10.0.0.1/hook",
		"https://192.168.1.1/hook",
		"https://172.16.0.1/hook",
	}

	for _, u := range privateURLs {
		_, err := svc.CreatePushConfig(context.Background(), "t-1", u, "")
		if err == nil {
			t.Errorf("expected error for private IP URL %s", u)
		}
	}
}

func TestA2AService_CreatePushConfig_BlockFTPScheme(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*a2adomain.NewA2ATask("t-1")},
	}
	svc := NewA2AService(ms, nil)

	_, err := svc.CreatePushConfig(context.Background(), "t-1", "ftp://example.com/hook", "")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestA2AService_CreatePushConfig_AllowHTTPS(t *testing.T) {
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*a2adomain.NewA2ATask("t-1")},
	}
	svc := NewA2AService(ms, nil)

	id, err := svc.CreatePushConfig(context.Background(), "t-1", "https://webhook.example.com/hook", "secret")
	if err != nil {
		t.Fatalf("expected https URL to be allowed, got error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty push config ID")
	}
}

func TestComputeHMAC(t *testing.T) {
	payload := []byte(`{"task_id":"t-1","state":"completed"}`)
	key := "my-secret-key"

	sig := computeHMAC(payload, key)
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("expected sha256= prefix, got %s", sig)
	}
	// Verify deterministic: same inputs → same output.
	sig2 := computeHMAC(payload, key)
	if sig != sig2 {
		t.Error("HMAC should be deterministic")
	}
	// Different key → different signature.
	sig3 := computeHMAC(payload, "other-key")
	if sig == sig3 {
		t.Error("different keys should produce different signatures")
	}
}

func TestA2AService_DispatchPushNotifications_WithWebhook(t *testing.T) {
	received := make(chan bool, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer my-token" {
			t.Errorf("expected Bearer my-token, got %q", r.Header.Get("Authorization"))
		}
		// Verify HMAC signature is present when token is set.
		sig := r.Header.Get("X-CodeForge-Signature")
		if !strings.HasPrefix(sig, "sha256=") {
			t.Errorf("expected X-CodeForge-Signature with sha256= prefix, got %q", sig)
		}
		w.WriteHeader(http.StatusOK)
		received <- true
	}))
	defer webhookSrv.Close()

	task := a2adomain.NewA2ATask("t-1")
	task.State = a2adomain.TaskStateCompleted
	ms := &mockStoreForA2A{
		a2aTasks: []a2adomain.A2ATask{*task},
		pushConfigs: []database.A2APushConfig{
			{ID: "pc-1", TaskID: "t-1", URL: webhookSrv.URL, Token: "my-token"},
		},
	}
	svc := NewA2AService(ms, nil)

	svc.DispatchPushNotifications(context.Background(), "t-1")

	// Wait for async webhook delivery.
	select {
	case <-received:
		// Success.
	case <-context.Background().Done():
		t.Fatal("webhook not received")
	}
}
