package git

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// CommandTimeout is the deadline of a git command (its first argument).
var CommandTimeout = commandTimeout

// SetCheckTimeout sets the deadline of OpenRepo's checks for one test.
func SetCheckTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := checkTimeout
	checkTimeout = d
	t.Cleanup(func() { checkTimeout = old })
}

// SetPreScanEntries sets the entry budget of OpenRepo's pre-scan of ignore
// and attributes files for one test.
func SetPreScanEntries(t *testing.T, n int) {
	t.Helper()
	old := maxPreScanEntries
	maxPreScanEntries = n
	t.Cleanup(func() { maxPreScanEntries = old })
}

// Command returns another program to run in the repository with the
// hardened environment; git processes it starts inherit the overrides. Only
// tests run other programs in a workspace: Go Core code runs git through
// runGit, with its deadline and process-group kill.
func (r *Repo) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.Dir
	cmd.Env = r.env(nil)
	killGroupOnCancel(cmd)
	cmd.WaitDelay = waitDelay
	return cmd
}
