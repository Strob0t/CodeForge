package branchprotection

import (
	"fmt"
	"path"
	"strings"
)

// PushAction describes a push operation to evaluate.
type PushAction struct {
	Branch     string
	ForcePush  bool
	HasChanges bool
}

// MergeAction describes a merge operation to evaluate.
type MergeAction struct {
	TargetBranch string
	TestsPassed  bool
	LintPassed   bool
	HasReviews   bool
}

// EvalResult captures the outcome of evaluating a branch operation.
type EvalResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Rule    string `json:"rule,omitempty"` // matched pattern, empty if no rule applies
}

// A rule protects the branches it matches (KI-205): a push or merge is
// evaluated against the first enabled rule that matches its branch, and a
// branch no enabled rule matches is not protected (the delivery branch
// codeforge/<id> is pushed unless a rule matches it).

// EvaluatePush checks whether a push is allowed under the given rules.
func EvaluatePush(rules []ProtectionRule, action PushAction) EvalResult {
	rule := firstMatch(rules, action.Branch)
	if rule == nil {
		return EvalResult{Allowed: true, Reason: "no protection rule matches the branch"}
	}
	if action.ForcePush && !rule.AllowForcePush {
		return EvalResult{
			Allowed: false,
			Reason:  "force push is not allowed on this branch",
			Rule:    rule.BranchPattern,
		}
	}
	return EvalResult{
		Allowed: true,
		Reason:  "push allowed",
		Rule:    rule.BranchPattern,
	}
}

// EvaluateMerge checks whether a merge into the target branch is allowed.
func EvaluateMerge(rules []ProtectionRule, action MergeAction) EvalResult {
	rule := firstMatch(rules, action.TargetBranch)
	if rule == nil {
		return EvalResult{Allowed: true, Reason: "no protection rule matches the branch"}
	}
	if rule.RequireTests && !action.TestsPassed {
		return EvalResult{
			Allowed: false,
			Reason:  "tests must pass before merging",
			Rule:    rule.BranchPattern,
		}
	}
	if rule.RequireLint && !action.LintPassed {
		return EvalResult{
			Allowed: false,
			Reason:  "lint must pass before merging",
			Rule:    rule.BranchPattern,
		}
	}
	if rule.RequireReviews && !action.HasReviews {
		return EvalResult{
			Allowed: false,
			Reason:  "at least one review is required before merging",
			Rule:    rule.BranchPattern,
		}
	}
	return EvalResult{
		Allowed: true,
		Reason:  "merge allowed",
		Rule:    rule.BranchPattern,
	}
}

// firstMatch is the first enabled rule whose pattern matches branch, nil
// when there is none.
func firstMatch(rules []ProtectionRule, branch string) *ProtectionRule {
	for i := range rules {
		if rules[i].Enabled && matchBranch(rules[i].BranchPattern, branch) {
			return &rules[i]
		}
	}
	return nil
}

// EvaluateDelete checks whether deleting a branch is allowed.
// Default-DENY (P1-4): if enabled rules exist but none match, delete is denied.
func EvaluateDelete(rules []ProtectionRule, branch string) EvalResult {
	hasEnabledRules := false
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled {
			continue
		}
		hasEnabledRules = true
		if !matchBranch(rule.BranchPattern, branch) {
			continue
		}
		if !rule.AllowDelete {
			return EvalResult{
				Allowed: false,
				Reason:  "branch deletion is not allowed",
				Rule:    rule.BranchPattern,
			}
		}
		return EvalResult{
			Allowed: true,
			Reason:  "delete allowed",
			Rule:    rule.BranchPattern,
		}
	}
	if hasEnabledRules {
		return EvalResult{Allowed: false, Reason: "no matching protection rule (default deny)"}
	}
	return EvalResult{Allowed: true, Reason: "no protection rules configured"}
}

// matchBranch reports whether branch matches the glob pattern. Patterns
// are matched per "/"-separated segment with path.Match, so "*" stays
// within one segment ("release/*" matches release/1.0, not
// release/1.0/hotfix), and a "**" segment matches any number of segments,
// none included ("release/**", "**/hotfix", "team/**/wip-*"). A malformed
// pattern matches every branch: a broken rule protects too much rather
// than nothing.
func matchBranch(pattern, branch string) bool {
	if validatePattern(pattern) != nil {
		return true
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(branch, "/"))
}

// matchSegments matches the segments of a branch name against those of a
// pattern.
func matchSegments(pattern, branch []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for skip := 0; skip <= len(branch); skip++ {
				if matchSegments(pattern[1:], branch[skip:]) {
					return true
				}
			}
			return false
		}
		if len(branch) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], branch[0]); !ok {
			return false
		}
		pattern, branch = pattern[1:], branch[1:]
	}
	return len(branch) == 0
}

// validatePattern reports a malformed branch pattern.
func validatePattern(pattern string) error {
	for _, segment := range strings.Split(pattern, "/") {
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("branch_pattern %q: %w", pattern, err)
		}
	}
	return nil
}
