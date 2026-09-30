package project

import "strings"

// Project config keys that set the commands of the project's quality gate.
const (
	ConfigTestCommand = "test_command"
	ConfigLintCommand = "lint_command"
)

// GateCommands are the test and lint commands a quality gate runs; "" is
// unset.
type GateCommands struct {
	Test string
	Lint string
}

// Or fills the unset commands of c from fallback.
func (c GateCommands) Or(fallback GateCommands) GateCommands {
	if c.Test == "" {
		c.Test = fallback.Test
	}
	if c.Lint == "" {
		c.Lint = fallback.Lint
	}
	return c
}

// languageGateCommands are the default gate commands per detected language
// (ScanWorkspace names). Earlier entries win ties.
var languageGateCommands = []struct {
	language string
	commands GateCommands
}{
	{"go", GateCommands{Test: "go test ./...", Lint: "golangci-lint run ./..."}},
	{"rust", GateCommands{Test: "cargo test", Lint: "cargo clippy -- -D warnings"}},
	{"typescript", GateCommands{Test: "npm test", Lint: "npm run lint"}},
	{"javascript", GateCommands{Test: "npm test", Lint: "npm run lint"}},
	{"python", GateCommands{Test: "pytest", Lint: "ruff check ."}},
}

// DefaultGateCommands returns the default gate commands of the detected
// language with the highest confidence that has defaults; none when no
// detected language has any.
func DefaultGateCommands(languages []Language) GateCommands {
	var best GateCommands
	bestConfidence := -1.0
	for _, entry := range languageGateCommands {
		for _, lang := range languages {
			if lang.Name == entry.language && lang.Confidence > bestConfidence {
				best, bestConfidence = entry.commands, lang.Confidence
			}
		}
	}
	return best
}

// GateCommandOverrides returns the gate commands set in the project config.
func (p *Project) GateCommandOverrides() GateCommands {
	return GateCommands{
		Test: strings.TrimSpace(p.Config[ConfigTestCommand]),
		Lint: strings.TrimSpace(p.Config[ConfigLintCommand]),
	}
}
