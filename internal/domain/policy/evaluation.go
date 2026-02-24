package policy

import (
	"fmt"
	"path/filepath"
)

// Scope defines at which level a policy was resolved.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
	ScopeRun     Scope = "run"
)

// EvaluationResult captures the full context of a policy evaluation,
// including which rule matched and why.
type EvaluationResult struct {
	Decision    Decision `json:"decision"`
	Profile     string   `json:"profile"`
	Scope       Scope    `json:"scope"`
	RuleIndex   int      `json:"rule_index"`   // -1 if no rule matched (mode default)
	MatchedRule string   `json:"matched_rule"` // human-readable rule description
	Reason      string   `json:"reason"`       // explanation of why this decision was made
}

// Evaluate checks a ToolCall against the profile's rules using first-match-wins.
// If no rule matches, the default decision is "deny" (deny-by-default).
func (p *PolicyProfile) Evaluate(call ToolCall) EvaluationResult {
	for i := range p.Rules {
		rule := &p.Rules[i]
		if !matchTool(rule.Specifier.Tool, call.Tool) {
			continue
		}
		if rule.Specifier.SubPattern != "" && call.Command != "" {
			if !matchTool(rule.Specifier.SubPattern, call.Command) {
				continue
			}
		}
		return EvaluationResult{
			Decision:    rule.Decision,
			Profile:     p.Name,
			Scope:       p.Scope,
			RuleIndex:   i,
			MatchedRule: fmt.Sprintf("%s → %s", rule.Specifier.Tool, rule.Decision),
			Reason:      fmt.Sprintf("matched rule[%d]: tool=%q", i, rule.Specifier.Tool),
		}
	}
	return EvaluationResult{
		Decision:    DecisionDeny,
		Profile:     p.Name,
		Scope:       p.Scope,
		RuleIndex:   -1,
		MatchedRule: "",
		Reason:      "no matching rule; deny by default",
	}
}

// matchTool checks whether a tool specifier pattern matches a tool name.
// Supports glob-style wildcards via filepath.Match:
//   - "*" matches everything
//   - "mcp:*" matches "mcp:filesystem:read_file"
//   - "mcp:filesystem:*" matches "mcp:filesystem:read_file"
//   - "mcp:filesystem:read_file" matches exactly
func matchTool(pattern, name string) bool {
	if pattern == name {
		return true
	}
	matched, err := filepath.Match(pattern, name)
	if err == nil && matched {
		return true
	}
	return false
}
