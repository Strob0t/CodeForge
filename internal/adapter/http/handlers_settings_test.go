package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/settings"
	"github.com/Strob0t/CodeForge/internal/domain/vcsaccount"
)

func TestGetSettings_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/settings", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

func TestGetSettings_WithData(t *testing.T) {
	store := &mockStore{
		settings: []settings.Setting{
			{Key: "theme", Value: json.RawMessage(`"dark"`)},
			{Key: "language", Value: json.RawMessage(`"en"`)},
		},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("GET", "/api/v1/settings", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]json.RawMessage
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 settings, got %d", len(result))
	}
	if string(result["theme"]) != `"dark"` {
		t.Fatalf("expected theme=dark, got %s", result["theme"])
	}
}

func TestUpdateSettings_EmptyBody(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("PUT", "/api/v1/settings", strings.NewReader(`{"settings":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSettings_Success(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("PUT", "/api/v1/settings", strings.NewReader(`{"settings":{"theme":"dark"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListVCSAccounts_Empty(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("GET", "/api/v1/vcs-accounts", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result []vcsaccount.VCSAccount
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %d", len(result))
	}
}

func TestDeleteVCSAccount_NotFound(t *testing.T) {
	r := newTestRouter()
	req := httptest.NewRequest("DELETE", "/api/v1/vcs-accounts/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteVCSAccount_Success(t *testing.T) {
	store := &mockStore{
		vcsAccounts: []vcsaccount.VCSAccount{
			{ID: "vcs-1", Label: "GitHub Test", Provider: "github"},
		},
	}
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest("DELETE", "/api/v1/vcs-accounts/vcs-1", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}
