package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

func TestListKnowledgeBases_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/knowledge-bases", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []knowledgebase.KnowledgeBase
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

func TestGetKnowledgeBase_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/knowledge-bases/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateKnowledgeBase_Success(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(knowledgebase.CreateRequest{
		Name:        "Go Patterns",
		Description: "Common Go patterns and idioms",
		Category:    knowledgebase.CategoryLanguage,
		ContentPath: "go-patterns",
	})
	req := httptest.NewRequest("POST", "/api/v1/knowledge-bases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var kb knowledgebase.KnowledgeBase
	if err := json.NewDecoder(w.Body).Decode(&kb); err != nil {
		t.Fatal(err)
	}
	if kb.Name != "Go Patterns" {
		t.Fatalf("expected name=Go Patterns, got %s", kb.Name)
	}
	if kb.ID == "" {
		t.Fatal("expected ID to be assigned")
	}
}

func TestCreateKnowledgeBase_MissingName(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(knowledgebase.CreateRequest{
		Category:    knowledgebase.CategoryLanguage,
		ContentPath: "/docs/something",
	})
	req := httptest.NewRequest("POST", "/api/v1/knowledge-bases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateKnowledgeBase_InvalidCategory(t *testing.T) {
	r := newTestRouter()
	body, _ := json.Marshal(map[string]string{
		"name":         "Bad Category",
		"category":     "nonexistent",
		"content_path": "/docs/test",
	})
	req := httptest.NewRequest("POST", "/api/v1/knowledge-bases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteKnowledgeBase_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("DELETE", "/api/v1/knowledge-bases/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteKnowledgeBase_Success(t *testing.T) {
	store := &mockStore{
		knowledgeBases: []knowledgebase.KnowledgeBase{
			{ID: "kb-1", Name: "To Delete", Category: knowledgebase.CategoryCustom},
		},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("DELETE", "/api/v1/knowledge-bases/kb-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListScopeKnowledgeBases_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/scopes/scope-1/knowledge-bases", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []knowledgebase.KnowledgeBase
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

// TestKnowledgeBaseWrites_NeedAdmin pins KI-105: creating, changing,
// deleting and indexing a knowledge base need the admin role.
func TestKnowledgeBaseWrites_NeedAdmin(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"POST", "/api/v1/knowledge-bases", `{"name":"kb","category":"custom","content_path":"docs"}`},
		{"PUT", "/api/v1/knowledge-bases/kb-1", `{"name":"renamed"}`},
		{"DELETE", "/api/v1/knowledge-bases/kb-1", ""},
		{"POST", "/api/v1/knowledge-bases/kb-1/index", ""},
	}
	for _, role := range []user.Role{user.RoleViewer, user.RoleEditor} {
		for _, rt := range routes {
			store := &mockStore{knowledgeBases: []knowledgebase.KnowledgeBase{
				{ID: "kb-1", Name: "KB", Category: knowledgebase.CategoryCustom, ContentPath: "docs"},
			}}
			r := newTestRouterWithStore(store)
			req := httptest.NewRequest(rt.method, rt.path, bytes.NewReader([]byte(rt.body)))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(middleware.ContextWithTestUser(req.Context(), &user.User{ID: "u-1", Role: role}))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s: status %d, want 403", rt.method, rt.path, role, w.Code)
			}
			if len(store.knowledgeBases) != 1 || store.knowledgeBases[0].Name != "KB" {
				t.Errorf("%s %s as %s changed the store: %+v", rt.method, rt.path, role, store.knowledgeBases)
			}
		}
	}
}

func TestCreateKnowledgeBase_RefusesPathsOutsideTheContentRoot(t *testing.T) {
	for _, contentPath := range []string{"/etc", "/etc/passwd", "../../etc", "docs/../../x"} {
		r := newTestRouter()
		body, _ := json.Marshal(knowledgebase.CreateRequest{
			Name: "KB", Category: knowledgebase.CategoryCustom, ContentPath: contentPath,
		})
		req := httptest.NewRequest("POST", "/api/v1/knowledge-bases", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("content_path %q: status %d, want 400 (%s)", contentPath, w.Code, w.Body.String())
		}
	}
}
