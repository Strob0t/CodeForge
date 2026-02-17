package policy

import "fmt"

// Validate checks that a PolicyProfile is well-formed.
func (p *PolicyProfile) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("policy: name is required")
	}
	if !isValidMode(p.Mode) {
		return fmt.Errorf("policy: invalid mode %q", p.Mode)
	}
	for i := range p.Rules {
		if err := p.Rules[i].Validate(); err != nil {
			return fmt.Errorf("policy: rule[%d]: %w", i, err)
		}
	}
	if p.Termination.MaxSteps < 0 {
		return fmt.Errorf("policy: max_steps must be >= 0")
	}
	if p.Termination.TimeoutSeconds < 0 {
		return fmt.Errorf("policy: timeout_seconds must be >= 0")
	}
	if p.Termination.MaxCost < 0 {
		return fmt.Errorf("policy: max_cost must be >= 0")
	}
	return nil
}

// Validate checks that a PermissionRule is well-formed.
func (r *PermissionRule) Validate() error {
	if r.Specifier.Tool == "" {
		return fmt.Errorf("tool is required")
	}
	if !isValidDecision(r.Decision) {
		return fmt.Errorf("invalid decision %q", r.Decision)
	}
	return nil
}

func isValidMode(m PermissionMode) bool {
	switch m {
	case ModeDefault, ModeAcceptEdits, ModePlan, ModeDelegate:
		return true
	}
	return false
}

func isValidDecision(d Decision) bool {
	switch d {
	case DecisionAllow, DecisionDeny, DecisionAsk:
		return true
	}
	return false
}
