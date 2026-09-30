package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// KI-8: profile reads (NATS tool-call handlers) and writes (HTTP handlers)
// run concurrently. Run with -race.
func TestPolicyService_ConcurrentEvaluateAndUpdate(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{
		{Name: "shared", Mode: policy.ModeDefault},
	})
	ctx := context.Background()
	const workers, iterations = 8, 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = svc.Evaluate(ctx, "shared", policy.ToolCall{Tool: "bash", Command: "go test ./..."})
				_, _ = svc.EvaluateWithReason(ctx, "headless-safe-sandbox", policy.ToolCall{Tool: "read_file", Path: "x"})
			}
		}()
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = svc.SaveProfile(&policy.PolicyProfile{Name: fmt.Sprintf("p-%d-%d", w, i%10), Mode: policy.ModeDefault})
				_ = svc.DeleteProfile(fmt.Sprintf("p-%d-%d", w, (i+5)%10))
			}
		}(w)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = svc.PrependRule("shared", &policy.PermissionRule{
					Specifier:    policy.ToolSpecifier{Tool: "Bash"},
					Decision:     policy.DecisionAllow,
					CommandAllow: []string{fmt.Sprintf("tool%d-%d", w, i)},
				})
			}
		}(w)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = svc.ListProfiles()
				_, _ = svc.GetProfile("shared")
			}
		}()
	}
	wg.Wait()

	p, ok := svc.GetProfile("shared")
	if !ok {
		t.Fatal("shared profile missing")
	}
	if len(p.Rules) != workers*iterations {
		t.Fatalf("expected %d prepended rules, got %d (lost updates)", workers*iterations, len(p.Rules))
	}
}

// KI-9: built-in presets cannot be replaced through SaveProfile.
func TestSaveProfile_RejectsBuiltinPresets(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)
	for _, name := range policy.PresetNames() {
		allowAll := policy.PolicyProfile{
			Name:  name,
			Mode:  policy.ModeAcceptEdits,
			Rules: []policy.PermissionRule{{Specifier: policy.ToolSpecifier{Tool: "*"}, Decision: policy.DecisionAllow}},
		}
		err := svc.SaveProfile(&allowAll)
		if !errors.Is(err, domain.ErrConflict) {
			t.Errorf("%s: SaveProfile error = %v, want ErrConflict", name, err)
		}
		got, _ := svc.GetProfile(name)
		want, _ := policy.PresetByName(name)
		if got.Mode != want.Mode || len(got.Rules) != len(want.Rules) {
			t.Errorf("%s: preset was replaced", name)
		}
	}
}

func TestSaveProfile_ValidationErrorIsValidation(t *testing.T) {
	svc := NewPolicyService("headless-safe-sandbox", nil)
	err := svc.SaveProfile(&policy.PolicyProfile{Name: "bad", Mode: "yolo"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("SaveProfile error = %v, want ErrValidation", err)
	}
}

func TestAllowAlwaysRule(t *testing.T) {
	tests := []struct {
		tool, command string
		wantTool      string
		wantAllow     []string
		wantErr       bool
	}{
		{"bash", "git status", "Bash", []string{"git"}, false},
		// Every executable of the approved command, so the same call matches again.
		{"bash", "cd frontend && npm test", "Bash", []string{"cd", "npm"}, false},
		{"bash", "/usr/bin/git log | head -5 && git diff", "Bash", []string{"git", "head"}, false},
		{"bash", "timeout 60 go test ./...", "Bash", []string{"go"}, false},
		{"command:execute", "make lint", "Bash", []string{"make"}, false},
		{"Bash", "FOO=1 npm test", "", nil, true},
		{"bash", "go test ./... && $(curl x)", "", nil, true},
		{"write_file", `{"file_path": "x"}`, "Write", nil, false},
		{"mcp__github__create_issue", "", "mcp__github__create_issue", nil, false},
		{"bash", `{"command":"ls"}`, "", nil, true},
		{"bash", "", "", nil, true},
		{"bash", "sh -c 'ls'", "", nil, true},
		{"*", "", "", nil, true},
		{"mcp__*", "", "", nil, true},
	}
	for _, tt := range tests {
		rule, err := allowAlwaysRule(tt.tool, tt.command)
		if (err != nil) != tt.wantErr {
			t.Errorf("allowAlwaysRule(%q, %q) error = %v, wantErr %v", tt.tool, tt.command, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			if !errors.Is(err, domain.ErrValidation) {
				t.Errorf("allowAlwaysRule(%q, %q) error = %v, want ErrValidation", tt.tool, tt.command, err)
			}
			continue
		}
		if rule.Specifier.Tool != tt.wantTool || rule.Specifier.SubPattern != "" || rule.Decision != policy.DecisionAllow {
			t.Errorf("allowAlwaysRule(%q, %q) = %+v", tt.tool, tt.command, rule)
		}
		if fmt.Sprint(rule.CommandAllow) != fmt.Sprint(tt.wantAllow) {
			t.Errorf("allowAlwaysRule(%q, %q).CommandAllow = %v, want %v", tt.tool, tt.command, rule.CommandAllow, tt.wantAllow)
		}
	}
}

// A rule prepended by Allow-Always never overrides a deny list (ADR-015).
func TestAllowAlways_DoesNotOverrideDenyLists(t *testing.T) {
	svc := NewPolicyService("headless-permissive-sandbox", nil)
	svc.SetPolicyDir(t.TempDir())
	projects := &stubProjects{proj: project.Project{ID: "p1"}}
	ctx := context.Background()

	if _, err := svc.AllowAlways(ctx, projects, "p1", "write_file", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AllowAlways(ctx, projects, "p1", "bash", "curl https://example.com"); err != nil {
		t.Fatal(err)
	}
	name := projects.proj.PolicyProfile
	for _, call := range []policy.ToolCall{
		{Tool: "write_file", Path: ".env"},
		{Tool: "bash", Command: "curl https://example.com"},
	} {
		if d, _ := svc.Evaluate(ctx, name, call); d != policy.DecisionDeny {
			t.Errorf("%+v after allow-always -> %s, want deny", call, d)
		}
	}
}

type stubProjects struct {
	proj project.Project
}

func (s *stubProjects) Get(_ context.Context, id string) (*project.Project, error) {
	if id != s.proj.ID {
		return nil, domain.ErrNotFound
	}
	p := s.proj
	return &p, nil
}

func (s *stubProjects) SetPolicyProfile(_ context.Context, _, profile string) error {
	s.proj.PolicyProfile = profile
	return nil
}
