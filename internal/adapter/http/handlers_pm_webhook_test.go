package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	_ "github.com/Strob0t/CodeForge/internal/adapter/githubpm"
	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	_ "github.com/Strob0t/CodeForge/internal/adapter/plane"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-56: the PM webhooks answered 200 although the sync could never run.

type countingSyncer struct {
	mu    sync.Mutex
	calls int
}

func (c *countingSyncer) Sync(_ context.Context, _ *roadmap.SyncConfig) (*roadmap.SyncResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return &roadmap.SyncResult{}, nil
}

func TestPMWebhookHandlers_AnswerWhatHappened(t *testing.T) {
	planeBody := `{"event":"issue.updated","data":{"id":"i-1","workspace":"ws-uuid","project":"p-1"}}`
	tests := []struct {
		name     string
		configs  map[string]map[string]string
		path     string
		header   [2]string
		body     string
		wantCode int
		wantBody string
	}{
		{
			name:     "plane sync started",
			configs:  map[string]map[string]string{"plane": {"api_token": "t"}},
			path:     "/plane",
			body:     planeBody,
			wantCode: http.StatusAccepted,
		},
		{
			name:     "plane without api_token",
			path:     "/plane",
			body:     planeBody,
			wantCode: http.StatusBadRequest,
			wantBody: "api_token",
		},
		{
			name:     "github repository without a project",
			path:     "/github",
			header:   [2]string{"X-GitHub-Event", "issues"},
			body:     `{"action":"opened","issue":{"number":1},"repository":{"full_name":"acme/none"}}`,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "github event that is not about issues",
			path:     "/github",
			header:   [2]string{"X-GitHub-Event", "push"},
			body:     `{}`,
			wantCode: http.StatusOK,
			wantBody: "ignored",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{projects: []project.Project{
				{ID: "pl", Config: map[string]string{"plane_workspace": "acme", "plane_project_id": "p-1"}},
			}}
			h := &cfhttp.Handlers{PMWebhook: service.NewPMWebhookService(nil, &countingSyncer{}, store, tc.configs)}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/pm"+tc.path, strings.NewReader(tc.body))
			if tc.header[0] != "" {
				req.Header.Set(tc.header[0], tc.header[1])
			}
			w := httptest.NewRecorder()
			switch tc.path {
			case "/plane":
				h.HandlePlaneWebhook(w, req)
			case "/github":
				h.HandleGitHubIssueWebhook(w, req)
			}
			if w.Code != tc.wantCode || !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("status %d body %s, want %d containing %q", w.Code, w.Body.String(), tc.wantCode, tc.wantBody)
			}
		})
	}
}
