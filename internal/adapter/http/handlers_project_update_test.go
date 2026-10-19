package http_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// KI-41: PUT /projects/{id} merges config keys (a null value removes a key)
// instead of replacing the whole config map.
func TestUpdateProject_ConfigPatch(t *testing.T) {
	stored := map[string]string{
		"policy_preset":      "trusted-mount-autonomous",
		"execution_mode":     "mount",
		"detected_languages": `["go"]`,
		"expansion_prompt":   "expand",
	}

	tests := []struct {
		name string
		body string
		want map[string]string
	}{
		{
			name: "one key set, others kept",
			body: `{"config":{"policy_preset":"plan-readonly"}}`,
			want: map[string]string{
				"policy_preset": "plan-readonly", "execution_mode": "mount",
				"detected_languages": `["go"]`, "expansion_prompt": "expand",
			},
		},
		{
			name: "null removes a key",
			body: `{"config":{"expansion_prompt":null}}`,
			want: map[string]string{
				"policy_preset": "trusted-mount-autonomous", "execution_mode": "mount",
				"detected_languages": `["go"]`,
			},
		},
		{name: "name-only update keeps config", body: `{"name":"renamed"}`, want: stored},
		{name: "null config keeps config", body: `{"config":null}`, want: stored},
		{name: "empty config keeps config", body: `{"config":{}}`, want: stored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha", Config: maps.Clone(stored)}}}
			r := newTestRouterWithStore(store)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/p1", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
			}
			var got project.Project
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !maps.Equal(got.Config, tt.want) {
				t.Fatalf("config = %v, want %v", got.Config, tt.want)
			}
		})
	}

	// S3 review finding 6: gate commands are checked like the worker checks them.
	for _, body := range []string{
		`{"config":{"test_command":"echo pwned"}}`,
		`{"config":{"lint_command":"ruff check 'unterminated"}}`,
		`{"config":{"test_command":"/bin/sh -c pytest"}}`,
	} {
		t.Run("gate command rejected: "+body, func(t *testing.T) {
			store := &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha", Config: maps.Clone(stored)}}}
			r := newTestRouterWithStore(store)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/p1", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
			}
			if !maps.Equal(store.projects[0].Config, stored) {
				t.Fatalf("config changed to %v", store.projects[0].Config)
			}
		})
	}
	t.Run("allowed gate commands are stored", func(t *testing.T) {
		store := &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha", Config: maps.Clone(stored)}}}
		r := newTestRouterWithStore(store)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/p1",
			strings.NewReader(`{"config":{"test_command":"pytest -q -k 'not slow'","lint_command":"make lint"}}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
		}
		if got := store.projects[0].Config[project.ConfigTestCommand]; got != "pytest -q -k 'not slow'" {
			t.Fatalf("test_command = %q", got)
		}
	})

	t.Run("non-string value is rejected", func(t *testing.T) {
		store := &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha", Config: maps.Clone(stored)}}}
		r := newTestRouterWithStore(store)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/p1", strings.NewReader(`{"config":{"policy_preset":3}}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
		}
		if !maps.Equal(store.projects[0].Config, stored) {
			t.Fatalf("config changed to %v", store.projects[0].Config)
		}
	})
}
