package http_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// fakeLiteLLM records the model IDs the handler asks LiteLLM to delete.
type fakeLiteLLM struct {
	mu      sync.Mutex
	deleted []string
	status  int
}

func (f *fakeLiteLLM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/model/delete" {
		http.NotFound(w, r)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	data, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(data, &body)
	f.mu.Lock()
	f.deleted = append(f.deleted, body.ID)
	f.mu.Unlock()
	w.WriteHeader(f.status)
	_, _ = w.Write([]byte(`{}`))
}

func (f *fakeLiteLLM) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

// TestDeleteLLMModel covers DELETE /api/v1/llm/models/{id} (KI-40, D12). It
// replaced POST /api/v1/llm/models/delete, which the UI never called.
func TestDeleteLLMModel(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		role        user.Role // "" = the test router's admin
		llmStatus   int
		wantStatus  int
		wantDeleted []string
	}{
		{
			name: "admin deletes by id", method: http.MethodDelete, path: "/api/v1/llm/models/model-123",
			llmStatus: http.StatusOK, wantStatus: http.StatusOK, wantDeleted: []string{"model-123"},
		},
		{
			name: "escaped id is decoded", method: http.MethodDelete, path: "/api/v1/llm/models/openai%2Fgpt-4o",
			llmStatus: http.StatusOK, wantStatus: http.StatusOK, wantDeleted: []string{"openai/gpt-4o"},
		},
		{
			name: "escaped space is decoded", method: http.MethodDelete, path: "/api/v1/llm/models/my%20model",
			llmStatus: http.StatusOK, wantStatus: http.StatusOK, wantDeleted: []string{"my model"},
		},
		{
			name: "escaped percent is decoded once", method: http.MethodDelete, path: "/api/v1/llm/models/50%25off",
			llmStatus: http.StatusOK, wantStatus: http.StatusOK, wantDeleted: []string{"50%off"},
		},
		{
			name: "whitespace-only id is rejected", method: http.MethodDelete, path: "/api/v1/llm/models/%20%20",
			llmStatus: http.StatusOK, wantStatus: http.StatusBadRequest,
		},
		{
			name: "LiteLLM failure is a bad gateway", method: http.MethodDelete, path: "/api/v1/llm/models/model-123",
			llmStatus: http.StatusInternalServerError, wantStatus: http.StatusBadGateway, wantDeleted: []string{"model-123"},
		},
		{
			name: "editor is forbidden", method: http.MethodDelete, path: "/api/v1/llm/models/model-123", role: user.RoleEditor,
			llmStatus: http.StatusOK, wantStatus: http.StatusForbidden,
		},
		{
			name: "viewer is forbidden", method: http.MethodDelete, path: "/api/v1/llm/models/model-123", role: user.RoleViewer,
			llmStatus: http.StatusOK, wantStatus: http.StatusForbidden,
		},
		{
			name: "old POST route is gone", method: http.MethodPost, path: "/api/v1/llm/models/delete", body: `{"id":"model-123"}`,
			llmStatus: http.StatusOK, wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeLiteLLM{status: tt.llmStatus}
			llmSrv := httptest.NewServer(fake)
			defer llmSrv.Close()
			r := newTestRouterWithLLM(&mockStore{}, service.NewPolicyService("headless-safe-sandbox", nil), llmSrv.URL)

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			if tt.role != "" {
				req = req.WithContext(middleware.ContextWithTestUser(req.Context(), &user.User{ID: "u-1", Role: tt.role}))
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			got := fake.calls()
			if len(got) != len(tt.wantDeleted) {
				t.Fatalf("LiteLLM deletes = %q, want %q", got, tt.wantDeleted)
			}
			for i := range got {
				if got[i] != tt.wantDeleted[i] {
					t.Fatalf("LiteLLM deletes = %q, want %q", got, tt.wantDeleted)
				}
			}
		})
	}
}
