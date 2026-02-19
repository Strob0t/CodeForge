package branchprotection

import "path/filepath"

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

// EvaluatePush checks whether a push is allowed under the given rules.
// Default-DENY (P1-4): if enabled rules exist but none match, push is denied.
func EvaluatePush(rules []ProtectionRule, action PushAction) EvalResult {
	hasEnabledRules := false
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled {
			continue
		}
		hasEnabledRules = true
		if !matchBranch(rule.BranchPattern, action.Branch) {
			continue
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
	if hasEnabledRules {
		return EvalResult{Allowed: false, Reason: "no matching protection rule (default deny)"}
	}
	return EvalResult{Allowed: true, Reason: "no protection rules configured"}
}

// EvaluateMerge checks whether a merge into the target branch is allowed.
// Default-DENY (P1-4): if enabled rules exist but none match, merge is denied.
func EvaluateMerge(rules []ProtectionRule, action MergeAction) EvalResult {
	hasEnabledRules := false
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled {
			continue
		}
		hasEnabledRules = true
		if !matchBranch(rule.BranchPattern, action.TargetBranch) {
			continue
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
	if hasEnabledRules {
		return EvalResult{Allowed: false, Reason: "no matching protection rule (default deny)"}
	}
	return EvalResult{Allowed: true, Reason: "no protection rules configured"}
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

// matchBranch checks if a branch name matches a glob pattern.
func matchBranch(pattern, branch string) bool {
	matched, _ := filepath.Match(pattern, branch)
	return matched
}
