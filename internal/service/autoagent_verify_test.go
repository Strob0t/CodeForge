package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-152: the auto-agent marked a feature done whenever its run ended. It
// now checks that the workspace changed, runs the test and lint commands in
// the worker and hands failures back to the agent for a bounded number of
// fix runs before the feature fails.

func writeWorkspaceFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTakeWorkspaceSnapshot(t *testing.T) {
	tests := []struct {
		name    string
		change  func(t *testing.T, ws string)
		changed bool
	}{
		{name: "nothing", change: func(*testing.T, string) {}},
		{name: "file content", changed: true, change: func(t *testing.T, ws string) { writeWorkspaceFile(t, ws, "app.py", "print(2)\n# more\n") }},
		{name: "same size, newer mtime", changed: true, change: func(t *testing.T, ws string) {
			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(filepath.Join(ws, "app.py"), later, later); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "new file", changed: true, change: func(t *testing.T, ws string) { writeWorkspaceFile(t, ws, "pkg/mod.py", "") }},
		{name: "new empty directory", changed: true, change: func(t *testing.T, ws string) {
			if err := os.Mkdir(filepath.Join(ws, "docs"), 0o750); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "removed file", changed: true, change: func(t *testing.T, ws string) {
			if err := os.Remove(filepath.Join(ws, "app.py")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "git state only", change: func(t *testing.T, ws string) { writeWorkspaceFile(t, ws, ".git/index", "refreshed") }},
		{name: "caches only", change: func(t *testing.T, ws string) {
			writeWorkspaceFile(t, ws, "tests/__pycache__/test_app.cpython-312.pyc", "x")
			writeWorkspaceFile(t, ws, ".pytest_cache/v/cache/lastfailed", "{}")
			writeWorkspaceFile(t, ws, "node_modules/x/index.js", "")
		}},
		// KI-152 review: build output is no change of the feature.
		{name: "build output only", change: func(t *testing.T, ws string) {
			for _, f := range []string{"target/debug/app", "build/lib/app.py", "dist/app-0.1.tar.gz", "coverage/lcov.info",
				".gradle/cache", ".next/build-manifest.json", "htmlcov/index.html", "app.egg-info/PKG-INFO", ".coverage"} {
				writeWorkspaceFile(t, ws, f, "x")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := t.TempDir()
			writeWorkspaceFile(t, ws, "app.py", "print(1)\n")
			writeWorkspaceFile(t, ws, ".git/HEAD", "ref: refs/heads/main\n")
			writeWorkspaceFile(t, ws, "tests/test_app.py", "")
			before := walkWorkspaceSnapshot(ws)
			if before.err != nil || before.digest == "" {
				t.Fatalf("snapshot = %+v", before)
			}
			tc.change(t, ws)
			after := walkWorkspaceSnapshot(ws)
			if after.err != nil {
				t.Fatalf("snapshot after: %v", after.err)
			}
			if got := after.digest != before.digest; got != tc.changed {
				t.Fatalf("changed = %v, want %v", got, tc.changed)
			}
		})
	}
}

// KI-152 review: in a git repository the change set comes from git status
// (through internal/git), which respects .gitignore.
func TestTakeWorkspaceSnapshot_GitRepository(t *testing.T) {
	ctx := context.Background()
	runGit := func(t *testing.T, ws string, args ...string) {
		t.Helper()
		args = append([]string{"-c", "user.name=T", "-c", "user.email=t@example.com"}, args...)
		if out, err := git.Run(ctx, ws, args...); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	tests := []struct {
		name    string
		change  func(t *testing.T, ws string)
		changed bool
	}{
		{name: "nothing", change: func(*testing.T, string) {}},
		{name: "ignored build output", change: func(t *testing.T, ws string) { writeWorkspaceFile(t, ws, "out/app.bin", "x") }},
		{name: "untracked file", changed: true, change: func(t *testing.T, ws string) { writeWorkspaceFile(t, ws, "cli.py", "x") }},
		{name: "tracked file", changed: true, change: func(t *testing.T, ws string) { writeWorkspaceFile(t, ws, "app.py", "print(2)\n") }},
		{name: "already dirty file changed again", changed: true, change: func(t *testing.T, ws string) {
			writeWorkspaceFile(t, ws, "dirty.py", "v3, longer\n")
		}},
		{name: "committed change", changed: true, change: func(t *testing.T, ws string) {
			writeWorkspaceFile(t, ws, "cli.py", "x")
			runGit(t, ws, "add", "-A")
			runGit(t, ws, "commit", "-qm", "feature")
		}},
		{name: "removed tracked file", changed: true, change: func(t *testing.T, ws string) {
			if err := os.Remove(filepath.Join(ws, "app.py")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := t.TempDir()
			runGit(t, ws, "init", "-q")
			writeWorkspaceFile(t, ws, ".gitignore", "out/\n")
			writeWorkspaceFile(t, ws, "app.py", "print(1)\n")
			writeWorkspaceFile(t, ws, "dirty.py", "v1\n")
			runGit(t, ws, "add", "-A")
			runGit(t, ws, "commit", "-qm", "init")
			writeWorkspaceFile(t, ws, "dirty.py", "v2\n") // dirty before the feature
			before := takeWorkspaceSnapshot(ctx, ws)
			if before.err != nil || before.source != "git" {
				t.Fatalf("snapshot = %+v, want one from git", before)
			}
			tc.change(t, ws)
			after := takeWorkspaceSnapshot(ctx, ws)
			if after.err != nil || after.source != "git" {
				t.Fatalf("snapshot after = %+v", after)
			}
			if got := after.digest != before.digest; got != tc.changed {
				t.Fatalf("changed = %v, want %v", got, tc.changed)
			}
		})
	}
}

// A repository internal/git refuses (here: an include of other config) is not
// run in; the walk decides.
func TestTakeWorkspaceSnapshot_UnsafeRepositoryFallsBackToTheWalk(t *testing.T) {
	ctx := context.Background()
	ws := t.TempDir()
	if out, err := git.Run(ctx, ws, "init", "-q"); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if out, err := git.Run(ctx, ws, "config", "include.path", "/etc/gitconfig"); err != nil {
		t.Fatalf("git config: %v %s", err, out)
	}
	writeWorkspaceFile(t, ws, "app.py", "x")

	s := takeWorkspaceSnapshot(ctx, ws)

	if s.err != nil || s.source != "walk" {
		t.Fatalf("snapshot = %+v, want one from the walk", s)
	}
}

func TestTakeWorkspaceSnapshot_MissingWorkspace(t *testing.T) {
	if s := takeWorkspaceSnapshot(context.Background(), filepath.Join(t.TempDir(), "missing")); s.err == nil {
		t.Fatalf("snapshot of a missing workspace = %+v, want an error", s)
	}
}

// verifyEnv is a project workspace, a worker that answers check requests
// with reply, and the fix runs the agent gets.
type verifyEnv struct {
	svc     *AutoAgentService
	queue   *workspaceTestQueue
	ws      string
	proj    *project.Project
	prompts []string
}

func newVerifyEnv(t *testing.T, reply func(*messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload) *verifyEnv {
	t.Helper()
	ws := t.TempDir()
	writeWorkspaceFile(t, ws, "README.md", "# demo\n")
	store := newAutoAgentMockStore()
	proj := &project.Project{ID: "proj-1", WorkspacePath: ws, Config: map[string]string{}}
	store.projects["proj-1"] = proj
	q := &workspaceTestQueue{reply: reply}
	svc := NewAutoAgentService(store, &noopBroadcaster{}, q, nil)
	q.svc = svc
	return &verifyEnv{svc: svc, queue: q, ws: ws, proj: proj}
}

// verify runs the verification of a feature whose run did implement (or not).
func (e *verifyEnv) verify(t *testing.T, implement, fixes func(attempt int)) (string, error) {
	t.Helper()
	fv := &featureVerification{projectID: "proj-1", conversationID: "conv-1", title: "Add CLI",
		before: takeWorkspaceSnapshot(context.Background(), e.ws)}
	if implement != nil {
		implement(0)
	}
	ctx := tenantctx.WithTenant(context.Background(), "tenant-1")
	return e.svc.verifyFeature(ctx, fv, func(prompt string) error {
		e.prompts = append(e.prompts, prompt)
		if fixes != nil {
			fixes(len(e.prompts))
		}
		return nil
	})
}

func (e *verifyEnv) requests() []messagequeue.WorkspaceTestRequestPayload {
	e.queue.mu.Lock()
	defer e.queue.mu.Unlock()
	return append([]messagequeue.WorkspaceTestRequestPayload(nil), e.queue.requests...)
}

// verdicts answers every request with the given test and lint verdicts.
func verdicts(test, lint *bool, output string) func(*messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
	return func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
		res := &messagequeue.WorkspaceTestResultPayload{RequestID: req.RequestID, TenantID: req.TenantID}
		if req.TestCommand != "" {
			res.Passed, res.Output = test, output
		}
		if req.LintCommand != "" {
			res.LintPassed, res.LintOutput = lint, output
		}
		return res
	}
}

func TestVerifyFeature_UnchangedWorkspaceIsHandedBackThenFails(t *testing.T) {
	e := newVerifyEnv(t, verdicts(passedPtr(true), nil, ""))
	e.proj.Config[project.ConfigTestCommand] = "pytest"

	_, err := e.verify(t, nil, nil)

	if err == nil || !strings.Contains(err.Error(), "the workspace did not change") ||
		!strings.Contains(err.Error(), "after 2 fix attempts") {
		t.Fatalf("verifyFeature = %v, want the change check to fail after 2 fix attempts", err)
	}
	if len(e.prompts) != DefaultAutoAgentFixAttempts {
		t.Fatalf("fix prompts = %d, want %d", len(e.prompts), DefaultAutoAgentFixAttempts)
	}
	if !strings.Contains(e.prompts[0], "no changes") || !strings.Contains(e.prompts[0], "fix attempt 1 of 2") {
		t.Fatalf("fix prompt = %q", e.prompts[0])
	}
	if reqs := e.requests(); len(reqs) != 0 {
		t.Fatalf("checks ran on an unchanged workspace: %+v", reqs)
	}
}

func TestVerifyFeature_FixRunThatChangesTheWorkspacePasses(t *testing.T) {
	e := newVerifyEnv(t, verdicts(passedPtr(true), nil, ""))
	e.proj.Config[project.ConfigTestCommand] = "pytest"

	result, err := e.verify(t, nil, func(int) { writeWorkspaceFile(t, e.ws, "cli.py", "def main(): pass\n") })

	if err != nil {
		t.Fatalf("verifyFeature: %v", err)
	}
	if len(e.prompts) != 1 {
		t.Fatalf("fix prompts = %d, want 1", len(e.prompts))
	}
	if !strings.HasPrefix(result, "verified after 1 fix attempt: ") || !strings.Contains(result, "tests passed (`pytest`)") {
		t.Fatalf("result = %q", result)
	}
}

func TestVerifyFeature_FailingTestsAreHandedBackWithTheirOutput(t *testing.T) {
	fixed := false
	e := newVerifyEnv(t, func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
		passed := fixed
		return &messagequeue.WorkspaceTestResultPayload{RequestID: req.RequestID, Passed: &passed,
			Output: "exit code 1\nFAILED test_cli.py::test_main - assert 0 == 1"}
	})
	e.proj.Config[project.ConfigTestCommand] = "pytest -q"

	result, err := e.verify(t, func(int) { writeWorkspaceFile(t, e.ws, "cli.py", "x") }, func(int) { fixed = true })

	if err != nil {
		t.Fatalf("verifyFeature: %v", err)
	}
	if len(e.prompts) != 1 || !strings.Contains(e.prompts[0], "FAILED test_cli.py::test_main") ||
		!strings.Contains(e.prompts[0], "`pytest -q`") {
		t.Fatalf("fix prompts = %q", e.prompts)
	}
	if !strings.Contains(result, "tests passed (`pytest -q`)") {
		t.Fatalf("result = %q", result)
	}
	reqs := e.requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2 (after the run and after the fix)", len(reqs))
	}
	if r := reqs[0]; r.TestCommand != "pytest -q" || r.TestFile != "" || r.TenantID != "tenant-1" ||
		r.ConversationID != "conv-1" || r.WorkspacePath != e.ws || r.TimeoutSeconds <= 0 {
		t.Fatalf("request = %+v", r)
	}
}

func TestVerifyFeature_FailsAfterTheFixAttempts(t *testing.T) {
	for _, attempts := range []int{0, 1, 3} {
		e := newVerifyEnv(t, verdicts(passedPtr(false), passedPtr(true), "exit code 2\nboom"))
		e.svc.SetVerification(AutoAgentVerification{FixAttempts: attempts, ToolOutputMaxChars: 500})
		e.proj.Config[project.ConfigTestCommand] = "go test ./..."
		e.proj.Config[project.ConfigLintCommand] = "golangci-lint run"

		_, err := e.verify(t, func(int) { writeWorkspaceFile(t, e.ws, "main.go", "package main") }, nil)

		if err == nil || !strings.Contains(err.Error(), "tests failed (`go test ./...`)") || strings.Contains(err.Error(), "lint failed") {
			t.Fatalf("attempts %d: verifyFeature = %v", attempts, err)
		}
		if len(e.prompts) != attempts {
			t.Fatalf("attempts %d: fix prompts = %d", attempts, len(e.prompts))
		}
		reqs := e.requests()
		if len(reqs) != attempts+1 {
			t.Fatalf("attempts %d: requests = %d", attempts, len(reqs))
		}
		if r := reqs[0]; r.LintCommand != "golangci-lint run" || r.ToolOutputMaxChars != 500 {
			t.Fatalf("request = %+v", r)
		}
	}
}

func TestVerifyFeature_FailingLintIsHandedBack(t *testing.T) {
	e := newVerifyEnv(t, verdicts(passedPtr(true), passedPtr(false), "exit code 1\nE501 line too long"))
	e.svc.SetVerification(AutoAgentVerification{FixAttempts: 1, Defaults: project.GateCommands{Lint: "ruff check ."}})

	_, err := e.verify(t, func(int) { writeWorkspaceFile(t, e.ws, "app.py", "x = 1") }, nil)

	if err == nil || !strings.Contains(err.Error(), "lint failed (`ruff check .`)") {
		t.Fatalf("verifyFeature = %v", err)
	}
	if len(e.prompts) != 1 || !strings.Contains(e.prompts[0], "E501 line too long") {
		t.Fatalf("fix prompts = %q", e.prompts)
	}
}

func TestVerifyFeature_TestCommandFallsBack(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		config     map[string]string
		defaults   project.GateCommands
		wantTest   string
		wantLint   string
		wantResult string
	}{
		{name: "detected default", files: map[string]string{"go.mod": "module demo\n", "main.go": "package main\n"},
			wantTest: "go test ./...", wantResult: "tests passed (`go test ./...`, detected default)"},
		{name: "project config wins", files: map[string]string{"go.mod": "module demo\n"},
			config: map[string]string{project.ConfigTestCommand: "make test"}, wantTest: "make test", wantResult: "tests passed (`make test`)"},
		{name: "runtime default", files: map[string]string{"notes.txt": "x"},
			defaults: project.GateCommands{Test: "make check"}, wantTest: "make check", wantResult: "tests passed (`make check`, runtime default)"},
		{name: "no detected linter", files: map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\n", "app.py": ""},
			wantTest: "pytest", wantLint: "", wantResult: "tests passed (`pytest`, detected default)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerifyEnv(t, verdicts(passedPtr(true), passedPtr(true), ""))
			e.svc.SetVerification(AutoAgentVerification{FixAttempts: 2, Defaults: tc.defaults})
			for k, v := range tc.config {
				e.proj.Config[k] = v
			}
			result, err := e.verify(t, func(int) {
				for name, content := range tc.files {
					writeWorkspaceFile(t, e.ws, name, content)
				}
			}, nil)
			if err != nil {
				t.Fatalf("verifyFeature: %v", err)
			}
			reqs := e.requests()
			if len(reqs) != 1 || reqs[0].TestCommand != tc.wantTest || reqs[0].LintCommand != tc.wantLint {
				t.Fatalf("requests = %+v, want test %q lint %q", reqs, tc.wantTest, tc.wantLint)
			}
			if !strings.HasPrefix(result, "verified: the workspace changed; ") || !strings.Contains(result, tc.wantResult) {
				t.Fatalf("result = %q, want it to contain %q", result, tc.wantResult)
			}
		})
	}
}

func TestVerifyFeature_WithoutATestCommandOnlyTheChangeCheckRuns(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"no stack":                          {"notes.txt": "x"},
		"detected runner not set up (S9-V)": {"pyproject.toml": "[project]\nname = \"app\"\n", "app.py": "print(1)\n"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newVerifyEnv(t, verdicts(passedPtr(false), nil, "exit code 5\nno tests ran"))

			result, err := e.verify(t, func(int) {
				for file, content := range files {
					writeWorkspaceFile(t, e.ws, file, content)
				}
			}, nil)

			if err != nil {
				t.Fatalf("verifyFeature: %v", err)
			}
			if reqs := e.requests(); len(reqs) != 0 {
				t.Fatalf("requests = %+v, want none", reqs)
			}
			if !strings.Contains(result, "only the change check ran") {
				t.Fatalf("result = %q", result)
			}
		})
	}
}

func TestVerifyFeature_ChecksThatCouldNotRunAreNotHandedBack(t *testing.T) {
	e := newVerifyEnv(t, func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
		return &messagequeue.WorkspaceTestResultPayload{RequestID: req.RequestID,
			Output: "command could not start: pytest not found", Error: "test check could not run"}
	})
	e.proj.Config[project.ConfigTestCommand] = "pytest"

	result, err := e.verify(t, func(int) { writeWorkspaceFile(t, e.ws, "app.py", "x") }, nil)

	if err != nil {
		t.Fatalf("verifyFeature: %v", err)
	}
	if len(e.prompts) != 0 {
		t.Fatalf("fix prompts = %q, want none", e.prompts)
	}
	if !strings.HasPrefix(result, "not fully verified: ") || !strings.Contains(result, "tests gave no verdict (`pytest`): command could not start") {
		t.Fatalf("result = %q", result)
	}
}

func TestVerifyFeature_TestFileFromTheDescriptionWithoutACommand(t *testing.T) {
	e := newVerifyEnv(t, func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
		return &messagequeue.WorkspaceTestResultPayload{RequestID: req.RequestID, Passed: passedPtr(true), Output: "=== 2 passed ==="}
	})
	fv := &featureVerification{projectID: "proj-1", conversationID: "conv-1", title: "x", testFile: "test_cli.py",
		before: takeWorkspaceSnapshot(context.Background(), e.ws)}
	writeWorkspaceFile(t, e.ws, "test_cli.py", "def test_x(): pass\n")

	result, err := e.svc.verifyFeature(context.Background(), fv, func(string) error { return errors.New("no fix expected") })

	if err != nil {
		t.Fatalf("verifyFeature: %v", err)
	}
	if reqs := e.requests(); len(reqs) != 1 || reqs[0].TestFile != "test_cli.py" || reqs[0].TestCommand != "" {
		t.Fatalf("requests = %+v", reqs)
	}
	if !strings.Contains(result, "tests passed (test_cli.py)") {
		t.Fatalf("result = %q", result)
	}
}

// KI-152 review: the description's test file fails the feature only on a
// verdict or when the file is missing; a run that could not happen (no
// answer, no verdict, an isolation refusal) leaves it "not fully verified".
func TestVerifyFeature_TestFileOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		writeTest bool
		reply     *messagequeue.WorkspaceTestResultPayload // nil: no answer
		wantFail  string
		wantNote  string
	}{
		{name: "failing verdict", writeTest: true, wantFail: "tests failed (test_cli.py)",
			reply: &messagequeue.WorkspaceTestResultPayload{Passed: passedPtr(false), Output: "=== 1 failed, 1 passed ==="}},
		{name: "missing test file", wantFail: "test file missing (test_cli.py)"},
		{name: "no verdict", writeTest: true, wantNote: "tests gave no verdict (test_cli.py)",
			reply: &messagequeue.WorkspaceTestResultPayload{Output: "tool isolation required", Error: "tool isolation required"}},
		{name: "no answer", writeTest: true, wantNote: "tests gave no verdict (test_cli.py)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerifyEnv(t, func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
				if tc.reply == nil {
					return nil
				}
				res := *tc.reply
				res.RequestID = req.RequestID
				return &res
			})
			e.svc.testTimeout, e.svc.testWaitMargin = 10*time.Millisecond, 10*time.Millisecond
			e.svc.SetVerification(AutoAgentVerification{FixAttempts: 1})
			fv := &featureVerification{projectID: "proj-1", conversationID: "conv-1", title: "x", testFile: "test_cli.py",
				before: takeWorkspaceSnapshot(context.Background(), e.ws)}
			writeWorkspaceFile(t, e.ws, "cli.py", "x")
			if tc.writeTest {
				writeWorkspaceFile(t, e.ws, "test_cli.py", "def test_x(): pass\n")
			}
			var prompts []string

			result, err := e.svc.verifyFeature(context.Background(), fv, func(p string) error {
				prompts = append(prompts, p)
				return nil
			})

			if tc.wantFail != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantFail) || len(prompts) != 1 {
					t.Fatalf("verifyFeature = %q, %v (prompts %d); want failure %q after one fix", result, err, len(prompts), tc.wantFail)
				}
				return
			}
			if err != nil || len(prompts) != 0 {
				t.Fatalf("verifyFeature = %v (prompts %d), want no failure", err, len(prompts))
			}
			if !strings.HasPrefix(result, "not fully verified: ") || !strings.Contains(result, tc.wantNote) {
				t.Fatalf("result = %q, want a note %q", result, tc.wantNote)
			}
		})
	}
}

func TestVerifyFeature_StoppedAutoAgentEndsTheVerification(t *testing.T) {
	e := newVerifyEnv(t, nil) // the worker never answers
	e.proj.Config[project.ConfigTestCommand] = "pytest"
	fv := &featureVerification{projectID: "proj-1", conversationID: "conv-1", before: takeWorkspaceSnapshot(context.Background(), e.ws)}
	writeWorkspaceFile(t, e.ws, "app.py", "x")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for deadline := time.Now().Add(5 * time.Second); len(e.requests()) == 0 && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()

	_, err := e.svc.verifyFeature(ctx, fv, func(string) error { return nil })

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("verifyFeature = %v, want context.Canceled", err)
	}
}

func TestVerifyFeature_FixRunErrorFailsTheFeature(t *testing.T) {
	e := newVerifyEnv(t, nil)
	fv := &featureVerification{projectID: "proj-1", conversationID: "conv-1", before: takeWorkspaceSnapshot(context.Background(), e.ws)}

	_, err := e.svc.verifyFeature(context.Background(), fv, func(string) error { return errors.New("conversation run failed") })

	if err == nil || !strings.Contains(err.Error(), "fix run: conversation run failed") {
		t.Fatalf("verifyFeature = %v", err)
	}
}

func TestFinishFeature_StoresTheResult(t *testing.T) {
	store := newAutoAgentMockStore()
	seedRoadmapWithFeatures(store, "proj-1", []roadmap.Feature{{ID: "f1", Status: roadmap.FeatureInProgress}})
	svc := NewAutoAgentService(store, &noopBroadcaster{}, &noopQueue{}, nil)

	long := strings.Repeat("é", maxFeatureResult+10)
	if err := svc.finishFeature(context.Background(), "f1", roadmap.FeatureCancelled, long); err != nil {
		t.Fatalf("finishFeature: %v", err)
	}
	got := store.featByID["f1"]
	if got.Status != roadmap.FeatureCancelled || len([]rune(got.Result)) != maxFeatureResult+3 {
		t.Fatalf("feature = %s, result of %d runes", got.Status, len([]rune(got.Result)))
	}
}

func TestNewAutoAgentService_VerifiesWithTwoFixAttempts(t *testing.T) {
	svc := NewAutoAgentService(newAutoAgentMockStore(), &noopBroadcaster{}, &noopQueue{}, nil)
	if svc.verify.FixAttempts != 2 {
		t.Fatalf("fix attempts = %d, want 2", svc.verify.FixAttempts)
	}
}
