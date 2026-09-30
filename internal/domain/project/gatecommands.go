package project

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// Project config keys that set the commands of the project's quality gate.
//
// Trust model: the worker runs a gate command in the project's workspace
// with the worker's rights, without the policy layer (a gate is no tool call
// of the agent). Whoever may change the project config (PUT /projects/{id}:
// admins and editors, the roles that may also start runs in the project)
// can therefore run code in the worker through the project's toolchain: the
// allowlist limits the executable, not what it runs (`python -c`, `make`,
// `npm run` and the test suite itself run arbitrary code from the
// workspace). The check at write time rejects commands the worker would
// refuse, so a misconfigured gate fails the update (HTTP 400) instead of
// every later gate.
const (
	ConfigTestCommand = "test_command"
	ConfigLintCommand = "lint_command"
)

// gateCommandAllowlist is the worker's allowlist of gate executables
// (_ALLOWED_COMMANDS in workers/codeforge/qualitygate.py; a test keeps both
// equal). The executable is the first word of the command, matched exactly.
var gateCommandAllowlist = []string{
	"black", "cargo", "eslint", "flake8", "go", "golangci-lint", "isort", "make", "mypy",
	"npm", "npx", "pnpm", "pre-commit", "prettier", "pylint", "pytest", "python", "ruff",
	"tsc", "yarn",
}

// ValidateGateCommand checks the gate command cmd set under the project
// config key: blank (unset), or a command the worker accepts - it splits
// like a POSIX shell word list (Python's shlex.split) and its executable is
// on the worker's allowlist.
func ValidateGateCommand(key, cmd string) error {
	if err := CheckGateCommand(cmd); err != nil {
		return fmt.Errorf("%s: %s: %w", key, err.Error(), domain.ErrValidation)
	}
	return nil
}

// CheckGateCommand is ValidateGateCommand without a config key: the check of
// the runtime defaults at config load (runtime.default_test_command and
// default_lint_command, S3 follow-up 1d).
func CheckGateCommand(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	argv, err := splitCommand(cmd)
	if err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}
	if !slices.Contains(gateCommandAllowlist, argv[0]) {
		return fmt.Errorf("%q is not on the worker's allowlist (%s)", argv[0], strings.Join(gateCommandAllowlist, ", "))
	}
	return nil
}

// validateGateCommands checks the gate commands of a project config.
func validateGateCommands(config map[string]*string) error {
	for _, key := range []string{ConfigTestCommand, ConfigLintCommand} {
		if cmd := config[key]; cmd != nil {
			if err := ValidateGateCommand(key, *cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

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
// (ScanWorkspace names), with how to tell that the language's test runner is
// set up at the workspace root. Earlier entries win ties.
var languageGateCommands = []struct {
	language   string
	commands   GateCommands
	testRunner func(fsys fs.FS) bool
}{
	{"go", GateCommands{Test: "go test ./...", Lint: "golangci-lint run ./..."}, hasFile("go.mod")},
	{"rust", GateCommands{Test: "cargo test", Lint: "cargo clippy -- -D warnings"}, hasFile("Cargo.toml")},
	{"typescript", GateCommands{Test: "npm test", Lint: "npm run lint"}, hasNpmTestScript},
	{"javascript", GateCommands{Test: "npm test", Lint: "npm run lint"}, hasNpmTestScript},
	{"python", GateCommands{Test: "pytest", Lint: "ruff check ."}, hasPytestConfig},
}

// DefaultGateCommands returns the default gate commands of the detected
// language whose test runner is set up at the root of the workspace fsys,
// else of any detected language; among them the one with the highest
// confidence, then the first in languageGateCommands. None when no detected
// language has defaults.
func DefaultGateCommands(fsys fs.FS, languages []Language) GateCommands {
	var best GateCommands
	bestRunner, bestConfidence := false, -1.0
	for _, entry := range languageGateCommands {
		for _, lang := range languages {
			if lang.Name != entry.language {
				continue
			}
			runner := fsys != nil && entry.testRunner(fsys)
			if (runner && !bestRunner) || (runner == bestRunner && lang.Confidence > bestConfidence) {
				best, bestRunner, bestConfidence = entry.commands, runner, lang.Confidence
			}
		}
	}
	return best
}

func hasFile(name string) func(fs.FS) bool {
	return func(fsys fs.FS) bool {
		_, err := fs.Stat(fsys, name)
		return err == nil
	}
}

// hasNpmTestScript tells whether package.json has a test script other than
// the placeholder `npm init` writes (which fails).
func hasNpmTestScript(fsys fs.FS) bool {
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(readFileCappedFS(fsys, "package.json", maxManifestRead)), &manifest); err != nil {
		return false
	}
	test := manifest.Scripts["test"]
	return test != "" && !strings.Contains(test, "no test specified")
}

// hasPytestConfig tells whether pytest is configured at the workspace root.
func hasPytestConfig(fsys fs.FS) bool {
	if hasFile("pytest.ini")(fsys) || hasFile("conftest.py")(fsys) {
		return true
	}
	for name, section := range map[string]string{
		"pyproject.toml": "[tool.pytest",
		"setup.cfg":      "[tool:pytest]",
		"tox.ini":        "[pytest]",
	} {
		if strings.Contains(readFileCappedFS(fsys, name, maxManifestRead), section) {
			return true
		}
	}
	return false
}

// GateCommandOverrides returns the gate commands set in the project config.
func (p *Project) GateCommandOverrides() GateCommands {
	return GateCommands{
		Test: strings.TrimSpace(p.Config[ConfigTestCommand]),
		Lint: strings.TrimSpace(p.Config[ConfigLintCommand]),
	}
}
