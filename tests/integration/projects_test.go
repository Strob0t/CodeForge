//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestProjectCRUDLifecycle(t *testing.T) {
	// Clean before this test
	cleanDB(testPool)

	// 1. List projects — should be empty
	resp, err := http.Get(testServer.URL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}

	var projects []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}

	// 2. Create a project
	createBody, _ := json.Marshal(map[string]any{
		"name":        "test-project",
		"description": "integration test project",
		"repo_url":    "https://github.com/example/repo",
		"provider":    "local",
		"config":      map[string]string{},
	})

	resp2, err := http.Post(testServer.URL+"/api/v1/projects", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp2.StatusCode)
	}

	var created map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}

	projectID, ok := created["id"].(string)
	if !ok || projectID == "" {
		t.Fatal("expected non-empty project ID")
	}
	if created["name"] != "test-project" {
		t.Fatalf("expected name 'test-project', got %v", created["name"])
	}

	// 3. Get the project by ID
	resp3, err := http.Get(testServer.URL + "/api/v1/projects/" + projectID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp3.StatusCode)
	}

	var fetched map[string]any
	if err := json.NewDecoder(resp3.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode fetched: %v", err)
	}
	if fetched["id"] != projectID {
		t.Fatalf("expected ID %q, got %v", projectID, fetched["id"])
	}

	// 4. List projects — should have 1
	resp4, err := http.Get(testServer.URL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("list after create: %v", err)
	}
	defer func() { _ = resp4.Body.Close() }()

	var listed []map[string]any
	if err := json.NewDecoder(resp4.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 project, got %d", len(listed))
	}

	// 5. Delete the project
	req, _ := http.NewRequest(http.MethodDelete, testServer.URL+"/api/v1/projects/"+projectID, http.NoBody)
	resp5, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete project: %v", err)
	}
	defer func() { _ = resp5.Body.Close() }()

	if resp5.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp5.StatusCode)
	}

	// 6. Get deleted project — should be 404
	resp6, err := http.Get(testServer.URL + "/api/v1/projects/" + projectID)
	if err != nil {
		t.Fatalf("get deleted: %v", err)
	}
	defer func() { _ = resp6.Body.Close() }()

	if resp6.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted: expected 404, got %d", resp6.StatusCode)
	}
}

func TestCreateProjectValidation(t *testing.T) {
	// Missing name should return 400
	body, _ := json.Marshal(map[string]any{
		"description": "no name",
	})

	resp, err := http.Post(testServer.URL+"/api/v1/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create without name: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetNonexistentProject(t *testing.T) {
	resp, err := http.Get(testServer.URL + "/api/v1/projects/00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("get nonexistent: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTaskCRUDLifecycle(t *testing.T) {
	cleanDB(testPool)

	// Create a project first
	projBody, _ := json.Marshal(map[string]any{
		"name":     "task-test-project",
		"provider": "local",
		"config":   map[string]string{},
	})
	resp, err := http.Post(testServer.URL+"/api/v1/projects", "application/json", bytes.NewReader(projBody))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var proj map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&proj)
	projectID := proj["id"].(string)

	// Create a task
	taskBody, _ := json.Marshal(map[string]any{
		"title":  "Fix the bug",
		"prompt": "Find and fix the null pointer exception",
	})
	resp2, err := http.Post(testServer.URL+"/api/v1/projects/"+projectID+"/tasks", "application/json", bytes.NewReader(taskBody))
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create task: expected 201, got %d", resp2.StatusCode)
	}

	var createdTask map[string]any
	_ = json.NewDecoder(resp2.Body).Decode(&createdTask)
	taskID := createdTask["id"].(string)

	if createdTask["title"] != "Fix the bug" {
		t.Fatalf("expected title 'Fix the bug', got %v", createdTask["title"])
	}
	if createdTask["status"] != "pending" {
		t.Fatalf("expected status 'pending', got %v", createdTask["status"])
	}

	// Get the task by ID
	resp3, err := http.Get(testServer.URL + "/api/v1/tasks/" + taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()

	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("get task: expected 200, got %d", resp3.StatusCode)
	}

	// List tasks for project
	resp4, err := http.Get(testServer.URL + "/api/v1/projects/" + projectID + "/tasks")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	defer func() { _ = resp4.Body.Close() }()

	var tasks []map[string]any
	_ = json.NewDecoder(resp4.Body).Decode(&tasks)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}
