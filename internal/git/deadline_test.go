package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/git"
)

// KI-187: git opens files of the agent-writable workspace by name with a
// blocking open(), so a FIFO there blocks the git process until something
// writes to it. No Go Core git command may block forever: each one ends at
// its deadline (git.command_timeout, git.network_timeout) or the caller's,
// whichever comes first.

// setTimeouts sets the git deadlines for one test.
func setTimeouts(t *testing.T, command, network time.Duration) {
	t.Helper()
	git.SetTimeouts(command, network)
	t.Cleanup(func() { git.SetTimeouts(git.DefaultCommandTimeout, git.DefaultNetworkTimeout) })
}

// mkfifo creates a FIFO at path. Cleanup opens it for writing once, which
// releases a git process still blocked on it.
func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil { //nolint:gosec // test FIFO
			_ = f.Close()
		}
	})
}

// openWithin runs OpenRepo and fails the test when it has not returned
// within 20 seconds.
func openWithin(ctx context.Context, t *testing.T, dir string) error {
	t.Helper()
	const limit = 20 * time.Second
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := git.OpenRepo(ctx, dir)
		done <- err
	}()
	select {
	case err := <-done:
		t.Logf("OpenRepo returned after %v: %v", time.Since(start).Round(time.Millisecond), err)
		return err
	case <-time.After(limit):
		t.Fatalf("OpenRepo still blocked after %v", limit)
		return nil
	}
}

// A FIFO .gitignore in a new directory that the pre-scan did not reach is
// read by the listing of ignored directories: the command's own deadline
// ends it, without a deadline on the caller's context.
func TestOpenRepo_FIFOEndsAtTheCommandDeadline(t *testing.T) {
	setTimeouts(t, 2*time.Second, time.Minute)
	git.SetPreScanEntries(t, 1) // the pre-scan would refuse the FIFO first
	dir := newRepo(t)
	mkfifo(t, filepath.Join(dir, "new", ".gitignore"))

	err := openWithin(context.Background(), t, dir)
	if !errors.Is(err, git.ErrGitTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("OpenRepo = %v, want an error wrapping ErrGitTimeout and context.DeadlineExceeded", err)
	}
}

// The caller's deadline still ends a command before its own.
func TestOpenRepo_FIFOEndsAtTheCallersDeadline(t *testing.T) {
	setTimeouts(t, time.Hour, time.Hour)
	git.SetPreScanEntries(t, 1)
	dir := newRepo(t)
	mkfifo(t, filepath.Join(dir, "new", ".gitignore"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := openWithin(ctx, t, dir); !errors.Is(err, git.ErrGitTimeout) {
		t.Fatalf("OpenRepo = %v, want an error wrapping ErrGitTimeout and context.DeadlineExceeded", err)
	}
}

// Commands outside a repository (clone, init) have deadlines too: a clone
// from a remote that never answers ends at the network deadline.
func TestRun_NetworkCommandEndsAtTheNetworkDeadline(t *testing.T) {
	setTimeouts(t, time.Hour, 2*time.Second)
	// A local "remote" whose HEAD is a FIFO: upload-pack blocks reading it.
	remote := newRepo(t)
	if err := os.Remove(filepath.Join(remote, ".git", "packed-refs")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	head := filepath.Join(remote, ".git", "HEAD")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	mkfifo(t, head)

	done := make(chan error, 1)
	go func() {
		_, err := git.Run(context.Background(), "", "clone", "-q", "--", remote, filepath.Join(t.TempDir(), "clone"))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, git.ErrGitTimeout) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("clone = %v, want an error wrapping ErrGitTimeout and context.DeadlineExceeded", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("clone still blocked after 20s")
	}
}

func TestSetTimeouts_NonPositiveKeepsTheDefault(t *testing.T) {
	setTimeouts(t, 0, -1)
	if got := git.CommandTimeout("status"); got != git.DefaultCommandTimeout {
		t.Fatalf("status deadline = %v, want the default %v", got, git.DefaultCommandTimeout)
	}
	for _, cmd := range []string{"clone", "fetch", "pull", "push", "ls-remote"} {
		if got := git.CommandTimeout(cmd); got != git.DefaultNetworkTimeout {
			t.Fatalf("%s deadline = %v, want the network default %v", cmd, got, git.DefaultNetworkTimeout)
		}
	}
	setTimeouts(t, 3*time.Second, 4*time.Second)
	if got := git.CommandTimeout("ls-files"); got != 3*time.Second {
		t.Fatalf("ls-files deadline = %v, want 3s", got)
	}
	if got := git.CommandTimeout("fetch"); got != 4*time.Second {
		t.Fatalf("fetch deadline = %v, want 4s", got)
	}
}
