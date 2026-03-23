package service_test

import (
	"context"
	"testing"

	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	"github.com/Strob0t/CodeForge/internal/service"
)

// bpMockStore overrides branch-protection methods on the base runtimeMockStore.
type bpMockStore struct {
	runtimeMockStore
	rules []bp.ProtectionRule
}

func (m *bpMockStore) CreateBranchProtectionRule(_ context.Context, req bp.CreateRuleRequest) (*bp.ProtectionRule, error) {
	r := bp.ProtectionRule{
		ID:             "bp-1",
		ProjectID:      req.ProjectID,
		BranchPattern:  req.BranchPattern,
		RequireReviews: req.RequireReviews,
		RequireTests:   req.RequireTests,
		RequireLint:    req.RequireLint,
		AllowForcePush: req.AllowForcePush,
		AllowDelete:    req.AllowDelete,
		Enabled:        req.Enabled,
	}
	m.rules = append(m.rules, r)
	return &r, nil
}

func (m *bpMockStore) GetBranchProtectionRule(_ context.Context, id string) (*bp.ProtectionRule, error) {
	for i := range m.rules {
		if m.rules[i].ID == id {
			return &m.rules[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *bpMockStore) ListBranchProtectionRules(_ context.Context, projectID string) ([]bp.ProtectionRule, error) {
	var result []bp.ProtectionRule
	for i := range m.rules {
		if m.rules[i].ProjectID == projectID {
			result = append(result, m.rules[i])
		}
	}
	return result, nil
}

func (m *bpMockStore) UpdateBranchProtectionRule(_ context.Context, rule *bp.ProtectionRule) error {
	for i := range m.rules {
		if m.rules[i].ID == rule.ID {
			m.rules[i] = *rule
			return nil
		}
	}
	return errMockNotFound
}

func (m *bpMockStore) DeleteBranchProtectionRule(_ context.Context, id string) error {
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

func TestBranchProtectionService_CreateRule(t *testing.T) {
	tests := []struct {
		name    string
		req     bp.CreateRuleRequest
		wantErr bool
	}{
		{
			name: "valid",
			req: bp.CreateRuleRequest{
				ProjectID:     "proj-1",
				BranchPattern: "main",
				Enabled:       true,
			},
			wantErr: false,
		},
		{
			name: "empty_pattern",
			req: bp.CreateRuleRequest{
				ProjectID:     "proj-1",
				BranchPattern: "",
			},
			wantErr: true,
		},
		{
			name: "empty_project_id",
			req: bp.CreateRuleRequest{
				ProjectID:     "",
				BranchPattern: "main",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{}
			svc := service.NewBranchProtectionService(store)
			rule, err := svc.CreateRule(context.Background(), tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rule.BranchPattern != tt.req.BranchPattern {
				t.Errorf("BranchPattern = %q, want %q", rule.BranchPattern, tt.req.BranchPattern)
			}
		})
	}
}

func TestBranchProtectionService_GetRule(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		seed    []bp.ProtectionRule
		wantErr bool
	}{
		{
			name: "found",
			id:   "bp-1",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main"},
			},
			wantErr: false,
		},
		{
			name:    "not_found",
			id:      "bp-999",
			seed:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{rules: tt.seed}
			svc := service.NewBranchProtectionService(store)
			rule, err := svc.GetRule(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rule.ID != tt.id {
				t.Errorf("ID = %q, want %q", rule.ID, tt.id)
			}
		})
	}
}

func TestBranchProtectionService_ListRules(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		seed      []bp.ProtectionRule
		wantCount int
	}{
		{
			name:      "empty",
			projectID: "proj-1",
			seed:      nil,
			wantCount: 0,
		},
		{
			name:      "with_rules",
			projectID: "proj-1",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main"},
				{ID: "bp-2", ProjectID: "proj-1", BranchPattern: "release/*"},
				{ID: "bp-3", ProjectID: "proj-2", BranchPattern: "main"},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{rules: tt.seed}
			svc := service.NewBranchProtectionService(store)
			rules, err := svc.ListRules(context.Background(), tt.projectID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rules) != tt.wantCount {
				t.Errorf("got %d rules, want %d", len(rules), tt.wantCount)
			}
		})
	}
}

func TestBranchProtectionService_UpdateRule(t *testing.T) {
	newPattern := "develop"

	tests := []struct {
		name    string
		id      string
		seed    []bp.ProtectionRule
		req     bp.UpdateRuleRequest
		wantErr bool
	}{
		{
			name: "valid",
			id:   "bp-1",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main"},
			},
			req:     bp.UpdateRuleRequest{BranchPattern: &newPattern},
			wantErr: false,
		},
		{
			name:    "not_found",
			id:      "bp-999",
			seed:    nil,
			req:     bp.UpdateRuleRequest{BranchPattern: &newPattern},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{rules: tt.seed}
			svc := service.NewBranchProtectionService(store)
			rule, err := svc.UpdateRule(context.Background(), tt.id, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rule.BranchPattern != newPattern {
				t.Errorf("BranchPattern = %q, want %q", rule.BranchPattern, newPattern)
			}
		})
	}
}

func TestBranchProtectionService_DeleteRule(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		seed    []bp.ProtectionRule
		wantErr bool
	}{
		{
			name: "valid",
			id:   "bp-1",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main"},
			},
			wantErr: false,
		},
		{
			name:    "not_found",
			id:      "bp-999",
			seed:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{rules: tt.seed}
			svc := service.NewBranchProtectionService(store)
			err := svc.DeleteRule(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBranchProtectionService_CheckBranch(t *testing.T) {
	tests := []struct {
		name        string
		seed        []bp.ProtectionRule
		action      bp.PushAction
		wantAllowed bool
	}{
		{
			name: "allowed",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main", Enabled: true, AllowForcePush: false},
			},
			action:      bp.PushAction{Branch: "main", ForcePush: false},
			wantAllowed: true,
		},
		{
			name: "blocked_force_push",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main", Enabled: true, AllowForcePush: false},
			},
			action:      bp.PushAction{Branch: "main", ForcePush: true},
			wantAllowed: false,
		},
		{
			name:        "no_rules_allows",
			seed:        nil,
			action:      bp.PushAction{Branch: "main", ForcePush: true},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{rules: tt.seed}
			svc := service.NewBranchProtectionService(store)
			result, err := svc.CheckBranch(context.Background(), "proj-1", tt.action)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v (reason: %s)", result.Allowed, tt.wantAllowed, result.Reason)
			}
		})
	}
}

func TestBranchProtectionService_CheckMerge(t *testing.T) {
	tests := []struct {
		name        string
		seed        []bp.ProtectionRule
		action      bp.MergeAction
		wantAllowed bool
	}{
		{
			name: "allowed",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main", Enabled: true, RequireTests: true, RequireLint: true},
			},
			action:      bp.MergeAction{TargetBranch: "main", TestsPassed: true, LintPassed: true},
			wantAllowed: true,
		},
		{
			name: "blocked_tests_failed",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main", Enabled: true, RequireTests: true},
			},
			action:      bp.MergeAction{TargetBranch: "main", TestsPassed: false},
			wantAllowed: false,
		},
		{
			name: "blocked_lint_failed",
			seed: []bp.ProtectionRule{
				{ID: "bp-1", ProjectID: "proj-1", BranchPattern: "main", Enabled: true, RequireLint: true},
			},
			action:      bp.MergeAction{TargetBranch: "main", LintPassed: false},
			wantAllowed: false,
		},
		{
			name:        "no_rules_allows",
			seed:        nil,
			action:      bp.MergeAction{TargetBranch: "main", TestsPassed: false},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &bpMockStore{rules: tt.seed}
			svc := service.NewBranchProtectionService(store)
			result, err := svc.CheckMerge(context.Background(), "proj-1", tt.action)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v (reason: %s)", result.Allowed, tt.wantAllowed, result.Reason)
			}
		})
	}
}
