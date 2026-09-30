package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runGit runs git in dir with env added to the process environment and
// returns its standard output.
func runGit(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: git with arguments CodeForge builds, no shell
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

// worktreeIndex is a private index of a workspace repository that holds the
// working tree as it is: every change, including untracked files that are not
// ignored. Git commands run with its env read and write it instead of the
// user's index, so a snapshot or diff of the working tree neither stages
// anything nor moves HEAD.
type worktreeIndex struct {
	env  []string
	path string
}

// newWorktreeIndex creates the private index of the workspace at dir from a
// copy of the user's index (which keeps git's view of tracked files and its
// stat cache) and adds the working tree to it. The caller removes it.
func newWorktreeIndex(ctx context.Context, dir string) (*worktreeIndex, error) {
	userIndex, err := runGit(ctx, dir, nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "codeforge-index-*")
	if err != nil {
		return nil, fmt.Errorf("create private index: %w", err)
	}
	idx := &worktreeIndex{path: f.Name()}
	idx.env = []string{"GIT_INDEX_FILE=" + idx.path}
	copyErr := copyIndex(filepath.Clean(strings.TrimSpace(userIndex)), f)
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		idx.remove()
		return nil, copyErr
	}
	if _, err := runGit(ctx, dir, idx.env, "add", "-A"); err != nil {
		idx.remove()
		return nil, err
	}
	return idx, nil
}

// copyIndex copies the user's index to dst; a repository without an index
// (nothing staged yet) starts from an empty one.
func copyIndex(src string, dst io.Writer) error {
	in, err := os.Open(src) //nolint:gosec // the path git reports for the workspace's index
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = in.Close() }()
	if _, err := io.Copy(dst, in); err != nil {
		return fmt.Errorf("copy index: %w", err)
	}
	return nil
}

func (i *worktreeIndex) remove() {
	_ = os.Remove(i.path)
}
