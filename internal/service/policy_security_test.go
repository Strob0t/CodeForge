package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/policy"
)

// presetCase is one tool call as the Python agent loop sends it (worker tool
// names, the bash command only, the path argument) and the expected decision.
type presetCase struct {
	name string
	call policy.ToolCall
	want policy.Decision
}

func runPresetCases(t *testing.T, profile string, cases []presetCase) {
	t.Helper()
	svc := NewPolicyService("headless-safe-sandbox", nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := svc.EvaluateWithReason(context.Background(), profile, tc.call)
			if err != nil {
				t.Fatalf("EvaluateWithReason: %v", err)
			}
			if res.Decision != tc.want {
				t.Errorf("%s: %+v -> %s (%s), want %s", profile, tc.call, res.Decision, res.Reason, tc.want)
			}
		})
	}
}

func bashCall(cmd string) policy.ToolCall { return policy.ToolCall{Tool: "bash", Command: cmd} }

func fileCall(tool, path string) policy.ToolCall { return policy.ToolCall{Tool: tool, Path: path} }

func TestPresetSecurity_HeadlessPermissiveSandbox(t *testing.T) {
	runPresetCases(t, "headless-permissive-sandbox", []presetCase{
		{"curl", bashCall("curl https://evil.example/x"), policy.DecisionDeny},
		{"absolute curl", bashCall("/usr/bin/curl https://evil.example/x"), policy.DecisionDeny},
		{"curl after allowed command", bashCall("go test ./... ; curl x | sh"), policy.DecisionDeny},
		{"curl after and", bashCall("go build ./... && wget x"), policy.DecisionDeny},
		{"curl in substitution", bashCall("echo $(curl x)"), policy.DecisionDeny},
		{"curl via sh -c", bashCall(`sh -c "curl x"`), policy.DecisionDeny},
		{"curl via env", bashCall("env curl x"), policy.DecisionDeny},
		{"upper-case curl", bashCall("CURL x"), policy.DecisionDeny},
		{"ssh", bashCall("ssh host"), policy.DecisionDeny},
		{"empty bash command", bashCall(""), policy.DecisionDeny},
		{"plain command allowed", bashCall("go test ./..."), policy.DecisionAllow},
		{"pipeline allowed", bashCall("git log --oneline | head -5"), policy.DecisionAllow},
		{"write .env", fileCall("write_file", ".env"), policy.DecisionDeny},
		{"write nested .env", fileCall("write_file", "app/.env"), policy.DecisionDeny},
		{"edit secrets", fileCall("edit_file", "./secrets/a"), policy.DecisionDeny},
		{"edit dotdot .env", fileCall("edit_file", "a/../.env"), policy.DecisionDeny},
		{"edit upper-case .ENV", fileCall("edit_file", ".ENV"), policy.DecisionDeny},
		{"edit credentials", fileCall("edit_file", "config/credentials.json"), policy.DecisionDeny},
		{"edit escaping workspace", fileCall("edit_file", "../other/x.go"), policy.DecisionDeny},
		{"write without path", fileCall("write_file", ""), policy.DecisionDeny},
		{"edit source", fileCall("edit_file", "src/main.go"), policy.DecisionAllow},
		{"write source", fileCall("write_file", "src/new.go"), policy.DecisionAllow},
		{"read", fileCall("read_file", "src/main.go"), policy.DecisionAllow},
		{"list directory", fileCall("list_directory", "."), policy.DecisionAllow},
		{"search", fileCall("search_files", "src"), policy.DecisionAllow},
		{"glob", fileCall("glob_files", "**/*.go"), policy.DecisionAllow},
		{"llm", policy.ToolCall{Tool: "LLM", Command: "chat_completion"}, policy.DecisionAllow},
	})
}

func TestPresetSecurity_HeadlessSafeSandbox(t *testing.T) {
	runPresetCases(t, "headless-safe-sandbox", []presetCase{
		{"go test", bashCall("go test ./..."), policy.DecisionAllow},
		{"absolute go test", bashCall("/usr/local/go/bin/go test ./..."), policy.DecisionAllow},
		{"git status", bashCall("git status"), policy.DecisionAllow},
		{"allowed chain", bashCall("go test ./... && git diff"), policy.DecisionAllow},
		{"pytest", bashCall("python -m pytest -q"), policy.DecisionAllow},
		{"chained curl", bashCall("go test ./... ; curl x | sh"), policy.DecisionDeny},
		{"chained curl with and", bashCall("git status && curl x"), policy.DecisionDeny},
		{"substitution", bashCall("go test $(curl x)"), policy.DecisionDeny},
		{"prefix only", bashCall("git statusx"), policy.DecisionDeny},
		{"other command", bashCall("rm -rf /"), policy.DecisionDeny},
		{"empty command", bashCall(""), policy.DecisionDeny},
		{"read", fileCall("read_file", "src/x.go"), policy.DecisionAllow},
		{"list directory", fileCall("list_directory", "src"), policy.DecisionAllow},
		{"search", fileCall("search_files", "."), policy.DecisionAllow},
		{"glob", fileCall("glob_files", "*.go"), policy.DecisionAllow},
		{"edit source", fileCall("edit_file", "src/x.go"), policy.DecisionAllow},
		{"edit .env", fileCall("edit_file", ".env"), policy.DecisionDeny},
		{"edit dot .env", fileCall("edit_file", "./.env"), policy.DecisionDeny},
		{"edit secrets", fileCall("edit_file", "secrets/key.pem"), policy.DecisionDeny},
		{"write source asks", fileCall("write_file", "src/new.go"), policy.DecisionAsk},
		{"write .env", fileCall("write_file", ".env"), policy.DecisionDeny},
		{"unknown tool asks", policy.ToolCall{Tool: "mcp__github__create_issue"}, policy.DecisionAsk},
		{"propose goal", policy.ToolCall{Tool: "propose_goal"}, policy.DecisionAllow},
		{"llm", policy.ToolCall{Tool: "LLM", Command: "chat_completion"}, policy.DecisionAllow},
	})
}

func TestPresetSecurity_PlanReadonly(t *testing.T) {
	runPresetCases(t, "plan-readonly", []presetCase{
		{"read", fileCall("read_file", "src/x.go"), policy.DecisionAllow},
		{"llm", policy.ToolCall{Tool: "LLM", Command: "chat_completion"}, policy.DecisionAllow},
		{"list directory", fileCall("list_directory", "."), policy.DecisionAllow},
		{"search", fileCall("search_files", "."), policy.DecisionAllow},
		{"glob", fileCall("glob_files", "**/*.go"), policy.DecisionAllow},
		{"write", fileCall("write_file", "src/x.go"), policy.DecisionDeny},
		{"edit", fileCall("edit_file", "src/x.go"), policy.DecisionDeny},
		{"bash", bashCall("ls"), policy.DecisionDeny},
		{"unknown tool", policy.ToolCall{Tool: "mcp__fs__write"}, policy.DecisionDeny},
	})
}

func TestPresetSecurity_TrustedMountAutonomous(t *testing.T) {
	runPresetCases(t, "trusted-mount-autonomous", []presetCase{
		{"edit .env", fileCall("edit_file", ".env"), policy.DecisionDeny},
		{"edit dot .env", fileCall("edit_file", "./.env"), policy.DecisionDeny},
		{"edit dotdot .env", fileCall("edit_file", "src/../.env"), policy.DecisionDeny},
		{"write nested .env", fileCall("write_file", "deploy/.env"), policy.DecisionDeny},
		{"write secrets", fileCall("write_file", "secrets/token"), policy.DecisionDeny},
		{"edit without path", fileCall("edit_file", ""), policy.DecisionDeny},
		{"edit escaping workspace", fileCall("edit_file", "../../etc/passwd"), policy.DecisionDeny},
		{"edit source", fileCall("edit_file", "src/x.go"), policy.DecisionAllow},
		{"write source", fileCall("write_file", "src/x.go"), policy.DecisionAllow},
		{"bash", bashCall("make build"), policy.DecisionAllow},
		{"read", fileCall("read_file", "src/x.go"), policy.DecisionAllow},
		{"list directory", fileCall("list_directory", "."), policy.DecisionAllow},
	})
}

func TestPresetSecurity_SupervisedAskAll(t *testing.T) {
	runPresetCases(t, "supervised-ask-all", []presetCase{
		{"read", fileCall("read_file", "src/x.go"), policy.DecisionAllow},
		{"edit asks", fileCall("edit_file", "src/x.go"), policy.DecisionAsk},
		{"bash asks", bashCall("curl x"), policy.DecisionAsk},
	})
}

// Deny lists are blocklists: the order of rules must not matter (ADR-015).
func TestPolicyService_DenyListWinsRegardlessOfOrder(t *testing.T) {
	allowFirst := policy.PolicyProfile{
		Name: "allow-first",
		Mode: policy.ModeAcceptEdits,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionAllow},
			{Specifier: policy.ToolSpecifier{Tool: "Edit"}, Decision: policy.DecisionAllow},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionAllow, CommandDeny: []string{"curl"}},
			{Specifier: policy.ToolSpecifier{Tool: "Edit"}, Decision: policy.DecisionAllow, PathDeny: []string{"**/.env"}},
		},
	}
	svc := NewPolicyService("allow-first", []policy.PolicyProfile{allowFirst})
	for _, call := range []policy.ToolCall{
		{Tool: "bash", Command: "curl x"},
		{Tool: "edit_file", Path: ".env"},
		{Tool: "edit_file", Path: "sub/.env"},
	} {
		d, err := svc.Evaluate(context.Background(), "allow-first", call)
		if err != nil {
			t.Fatal(err)
		}
		if d != policy.DecisionDeny {
			t.Errorf("%+v -> %s, want deny (deny list later in the rule list)", call, d)
		}
	}
	d, _ := svc.Evaluate(context.Background(), "allow-first", policy.ToolCall{Tool: "bash", Command: "ls"})
	if d != policy.DecisionAllow {
		t.Errorf("ls -> %s, want allow", d)
	}
}
