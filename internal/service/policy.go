package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/policy"
)

// PolicyService evaluates tool calls against policy profiles and
// provides access to built-in presets and loaded custom policies.
type PolicyService struct {
	defaultProfile string
	profiles       map[string]policy.PolicyProfile
}

// NewPolicyService creates a PolicyService with built-in presets
// and optional custom profiles. Custom profiles override presets
// with the same name.
func NewPolicyService(defaultProfile string, custom []policy.PolicyProfile) *PolicyService {
	profiles := make(map[string]policy.PolicyProfile)

	// Register built-in presets.
	for _, name := range policy.PresetNames() {
		p, _ := policy.PresetByName(name)
		profiles[name] = p
	}

	// Register custom profiles (override presets if same name).
	for i := range custom {
		profiles[custom[i].Name] = custom[i]
	}

	return &PolicyService{
		defaultProfile: defaultProfile,
		profiles:       profiles,
	}
}

// Evaluate checks a ToolCall against a named PolicyProfile and returns a Decision.
func (s *PolicyService) Evaluate(_ context.Context, profileName string, call policy.ToolCall) (policy.Decision, error) {
	p, ok := s.profiles[profileName]
	if !ok {
		return policy.DecisionDeny, fmt.Errorf("unknown policy profile %q", profileName)
	}
	return evaluate(&p, call), nil
}

// GetProfile returns a policy profile by name.
func (s *PolicyService) GetProfile(name string) (policy.PolicyProfile, bool) {
	p, ok := s.profiles[name]
	return p, ok
}

// ListProfiles returns all available profile names, sorted alphabetically.
func (s *PolicyService) ListProfiles() []string {
	names := make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultProfile returns the name of the default policy profile.
func (s *PolicyService) DefaultProfile() string {
	return s.defaultProfile
}

// evaluate performs first-match rule evaluation against a profile.
func evaluate(profile *policy.PolicyProfile, call policy.ToolCall) policy.Decision {
	for i := range profile.Rules {
		rule := &profile.Rules[i]
		if !matchesSpecifier(rule.Specifier, call) {
			continue
		}
		if !matchesPathConstraints(rule, call.Path) {
			continue
		}
		if !matchesCommandConstraints(rule, call.Command) {
			continue
		}
		return rule.Decision
	}
	return defaultDecisionForMode(profile.Mode)
}

// matchesSpecifier checks if a ToolCall matches a ToolSpecifier.
func matchesSpecifier(spec policy.ToolSpecifier, call policy.ToolCall) bool {
	if spec.Tool != call.Tool {
		return false
	}
	if spec.SubPattern == "" {
		return true
	}
	return matchGlob(spec.SubPattern, call.Command)
}

// matchesPathConstraints checks path_allow/path_deny glob patterns.
// Rules: if path_deny matches, the rule does NOT match (skip to next rule).
// If path_allow is set and path does NOT match, the rule does NOT match.
// This means deny lists take precedence, and empty lists match everything.
func matchesPathConstraints(rule *policy.PermissionRule, path string) bool {
	if path == "" {
		return true
	}

	// Path deny: if any pattern matches, skip this rule.
	for _, pattern := range rule.PathDeny {
		if matchGlob(pattern, path) {
			return false
		}
	}

	// Path allow: if set, at least one must match.
	if len(rule.PathAllow) > 0 {
		for _, pattern := range rule.PathAllow {
			if matchGlob(pattern, path) {
				return true
			}
		}
		return false
	}

	return true
}

// matchesCommandConstraints checks command_allow/command_deny patterns.
// If command_deny matches, the rule does NOT match (skip).
// If command_allow is set, the command must match at least one pattern.
func matchesCommandConstraints(rule *policy.PermissionRule, command string) bool {
	if command == "" {
		return true
	}

	// Command deny: if any matches, skip this rule.
	for _, pattern := range rule.CommandDeny {
		if matchCommandPattern(pattern, command) {
			return false
		}
	}

	// Command allow: if set, at least one must match.
	if len(rule.CommandAllow) > 0 {
		for _, pattern := range rule.CommandAllow {
			if matchCommandPattern(pattern, command) {
				return true
			}
		}
		return false
	}

	return true
}

// matchCommandPattern matches a command against a pattern.
// The pattern matches if the command starts with it (prefix match).
func matchCommandPattern(pattern, command string) bool {
	return command == pattern || strings.HasPrefix(command, pattern+" ")
}

// matchGlob matches a string against a glob pattern. Supports:
// - Standard filepath.Match patterns (*, ?)
// - ** for recursive directory matching
func matchGlob(pattern, value string) bool {
	// Handle ** patterns by splitting on path separator.
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, value)
	}
	matched, _ := filepath.Match(pattern, value)
	return matched
}

// matchDoubleStar handles ** glob patterns for recursive path matching.
func matchDoubleStar(pattern, value string) bool {
	// Split pattern and value into segments.
	patParts := strings.Split(pattern, "/")
	valParts := strings.Split(value, "/")
	return matchSegments(patParts, valParts)
}

// matchSegments recursively matches pattern segments against value segments.
func matchSegments(pat, val []string) bool {
	for len(pat) > 0 && len(val) > 0 {
		if pat[0] == "**" {
			// ** matches zero or more path segments.
			pat = pat[1:]
			if len(pat) == 0 {
				return true // trailing ** matches everything
			}
			// Try matching remaining pattern at each position.
			for i := 0; i <= len(val); i++ {
				if matchSegments(pat, val[i:]) {
					return true
				}
			}
			return false
		}
		matched, _ := filepath.Match(pat[0], val[0])
		if !matched {
			return false
		}
		pat = pat[1:]
		val = val[1:]
	}

	// Remaining pattern segments must all be ** (or pattern is empty).
	for _, p := range pat {
		if p != "**" {
			return false
		}
	}
	return len(val) == 0
}

// defaultDecisionForMode returns the fallback decision when no rule matches.
func defaultDecisionForMode(mode policy.PermissionMode) policy.Decision {
	switch mode {
	case policy.ModePlan:
		return policy.DecisionDeny
	case policy.ModeDefault:
		return policy.DecisionAsk
	case policy.ModeAcceptEdits:
		return policy.DecisionAllow
	case policy.ModeDelegate:
		return policy.DecisionAllow
	default:
		return policy.DecisionAsk
	}
}
