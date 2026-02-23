package policy

// PresetPlanReadonly returns the "plan-readonly" preset.
// Debug/Preview mode: no side-effects, read-only tools only.
func PresetPlanReadonly() PolicyProfile {
	return PolicyProfile{
		Name:        "plan-readonly",
		Description: "Read-only mode for debugging and previewing. No side-effects allowed.",
		Mode:        ModePlan,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Glob"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Grep"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionDeny},
			{Specifier: ToolSpecifier{Tool: "Write"}, Decision: DecisionDeny},
			{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionDeny},
		},
		Termination: TerminationCondition{
			MaxSteps:       30,
			TimeoutSeconds: 300,
			MaxCost:        1.0,
		},
	}
}

// PresetHeadlessSafeSandbox returns the "headless-safe-sandbox" preset.
// Default for autonomous server jobs: safety first, strict limits.
func PresetHeadlessSafeSandbox() PolicyProfile {
	return PolicyProfile{
		Name:        "headless-safe-sandbox",
		Description: "Safe sandbox for headless/autonomous execution. Strict safety limits.",
		Mode:        ModeDefault,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "LLM"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Glob"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Grep"}, Decision: DecisionAllow},
			{
				Specifier: ToolSpecifier{Tool: "Edit"},
				Decision:  DecisionAllow,
				PathDeny:  []string{".env", "**/.env", "secrets/**", "**/credentials.*"},
			},
			{
				Specifier:    ToolSpecifier{Tool: "Bash"},
				Decision:     DecisionAllow,
				CommandAllow: []string{"git status", "git diff", "git log", "go test", "python -m pytest", "npm test", "make test", "make lint"},
			},
			{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionDeny},
		},
		QualityGate: QualityGate{
			RequireTestsPass:   true,
			RequireLintPass:    true,
			RollbackOnGateFail: true,
		},
		Termination: TerminationCondition{
			MaxSteps:       50,
			TimeoutSeconds: 600,
			MaxCost:        5.0,
			StallDetection: true,
			StallThreshold: 5,
		},
	}
}

// PresetHeadlessPermissiveSandbox returns the "headless-permissive-sandbox" preset.
// Batch/refactor: more freedom, still sandboxed, relaxed limits.
func PresetHeadlessPermissiveSandbox() PolicyProfile {
	return PolicyProfile{
		Name:        "headless-permissive-sandbox",
		Description: "Permissive sandbox for batch operations and refactoring.",
		Mode:        ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "LLM"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Glob"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Grep"}, Decision: DecisionAllow},
			{
				Specifier: ToolSpecifier{Tool: "Edit"},
				Decision:  DecisionAllow,
				PathDeny:  []string{".env", "**/.env", "secrets/**"},
			},
			{Specifier: ToolSpecifier{Tool: "Write"}, Decision: DecisionAllow},
			{
				Specifier:    ToolSpecifier{Tool: "Bash"},
				Decision:     DecisionDeny,
				CommandAllow: []string{"curl", "wget", "ssh", "scp", "nc", "ncat"},
			},
			{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow},
		},
		QualityGate: QualityGate{
			RequireTestsPass: true,
		},
		Termination: TerminationCondition{
			MaxSteps:       100,
			TimeoutSeconds: 1800,
			MaxCost:        20.0,
			StallDetection: true,
			StallThreshold: 5,
		},
	}
}

// PresetTrustedMountAutonomous returns the "trusted-mount-autonomous" preset.
// Power-user: direct mount, all local tools allowed, minimal restrictions.
func PresetTrustedMountAutonomous() PolicyProfile {
	return PolicyProfile{
		Name:        "trusted-mount-autonomous",
		Description: "Fully autonomous with direct file access. Minimal restrictions.",
		Mode:        ModeAcceptEdits,
		Rules: []PermissionRule{
			{Specifier: ToolSpecifier{Tool: "LLM"}, Decision: DecisionAllow},
			{
				Specifier: ToolSpecifier{Tool: "Edit"},
				Decision:  DecisionDeny,
				PathAllow: []string{".env", "**/.env", "secrets/**"},
			},
			{
				Specifier: ToolSpecifier{Tool: "Write"},
				Decision:  DecisionDeny,
				PathAllow: []string{".env", "**/.env", "secrets/**"},
			},
			{Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Glob"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Grep"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Edit"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Write"}, Decision: DecisionAllow},
			{Specifier: ToolSpecifier{Tool: "Bash"}, Decision: DecisionAllow},
		},
		Termination: TerminationCondition{
			MaxSteps:       200,
			TimeoutSeconds: 3600,
			MaxCost:        50.0,
		},
	}
}

// PresetNames returns the names of all built-in presets.
func PresetNames() []string {
	return []string{
		"plan-readonly",
		"headless-safe-sandbox",
		"headless-permissive-sandbox",
		"trusted-mount-autonomous",
	}
}

// IsPreset returns true if the given name is a built-in preset.
func IsPreset(name string) bool {
	_, ok := PresetByName(name)
	return ok
}

// PresetByName returns a preset by name, or false if not found.
func PresetByName(name string) (PolicyProfile, bool) {
	switch name {
	case "plan-readonly":
		return PresetPlanReadonly(), true
	case "headless-safe-sandbox":
		return PresetHeadlessSafeSandbox(), true
	case "headless-permissive-sandbox":
		return PresetHeadlessPermissiveSandbox(), true
	case "trusted-mount-autonomous":
		return PresetTrustedMountAutonomous(), true
	default:
		return PolicyProfile{}, false
	}
}
