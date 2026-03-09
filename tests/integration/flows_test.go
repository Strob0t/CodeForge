//go:build smoke

package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// stringReader is a convenience wrapper around strings.NewReader.
func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}

// authRequest creates an HTTP request with the Authorization: Bearer header set.
func authRequest(method, url string, body io.Reader, token string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// doJSON executes an HTTP request and parses the JSON response into a map.
// It fails the test on HTTP or decode errors.
func doJSON(t *testing.T, req *http.Request) (int, map[string]any) {
	t.Helper()
	client := httpClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Some endpoints return empty body (e.g. 204), so only fail if
		// we expected JSON (non-204 status).
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("decode response from %s %s (status %d): %v", req.Method, req.URL.Path, resp.StatusCode, err)
		}
	}
	return resp.StatusCode, result
}

// doRaw executes an HTTP request and returns the status code. It discards the body.
func doRaw(t *testing.T, req *http.Request) int {
	t.Helper()
	client := httpClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// doJSONList executes an HTTP request and parses the JSON response as an array.
func doJSONList(t *testing.T, req *http.Request) (int, []map[string]any) {
	t.Helper()
	client := httpClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode list response from %s %s (status %d): %v", req.Method, req.URL.Path, resp.StatusCode, err)
	}
	return resp.StatusCode, result
}

// TestFlow_ProjectLifecycle exercises the full project CRUD lifecycle:
// create, get, list, delete, and verify deletion.
func TestFlow_ProjectLifecycle(t *testing.T) {
	skipIfBackendDown(t)

	token := loginAndGetToken(t)

	// Create a project.
	createReq, err := authRequest(
		http.MethodPost,
		baseURL()+"/api/v1/projects",
		stringReader(`{"name":"smoke-test-project","repo_url":""}`),
		token,
	)
	if err != nil {
		t.Fatalf("build create request: %v", err)
	}

	status, created := doJSON(t, createReq)
	if status != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", status)
	}

	projectID, ok := created["id"].(string)
	if !ok || projectID == "" {
		t.Fatal("create project: missing or empty 'id' in response")
	}

	// Register cleanup to delete the project even if subsequent assertions fail.
	t.Cleanup(func() {
		delReq, _ := authRequest(http.MethodDelete, baseURL()+"/api/v1/projects/"+projectID, http.NoBody, token)
		_ = doRaw(t, delReq)
	})

	// Verify name matches.
	if name, _ := created["name"].(string); name != "smoke-test-project" {
		t.Fatalf("expected name 'smoke-test-project', got %q", name)
	}

	// Get the project by ID.
	getReq, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/projects/"+projectID, http.NoBody, token)
	status, fetched := doJSON(t, getReq)
	if status != http.StatusOK {
		t.Fatalf("get project: expected 200, got %d", status)
	}
	if fetched["id"] != projectID {
		t.Fatalf("get project: expected id %q, got %v", projectID, fetched["id"])
	}

	// List projects and verify our project is included.
	listReq, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/projects", http.NoBody, token)
	status, projects := doJSONList(t, listReq)
	if status != http.StatusOK {
		t.Fatalf("list projects: expected 200, got %d", status)
	}
	found := false
	for _, p := range projects {
		if p["id"] == projectID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list projects: project %s not found in list of %d", projectID, len(projects))
	}

	// Delete the project.
	delReq, _ := authRequest(http.MethodDelete, baseURL()+"/api/v1/projects/"+projectID, http.NoBody, token)
	status = doRaw(t, delReq)
	if status != http.StatusNoContent {
		t.Fatalf("delete project: expected 204, got %d", status)
	}

	// Verify the project is gone (GET should return 404).
	getReq2, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/projects/"+projectID, http.NoBody, token)
	status = doRaw(t, getReq2)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted project: expected 404, got %d", status)
	}
}

// TestFlow_SimpleConversation creates a project, starts a conversation,
// sends a message, and polls for an assistant response.
// Skipped if SMOKE_SKIP_LLM=true.
func TestFlow_SimpleConversation(t *testing.T) {
	if os.Getenv("SMOKE_SKIP_LLM") == "true" {
		t.Skip("SMOKE_SKIP_LLM=true — skipping LLM conversation test")
	}
	skipIfBackendDown(t)

	token := loginAndGetToken(t)

	// Create a project for the conversation.
	createProjReq, _ := authRequest(
		http.MethodPost,
		baseURL()+"/api/v1/projects",
		stringReader(`{"name":"smoke-conv-project","repo_url":""}`),
		token,
	)
	status, projResp := doJSON(t, createProjReq)
	if status != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", status)
	}
	projectID := projResp["id"].(string)

	t.Cleanup(func() {
		delReq, _ := authRequest(http.MethodDelete, baseURL()+"/api/v1/projects/"+projectID, http.NoBody, token)
		_ = doRaw(t, delReq)
	})

	// Create a conversation.
	createConvReq, _ := authRequest(
		http.MethodPost,
		baseURL()+"/api/v1/projects/"+projectID+"/conversations",
		stringReader(`{"title":"smoke-test-conv"}`),
		token,
	)
	status, convResp := doJSON(t, createConvReq)
	if status != http.StatusCreated {
		t.Fatalf("create conversation: expected 201, got %d", status)
	}
	convID, ok := convResp["id"].(string)
	if !ok || convID == "" {
		t.Fatal("create conversation: missing or empty 'id'")
	}

	// Send a message (returns 202 — dispatched to worker).
	sendReq, _ := authRequest(
		http.MethodPost,
		baseURL()+"/api/v1/conversations/"+convID+"/messages",
		stringReader(`{"content":"Say hello"}`),
		token,
	)
	status, _ = doJSON(t, sendReq)
	if status != http.StatusAccepted {
		t.Fatalf("send message: expected 202, got %d", status)
	}

	// Poll for assistant response (max 60s).
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		listMsgReq, _ := authRequest(
			http.MethodGet,
			baseURL()+"/api/v1/conversations/"+convID+"/messages",
			http.NoBody,
			token,
		)
		status, messages := doJSONList(t, listMsgReq)
		if status != http.StatusOK {
			t.Fatalf("list messages: expected 200, got %d", status)
		}

		// Look for an assistant message.
		for _, msg := range messages {
			if msg["role"] == "assistant" {
				content, _ := msg["content"].(string)
				if content != "" {
					t.Logf("assistant responded: %s", truncate(content, 100))
					return // Success.
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	t.Fatal("timed out waiting for assistant response (60s)")
}

// TestFlow_CostTracking verifies that the cost summary endpoint is accessible
// and returns valid JSON.
func TestFlow_CostTracking(t *testing.T) {
	skipIfBackendDown(t)

	token := loginAndGetToken(t)

	req, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/costs", http.NoBody, token)
	client := httpClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/costs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cost summary: expected 200, got %d", resp.StatusCode)
	}

	// The response should be valid JSON (array of project summaries).
	var result json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode cost summary: %v", err)
	}
}

// TestFlow_ModesAvailable verifies that the modes endpoint returns at least
// one built-in mode.
func TestFlow_ModesAvailable(t *testing.T) {
	skipIfBackendDown(t)

	token := loginAndGetToken(t)

	req, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/modes", http.NoBody, token)
	status, modes := doJSONList(t, req)
	if status != http.StatusOK {
		t.Fatalf("list modes: expected 200, got %d", status)
	}
	if len(modes) == 0 {
		t.Fatal("expected at least 1 mode, got 0")
	}
	t.Logf("found %d modes", len(modes))
}

// TestFlow_LLMModelsAvailable verifies that the model registry returns at least
// one available LLM model. Skipped if SMOKE_SKIP_LLM=true.
func TestFlow_LLMModelsAvailable(t *testing.T) {
	if os.Getenv("SMOKE_SKIP_LLM") == "true" {
		t.Skip("SMOKE_SKIP_LLM=true — skipping LLM models check")
	}
	skipIfBackendDown(t)

	token := loginAndGetToken(t)

	req, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/llm/available", http.NoBody, token)
	client := httpClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/llm/available: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// The endpoint may return 503 if model registry is not initialized.
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Skip("model registry not initialized — skipping")
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("available models: expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode available models: %v", err)
	}

	models, ok := result["models"].([]any)
	if !ok {
		t.Fatal("response missing 'models' array")
	}
	if len(models) == 0 {
		t.Fatal("expected at least 1 available model, got 0")
	}
	t.Logf("found %d available models", len(models))
}

// TestFlow_PoliciesAvailable verifies that the policies endpoint returns at
// least one built-in policy profile.
func TestFlow_PoliciesAvailable(t *testing.T) {
	skipIfBackendDown(t)

	token := loginAndGetToken(t)

	req, _ := authRequest(http.MethodGet, baseURL()+"/api/v1/policies", http.NoBody, token)
	status, result := doJSON(t, req)
	if status != http.StatusOK {
		t.Fatalf("list policies: expected 200, got %d", status)
	}

	profiles, ok := result["profiles"].([]any)
	if !ok {
		t.Fatal("response missing 'profiles' array")
	}
	if len(profiles) == 0 {
		t.Fatal("expected at least 1 policy profile, got 0")
	}
	t.Logf("found %d policy profiles", len(profiles))
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return fmt.Sprintf("%s...", s[:maxLen])
}
