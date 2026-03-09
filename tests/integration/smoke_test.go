//go:build smoke

// Package integration_test contains smoke tests that verify the full stack
// health when running in Docker Compose. These tests talk to a REAL running
// stack (not mocked) and only run with: go test -tags=smoke ./tests/integration/...
package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// baseURL returns the backend URL from env or default.
func baseURL() string {
	if u := os.Getenv("SMOKE_BASE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

// litellmURL returns the LiteLLM proxy URL from env or default.
func litellmURL() string {
	if u := os.Getenv("SMOKE_LITELLM_URL"); u != "" {
		return u
	}
	return "http://localhost:4000"
}

// natsURL returns the NATS server URL from env or default.
func natsURL() string {
	if u := os.Getenv("SMOKE_NATS_URL"); u != "" {
		return u
	}
	return "nats://localhost:4222"
}

// httpClient returns an HTTP client with a reasonable timeout.
func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// backendReachable checks whether the backend is reachable at all.
// Returns true if a TCP connection can be established.
func backendReachable(t *testing.T) bool {
	t.Helper()
	client := httpClient()
	resp, err := client.Get(baseURL() + "/health")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// skipIfBackendDown skips the test if the backend is not reachable.
func skipIfBackendDown(t *testing.T) {
	t.Helper()
	if !backendReachable(t) {
		t.Skipf("backend not reachable at %s — skipping smoke test", baseURL())
	}
}

// TestSmoke_HealthEndpoint verifies GET /health returns 200 with a JSON body
// containing a "status" field.
func TestSmoke_HealthEndpoint(t *testing.T) {
	skipIfBackendDown(t)

	client := httpClient()
	resp, err := client.Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /health response: %v", err)
	}

	status, ok := body["status"]
	if !ok {
		t.Fatal("response missing 'status' field")
	}
	if status != "ok" {
		t.Fatalf("expected status 'ok', got %v", status)
	}
}

// TestSmoke_HealthDevMode verifies that GET /health returns JSON with
// "dev_mode": true when APP_ENV=development is set on the backend.
func TestSmoke_HealthDevMode(t *testing.T) {
	skipIfBackendDown(t)

	client := httpClient()
	resp, err := client.Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /health response: %v", err)
	}

	devMode, ok := body["dev_mode"]
	if !ok {
		t.Fatal("response missing 'dev_mode' field")
	}
	if devMode != true {
		t.Fatalf("expected dev_mode=true (APP_ENV=development), got %v", devMode)
	}
}

// TestSmoke_APIVersionPrefix verifies that the API route prefix is registered
// by checking that GET /api/v1/projects does NOT return 404.
func TestSmoke_APIVersionPrefix(t *testing.T) {
	skipIfBackendDown(t)

	client := httpClient()
	resp, err := client.Get(baseURL() + "/api/v1/projects")
	if err != nil {
		t.Fatalf("GET /api/v1/projects: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("GET /api/v1/projects returned 404 — API routes not registered")
	}
}

// TestSmoke_AuthRequired verifies that GET /api/v1/projects without a JWT
// returns 401 Unauthorized.
func TestSmoke_AuthRequired(t *testing.T) {
	skipIfBackendDown(t)

	client := httpClient()
	req, err := http.NewRequest(http.MethodGet, baseURL()+"/api/v1/projects", http.NoBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	// Explicitly no Authorization header.
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/projects (no auth): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without JWT, got %d", resp.StatusCode)
	}
}

// TestSmoke_LiteLLMHealth verifies that the LiteLLM proxy is running and
// responds to GET /health with 200. Skipped if SMOKE_SKIP_LITELLM=true.
func TestSmoke_LiteLLMHealth(t *testing.T) {
	if os.Getenv("SMOKE_SKIP_LITELLM") == "true" {
		t.Skip("SMOKE_SKIP_LITELLM=true — skipping LiteLLM health check")
	}

	client := httpClient()
	resp, err := client.Get(litellmURL() + "/health")
	if err != nil {
		t.Skipf("LiteLLM not reachable at %s: %v", litellmURL(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LiteLLM /health: expected 200, got %d", resp.StatusCode)
	}
}

// TestSmoke_NATSConnection verifies that NATS is running and the CODEFORGE
// JetStream stream exists. Skipped if SMOKE_SKIP_NATS=true.
func TestSmoke_NATSConnection(t *testing.T) {
	if os.Getenv("SMOKE_SKIP_NATS") == "true" {
		t.Skip("SMOKE_SKIP_NATS=true — skipping NATS connection check")
	}

	nc, err := nats.Connect(natsURL(), nats.Timeout(5*time.Second))
	if err != nil {
		t.Skipf("NATS not reachable at %s: %v", natsURL(), err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := js.Stream(ctx, "CODEFORGE")
	if err != nil {
		t.Fatalf("CODEFORGE stream not found: %v", err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("get stream info: %v", err)
	}

	if info.Config.Name != "CODEFORGE" {
		t.Fatalf("expected stream name 'CODEFORGE', got %q", info.Config.Name)
	}

	// Verify at least some expected subject patterns are configured.
	subjects := info.Config.Subjects
	if len(subjects) == 0 {
		t.Fatal("CODEFORGE stream has no subject patterns configured")
	}

	// Check that key subject patterns are present.
	expectedPatterns := []string{"tasks.>", "runs.>", "conversation.>"}
	for _, pattern := range expectedPatterns {
		found := false
		for _, s := range subjects {
			if s == pattern {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subject pattern %q in stream, got %v", pattern, subjects)
		}
	}

	t.Logf("CODEFORGE stream: %d subjects, %d messages", len(subjects), info.State.Msgs)
}

// --- Authentication helper ---

// loginAndGetToken authenticates with the backend and returns a JWT token.
// It uses the seeded admin credentials (admin@localhost / Changeme123).
func loginAndGetToken(t *testing.T) string {
	t.Helper()

	client := httpClient()
	body := fmt.Sprintf(`{"email":"admin@localhost","password":"Changeme123"}`)
	resp, err := client.Post(
		baseURL()+"/api/v1/auth/login",
		"application/json",
		stringReader(body),
	)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	token, ok := result["access_token"].(string)
	if !ok || token == "" {
		t.Fatal("login response missing or empty 'access_token'")
	}
	return token
}
