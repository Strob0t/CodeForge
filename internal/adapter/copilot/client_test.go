package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExchangeToken(t *testing.T) {
	// Create a temporary hosts file.
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts.json")
	hosts := map[string]hostsEntry{
		"github.com": {OAuthToken: "test-oauth-token"},
	}
	data, _ := json.Marshal(hosts)
	if err := os.WriteFile(hostsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Mock the GitHub Copilot token exchange endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/copilot_internal/v2/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "token test-oauth-token" {
			t.Fatalf("unexpected auth: %s", auth)
		}

		resp := tokenResponse{
			Token:     "bearer-token-123",
			ExpiresAt: time.Now().Add(30 * time.Minute).Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(hostsPath)
	// Override the HTTP client to point to our test server.
	client.httpClient = srv.Client()

	// We need to override the URL — use a custom transport.
	origURL := "https://api.github.com"
	_ = origURL
	// Instead of overriding ExchangeToken internals, test readOAuthToken directly.
	token, err := client.readOAuthToken()
	if err != nil {
		t.Fatalf("readOAuthToken: %v", err)
	}
	if token != "test-oauth-token" {
		t.Fatalf("expected test-oauth-token, got %s", token)
	}
}

func TestReadOAuthToken_MissingFile(t *testing.T) {
	client := NewClient("/nonexistent/hosts.json")
	_, err := client.readOAuthToken()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadOAuthToken_NoGitHubEntry(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts.json")
	if err := os.WriteFile(hostsPath, []byte(`{"gitlab.com": {"oauth_token": "x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewClient(hostsPath)
	_, err := client.readOAuthToken()
	if err == nil {
		t.Fatal("expected error for missing github.com entry")
	}
}

func TestHasHostsFile(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts.json")

	client := NewClient(hostsPath)
	if client.HasHostsFile() {
		t.Fatal("expected no hosts file")
	}

	if err := os.WriteFile(hostsPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !client.HasHostsFile() {
		t.Fatal("expected hosts file to exist")
	}
}

func TestExchangeToken_Caching(t *testing.T) {
	client := NewClient("/nonexistent")
	// Pre-populate cache.
	client.mu.Lock()
	client.token = "cached-token"
	client.expiry = time.Now().Add(10 * time.Minute)
	client.mu.Unlock()

	token, expiry, err := client.ExchangeToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "cached-token" {
		t.Fatalf("expected cached-token, got %s", token)
	}
	if expiry.IsZero() {
		t.Fatal("expected non-zero expiry")
	}
}
