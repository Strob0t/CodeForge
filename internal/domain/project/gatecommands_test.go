package project

import (
	"testing"
	"testing/fstest"
)

func TestDefaultGateCommands(t *testing.T) {
	goCmds := GateCommands{Test: "go test ./...", Lint: "golangci-lint run ./..."}
	pyCmds := GateCommands{Test: "pytest", Lint: "ruff check ."}
	npmCmds := GateCommands{Test: "npm test", Lint: "npm run lint"}
	tests := []struct {
		name      string
		languages []Language
		want      GateCommands
	}{
		{name: "nothing detected", want: GateCommands{}},
		{name: "empty list", languages: []Language{}, want: GateCommands{}},
		{name: "go", languages: []Language{{Name: "go", Confidence: 0.9}}, want: goCmds},
		{name: "python", languages: []Language{{Name: "python", Confidence: 0.7}}, want: pyCmds},
		{name: "typescript", languages: []Language{{Name: "typescript", Confidence: 0.9}}, want: npmCmds},
		{name: "javascript", languages: []Language{{Name: "javascript", Confidence: 0.7}}, want: npmCmds},
		{name: "rust", languages: []Language{{Name: "rust", Confidence: 0.7}}, want: GateCommands{Test: "cargo test", Lint: "cargo clippy -- -D warnings"}},
		{name: "language without defaults", languages: []Language{{Name: "docker", Confidence: 1.0}, {Name: "make", Confidence: 0.7}}, want: GateCommands{}},
		{name: "highest confidence wins", languages: []Language{{Name: "python", Confidence: 0.7}, {Name: "go", Confidence: 0.9}}, want: goCmds},
		{name: "highest confidence wins in any order", languages: []Language{{Name: "go", Confidence: 0.7}, {Name: "python", Confidence: 0.9}}, want: pyCmds},
		{name: "a tie follows the table order", languages: []Language{{Name: "python", Confidence: 0.9}, {Name: "go", Confidence: 0.9}}, want: goCmds},
		{name: "languages without defaults are skipped", languages: []Language{{Name: "docker", Confidence: 1.0}, {Name: "python", Confidence: 0.7}}, want: pyCmds},
		{name: "case sensitive names from the scanner", languages: []Language{{Name: "Go", Confidence: 0.9}}, want: GateCommands{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultGateCommands(fstest.MapFS{}, tc.languages); got != tc.want {
				t.Fatalf("DefaultGateCommands() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// DetectGateCommands tells whether the chosen test runner is set up: the
// auto-agent's verification uses a detected test command only then (KI-152
// review: pyproject.toml without tests made pytest exit 5).
func TestDetectGateCommands_TestRunnerSetUp(t *testing.T) {
	file := func(content string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(content)} }
	python := []Language{{Name: "python", Confidence: 0.9}}
	tests := []struct {
		name      string
		files     fstest.MapFS
		languages []Language
		wantTest  string
		wantSetUp bool
	}{
		{name: "pyproject without pytest config", files: fstest.MapFS{"pyproject.toml": file("[project]\nname = \"x\"\n")}, languages: python, wantTest: "pytest"},
		{name: "pyproject with pytest config", files: fstest.MapFS{"pyproject.toml": file("[tool.pytest.ini_options]\n")}, languages: python, wantTest: "pytest", wantSetUp: true},
		{name: "go.mod", files: fstest.MapFS{"go.mod": file("module x\n")}, languages: []Language{{Name: "go", Confidence: 0.9}}, wantTest: "go test ./...", wantSetUp: true},
		{name: "nothing detected", files: fstest.MapFS{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmds, setUp := DetectGateCommands(tc.files, tc.languages)
			if cmds.Test != tc.wantTest || setUp != tc.wantSetUp {
				t.Fatalf("DetectGateCommands() = %q, %v; want %q, %v", cmds.Test, setUp, tc.wantTest, tc.wantSetUp)
			}
		})
	}
}

func TestGateCommandOverrides(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		want   GateCommands
	}{
		{name: "nil config", want: GateCommands{}},
		{name: "both set", config: map[string]string{ConfigTestCommand: "make test", ConfigLintCommand: "make lint"}, want: GateCommands{Test: "make test", Lint: "make lint"}},
		{name: "test only", config: map[string]string{ConfigTestCommand: "pytest -q"}, want: GateCommands{Test: "pytest -q"}},
		{name: "surrounding whitespace trimmed", config: map[string]string{ConfigLintCommand: "  ruff check .\n"}, want: GateCommands{Lint: "ruff check ."}},
		{name: "blank is unset", config: map[string]string{ConfigTestCommand: "   ", ConfigLintCommand: ""}, want: GateCommands{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Project{Config: tc.config}
			if got := p.GateCommandOverrides(); got != tc.want {
				t.Fatalf("GateCommandOverrides() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGateCommandsOr(t *testing.T) {
	got := GateCommands{Test: "a"}.Or(GateCommands{Test: "b", Lint: "c"}).Or(GateCommands{Lint: "d"})
	if want := (GateCommands{Test: "a", Lint: "c"}); got != want {
		t.Fatalf("Or() = %+v, want %+v", got, want)
	}
}
