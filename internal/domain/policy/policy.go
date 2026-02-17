// Package policy defines the domain model for CodeForge's policy layer.
// Policies govern what tools agents may use, under what conditions,
// and with what limits (steps, cost, time).
package policy

// Decision is the result of evaluating a ToolCall against a PolicyProfile.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

// PermissionMode controls the baseline behavior of a policy profile.
type PermissionMode string

const (
	ModeDefault     PermissionMode = "default"
	ModeAcceptEdits PermissionMode = "acceptEdits"
	ModePlan        PermissionMode = "plan"
	ModeDelegate    PermissionMode = "delegate"
)

// ToolSpecifier identifies a tool and optionally a sub-command pattern.
// Examples: Tool="Read", Tool="Bash" SubPattern="git status:*"
type ToolSpecifier struct {
	Tool       string `json:"tool" yaml:"tool"`
	SubPattern string `json:"sub_pattern,omitempty" yaml:"sub_pattern,omitempty"`
}

// PermissionRule maps a ToolSpecifier to a Decision with optional constraints.
type PermissionRule struct {
	Specifier    ToolSpecifier `json:"specifier" yaml:"specifier"`
	Decision     Decision      `json:"decision" yaml:"decision"`
	PathAllow    []string      `json:"path_allow,omitempty" yaml:"path_allow,omitempty"`
	PathDeny     []string      `json:"path_deny,omitempty" yaml:"path_deny,omitempty"`
	CommandAllow []string      `json:"command_allow,omitempty" yaml:"command_allow,omitempty"`
	CommandDeny  []string      `json:"command_deny,omitempty" yaml:"command_deny,omitempty"`
}

// QualityGate defines the "Definition of Done" for a task.
type QualityGate struct {
	RequireTestsPass   bool `json:"require_tests_pass" yaml:"require_tests_pass"`
	RequireLintPass    bool `json:"require_lint_pass" yaml:"require_lint_pass"`
	RollbackOnGateFail bool `json:"rollback_on_gate_fail" yaml:"rollback_on_gate_fail"`
}

// TerminationCondition defines when an agent run should stop.
type TerminationCondition struct {
	MaxSteps       int     `json:"max_steps,omitempty" yaml:"max_steps,omitempty"`
	TimeoutSeconds int     `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	MaxCost        float64 `json:"max_cost,omitempty" yaml:"max_cost,omitempty"`
	StallDetection bool    `json:"stall_detection,omitempty" yaml:"stall_detection,omitempty"`
	StallThreshold int     `json:"stall_threshold,omitempty" yaml:"stall_threshold,omitempty"`
}

// PolicyProfile is the top-level policy configuration for an agent run.
type PolicyProfile struct {
	Name        string               `json:"name" yaml:"name"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Mode        PermissionMode       `json:"mode" yaml:"mode"`
	Rules       []PermissionRule     `json:"rules" yaml:"rules"`
	QualityGate QualityGate          `json:"quality_gate" yaml:"quality_gate"`
	Termination TerminationCondition `json:"termination" yaml:"termination"`
}

// ToolCall represents a request to use a tool, submitted to the policy evaluator.
type ToolCall struct {
	Tool    string `json:"tool"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
}
