package project

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// Review of S3 (findings 6 and 12): gate commands set in the project config
// are checked when they are set, like the worker checks them, and the
// default commands come from the language whose test runner is set up.

func TestSplitCommand_MatchesPythonShlex(t *testing.T) {
	// Expected values are what Python's shlex.split returns (POSIX mode).
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{in: "pytest -q", want: []string{"pytest", "-q"}},
		{in: "  go   test\t./...\n", want: []string{"go", "test", "./..."}},
		{in: `pytest -k 'not slow and fast'`, want: []string{"pytest", "-k", "not slow and fast"}},
		{in: `pytest -k "a b"`, want: []string{"pytest", "-k", "a b"}},
		{in: `'pytest'`, want: []string{"pytest"}},
		{in: `py"test"`, want: []string{"pytest"}},
		{in: `echo a\ b`, want: []string{"echo", "a b"}},
		{in: `echo "a\"b"`, want: []string{"echo", `a"b`}},
		{in: `echo "a\\b"`, want: []string{"echo", `a\b`}},
		{in: `echo "a\nb"`, want: []string{"echo", `a\nb`}},
		{in: `echo 'a\b'`, want: []string{"echo", `a\b`}},
		{in: `echo a\\b`, want: []string{"echo", `a\b`}},
		{in: `echo '' x`, want: []string{"echo", "", "x"}},
		{in: `echo # not a comment`, want: []string{"echo", "#", "not", "a", "comment"}},
		{in: "", want: nil},
		{in: "   ", want: nil},
		{in: `pytest 'unterminated`, wantErr: true},
		{in: `ruff check "tests`, wantErr: true},
		{in: `pytest \`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := splitCommand(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("splitCommand(%q) = %q, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitCommand(%q): %v", tc.in, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("splitCommand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateGateCommand(t *testing.T) {
	tests := []struct {
		cmd     string
		wantErr string
	}{
		{cmd: ""},
		{cmd: "   "},
		{cmd: "pytest -q tests/unit"},
		{cmd: "go test ./..."},
		{cmd: "make test"},
		{cmd: "'npm' test"},
		{cmd: "echo ok", wantErr: "not on the worker's allowlist"},
		{cmd: "/usr/bin/pytest", wantErr: "not on the worker's allowlist"},
		{cmd: "sh -c 'pytest'", wantErr: "not on the worker's allowlist"},
		{cmd: "'' pytest", wantErr: "not on the worker's allowlist"},
		{cmd: "pytest 'unterminated", wantErr: "invalid command"},
		{cmd: `pytest \`, wantErr: "invalid command"},
	}
	for _, tc := range tests {
		t.Run(tc.cmd, func(t *testing.T) {
			err := ValidateGateCommand(ConfigTestCommand, tc.cmd)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateGateCommand(%q): %v", tc.cmd, err)
				}
				return
			}
			if err == nil || !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateGateCommand(%q) = %v, want a validation error containing %q", tc.cmd, err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), ConfigTestCommand) {
				t.Errorf("error %q does not name the config key", err)
			}
		})
	}
}

func TestValidateRequests_GateCommands(t *testing.T) {
	bad, good := "echo hi", "pytest"
	if err := ValidateUpdateRequest(UpdateRequest{Config: ConfigPatch{ConfigLintCommand: &bad}}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("update with a disallowed lint_command = %v, want a validation error", err)
	}
	if err := ValidateUpdateRequest(UpdateRequest{Config: ConfigPatch{ConfigTestCommand: &good, ConfigLintCommand: nil, "other": &bad}}); err != nil {
		t.Fatalf("update with an allowed test_command, a deleted lint_command and another key: %v", err)
	}
	if err := ValidateCreateRequest(&CreateRequest{Name: "p", Config: map[string]string{ConfigTestCommand: bad}}, nil); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("create with a disallowed test_command = %v, want a validation error", err)
	}
	if err := ValidateCreateRequest(&CreateRequest{Name: "p", Config: map[string]string{ConfigTestCommand: good}}, nil); err != nil {
		t.Fatalf("create with an allowed test_command: %v", err)
	}
}

// TestGateCommandAllowlist_MatchesTheWorker keeps the Go copy of the
// worker's allowlist in sync with workers/codeforge/qualitygate.py.
func TestGateCommandAllowlist_MatchesTheWorker(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "workers", "codeforge", "qualitygate.py")
	src, err := os.ReadFile(path) //nolint:gosec // test reads a file of the repository
	if err != nil {
		t.Fatalf("read worker allowlist: %v", err)
	}
	block := regexp.MustCompile(`(?s)_ALLOWED_COMMANDS: frozenset\[str\] = frozenset\(\s*\{(.*?)\}\s*\)`).FindSubmatch(src)
	if block == nil {
		t.Fatal("_ALLOWED_COMMANDS not found in qualitygate.py")
	}
	var worker []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(block[1], -1) {
		worker = append(worker, string(m[1]))
	}
	slices.Sort(worker)
	goList := slices.Sorted(slices.Values(gateCommandAllowlist))
	if !slices.Equal(worker, goList) {
		t.Fatalf("allowlists differ:\nworker: %v\ngo:     %v", worker, goList)
	}
}

func TestDefaultGateCommands_AreAllowed(t *testing.T) {
	for _, entry := range languageGateCommands {
		for _, cmd := range []string{entry.commands.Test, entry.commands.Lint} {
			if err := ValidateGateCommand(ConfigTestCommand, cmd); err != nil {
				t.Errorf("default %s command %q: %v", entry.language, cmd, err)
			}
		}
	}
}

func TestDefaultGateCommands_PreferTheLanguageWithATestRunnerSetUp(t *testing.T) {
	goCmds := GateCommands{Test: "go test ./...", Lint: "golangci-lint run ./..."}
	pyCmds := GateCommands{Test: "pytest", Lint: "ruff check ."}
	npmCmds := GateCommands{Test: "npm test", Lint: "npm run lint"}
	file := func(content string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(content)} }
	tests := []struct {
		name      string
		files     fstest.MapFS
		languages []Language
		want      GateCommands
	}{
		{
			name:      "tie: python has a pytest config, go has no go.mod",
			files:     fstest.MapFS{"pytest.ini": file("[pytest]\n"), "go.sum": file("")},
			languages: []Language{{Name: "go", Confidence: 0.9}, {Name: "python", Confidence: 0.9}},
			want:      pyCmds,
		},
		{
			name:      "tie: both set up, table order",
			files:     fstest.MapFS{"go.mod": file("module x\n"), "conftest.py": file("")},
			languages: []Language{{Name: "python", Confidence: 0.9}, {Name: "go", Confidence: 0.9}},
			want:      goCmds,
		},
		{
			name:      "set-up runner beats higher confidence: package.json without a test script",
			files:     fstest.MapFS{"package.json": file(`{"scripts":{"build":"tsc"}}`), "tsconfig.json": file("{}"), "go.mod": file("module x\n")},
			languages: []Language{{Name: "typescript", Confidence: 1.0}, {Name: "go", Confidence: 0.7}},
			want:      goCmds,
		},
		{
			name:      "npm's placeholder test script is no test runner",
			files:     fstest.MapFS{"package.json": file(`{"scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`), "pyproject.toml": file("[tool.pytest.ini_options]\n")},
			languages: []Language{{Name: "javascript", Confidence: 0.9}, {Name: "python", Confidence: 0.7}},
			want:      pyCmds,
		},
		{
			name:      "a real test script",
			files:     fstest.MapFS{"package.json": file(`{"scripts":{"test":"vitest run"}}`), "requirements.txt": file("")},
			languages: []Language{{Name: "javascript", Confidence: 0.7}, {Name: "python", Confidence: 0.7}},
			want:      npmCmds,
		},
		{
			name:      "pytest configured in setup.cfg",
			files:     fstest.MapFS{"setup.cfg": file("[tool:pytest]\naddopts = -q\n")},
			languages: []Language{{Name: "rust", Confidence: 0.9}, {Name: "python", Confidence: 0.7}},
			want:      pyCmds,
		},
		{
			name:      "pytest configured in tox.ini",
			files:     fstest.MapFS{"tox.ini": file("[tox]\n[pytest]\n"), "package.json": file(`{"scripts":{}}`)},
			languages: []Language{{Name: "javascript", Confidence: 0.9}, {Name: "python", Confidence: 0.7}},
			want:      pyCmds,
		},
		{
			name:      "both set up: Cargo.toml and a pytest config, table order",
			files:     fstest.MapFS{"setup.cfg": file("[tool:pytest]\n"), "Cargo.toml": file("")},
			languages: []Language{{Name: "python", Confidence: 0.7}, {Name: "rust", Confidence: 0.7}},
			want:      GateCommands{Test: "cargo test", Lint: "cargo clippy -- -D warnings"},
		},
		{
			name:      "nothing set up: highest confidence, then table order",
			files:     fstest.MapFS{"requirements.txt": file(""), "package.json": file("{}")},
			languages: []Language{{Name: "python", Confidence: 0.7}, {Name: "javascript", Confidence: 0.7}},
			want:      npmCmds,
		},
		{
			name:      "invalid package.json",
			files:     fstest.MapFS{"package.json": file("{"), "go.mod": file("module x\n")},
			languages: []Language{{Name: "javascript", Confidence: 0.9}, {Name: "go", Confidence: 0.7}},
			want:      goCmds,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DefaultGateCommands(tc.files, tc.languages); got != tc.want {
				t.Fatalf("DefaultGateCommands() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
