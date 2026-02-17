//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHealthLiveness(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected status 'ok', got %q", body.Status)
	}
}

func TestAPIVersion(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/api/v1/")
	if err != nil {
		t.Fatalf("GET /api/v1/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Version == "" {
		t.Fatal("expected non-empty version")
	}
}
