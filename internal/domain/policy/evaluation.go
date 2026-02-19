package policy

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
