package service

import (
	"context"
	"testing"
)

func TestHandleGitHubPush(t *testing.T) {
	bc := &mockBroadcaster{}
	svc := NewVCSWebhookService(bc)

	payload := []byte(`{
		"ref": "refs/heads/main",
		"before": "aaa",
		"after": "bbb",
		"forced": false,
		"repository": {"full_name": "owner/repo"},
		"sender": {"login": "user1"},
		"commits": [
			{
				"id": "bbb",
				"message": "fix bug",
				"author": {"name": "User One"},
				"added": ["new.txt"],
				"modified": ["old.txt"],
				"removed": []
			}
		]
	}`)

	ev, err := svc.HandleGitHubPush(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Repository != "owner/repo" {
		t.Fatalf("expected 'owner/repo', got %q", ev.Repository)
	}
	if ev.Branch != "main" {
		t.Fatalf("expected 'main', got %q", ev.Branch)
	}
	if len(ev.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(ev.Commits))
	}
	if ev.FileCount != 2 {
		t.Fatalf("expected 2 files, got %d", ev.FileCount)
	}
	if len(bc.events) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(bc.events))
	}
}

func TestHandleGitLabPush(t *testing.T) {
	bc := &mockBroadcaster{}
	svc := NewVCSWebhookService(bc)

	payload := []byte(`{
		"ref": "refs/heads/develop",
		"before": "ccc",
		"after": "ddd",
		"project": {"path_with_namespace": "group/project"},
		"user_username": "dev1",
		"commits": [
			{
				"id": "ddd",
				"message": "add feature",
				"author": {"name": "Dev One"},
				"added": ["feature.go"],
				"modified": [],
				"removed": []
			}
		]
	}`)

	ev, err := svc.HandleGitLabPush(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Provider != "gitlab" {
		t.Fatalf("expected 'gitlab', got %q", ev.Provider)
	}
	if ev.Branch != "develop" {
		t.Fatalf("expected 'develop', got %q", ev.Branch)
	}
}

func TestHandleGitHubPullRequest(t *testing.T) {
	bc := &mockBroadcaster{}
	svc := NewVCSWebhookService(bc)

	payload := []byte(`{
		"action": "opened",
		"pull_request": {
			"number": 42,
			"title": "Add new feature",
			"draft": false,
			"head": {"ref": "feature/x", "sha": "abc123"},
			"base": {"ref": "main"}
		},
		"repository": {"full_name": "owner/repo"},
		"sender": {"login": "contributor"}
	}`)

	ev, err := svc.HandleGitHubPullRequest(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.PRNumber != 42 {
		t.Fatalf("expected PR 42, got %d", ev.PRNumber)
	}
	if ev.Action != "opened" {
		t.Fatalf("expected 'opened', got %q", ev.Action)
	}
	if ev.HeadBranch != "feature/x" {
		t.Fatalf("expected 'feature/x', got %q", ev.HeadBranch)
	}
}

func TestExtractBranchFromRef(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"refs/heads/main", "main"},
		{"refs/heads/feature/x", "feature/x"},
		{"main", "main"},
	}
	for _, tt := range tests {
		got := extractBranchFromRef(tt.ref)
		if got != tt.want {
			t.Errorf("extractBranchFromRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
