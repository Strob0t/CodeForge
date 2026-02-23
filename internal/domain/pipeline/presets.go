package pipeline

import "github.com/Strob0t/CodeForge/internal/domain/plan"

// BuiltinTemplates returns the set of built-in pipeline templates.
func BuiltinTemplates() []Template {
	return []Template{
		standardDev(),
		securityAudit(),
		reviewOnly(),
	}
}

// standardDev defines a sequential 4-step development pipeline:
// architect (PLAN.md) → coder (DIFF) → reviewer (REVIEW.md) → tester (TEST_REPORT)
func standardDev() Template {
	return Template{
		ID:          "standard-dev",
		Name:        "Standard Development",
		Description: "Sequential pipeline: architect plans, coder implements, reviewer checks, tester validates.",
		Builtin:     true,
		Protocol:    plan.ProtocolSequential,
		Steps: []Step{
			{Name: "Plan", ModeID: "architect", DeliverMode: "append"},
			{Name: "Implement", ModeID: "coder", DeliverMode: "diff", DependsOn: []int{0}},
			{Name: "Review", ModeID: "reviewer", DeliverMode: "append", DependsOn: []int{1}},
			{Name: "Test", ModeID: "tester", DeliverMode: "append", DependsOn: []int{2}},
		},
	}
}

// securityAudit defines a sequential 3-step security audit pipeline:
// architect (PLAN.md) → coder (DIFF) → security (AUDIT_REPORT)
func securityAudit() Template {
	return Template{
		ID:          "security-audit",
		Name:        "Security Audit",
		Description: "Sequential pipeline: architect plans, coder implements, security auditor reviews.",
		Builtin:     true,
		Protocol:    plan.ProtocolSequential,
		Steps: []Step{
			{Name: "Plan", ModeID: "architect", DeliverMode: "append"},
			{Name: "Implement", ModeID: "coder", DeliverMode: "diff", DependsOn: []int{0}},
			{Name: "Audit", ModeID: "security", DeliverMode: "append", DependsOn: []int{1}},
		},
	}
}

// reviewOnly defines a parallel 2-step review pipeline:
// reviewer (REVIEW.md) + security (AUDIT_REPORT) run simultaneously.
func reviewOnly() Template {
	return Template{
		ID:          "review-only",
		Name:        "Review Only",
		Description: "Parallel pipeline: code review and security audit run simultaneously.",
		Builtin:     true,
		Protocol:    plan.ProtocolParallel,
		MaxParallel: 2,
		Steps: []Step{
			{Name: "Review", ModeID: "reviewer", DeliverMode: "append"},
			{Name: "Audit", ModeID: "security", DeliverMode: "append"},
		},
	}
}
