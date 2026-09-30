package policy

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

const testWorkspace = "/srv/ws/p1"

func TestEvaluate_DenyListsIgnoreRuleOrder(t *testing.T) {
	// The broad allow rules come first; the deny lists sit on later rules.
	p := PolicyProfile{
		Name: "order",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: ToolBash}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAsk, PathDeny: []string{"**/.env"}},
			{Specifier: ToolSpecifier{Tool: ToolBash}, Decision: DecisionAsk, CommandDeny: []string{"curl"}},
		},
	}
	tests := []struct {
		name string
		call ToolCall
		want Decision
		rule int
	}{
		{"denied path", ToolCall{Tool: "edit_file", Path: "a/.env"}, DecisionDeny, 2},
		{"allowed path", ToolCall{Tool: "edit_file", Path: "a/b.go"}, DecisionAllow, 0},
		{"denied command", ToolCall{Tool: "bash", Command: "ls; curl x"}, DecisionDeny, 3},
		{"allowed command", ToolCall{Tool: "bash", Command: "ls"}, DecisionAllow, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := p.Evaluate(tt.call, WithWorkspace(testWorkspace))
			if res.Decision != tt.want || res.RuleIndex != tt.rule {
				t.Errorf("got %s rule %d (%s), want %s rule %d", res.Decision, res.RuleIndex, res.Reason, tt.want, tt.rule)
			}
		})
	}
}

func TestEvaluate_DenyListsFailClosedWithoutValue(t *testing.T) {
	p := PolicyProfile{
		Name: "fail-closed",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAllow, PathDeny: []string{".env"}},
			{Specifier: ToolSpecifier{Tool: ToolBash}, Decision: DecisionAllow, CommandDeny: []string{"curl"}},
		},
	}
	for _, call := range []ToolCall{
		{Tool: ToolEdit},
		{Tool: ToolBash},
		{Tool: ToolBash, Command: "   "},
		{Tool: ToolBash, Command: "FOO=1"},
	} {
		res := p.Evaluate(call)
		if res.Decision != DecisionDeny {
			t.Errorf("%+v -> %s (%s), want deny", call, res.Decision, res.Reason)
		}
	}
}

func TestEvaluate_EmptyValueNeverMatchesAllowList(t *testing.T) {
	p := PolicyProfile{
		Name: "allow-lists",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAllow, PathAllow: []string{"**"}},
			{Specifier: ToolSpecifier{Tool: ToolBash}, Decision: DecisionAllow, CommandAllow: []string{"ls"}},
		},
	}
	for _, call := range []ToolCall{{Tool: ToolEdit}, {Tool: ToolBash}} {
		res := p.Evaluate(call)
		if res.Decision != DecisionAsk || res.RuleIndex != -1 {
			t.Errorf("%+v -> %s rule %d, want mode default ask", call, res.Decision, res.RuleIndex)
		}
	}
}

func TestEvaluate_PathNormalization(t *testing.T) {
	p := PolicyProfile{
		Name: "paths",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAllow, PathDeny: []string{".env", "secrets/**"}},
			{Specifier: ToolSpecifier{Tool: ToolRead}, Decision: DecisionAllow, PathAllow: []string{"src/**"}},
		},
	}
	tests := []struct {
		name string
		call ToolCall
		want Decision
	}{
		{"dot slash", ToolCall{Tool: ToolEdit, Path: "./.env"}, DecisionDeny},
		{"inner dotdot", ToolCall{Tool: ToolEdit, Path: "a/../.env"}, DecisionDeny},
		{"absolute inside", ToolCall{Tool: ToolEdit, Path: testWorkspace + "/.env"}, DecisionDeny},
		{"absolute secrets", ToolCall{Tool: ToolEdit, Path: testWorkspace + "/secrets/k"}, DecisionDeny},
		{"upper case", ToolCall{Tool: ToolEdit, Path: "SECRETS/k"}, DecisionDeny},
		{"absolute outside", ToolCall{Tool: ToolEdit, Path: "/etc/passwd"}, DecisionDeny},
		{"relative escape", ToolCall{Tool: ToolEdit, Path: "../p2/x.go"}, DecisionDeny},
		{"read outside workspace", ToolCall{Tool: ToolRead, Path: "/etc/passwd"}, DecisionDeny},
		{"absolute allowed", ToolCall{Tool: ToolEdit, Path: testWorkspace + "/src/x.go"}, DecisionAllow},
		{"allow list absolute inside", ToolCall{Tool: ToolRead, Path: testWorkspace + "/src/x.go"}, DecisionAllow},
		{"allow list is case sensitive", ToolCall{Tool: ToolRead, Path: "SRC/x.go"}, DecisionAllow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := p.Evaluate(tt.call, WithWorkspace(testWorkspace))
			if res.Decision != tt.want {
				t.Errorf("%+v -> %s (%s), want %s", tt.call, res.Decision, res.Reason, tt.want)
			}
		})
	}
	// "SRC/x.go" is outside the Read allow list and falls to the mode default (allow).
	if res := p.Evaluate(ToolCall{Tool: ToolRead, Path: "SRC/x.go"}, WithWorkspace(testWorkspace)); res.RuleIndex != -1 {
		t.Errorf("SRC/x.go matched rule %d, want no rule (allow lists are case sensitive)", res.RuleIndex)
	}
}

func TestEvaluate_SubPattern(t *testing.T) {
	p := PolicyProfile{
		Name: "sub",
		Mode: ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolBash, SubPattern: "rm *"}, Decision: DecisionDeny},
			{Specifier: ToolSpecifier{Tool: ToolBash, SubPattern: "git *"}, Decision: DecisionAllow},
		},
	}
	tests := []struct {
		cmd  string
		want Decision
	}{
		{"git status", DecisionAllow},
		{"/usr/bin/git log -1", DecisionAllow},
		{"git status && git diff", DecisionAllow},
		{"git status; curl x", DecisionAsk},      // allow rule needs every segment
		{"git status; rm -rf /", DecisionDeny},   // deny rule matches any segment
		{"git status $(rm -rf /)", DecisionDeny}, // unanalysable: restrictive rules match
		{"RM -rf /", DecisionDeny},               // restrictive matching ignores case
		{"gitk", DecisionAsk},
		{"", DecisionAsk},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			res := p.Evaluate(ToolCall{Tool: "bash", Command: tt.cmd})
			if res.Decision != tt.want {
				t.Errorf("%q -> %s (%s), want %s", tt.cmd, res.Decision, res.Reason, tt.want)
			}
		})
	}
}

func TestEvaluate_CanonicalRuleAndCallNames(t *testing.T) {
	p := PolicyProfile{
		Name: "names",
		Mode: ModePlan,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "read_file"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "mcp__github__*"}, Decision: DecisionAllow},
		},
	}
	for _, tool := range []string{"Read", "read_file", "file:read", "READ"} {
		if res := p.Evaluate(ToolCall{Tool: tool}); res.Decision != DecisionAllow {
			t.Errorf("%s -> %s, want allow", tool, res.Decision)
		}
	}
	if res := p.Evaluate(ToolCall{Tool: "mcp__github__create_issue"}); res.Decision != DecisionAllow {
		t.Errorf("mcp glob -> %s, want allow", res.Decision)
	}
	if res := p.Evaluate(ToolCall{Tool: "mcp__slack__post"}); res.Decision != DecisionDeny {
		t.Errorf("other mcp -> %s, want deny (mode plan)", res.Decision)
	}
}

// Rule tool patterns written for worker names or legacy categories keep
// working for restrictive rules (fail closed): deny/ask rules and deny lists
// match the raw call name as well as the canonical one. Allow rules match
// only canonical names, so a legacy glob never grants more than intended.
func TestEvaluate_LegacyToolGlobs(t *testing.T) {
	p := PolicyProfile{
		Name: "legacy",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "file:*"}, Decision: DecisionDeny},
			{Specifier: ToolSpecifier{Tool: "edit_*"}, Decision: DecisionAsk},
			{Specifier: ToolSpecifier{Tool: "*_directory"}, Decision: DecisionAllow, PathDeny: []string{"secrets/**"}},
			{Specifier: ToolSpecifier{Tool: "read_*"}, Decision: DecisionAllow},
		},
	}
	pPlan := p
	pPlan.Mode = ModePlan
	tests := []struct {
		name    string
		profile *PolicyProfile
		call    ToolCall
		want    Decision
	}{
		{"deny glob on legacy category", &p, ToolCall{Tool: "file:write", Path: "a"}, DecisionDeny},
		{"ask glob on worker name", &p, ToolCall{Tool: "edit_file", Path: "a"}, DecisionAsk},
		{"deny list on worker-name glob", &p, ToolCall{Tool: "list_directory", Path: "secrets/x"}, DecisionDeny},
		{"allow glob does not match worker name", &pPlan, ToolCall{Tool: "read_file", Path: "a"}, DecisionDeny},
		{"canonical deny still matches", &PolicyProfile{Name: "c", Mode: ModeAcceptEdits, Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Write"}, Decision: DecisionDeny},
		}}, ToolCall{Tool: "write_file", Path: "a"}, DecisionDeny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := tt.profile.Evaluate(tt.call); res.Decision != tt.want {
				t.Errorf("%+v -> %s (%s), want %s", tt.call, res.Decision, res.Reason, tt.want)
			}
		})
	}
}

func TestEvaluate_ModeTools(t *testing.T) {
	p := PresetTrustedMountAutonomous()
	architect := WithModeTools("architect", []string{"Read", "Glob", "Grep", "ListDir"}, []string{"Write", "Edit", "Bash"})
	coder := WithModeTools("coder", []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep", "ListDir"}, nil)
	docs := WithModeTools("documenter", []string{"Read", "Write", "Edit", "Glob", "Grep"}, []string{"Bash"})
	tests := []struct {
		name string
		call ToolCall
		mode EvalOption
		want Decision
	}{
		{"architect read", ToolCall{Tool: "read_file", Path: "a.go"}, architect, DecisionAllow},
		{"architect write denied", ToolCall{Tool: "write_file", Path: "a.go"}, architect, DecisionDeny},
		{"architect edit denied", ToolCall{Tool: "edit_file", Path: "a.go"}, architect, DecisionDeny},
		{"architect bash denied", ToolCall{Tool: "bash", Command: "ls"}, architect, DecisionDeny},
		{"architect llm allowed", ToolCall{Tool: "LLM"}, architect, DecisionAllow},
		{"architect mcp governed by profile", ToolCall{Tool: "mcp__fs__read"}, architect, DecisionAllow},
		{"coder bash", ToolCall{Tool: "bash", Command: "ls"}, coder, DecisionAllow},
		{"documenter list dir not in tools", ToolCall{Tool: "list_directory", Path: "."}, docs, DecisionDeny},
		{"documenter bash denied", ToolCall{Tool: "bash", Command: "ls"}, docs, DecisionDeny},
		{"denied non-builtin tool", ToolCall{Tool: "propose_goal"}, WithModeTools("m", nil, []string{"propose_goal"}), DecisionDeny},
		{"denied tool by worker name", ToolCall{Tool: "Bash", Command: "ls"}, WithModeTools("m", nil, []string{"bash"}), DecisionDeny},
		{"empty tools list restricts nothing", ToolCall{Tool: "bash", Command: "ls"}, WithModeTools("m", nil, nil), DecisionAllow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := p.Evaluate(tt.call, tt.mode, WithWorkspace(testWorkspace))
			if res.Decision != tt.want {
				t.Errorf("%+v -> %s (%s), want %s", tt.call, res.Decision, res.Reason, tt.want)
			}
		})
	}
}

func TestEvaluate_TrustDoesNotBypassDenyLists(t *testing.T) {
	p := PolicyProfile{
		Name: "trust",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAllow, PathDeny: []string{".env"}, TrustMinimum: trust.LevelVerified},
		},
	}
	ann := &trust.Annotation{TrustLevel: trust.LevelUntrusted}
	if res := p.Evaluate(ToolCall{Tool: ToolEdit, Path: ".env"}, WithTrust(ann)); res.Decision != DecisionDeny {
		t.Errorf("untrusted .env edit -> %s, want deny", res.Decision)
	}
}

func TestEvaluate_MalformedDenyPatternFailsClosed(t *testing.T) {
	p := PolicyProfile{
		Name: "bad-pattern",
		Mode: ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: ToolEdit}, Decision: DecisionAllow, PathDeny: []string{"secrets/["}},
		},
	}
	if res := p.Evaluate(ToolCall{Tool: ToolEdit, Path: "src/x.go"}); res.Decision != DecisionDeny {
		t.Errorf("malformed deny pattern -> %s, want deny", res.Decision)
	}
}

func TestEvaluate_ReasonsExplainDenials(t *testing.T) {
	p := PresetHeadlessPermissiveSandbox()
	tests := []struct {
		call ToolCall
		want string
	}{
		{ToolCall{Tool: "bash", Command: "curl x"}, "command_deny"},
		{ToolCall{Tool: "bash", Command: "echo $(id)"}, "cannot be analysed"},
		{ToolCall{Tool: "write_file", Path: ".env"}, "path_deny"},
		{ToolCall{Tool: "write_file", Path: "/etc/x"}, "outside the workspace"},
		{ToolCall{Tool: "bash", Command: "ls"}, ""},
	}
	for _, tt := range tests {
		res := p.Evaluate(tt.call, WithWorkspace(testWorkspace), WithModeTools("architect", nil, []string{"Edit"}))
		if tt.want == "" {
			if res.Decision != DecisionAllow {
				t.Errorf("%+v -> %s, want allow", tt.call, res.Decision)
			}
			continue
		}
		if res.Decision != DecisionDeny || !strings.Contains(res.Reason, tt.want) {
			t.Errorf("%+v -> %s %q, want deny mentioning %q", tt.call, res.Decision, res.Reason, tt.want)
		}
	}
	res := p.Evaluate(ToolCall{Tool: "edit_file", Path: "a.go"}, WithModeTools("architect", nil, []string{"Edit"}))
	if res.Decision != DecisionDeny || !strings.Contains(res.Reason, `mode "architect"`) {
		t.Errorf("mode denial -> %s %q", res.Decision, res.Reason)
	}
}

func TestPermissionRuleValidatePatterns(t *testing.T) {
	tests := []struct {
		name    string
		rule    PermissionRule
		wantErr bool
	}{
		{"valid lists", PermissionRule{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow, PathDeny: []string{"**/.env", "secrets/*.key"}, CommandDeny: []string{"curl"}}, false},
		{"malformed path_deny", PermissionRule{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow, PathDeny: []string{"secrets/["}}, true},
		{"malformed path_allow", PermissionRule{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow, PathAllow: []string{"**/[a-"}}, true},
		{"empty path pattern", PermissionRule{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow, PathDeny: []string{" "}}, true},
		{"empty command pattern", PermissionRule{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow, CommandDeny: []string{""}}, true},
		{"empty command allow pattern", PermissionRule{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow, CommandAllow: []string{"\t"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPresetsReadOnlyToolsIncludeListDir(t *testing.T) {
	for _, name := range []string{"plan-readonly", "headless-safe-sandbox", "headless-permissive-sandbox", "trusted-mount-autonomous"} {
		p, _ := PresetByName(name)
		for _, tool := range []string{ToolLLM, ToolRead, ToolGlob, ToolGrep, ToolListDir} {
			if res := p.Evaluate(ToolCall{Tool: tool, Path: "."}); res.Decision != DecisionAllow {
				t.Errorf("%s: %s -> %s, want allow", name, tool, res.Decision)
			}
		}
	}
}
