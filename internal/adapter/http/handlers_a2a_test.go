package http_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// a2aTestStore extends mockStore with A2A-specific overrides.
type a2aTestStore struct {
	mockStore

	remoteAgents    []a2adomain.RemoteAgent
	a2aTasks        []a2adomain.A2ATask
	pushConfigs     []database.A2APushConfig
	createAgentErr  error
	deleteAgentErr  error
	getTaskErr      error
	createPushErr   error
	deletePushErr   error
	pushConfigIDSeq int
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

func (m *a2aTestStore) CreateA2APushConfig(_ context.Context, taskID, url, token string) (string, error) {
	if m.createPushErr != nil {
		return "", m.createPushErr
	}
	m.pushConfigIDSeq++
	id := fmt.Sprintf("pc-%d", m.pushConfigIDSeq)
	m.pushConfigs = append(m.pushConfigs, database.A2APushConfig{
		ID: id, TaskID: taskID, URL: url, Token: token,
	})
	return id, nil
}

func (m *a2aTestStore) ListA2APushConfigs(_ context.Context, taskID string) ([]database.A2APushConfig, error) {
	var result []database.A2APushConfig
	for _, c := range m.pushConfigs {
		if c.TaskID == taskID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *a2aTestStore) DeleteA2APushConfig(_ context.Context, id string) error {
	if m.deletePushErr != nil {
		return m.deletePushErr
	}
	for i, c := range m.pushConfigs {
		if c.ID == id {
			m.pushConfigs = append(m.pushConfigs[:i], m.pushConfigs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (m *a2aTestStore) GetA2APushConfig(_ context.Context, _ string) (_, _, _ string, _ error) {
	return "", "", "", nil
}

func (m *a2aTestStore) DeleteAllA2APushConfigs(_ context.Context, _ string) error { return nil }

func newA2AHandlers(store database.Store) *cfhttp.Handlers {
	a2aSvc := service.NewA2AService(store, nil)
	return &cfhttp.Handlers{
		A2A:    a2aSvc,
		Limits: &config.Limits{MaxRequestBodySize: 1 << 20, MaxQueryLength: 2000},
	}
}

// withChiParam adds a chi URL parameter to the request context.
func withChiParam(r *http.Request, key, value string) *http.Request { //nolint:unparam // key is generic for future use
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
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

// --- Push Config Tests (Phase 27O) ---

func TestCreateA2APushConfig_Success(t *testing.T) {
	ms := &a2aTestStore{
		a2aTasks: []a2adomain.A2ATask{
			{ID: "t-1", State: a2adomain.TaskStateWorking, Direction: a2adomain.DirectionInbound},
		},
	}
	h := newA2AHandlers(ms)

	body := strings.NewReader(`{"url":"https://example.com/hook","token":"secret"}`)
	req := withChiParam(httptest.NewRequest(http.MethodPost, "/api/v1/a2a/tasks/t-1/push-config", body), "id", "t-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateA2APushConfig(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(ms.pushConfigs) != 1 {
		t.Errorf("expected 1 push config, got %d", len(ms.pushConfigs))
	}
}

func TestCreateA2APushConfig_MissingURL(t *testing.T) {
	ms := &a2aTestStore{
		a2aTasks: []a2adomain.A2ATask{
			{ID: "t-1", State: a2adomain.TaskStateWorking},
		},
	}
	h := newA2AHandlers(ms)

	body := strings.NewReader(`{"token":"secret"}`)
	req := withChiParam(httptest.NewRequest(http.MethodPost, "/api/v1/a2a/tasks/t-1/push-config", body), "id", "t-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateA2APushConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateA2APushConfig_TaskNotFound(t *testing.T) {
	ms := &a2aTestStore{} // no tasks
	h := newA2AHandlers(ms)

	body := strings.NewReader(`{"url":"https://example.com/hook"}`)
	req := withChiParam(httptest.NewRequest(http.MethodPost, "/api/v1/a2a/tasks/missing/push-config", body), "id", "missing")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CreateA2APushConfig(w, req)

	if w.Code == http.StatusCreated {
		t.Errorf("expected error status, got 201")
	}
}

func TestListA2APushConfigs_Empty(t *testing.T) {
	ms := &a2aTestStore{}
	h := newA2AHandlers(ms)

	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks/t-1/push-config", http.NoBody), "id", "t-1")
	w := httptest.NewRecorder()
	h.ListA2APushConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "[]" && w.Body.String() != "[]\n" {
		t.Errorf("expected empty JSON array, got %q", w.Body.String())
	}
}

func TestListA2APushConfigs_WithData(t *testing.T) {
	ms := &a2aTestStore{
		pushConfigs: []database.A2APushConfig{
			{ID: "pc-1", TaskID: "t-1", URL: "https://example.com/hook1"},
			{ID: "pc-2", TaskID: "t-1", URL: "https://example.com/hook2"},
			{ID: "pc-3", TaskID: "t-2", URL: "https://other.com/hook"},
		},
	}
	h := newA2AHandlers(ms)

	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks/t-1/push-config", http.NoBody), "id", "t-1")
	w := httptest.NewRecorder()
	h.ListA2APushConfigs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// Should only contain t-1's configs.
	if !strings.Contains(w.Body.String(), "pc-1") || !strings.Contains(w.Body.String(), "pc-2") {
		t.Errorf("expected pc-1 and pc-2 in response, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "pc-3") {
		t.Errorf("expected pc-3 NOT in response (different task), got %s", w.Body.String())
	}
}

func TestDeleteA2APushConfig_Success(t *testing.T) {
	ms := &a2aTestStore{
		pushConfigs: []database.A2APushConfig{
			{ID: "pc-1", TaskID: "t-1", URL: "https://example.com/hook"},
		},
	}
	h := newA2AHandlers(ms)

	req := withChiParam(httptest.NewRequest(http.MethodDelete, "/api/v1/a2a/push-config/pc-1", http.NoBody), "id", "pc-1")
	w := httptest.NewRecorder()
	h.DeleteA2APushConfig(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestDeleteA2APushConfig_NotFound(t *testing.T) {
	ms := &a2aTestStore{deletePushErr: fmt.Errorf("not found")}
	h := newA2AHandlers(ms)

	req := withChiParam(httptest.NewRequest(http.MethodDelete, "/api/v1/a2a/push-config/missing", http.NoBody), "id", "missing")
	w := httptest.NewRecorder()
	h.DeleteA2APushConfig(w, req)

	if w.Code == http.StatusNoContent {
		t.Errorf("expected error status, got 204")
	}
}

func TestSubscribeA2ATask_TerminalState(t *testing.T) {
	ms := &a2aTestStore{
		a2aTasks: []a2adomain.A2ATask{
			{ID: "t-done", State: a2adomain.TaskStateCompleted, Direction: a2adomain.DirectionInbound},
		},
	}
	h := newA2AHandlers(ms)

	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks/t-done/subscribe", http.NoBody), "id", "t-done")
	w := httptest.NewRecorder()
	h.SubscribeA2ATask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: status") {
		t.Errorf("expected initial status event, got %s", respBody)
	}
	if !strings.Contains(respBody, "event: done") {
		t.Errorf("expected done event for terminal state, got %s", respBody)
	}
	if !strings.Contains(respBody, `"state":"completed"`) {
		t.Errorf("expected completed state in SSE data, got %s", respBody)
	}
}

func TestSubscribeA2ATask_NotFound(t *testing.T) {
	ms := &a2aTestStore{getTaskErr: fmt.Errorf("not found")}
	h := newA2AHandlers(ms)

	req := withChiParam(httptest.NewRequest(http.MethodGet, "/api/v1/a2a/tasks/missing/subscribe", http.NoBody), "id", "missing")
	w := httptest.NewRecorder()
	h.SubscribeA2ATask(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected error status, got 200")
	}
}
