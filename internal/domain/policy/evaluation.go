package policy

import (
	"fmt"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
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
	RuleIndex   int      `json:"rule_index"`   // -1 if no rule decided (mode default, mode restriction, path outside workspace)
	MatchedRule string   `json:"matched_rule"` // human-readable rule description
	Reason      string   `json:"reason"`       // explanation of why this decision was made
}

// evalContext holds optional evaluation parameters.
type evalContext struct {
	trust     *trust.Annotation
	workspace string
	mode      *modeRestriction
}

// modeRestriction carries the tool lists of the agent mode a call runs in.
type modeRestriction struct {
	id     string
	tools  []string
	denied []string
}

// EvalOption configures optional parameters for Evaluate.
type EvalOption func(*evalContext)

// WithTrust attaches a trust annotation to the evaluation context. An allow
// rule with a TrustMinimum only allows calls whose annotation meets it; a
// call without an annotation never meets it. Deny and ask rules apply to
// every caller.
func WithTrust(t *trust.Annotation) EvalOption {
	return func(c *evalContext) { c.trust = t }
}

// WithWorkspace sets the workspace root that call paths are resolved against.
func WithWorkspace(dir string) EvalOption {
	return func(c *evalContext) { c.workspace = dir }
}

// WithModeTools restricts the call to the tool lists of an agent mode: a
// tool in denied is denied, and a built-in tool (see BuiltinTools) that is
// missing from a non-empty tools list is denied. Tools that are not built
// in (LLM, MCP tools, propose_goal, ...) are only restricted by denied.
func WithModeTools(modeID string, tools, denied []string) EvalOption {
	return func(c *evalContext) {
		c.mode = &modeRestriction{id: modeID, tools: tools, denied: denied}
	}
}

// Evaluate decides a ToolCall against the profile (ADR-007, deny-list
// semantics amended by ADR-015):
//
//  1. The tool name is mapped to its canonical name (CanonicalTool); rule
//     and mode tool names are canonicalized the same way.
//  2. Mode restrictions (WithModeTools) deny tools the mode may not use.
//  3. The path is normalized relative to the workspace (WithWorkspace); a
//     path that leaves the workspace is denied.
//  4. Deny lists are blocklists: if any rule for the tool has a path_deny or
//     command_deny list that matches the call, or the call carries no value
//     for that list, the call is denied regardless of rule order.
//  5. Otherwise the first rule whose specifier and allow lists match decides.
//  6. If no rule matches, the profile's permission mode decides.
func (p *PolicyProfile) Evaluate(call ToolCall, opts ...EvalOption) EvaluationResult {
	var ctx evalContext
	for _, o := range opts {
		o(&ctx)
	}
	scope := p.Scope
	if scope == "" {
		scope = ScopeGlobal
	}
	decide := func(d Decision, ruleIndex int, reason string) EvaluationResult {
		res := EvaluationResult{Decision: d, Profile: p.Name, Scope: scope, RuleIndex: ruleIndex, Reason: reason}
		if ruleIndex >= 0 {
			r := &p.Rules[ruleIndex]
			res.MatchedRule = fmt.Sprintf("rule[%d]: %s %s -> %s", ruleIndex, r.Specifier.Tool, r.Specifier.SubPattern, r.Decision)
		}
		return res
	}

	tool := CanonicalTool(call.Tool)

	if ctx.mode != nil {
		if reason := ctx.mode.deniedReason(tool); reason != "" {
			return decide(DecisionDeny, -1, reason)
		}
	}

	path, ok := NormalizePath(ctx.workspace, call.Path)
	if !ok {
		return decide(DecisionDeny, -1, fmt.Sprintf("path %q is outside the workspace", call.Path))
	}
	cmd := parseShellCommand(call.Command)

	for i := range p.Rules {
		rule := &p.Rules[i]
		if !rule.matchesTool(call.Tool, tool, true) || !rule.subPatternMatches(cmd, false) {
			continue
		}
		if reason := rule.denyListReason(path, cmd); reason != "" {
			return decide(DecisionDeny, i, fmt.Sprintf("denied by rule %d in profile %q: %s", i, p.Name, reason))
		}
	}

	for i := range p.Rules {
		rule := &p.Rules[i]
		allow := rule.Decision == DecisionAllow
		if !rule.matchesTool(call.Tool, tool, !allow) || !rule.subPatternMatches(cmd, allow) {
			continue
		}
		if len(rule.PathAllow) > 0 && (path == "" || !matchesAnyGlob(rule.PathAllow, path, false)) {
			continue
		}
		if len(rule.CommandAllow) > 0 && !cmd.allowedBy(rule.CommandAllow) {
			continue
		}
		if allow && rule.TrustMinimum != "" && (ctx.trust == nil || !ctx.trust.MeetsMinimum(rule.TrustMinimum)) {
			continue
		}
		reason := fmt.Sprintf("matched rule %d in profile %q: tool=%s", i, p.Name, rule.Specifier.Tool)
		if rule.Specifier.SubPattern != "" {
			reason += " sub_pattern=" + rule.Specifier.SubPattern
		}
		return decide(rule.Decision, i, reason)
	}

	d := DefaultDecision(p.Mode)
	return decide(d, -1, fmt.Sprintf("no rule matched, using mode default %q -> %s", p.Mode, d))
}

// DefaultDecision returns the decision for calls that no rule matches.
func DefaultDecision(mode PermissionMode) Decision {
	switch mode {
	case ModePlan:
		return DecisionDeny
	case ModeAcceptEdits, ModeDelegate:
		return DecisionAllow
	default:
		return DecisionAsk
	}
}

// deniedReason explains why the mode forbids tool, or returns "".
func (m *modeRestriction) deniedReason(tool string) string {
	for _, d := range m.denied {
		if CanonicalTool(d) == tool {
			return fmt.Sprintf("tool %s is denied by mode %q", tool, m.id)
		}
	}
	if len(m.tools) == 0 || !IsBuiltinTool(tool) {
		return ""
	}
	for _, t := range m.tools {
		if CanonicalTool(t) == tool {
			return ""
		}
	}
	return fmt.Sprintf("tool %s is not in the tools of mode %q", tool, m.id)
}

// denyListReason explains why the rule's deny lists deny the call, or
// returns "". A deny list fails closed when the call has no value for it.
func (r *PermissionRule) denyListReason(path string, cmd shellCommand) string {
	if len(r.PathDeny) > 0 {
		if path == "" {
			return "path_deny is set and the call has no path"
		}
		if matchesAnyGlob(r.PathDeny, path, true) {
			return fmt.Sprintf("path %q matches path_deny", path)
		}
	}
	if len(r.CommandDeny) > 0 && cmd.deniedBy(r.CommandDeny) {
		switch {
		case cmd.opaque:
			return "command_deny is set and the command cannot be analysed statically"
		case len(cmd.segments) == 0:
			return "command_deny is set and the call has no command"
		default:
			return "command matches command_deny"
		}
	}
	return ""
}

// subPatternMatches matches the specifier's sub-pattern (a glob over each
// simple command, e.g. "git *") against the command. A permissive rule
// (allOf) needs every simple command to match and never matches a command
// that cannot be analysed; a restrictive rule matches if any simple command
// matches, and always matches an unanalysable command.
func (r *PermissionRule) subPatternMatches(cmd shellCommand, allOf bool) bool {
	pattern := r.Specifier.SubPattern
	if pattern == "" {
		return true
	}
	if cmd.opaque {
		return !allOf
	}
	if len(cmd.segments) == 0 {
		return false
	}
	for _, seg := range cmd.segments {
		matched := matchWildcard(pattern, joinWords(seg), !allOf)
		if allOf && !matched {
			return false
		}
		if !allOf && matched {
			return true
		}
	}
	return allOf
}

// matchesTool matches the rule's tool pattern against a call, given the raw
// tool name the worker sent and its canonical name. The canonicalized
// pattern is matched against the canonical name. A restrictive match (deny
// lists, deny and ask rules) also matches the pattern against the raw name,
// so rules written for worker names or legacy categories ("file:*",
// "*_file") keep denying (fail closed); allow rules only match canonical
// names, so a legacy glob never grants more than a canonical rule would.
func (r *PermissionRule) matchesTool(raw, canonical string, restrictive bool) bool {
	pattern := r.Specifier.Tool
	if matchTool(CanonicalTool(pattern), canonical) {
		return true
	}
	return restrictive && (matchTool(pattern, raw) || matchTool(pattern, canonical))
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
	return err == nil && matched
}
