package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-29: quality gates run the project's commands (project config, else the
// defaults of the detected language, else the configured defaults), and a
// gate that cannot run or reports no result for a required check fails.

// testsOnlyGate requires tests but no lint.
var testsOnlyGate = policy.PolicyProfile{
	Name:        "tests-only-gate",
	Mode:        policy.ModeDefault,
	QualityGate: policy.QualityGate{RequireTestsPass: true},
}

// newGateCommandEnv is a runtime whose project "proj-1" has the workspace
// with the given manifests and config, and whose configured default commands
// are defaults.
func newGateCommandEnv(t *testing.T, manifests []string, projectConfig map[string]string, defaults project.GateCommands) *gateDeliveryEnv {
	t.Helper()
	workspace := t.TempDir()
	for _, name := range manifests {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, store, queue, bc := newRuntimeTestEnv()
	store.projects[0].WorkspacePath = workspace
	store.projects[0].Config = projectConfig
	policySvc := service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{gateNoRollback, testsOnlyGate})
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, policySvc, &config.Runtime{
		StallThreshold:     5,
		DefaultTestCommand: defaults.Test,
		DefaultLintCommand: defaults.Lint,
	})
	env := &gateDeliveryEnv{svc: svc, store: store, queue: queue, deliverer: &recordingDeliverer{store: store}, checkpoints: &recordingCheckpointer{}}
	svc.SetDeliverService(env.deliverer)
	svc.SetCheckpointService(env.checkpoints)
	return env
}

func completeRun(t *testing.T, env *gateDeliveryEnv, runID string) {
	t.Helper()
	if err := env.svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{RunID: runID, Status: "completed", Output: "done"}); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}
}

func gateRequest(t *testing.T, env *gateDeliveryEnv) (messagequeue.QualityGateRequestPayload, bool) {
	t.Helper()
	msg, ok := env.queue.lastMessage(messagequeue.SubjectQualityGateRequest)
	if !ok {
		return messagequeue.QualityGateRequestPayload{}, false
	}
	var req messagequeue.QualityGateRequestPayload
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		t.Fatalf("unmarshal gate request: %v", err)
	}
	return req, true
}

func TestEnterQualityGate_RunsTheProjectsCommands(t *testing.T) {
	goDefaults := project.GateCommands{Test: "go test ./...", Lint: "golangci-lint run ./..."}
	configured := project.GateCommands{Test: "make check", Lint: "make lint"}
	tests := []struct {
		name      string
		manifests []string
		config    map[string]string
		defaults  project.GateCommands
		want      project.GateCommands
	}{
		{name: "python project", manifests: []string{"pyproject.toml"}, want: project.GateCommands{Test: "pytest", Lint: "ruff check ."}},
		{name: "typescript project", manifests: []string{"package.json", "tsconfig.json"}, want: project.GateCommands{Test: "npm test", Lint: "npm run lint"}},
		{name: "rust project", manifests: []string{"Cargo.toml"}, want: project.GateCommands{Test: "cargo test", Lint: "cargo clippy -- -D warnings"}},
		{name: "go project", manifests: []string{"go.mod", "go.sum"}, want: goDefaults},
		{name: "language wins over configured defaults", manifests: []string{"pyproject.toml"}, defaults: configured, want: project.GateCommands{Test: "pytest", Lint: "ruff check ."}},
		{
			name: "project config wins", manifests: []string{"pyproject.toml"}, defaults: configured,
			config: map[string]string{project.ConfigTestCommand: "pytest -q tests", project.ConfigLintCommand: "ruff check src"},
			want:   project.GateCommands{Test: "pytest -q tests", Lint: "ruff check src"},
		},
		{
			name: "project config per command", manifests: []string{"go.mod"},
			config: map[string]string{project.ConfigLintCommand: "go vet ./..."},
			want:   project.GateCommands{Test: "go test ./...", Lint: "go vet ./..."},
		},
		{name: "blank project config is unset", manifests: []string{"Cargo.toml"}, config: map[string]string{project.ConfigTestCommand: "  "}, want: project.GateCommands{Test: "cargo test", Lint: "cargo clippy -- -D warnings"}},
		{name: "unknown language uses configured defaults", manifests: []string{"Makefile"}, defaults: configured, want: configured},
		{name: "empty workspace uses configured defaults", defaults: goDefaults, want: goDefaults},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateCommandEnv(t, tc.manifests, tc.config, tc.defaults)
			env.addRun("run-cmd", "headless-safe-sandbox", run.StatusRunning, run.DeliverModeNone)

			completeRun(t, env, "run-cmd")

			req, ok := gateRequest(t, env)
			if !ok {
				t.Fatalf("no gate request; run = %+v", storedRun(t, env.store, "run-cmd"))
			}
			if got := (project.GateCommands{Test: req.TestCommand, Lint: req.LintCommand}); got != tc.want {
				t.Fatalf("gate commands = %+v, want %+v", got, tc.want)
			}
			if r := storedRun(t, env.store, "run-cmd"); r.Status != run.StatusQualityGate {
				t.Fatalf("run status = %s, want quality_gate", r.Status)
			}
		})
	}
}

func TestEnterQualityGate_MissingCommandFailsTheGate(t *testing.T) {
	tests := []struct {
		name      string
		profile   string
		defaults  project.GateCommands
		config    map[string]string
		wantError string
	}{
		{name: "no commands at all", profile: "headless-safe-sandbox", wantError: project.ConfigTestCommand},
		{name: "no lint command", profile: "headless-safe-sandbox", config: map[string]string{project.ConfigTestCommand: "make test"}, wantError: project.ConfigLintCommand},
		{name: "no test command", profile: testsOnlyGate.Name, defaults: project.GateCommands{Lint: "make lint"}, wantError: project.ConfigTestCommand},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateCommandEnv(t, []string{"Makefile"}, tc.config, tc.defaults)
			env.addRun("run-nocmd", tc.profile, run.StatusRunning, run.DeliverModeCommitLocal)

			completeRun(t, env, "run-nocmd")

			if _, ok := gateRequest(t, env); ok {
				t.Fatal("a gate without a command for a required check was requested")
			}
			r := storedRun(t, env.store, "run-nocmd")
			if r.Status != run.StatusFailed || !strings.Contains(r.Error, "quality gate failed") || !strings.Contains(r.Error, tc.wantError) {
				t.Fatalf("run = %s %q, want failed naming %q", r.Status, r.Error, tc.wantError)
			}
			if runs, _ := env.deliverer.deliveries(); len(runs) != 0 {
				t.Fatalf("delivered %v", runs)
			}
		})
	}
}

func TestEnterQualityGate_UnrequiredCheckNeedsNoCommand(t *testing.T) {
	env := newGateCommandEnv(t, []string{"Makefile"}, map[string]string{project.ConfigTestCommand: "make test"}, project.GateCommands{})
	env.addRun("run-tests-only", testsOnlyGate.Name, run.StatusRunning, run.DeliverModeNone)

	completeRun(t, env, "run-tests-only")

	req, ok := gateRequest(t, env)
	if !ok {
		t.Fatalf("no gate request; run = %+v", storedRun(t, env.store, "run-tests-only"))
	}
	if !req.RunTests || req.RunLint || req.TestCommand != "make test" {
		t.Fatalf("gate request = %+v, want tests only with make test", req)
	}
}

func TestHandleQualityGateResult_MissingResultFailsTheGate(t *testing.T) {
	passed, failed := true, false
	tests := []struct {
		name       string
		profile    string
		result     messagequeue.QualityGateResultPayload
		wantStatus run.Status
		wantError  string
	}{
		{name: "no results", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{}, wantStatus: run.StatusFailed, wantError: "no test result"},
		{name: "no lint result", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed}, wantStatus: run.StatusFailed, wantError: "no lint result"},
		{name: "no test result", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{LintPassed: &passed}, wantStatus: run.StatusFailed, wantError: "no test result"},
		{name: "tests-only gate without result", profile: testsOnlyGate.Name, result: messagequeue.QualityGateResultPayload{}, wantStatus: run.StatusFailed, wantError: "no test result"},
		{name: "tests-only gate passes", profile: testsOnlyGate.Name, result: messagequeue.QualityGateResultPayload{TestsPassed: &passed}, wantStatus: run.StatusCompleted},
		{name: "a failed extra check fails", profile: testsOnlyGate.Name, result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &failed}, wantStatus: run.StatusFailed, wantError: "lint failed"},
		{name: "all required checks pass", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &passed}, wantStatus: run.StatusCompleted},
		{name: "unknown profile", profile: "deleted-profile", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &passed}, wantStatus: run.StatusFailed, wantError: "deleted-profile"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateCommandEnv(t, nil, nil, project.GateCommands{})
			env.addRun("run-result", tc.profile, run.StatusQualityGate, run.DeliverModeCommitLocal)

			result := tc.result
			result.RunID = "run-result"
			if err := env.svc.HandleQualityGateResult(context.Background(), &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			r := storedRun(t, env.store, "run-result")
			if r.Status != tc.wantStatus || !strings.Contains(r.Error, tc.wantError) {
				t.Fatalf("run = %s %q, want %s with an error containing %q", r.Status, r.Error, tc.wantStatus, tc.wantError)
			}
			runs, _ := env.deliverer.deliveries()
			if delivered := len(runs) == 1; delivered != (tc.wantStatus == run.StatusCompleted) {
				t.Fatalf("deliveries = %v for a %s run", runs, tc.wantStatus)
			}
		})
	}
}

func TestHandleRunComplete_UnknownProfileFailsTheRun(t *testing.T) {
	env := newGateDeliveryEnv()
	env.addRun("run-noprofile", "deleted-profile", run.StatusRunning, run.DeliverModeCommitLocal)

	completeRun(t, env, "run-noprofile")

	r := storedRun(t, env.store, "run-noprofile")
	if r.Status != run.StatusFailed || !strings.Contains(r.Error, "deleted-profile") {
		t.Fatalf("run = %s %q, want failed naming the unknown profile", r.Status, r.Error)
	}
	if runs, _ := env.deliverer.deliveries(); len(runs) != 0 {
		t.Fatalf("delivered %v", runs)
	}
}
