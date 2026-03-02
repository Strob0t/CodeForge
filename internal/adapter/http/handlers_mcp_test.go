package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

func TestListMCPServers_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/mcp/servers", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []mcp.ServerDef
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

func TestGetMCPServer_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/mcp/servers/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMCPServer_Success(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(mcp.ServerDef{
		Name:      "test-server",
		Transport: mcp.TransportSSE,
		URL:       "http://localhost:3001/sse",
	})
	req := httptest.NewRequest("POST", "/api/v1/mcp/servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var srv mcp.ServerDef
	if err := json.NewDecoder(w.Body).Decode(&srv); err != nil {
		t.Fatal(err)
	}
	if srv.Name != "test-server" {
		t.Fatalf("expected name=test-server, got %s", srv.Name)
	}
	if srv.ID == "" {
		t.Fatal("expected server ID to be assigned")
	}
}

func TestCreateMCPServer_MissingName(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(mcp.ServerDef{
		Transport: mcp.TransportSSE,
		URL:       "http://localhost:3001/sse",
	})
	req := httptest.NewRequest("POST", "/api/v1/mcp/servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateMCPServer_InvalidTransport(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{
		"name":      "bad-transport",
		"transport": "grpc",
	})
	req := httptest.NewRequest("POST", "/api/v1/mcp/servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteMCPServer_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("DELETE", "/api/v1/mcp/servers/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteMCPServer_Success(t *testing.T) {
	store := &mockStore{
		mcpServers: []mcp.ServerDef{
			{ID: "srv-1", Name: "to-delete", Transport: mcp.TransportSSE, URL: "http://localhost"},
		},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("DELETE", "/api/v1/mcp/servers/srv-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListMCPServerTools_Empty(t *testing.T) {
	store := &mockStore{
		mcpServers: []mcp.ServerDef{
			{ID: "srv-1", Name: "test", Transport: mcp.TransportSSE, URL: "http://localhost"},
		},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("GET", "/api/v1/mcp/servers/srv-1/tools", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []mcp.ServerTool
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

func TestListProjectMCPServers_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/projects/proj-1/mcp-servers", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []mcp.ServerDef
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

func TestAssignMCPServerToProject_MissingServerID(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{"server_id": ""})
	req := httptest.NewRequest("POST", "/api/v1/projects/proj-1/mcp-servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
