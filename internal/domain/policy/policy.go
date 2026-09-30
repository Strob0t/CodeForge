// Package policy defines the domain model for CodeForge's policy layer.
// Policies govern what tools agents may use, under what conditions,
// and with what limits (steps, cost, time).
package policy

import (
	"slices"

	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

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
// Tool is a canonical tool name (see CanonicalTool) or a filepath.Match glob
// such as "mcp__*". SubPattern is a glob ("*" matches any characters) over
// each simple command of a shell command, e.g. Tool="Bash" SubPattern="git *".
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
	TrustMinimum trust.Level   `json:"trust_minimum,omitempty" yaml:"trust_minimum,omitempty"`
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
	Name           string               `json:"name" yaml:"name"`
	Description    string               `json:"description,omitempty" yaml:"description,omitempty"`
	Scope          Scope                `json:"scope,omitempty" yaml:"scope,omitempty"`
	Mode           PermissionMode       `json:"mode" yaml:"mode"`
	Rules          []PermissionRule     `json:"rules" yaml:"rules"`
	QualityGate    QualityGate          `json:"quality_gate" yaml:"quality_gate"`
	Termination    TerminationCondition `json:"termination" yaml:"termination"`
	ResourceLimits *resource.Limits     `json:"resource_limits,omitempty" yaml:"resource_limits,omitempty"`
}

// ToolCall represents a request to use a tool, submitted to the policy evaluator.
type ToolCall struct {
	Tool    string `json:"tool"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
}

// HasRule returns true if the profile already contains a rule equal to rule
// (same specifier, decision, path and command lists and trust minimum).
func (p *PolicyProfile) HasRule(rule *PermissionRule) bool {
	for i := range p.Rules {
		if p.Rules[i].Equal(rule) {
			return true
		}
	}
	return false
}

// Equal reports whether two rules are identical. Nil and empty lists are equal.
func (r *PermissionRule) Equal(o *PermissionRule) bool {
	return r.Specifier == o.Specifier &&
		r.Decision == o.Decision &&
		r.TrustMinimum == o.TrustMinimum &&
		slices.Equal(r.PathAllow, o.PathAllow) &&
		slices.Equal(r.PathDeny, o.PathDeny) &&
		slices.Equal(r.CommandAllow, o.CommandAllow) &&
		slices.Equal(r.CommandDeny, o.CommandDeny)
}
