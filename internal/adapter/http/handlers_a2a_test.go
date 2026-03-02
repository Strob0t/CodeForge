package http_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// a2aTestStore extends mockStore with A2A-specific overrides.
type a2aTestStore struct {
	mockStore

	remoteAgents   []a2adomain.RemoteAgent
	a2aTasks       []a2adomain.A2ATask
	createAgentErr error
	deleteAgentErr error
	getTaskErr     error
}

func (m *a2aTestStore) CreateRemoteAgent(_ context.Context, a *a2adomain.RemoteAgent) error {
	if m.createAgentErr != nil {
		return m.createAgentErr
	}
	a.ID = "ra-1"
	m.remoteAgents = append(m.remoteAgents, *a)
	return nil
}

func (m *a2aTestStore) ListRemoteAgents(_ context.Context, _ string, _ bool) ([]a2adomain.RemoteAgent, error) {
	return m.remoteAgents, nil
}

func (m *a2aTestStore) DeleteRemoteAgent(_ context.Context, id string) error {
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

func (m *a2aTestStore) ListA2ATasks(_ context.Context, _ *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	return m.a2aTasks, len(m.a2aTasks), nil
}

func (m *a2aTestStore) GetA2ATask(_ context.Context, id string) (*a2adomain.A2ATask, error) {
	if m.getTaskErr != nil {
		return nil, m.getTaskErr
	}
	for i := range m.a2aTasks {
		if m.a2aTasks[i].ID == id {
			return &m.a2aTasks[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *a2aTestStore) UpdateA2ATask(_ context.Context, _ *a2adomain.A2ATask) error { return nil }

func newA2AHandlers(store database.Store) *cfhttp.Handlers {
	a2aSvc := service.NewA2AService(store, nil)
	return &cfhttp.Handlers{
		A2A:    a2aSvc,
		Limits: &config.Limits{MaxRequestBodySize: 1 << 20, MaxQueryLength: 2000},
	}
}

func TestListRemoteAgents_Empty(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/a2a/agents", http.NoBody)
	w := httptest.NewRecorder()
	h.ListRemoteAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRegisterRemoteAgent_MissingName(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{})

	body := strings.NewReader(`{"url":"http://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/a2a/agents", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.RegisterRemoteAgent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterRemoteAgent_MissingURL(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{})

	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/a2a/agents", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.RegisterRemoteAgent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteRemoteAgent_NotFound(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{deleteAgentErr: fmt.Errorf("not found")})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/a2a/agents/missing", http.NoBody)
	w := httptest.NewRecorder()
	h.DeleteRemoteAgent(w, req)

	if w.Code == http.StatusNoContent {
		t.Errorf("expected error status, got 204")
	}
}

func TestListA2ATasks_Empty(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks", http.NoBody)
	w := httptest.NewRecorder()
	h.ListA2ATasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetA2ATask_NotFound(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{getTaskErr: fmt.Errorf("not found")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks/missing", http.NoBody)
	w := httptest.NewRecorder()
	h.GetA2ATask(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected error status, got 200")
	}
}

func TestCancelA2ATask_NotFound(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{getTaskErr: fmt.Errorf("not found")})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/a2a/tasks/missing/cancel", http.NoBody)
	w := httptest.NewRecorder()
	h.CancelA2ATask(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected error status, got 200")
	}
}

func TestSendA2ATask_MissingPrompt(t *testing.T) {
	h := newA2AHandlers(&a2aTestStore{})

	body := strings.NewReader(`{"skill_id":"code"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/a2a/agents/ra-1/send", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.SendA2ATask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListA2ATasks_WithFilter(t *testing.T) {
	ms := &a2aTestStore{
		a2aTasks: []a2adomain.A2ATask{
			{ID: "t-1", State: a2adomain.TaskStateWorking, Direction: a2adomain.DirectionInbound},
		},
	}
	h := newA2AHandlers(ms)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks?state=working&direction=inbound", http.NoBody)
	w := httptest.NewRecorder()
	h.ListA2ATasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
