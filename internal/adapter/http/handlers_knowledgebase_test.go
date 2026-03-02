package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
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
		ContentPath: "/docs/go-patterns",
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
