package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// Feature verification (KI-152): the auto-agent no longer takes a run that
// ended for a done feature. After the run it checks that the workspace
// changed and runs the project's test and lint commands in the worker
// (conversation.test.request; the Go Core never runs workspace code). What
// failed goes back to the agent for at most fixAttempts more runs; then the
// feature fails with the reason.

// DefaultAutoAgentFixAttempts is agent.auto_agent_fix_attempts' default.
const DefaultAutoAgentFixAttempts = 2

// AutoAgentVerification configures the verification of a feature.
type AutoAgentVerification struct {
	// FixAttempts bounds the runs that fix a failed verification.
	FixAttempts int
	// ToolOutputMaxChars bounds each check's output fed back to the agent
	// (agent.tool_output_max_chars; 0: the worker's default).
	ToolOutputMaxChars int
	// Defaults are runtime.default_test_command and default_lint_command.
	Defaults project.GateCommands
}

// SetVerification configures how features are verified.
func (s *AutoAgentService) SetVerification(v AutoAgentVerification) {
	s.verify = v
}

// featureVerification is what the checks of one feature need.
type featureVerification struct {
	projectID      string
	conversationID string
	title          string
	// testFile is the pytest file named in the description ("Tests:
	// test_x.py"), run when no test command is configured or detected.
	testFile string
	// before is the workspace before the feature's first run.
	before workspaceSnapshot
}

// featureChecks is the outcome of checking a feature once.
type featureChecks struct {
	passed   []string // checks that passed
	failures []string // checks that failed, with their output, as the agent gets them
	failed   []string // the failed checks in short, for the feature's result
	notes    []string // checks that were skipped or could not run
}

func (c *featureChecks) fail(short, detail string) {
	c.failed = append(c.failed, short)
	c.failures = append(c.failures, detail)
}

// verifyFeature checks the feature after its run and hands what failed back
// to the agent through fix (a run of the feature's conversation), at most
// FixAttempts times. It returns the feature's result, or an error when the
// feature failed.
func (s *AutoAgentService) verifyFeature(ctx context.Context, fv *featureVerification, fix func(prompt string) error) (string, error) {
	for attempt := 0; ; attempt++ {
		checks, err := s.checkFeature(ctx, fv)
		if err != nil {
			return "", err
		}
		if len(checks.failed) == 0 {
			return checks.result(attempt), nil
		}
		if attempt >= s.verify.FixAttempts {
			return "", fmt.Errorf("verification failed after %s: %s",
				plural(attempt, "fix attempt"), strings.Join(checks.failed, "; "))
		}
		if err := fix(checks.fixPrompt(fv.title, attempt+1, s.verify.FixAttempts)); err != nil {
			return "", fmt.Errorf("fix run: %w", err)
		}
	}
}

// checkFeature checks the workspace once: it must have changed since the
// feature started, then its test and lint commands must pass. An error is
// returned only when ctx ended.
func (s *AutoAgentService) checkFeature(ctx context.Context, fv *featureVerification) (*featureChecks, error) {
	checks := &featureChecks{}
	proj, err := s.db.GetProject(ctx, fv.projectID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		checks.notes = append(checks.notes, "the checks could not run: "+err.Error())
		return checks, nil
	}

	after := takeWorkspaceSnapshot(ctx, proj.WorkspacePath)
	switch {
	case fv.before.err != nil || after.err != nil:
		checks.notes = append(checks.notes, "change check skipped: "+errors.Join(fv.before.err, after.err).Error())
	case fv.before.source != after.source:
		// git could read the workspace before but not now, or the other way round.
		checks.notes = append(checks.notes, fmt.Sprintf("change check skipped: the workspace was read by %s before and by %s after",
			fv.before.source, after.source))
	case fv.before.digest == after.digest:
		checks.fail("the workspace did not change",
			"The workspace has no changes: nothing of the feature was implemented.")
		return checks, nil // the tests of an unchanged workspace tell nothing
	default:
		checks.passed = append(checks.passed, "the workspace changed")
	}

	cmds, source := s.featureCommands(proj)
	configuredTest := cmds.Test
	if cmds.Test, err = s.allowedByPolicy(ctx, fv.conversationID, "tests", cmds.Test, checks); err != nil {
		return nil, err
	}
	if cmds.Lint, err = s.allowedByPolicy(ctx, fv.conversationID, "lint", cmds.Lint, checks); err != nil {
		return nil, err
	}
	switch {
	case configuredTest != "":
		// It runs below, or the policy skipped it.
	case fv.testFile != "":
		if err := s.checkTestFile(ctx, fv, checks); err != nil {
			return nil, err
		}
	default:
		checks.notes = append(checks.notes, "no test command configured or detected, only the change check ran")
	}
	if cmds.Test == "" && cmds.Lint == "" {
		return checks, nil
	}
	res, err := s.runWorkspaceChecks(ctx, proj, fv.conversationID, cmds)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		checks.notes = append(checks.notes, "the test and lint commands could not run: "+err.Error())
		return checks, nil
	}
	if cmds.Test != "" {
		checks.add("tests", fmt.Sprintf("`%s`%s", cmds.Test, source), res.Passed, res.Output, res.Error)
	}
	if cmds.Lint != "" {
		checks.add("lint", fmt.Sprintf("`%s`", cmds.Lint), res.LintPassed, res.LintOutput, res.Error)
	}
	return checks, nil
}

// add records the verdict of one command; nil: it could not run.
func (c *featureChecks) add(check, command string, passed *bool, output, errMsg string) {
	switch {
	case passed == nil:
		reason := strings.TrimSpace(output)
		if reason == "" {
			reason = errMsg
		}
		c.notes = append(c.notes, fmt.Sprintf("%s gave no verdict (%s): %s", check, command, firstOutputLine(reason)))
	case *passed:
		c.passed = append(c.passed, fmt.Sprintf("%s passed (%s)", check, command))
	default:
		c.fail(fmt.Sprintf("%s failed (%s)", check, command),
			fmt.Sprintf("The %s failed (%s):\n```\n%s\n```", check, command, strings.TrimSpace(output)))
	}
}

// checkTestFile runs the pytest file the description names, as before KI-152.
func (s *AutoAgentService) checkTestFile(ctx context.Context, fv *featureVerification, checks *featureChecks) error {
	// The command the worker runs for it (_workspace_test.py).
	command := "python -m pytest " + fv.testFile + " -v --tb=short"
	if allowed, err := s.allowedByPolicy(ctx, fv.conversationID, "tests", command, checks); err != nil || allowed == "" {
		return err
	}
	result, err := s.runWorkspaceTest(ctx, fv.projectID, fv.conversationID, fv.testFile)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	switch {
	case errors.Is(err, errTestFileMissing):
		checks.fail(fmt.Sprintf("test file missing (%s)", fv.testFile),
			fmt.Sprintf("The feature names the test file %s, which does not exist in the workspace root: write it.", fv.testFile))
	case err != nil:
		// No verdict (no answer, an isolation refusal, a timeout): nothing
		// the agent can fix (KI-152 review).
		checks.notes = append(checks.notes, fmt.Sprintf("tests gave no verdict (%s): %s", fv.testFile, firstOutputLine(err.Error())))
	case result.AllPassed:
		checks.passed = append(checks.passed, fmt.Sprintf("tests passed (%s)", fv.testFile))
	default:
		checks.fail(fmt.Sprintf("tests failed (%s)", fv.testFile),
			fmt.Sprintf("The tests are failing. %d/%d tests passed.\n\nTest output:\n```\n%s\n```",
				result.Passed, result.Total, testOutputForPrompt(strings.TrimSpace(result.Output))))
	}
	return nil
}

// allowedByPolicy returns cmd when the feature conversation's agent may run
// it on its own; otherwise "" and a note why it was skipped (KI-152 review:
// the verification ran commands whatever the policy profile said). An
// error is returned only when ctx ended.
func (s *AutoAgentService) allowedByPolicy(ctx context.Context, conversationID, check, cmd string, checks *featureChecks) (string, error) {
	if cmd == "" || s.commandPolicy == nil {
		return cmd, nil
	}
	allowed, reason, err := s.commandPolicy.CommandAllowed(ctx, conversationID, cmd)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil { // fail closed
		allowed, reason = false, err.Error()
	}
	if !allowed {
		checks.notes = append(checks.notes, fmt.Sprintf("%s skipped (`%s`): %s", check, cmd, reason))
		return "", nil
	}
	return cmd, nil
}

// errTestFileMissing: the test file a feature names is not in the workspace.
var errTestFileMissing = errors.New("test file missing")

// result is the feature's result after the checks passed.
func (c *featureChecks) result(fixes int) string {
	verdict := "verified"
	if len(c.notes) > 0 {
		verdict = "not fully verified"
	}
	if fixes > 0 {
		verdict += " after " + plural(fixes, "fix attempt")
	}
	return verdict + ": " + strings.Join(append(append([]string{}, c.passed...), c.notes...), "; ")
}

// fixPrompt asks the agent to fix what failed.
func (c *featureChecks) fixPrompt(title string, attempt, attempts int) string {
	return fmt.Sprintf(
		"The feature %q is not done yet: CodeForge's verification failed (fix attempt %d of %d).\n\n%s\n\n"+
			"Fix this now: change the files with your tools instead of describing the changes, "+
			"then run the checks yourself if you can.",
		title, attempt, attempts, strings.Join(c.failures, "\n\n"))
}

// featureCommands returns the commands that verify a feature and how the
// test command was chosen: the project's test_command, else the default of
// the language detected in the workspace now (the run may have created the
// project), else runtime.default_test_command. Lint runs only when
// configured (lint_command, else runtime.default_lint_command): a detected
// linter may not be set up, and its findings would fail features that work.
func (s *AutoAgentService) featureCommands(proj *project.Project) (cmds project.GateCommands, testSource string) {
	cmds = proj.GateCommandOverrides()
	if cmds.Test == "" && proj.WorkspacePath != "" {
		// Only a runner that is set up: pytest in a project without tests
		// exits 5 and would fail a working feature (KI-152 review).
		if detected, setUp := detectGateCommands(proj); setUp && detected.Test != "" {
			cmds.Test, testSource = detected.Test, ", detected default"
		}
	}
	if cmds.Test == "" && s.verify.Defaults.Test != "" {
		cmds.Test, testSource = s.verify.Defaults.Test, ", runtime default"
	}
	if cmds.Lint == "" {
		cmds.Lint = s.verify.Defaults.Lint
	}
	return cmds, testSource
}

// runWorkspaceChecks runs the test and lint commands in the worker.
func (s *AutoAgentService) runWorkspaceChecks(ctx context.Context, proj *project.Project, conversationID string, cmds project.GateCommands) (*messagequeue.WorkspaceTestResultPayload, error) {
	tenantID := tenantctx.FromContext(ctx)
	toolUID, err := s.toolUIDs.PayloadToolUID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tool uid: %w", err)
	}
	return s.requestWorkspaceTest(ctx, &messagequeue.WorkspaceTestRequestPayload{
		RequestID:          uuid.New().String(),
		TenantID:           tenantID,
		ProjectID:          proj.ID,
		ConversationID:     conversationID,
		WorkspacePath:      proj.WorkspacePath,
		TestCommand:        cmds.Test,
		LintCommand:        cmds.Lint,
		TimeoutSeconds:     int(s.testTimeout.Seconds() + 0.999),
		ToolOutputMaxChars: s.verify.ToolOutputMaxChars,
		ToolUID:            toolUID,
	})
}

// workspaceSnapshot is a digest of a workspace's entries (path, type and
// permissions; size and modification time of files); err when the
// workspace could not be read. source names how it was taken ("git" or
// "walk"): only snapshots of one source compare.
type workspaceSnapshot struct {
	digest string
	source string
	err    error
}

// snapshotSkipDirs are left out of a walk snapshot: git's own state (a
// status rewrites the index), caches and dependencies that running the
// tests or the toolchain changes, and build output (KI-152 review).
var snapshotSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true, ".pytest_cache": true,
	".mypy_cache": true, ".ruff_cache": true, ".venv": true, ".tox": true,
	"target": true, "build": true, "dist": true, "coverage": true, ".gradle": true,
	".next": true, "htmlcov": true,
}

// snapshotSkipFiles are files left out of a walk snapshot (coverage data).
var snapshotSkipFiles = map[string]bool{".coverage": true}

// skipInSnapshot tells whether a walk snapshot leaves the entry out.
func skipInSnapshot(d fs.DirEntry) bool {
	if d.IsDir() {
		return snapshotSkipDirs[d.Name()] || strings.HasSuffix(d.Name(), ".egg-info")
	}
	return snapshotSkipFiles[d.Name()]
}

// maxSnapshotEntries bounds a snapshot: the entries a walk reads, the
// changed paths git reports.
const maxSnapshotEntries = 100_000

// takeWorkspaceSnapshot takes the workspace's change set from git when it is
// a repository git may run in (internal/git refuses unsafe ones), which
// respects .gitignore; otherwise, or when git fails, it walks the workspace.
func takeWorkspaceSnapshot(ctx context.Context, dir string) workspaceSnapshot {
	if snap, err := gitWorkspaceSnapshot(ctx, dir); err == nil {
		return snap
	} else if !errors.Is(err, git.ErrNotRepository) {
		slog.Debug("auto-agent change check: git not used, walking the workspace", "workspace", dir, "error", err)
	}
	return walkWorkspaceSnapshot(dir)
}

// gitWorkspaceSnapshot digests HEAD and the paths git status reports
// (tracked changes and untracked files that are not ignored) with their
// type, size and modification time: a commit, a new change and a further
// change of a file that was already dirty all change it. The status runs
// without optional locks (it does not rewrite the index).
func gitWorkspaceSnapshot(ctx context.Context, dir string) (workspaceSnapshot, error) {
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	env := []string{"GIT_OPTIONAL_LOCKS=0"}
	status, err := repo.Run(ctx, env, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return workspaceSnapshot{}, err
	}
	// Before the first commit there is no HEAD.
	head, _ := repo.Run(ctx, env, "rev-parse", "--verify", "--quiet", "HEAD")
	ws, err := workspacefs.Open(dir)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	defer func() { _ = ws.Close() }()

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "HEAD %s\n", strings.TrimSpace(head))
	fields := strings.Split(strings.TrimSuffix(status, "\x00"), "\x00")
	if len(fields) > maxSnapshotEntries {
		return workspaceSnapshot{}, fmt.Errorf("more than %d changed paths", maxSnapshotEntries)
	}
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 { // "XY path"
			continue
		}
		_, _ = fmt.Fprintf(h, "%s\x00", entry)
		if entry[0] == 'R' || entry[0] == 'C' { // the next field is the origin
			i++
		}
		if info, err := ws.Lstat(entry[3:]); err == nil {
			_, _ = fmt.Fprintf(h, "%v\x00%d\x00%d\n", info.Mode(), info.Size(), info.ModTime().UnixNano())
		} else {
			_, _ = fmt.Fprintf(h, "absent\n")
		}
	}
	return workspaceSnapshot{digest: hex.EncodeToString(h.Sum(nil)), source: "git"}, nil
}

// walkWorkspaceSnapshot walks the workspace through workspacefs (KI-95).
// Directories count by name only: their modification time changes when a
// cache directory is created in them.
func walkWorkspaceSnapshot(dir string) workspaceSnapshot {
	ws, err := workspacefs.Open(dir)
	if err != nil {
		return workspaceSnapshot{err: err, source: "walk"}
	}
	defer func() { _ = ws.Close() }()

	h := sha256.New()
	// The budget applies while directories are read (KI-152 review).
	err = ws.WalkDirBounded(".", maxSnapshotEntries, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name != "." && skipInSnapshot(d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			_, _ = fmt.Fprintf(h, "%s\x00%v\n", name, info.Mode())
		} else {
			_, _ = fmt.Fprintf(h, "%s\x00%v\x00%d\x00%d\n", name, info.Mode(), info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	if err != nil {
		return workspaceSnapshot{err: err, source: "walk"}
	}
	return workspaceSnapshot{digest: hex.EncodeToString(h.Sum(nil)), source: "walk"}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func firstOutputLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
