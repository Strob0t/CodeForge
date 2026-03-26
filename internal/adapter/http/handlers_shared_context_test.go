package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// sharedCtxMockStore is a minimal mock that supports SharedContext operations.
type sharedCtxMockStore struct {
	mockStore
	shared cfcontext.SharedContext
	items  []cfcontext.SharedContextItem
}

func (m *sharedCtxMockStore) CreateSharedContext(_ context.Context, sc *cfcontext.SharedContext) error {
	sc.ID = "sc-001"
	m.shared = *sc
	return nil
}

func (m *sharedCtxMockStore) GetSharedContextByTeam(_ context.Context, teamID string) (*cfcontext.SharedContext, error) {
	if m.shared.TeamID == teamID {
		sc := m.shared
		sc.Items = m.items
		return &sc, nil
	}
	return &cfcontext.SharedContext{TeamID: teamID, Items: []cfcontext.SharedContextItem{}}, nil
}

func (m *sharedCtxMockStore) AddSharedContextItem(_ context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error) {
	item := &cfcontext.SharedContextItem{
		ID:       "item-001",
		SharedID: "sc-001",
		Key:      req.Key,
		Value:    req.Value,
		Author:   req.Author,
	}
	m.items = append(m.items, *item)
	return item, nil
}

func newSharedCtxTestRouter(store *sharedCtxMockStore) chi.Router {
	bc := &mockBroadcaster{}
	queue := &mockQueue{}
	sharedCtxSvc := service.NewSharedContextService(store, bc, queue)

	handlers := &cfhttp.Handlers{
		SharedContext: sharedCtxSvc,
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(middleware.ContextWithTestUser(r.Context(), &user.User{
				ID:   "test-admin",
				Name: "Test Admin",
				Role: user.RoleAdmin,
			})))
		})
	})

	// Mount only shared context routes to keep the test focused.
	r.Route("/api/v1", func(r chi.Router) {
		if handlers.SharedContext != nil {
			r.Post("/teams/{teamId}/shared-context", handlers.InitSharedContext)
			r.Get("/teams/{teamId}/shared-context", handlers.GetSharedContext)
			r.Post("/teams/{teamId}/shared-context/items", handlers.AddSharedContextItem)
		}
	})

	return r
}

func TestInitSharedContext(t *testing.T) {
	t.Parallel()

	t.Run("returns 201 with valid request", func(t *testing.T) {
		t.Parallel()

		store := &sharedCtxMockStore{}
		r := newSharedCtxTestRouter(store)

		body := `{"project_id":"proj-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/team-1/shared-context", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var sc cfcontext.SharedContext
		if err := json.Unmarshal(w.Body.Bytes(), &sc); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if sc.TeamID != "team-1" {
			t.Errorf("expected team_id 'team-1', got %q", sc.TeamID)
		}
		if sc.ProjectID != "proj-1" {
			t.Errorf("expected project_id 'proj-1', got %q", sc.ProjectID)
		}
	})

	t.Run("returns 400 with invalid JSON body", func(t *testing.T) {
		t.Parallel()

		store := &sharedCtxMockStore{}
		r := newSharedCtxTestRouter(store)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/team-1/shared-context", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestGetSharedContext(t *testing.T) {
	t.Parallel()

	t.Run("returns 200 with shared context", func(t *testing.T) {
		t.Parallel()

		store := &sharedCtxMockStore{}
		store.shared = cfcontext.SharedContext{
			ID:        "sc-001",
			TeamID:    "team-1",
			ProjectID: "proj-1",
		}
		r := newSharedCtxTestRouter(store)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/teams/team-1/shared-context", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var sc cfcontext.SharedContext
		if err := json.Unmarshal(w.Body.Bytes(), &sc); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if sc.TeamID != "team-1" {
			t.Errorf("expected team_id 'team-1', got %q", sc.TeamID)
		}
	})
}

func TestAddSharedContextItem(t *testing.T) {
	t.Parallel()

	t.Run("returns 201 with valid item", func(t *testing.T) {
		t.Parallel()

		store := &sharedCtxMockStore{}
		r := newSharedCtxTestRouter(store)

		body := map[string]string{
			"key":   "architecture",
			"value": "microservices pattern",
		}
		bodyJSON, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/team-1/shared-context/items", bytes.NewReader(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var item cfcontext.SharedContextItem
		if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if item.Key != "architecture" {
			t.Errorf("expected key 'architecture', got %q", item.Key)
		}
	})

	t.Run("returns 400 with invalid JSON", func(t *testing.T) {
		t.Parallel()

		store := &sharedCtxMockStore{}
		r := newSharedCtxTestRouter(store)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/team-1/shared-context/items", strings.NewReader("bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestSharedContextIntegration(t *testing.T) {
	t.Parallel()

	store := &sharedCtxMockStore{}
	r := newSharedCtxTestRouter(store)

	// Step 1: Create shared context.
	initBody := `{"project_id":"proj-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams/team-int/shared-context", strings.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("init: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Step 2: Get shared context.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/teams/team-int/shared-context", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Step 3: Add item.
	itemBody := `{"key":"design","value":"hexagonal"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/teams/team-int/shared-context/items", strings.NewReader(itemBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("add item: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Step 4: Get again and confirm item present.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/teams/team-int/shared-context", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get after add: expected 200, got %d", w.Code)
	}

	var sc cfcontext.SharedContext
	if err := json.Unmarshal(w.Body.Bytes(), &sc); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(sc.Items) != 1 {
		t.Fatalf("expected 1 item after add, got %d", len(sc.Items))
	}
	if sc.Items[0].Key != "design" {
		t.Errorf("expected item key 'design', got %q", sc.Items[0].Key)
	}
}
