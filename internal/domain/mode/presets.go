package mode

// BuiltinModes returns all built-in agent mode presets.
func BuiltinModes() []Mode {
	return []Mode{
		{
			ID:               "architect",
			Name:             "Architect",
			Description:      "Designs system architecture, analyzes structure, and produces technical plans.",
			Builtin:          true,
			Tools:            []string{"Read", "Glob", "Grep"},
			DeniedTools:      []string{"Write", "Edit", "Bash"},
			DeniedActions:    []string{"rm", "curl", "wget"},
			RequiredArtifact: "PLAN.md",
			LLMScenario:      "think",
			Autonomy:         2,
			PromptPrefix: "You are a software architect. Analyze the codebase structure, " +
				"design components, and produce clear technical plans. Focus on modularity, " +
				"separation of concerns, and long-term maintainability.",
		},
		{
			ID:               "coder",
			Name:             "Coder",
			Description:      "Implements features and writes production code.",
			Builtin:          true,
			Tools:            []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
			DeniedActions:    []string{"rm -rf", "curl", "wget"},
			RequiredArtifact: "DIFF",
			LLMScenario:      "default",
			Autonomy:         3,
			PromptPrefix: "You are a software developer. Write clean, well-structured, " +
				"and tested code. Follow project conventions and keep changes focused.",
		},
		{
			ID:               "reviewer",
			Name:             "Reviewer",
			Description:      "Reviews code for correctness, style, and potential issues.",
			Builtin:          true,
			Tools:            []string{"Read", "Glob", "Grep"},
			DeniedTools:      []string{"Write", "Edit", "Bash"},
			RequiredArtifact: "REVIEW.md",
			LLMScenario:      "review",
			Autonomy:         2,
			PromptPrefix: "You are a code reviewer. Examine code for bugs, style violations, " +
				"security issues, and performance problems. Provide actionable feedback.",
		},
		{
			ID:            "debugger",
			Name:          "Debugger",
			Description:   "Diagnoses and fixes bugs by analyzing symptoms and tracing root causes.",
			Builtin:       true,
			Tools:         []string{"Read", "Edit", "Bash", "Glob", "Grep"},
			DeniedActions: []string{"rm -rf", "curl", "wget"},
			LLMScenario:   "default",
			Autonomy:      3,
			PromptPrefix: "You are a debugging specialist. Analyze error messages, trace execution " +
				"paths, and identify root causes. Apply minimal, targeted fixes.",
		},
		{
			ID:               "tester",
			Name:             "Tester",
			Description:      "Writes and maintains tests, ensuring comprehensive coverage.",
			Builtin:          true,
			Tools:            []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep"},
			DeniedActions:    []string{"rm -rf", "curl", "wget"},
			RequiredArtifact: "TEST_REPORT",
			LLMScenario:      "default",
			Autonomy:         3,
			PromptPrefix: "You are a test engineer. Write thorough unit, integration, and " +
				"end-to-end tests. Aim for high coverage and clear failure messages.",
		},
		{
			ID:            "documenter",
			Name:          "Documenter",
			Description:   "Writes and maintains documentation, READMEs, and code comments.",
			Builtin:       true,
			Tools:         []string{"Read", "Write", "Edit", "Glob", "Grep"},
			DeniedTools:   []string{"Bash"},
			DeniedActions: []string{"rm", "curl", "wget"},
			LLMScenario:   "default",
			Autonomy:      3,
			PromptPrefix: "You are a technical writer. Produce clear, accurate documentation " +
				"that helps developers understand and use the codebase effectively.",
		},
		{
			ID:               "refactorer",
			Name:             "Refactorer",
			Description:      "Improves code structure without changing external behavior.",
			Builtin:          true,
			Tools:            []string{"Read", "Write", "Edit", "Glob", "Grep"},
			DeniedTools:      []string{"Bash"},
			DeniedActions:    []string{"rm", "curl", "wget"},
			RequiredArtifact: "DIFF",
			LLMScenario:      "default",
			Autonomy:         2,
			PromptPrefix: "You are a refactoring specialist. Improve code structure, reduce " +
				"duplication, and enhance readability while preserving existing behavior.",
		},
		{
			ID:               "security",
			Name:             "Security Auditor",
			Description:      "Audits code for security vulnerabilities and compliance issues.",
			Builtin:          true,
			Tools:            []string{"Read", "Glob", "Grep", "Bash"},
			DeniedTools:      []string{"Write", "Edit"},
			DeniedActions:    []string{"rm", "curl", "wget"},
			RequiredArtifact: "AUDIT_REPORT",
			LLMScenario:      "review",
			Autonomy:         2,
			PromptPrefix: "You are a security auditor. Identify vulnerabilities, insecure patterns, " +
				"and compliance issues. Recommend concrete mitigations following OWASP guidelines.",
		},
	}
}
