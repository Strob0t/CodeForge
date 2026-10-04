package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// tenantProjectStore finds a project only in its own tenant, as the
// PostgreSQL store does (store_project.go:GetProject).
type tenantProjectStore struct{ *mockStore }

func (s tenantProjectStore) GetProject(ctx context.Context, id string) (*project.Project, error) {
	p, err := s.mockStore.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.TenantID != tenantctx.FromContext(ctx) {
		return nil, errNotFound
	}
	return p, nil
}

// subjectRecordingQueue records the subjects published.
type subjectRecordingQueue struct {
	mockQueue
	mu       sync.Mutex
	subjects []string
}

func (q *subjectRecordingQueue) Publish(_ context.Context, subject string, _ []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.subjects = append(q.subjects, subject)
	return nil
}

func (q *subjectRecordingQueue) published() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.subjects)
}

const (
	retrievalTenantA = "aaaaaaaa-0000-0000-0000-000000000001"
	retrievalTenantB = "bbbbbbbb-0000-0000-0000-000000000002"
)

// newRetrievalTenantRouter serves the retrieval, graph and repo map routes
// with two projects in two tenants; both have an index and a graph.
func newRetrievalTenantRouter(t *testing.T) (chi.Router, *subjectRecordingQueue) {
	t.Helper()
	store := &mockStore{projects: []project.Project{
		{ID: "proj-a", Name: "a", TenantID: retrievalTenantA, WorkspacePath: "/ws/a"},
		{ID: "proj-b", Name: "b", TenantID: retrievalTenantB, WorkspacePath: "/ws/b"},
	}}
	ts := tenantProjectStore{store}
	queue := &subjectRecordingQueue{}
	bc := &mockBroadcaster{}
	orchCfg := &config.Orchestrator{SubAgentTimeout: time.Millisecond}
	limits := &config.Limits{}
	retrieval := service.NewRetrievalService(ts, queue, bc, orchCfg, limits)
	graph := service.NewGraphService(ts, queue, bc, orchCfg, limits)
	ctx := context.Background()
	for _, id := range []string{"proj-a", "proj-b"} {
		if err := retrieval.HandleIndexResult(ctx, &messagequeue.RetrievalIndexResultPayload{ProjectID: id, Status: "ready", FileCount: 3}); err != nil {
			t.Fatal(err)
		}
		if err := graph.HandleBuildResult(ctx, &messagequeue.GraphBuildResultPayload{ProjectID: id, Status: "ready", NodeCount: 5}); err != nil {
			t.Fatal(err)
		}
	}
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000", func(h *cfhttp.Handlers) {
		h.Projects = service.NewProjectService(ts, os.TempDir())
		h.Retrieval = retrieval
		h.Graph = graph
		h.RepoMap = service.NewRepoMapService(ts, queue, bc, orchCfg)
	})
	return r, queue
}

func retrievalTenantRequest(r chi.Router, tenantID, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(tenantctx.WithTenant(req.Context(), tenantID))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The retrieval index, the graph and the searches of a project are keyed by
// its ID only (in the Go Core's memory and on the worker): the handlers load
// the project in the request's tenant first, so another tenant's project is
// not found and nothing is sent to the worker (S7-F review).
func TestRetrievalHandlers_OtherTenantsProjectIsNotFound(t *testing.T) {
	tests := []struct {
		name, method, path, body string
	}{
		{"index status", http.MethodGet, "/api/v1/projects/proj-b/index", ""},
		{"graph status", http.MethodGet, "/api/v1/projects/proj-b/graph/status", ""},
		{"search", http.MethodPost, "/api/v1/projects/proj-b/search", `{"query":"secret"}`},
		{"agent search", http.MethodPost, "/api/v1/projects/proj-b/search/agent", `{"query":"secret"}`},
		{"graph search", http.MethodPost, "/api/v1/projects/proj-b/graph/search", `{"seed_symbols":["main"]}`},
		{"graph build", http.MethodPost, "/api/v1/projects/proj-b/graph/build", ""},
		{"index build", http.MethodPost, "/api/v1/projects/proj-b/index", ""},
		{"repo map", http.MethodGet, "/api/v1/projects/proj-b/repomap", ""},
		{"repo map generation", http.MethodPost, "/api/v1/projects/proj-b/repomap", ""},
		{"unknown project", http.MethodGet, "/api/v1/projects/proj-x/index", ""},
		{"global search", http.MethodPost, "/api/v1/search", `{"query":"secret","project_ids":["proj-a","proj-b"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, queue := newRetrievalTenantRouter(t)
			w := retrievalTenantRequest(r, retrievalTenantA, tt.method, tt.path, tt.body)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body %s)", w.Code, w.Body.String())
			}
			if got := queue.published(); len(got) != 0 {
				t.Fatalf("published %v for another tenant's project", got)
			}
		})
	}
}

// The project's own tenant keeps using every route.
func TestRetrievalHandlers_OwnProject(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		want                     int
		publishes                string
	}{
		{"index status", http.MethodGet, "/api/v1/projects/proj-b/index", "", http.StatusOK, ""},
		{"graph status", http.MethodGet, "/api/v1/projects/proj-b/graph/status", "", http.StatusOK, ""},
		// No worker answers here: the searches are sent and time out.
		{"search", http.MethodPost, "/api/v1/projects/proj-b/search", `{"query":"q"}`, http.StatusGatewayTimeout, messagequeue.SubjectRetrievalSearchRequest},
		{"agent search", http.MethodPost, "/api/v1/projects/proj-b/search/agent", `{"query":"q"}`, http.StatusGatewayTimeout, messagequeue.SubjectSubAgentSearchRequest},
		{"graph search", http.MethodPost, "/api/v1/projects/proj-b/graph/search", `{"seed_symbols":["main"]}`, http.StatusGatewayTimeout, messagequeue.SubjectGraphSearchRequest},
		{"global search", http.MethodPost, "/api/v1/search", `{"query":"q","project_ids":["proj-b"]}`, http.StatusOK, messagequeue.SubjectRetrievalSearchRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, queue := newRetrievalTenantRouter(t)
			w := retrievalTenantRequest(r, retrievalTenantB, tt.method, tt.path, tt.body)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.want, w.Body.String())
			}
			if tt.publishes != "" && !slices.Contains(queue.published(), tt.publishes) {
				t.Fatalf("published %v, want %s", queue.published(), tt.publishes)
			}
		})
	}
}
