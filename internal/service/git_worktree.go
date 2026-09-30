package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/git"
)

// worktreeIndex is a private index of a workspace repository that holds the
// working tree as it is: every change, including untracked files that are not
// ignored. Git commands run with its env read and write it instead of the
// user's index, so a snapshot or diff of the working tree neither stages
// anything nor moves HEAD.
type worktreeIndex struct {
	env  []string
	path string
}

// newWorktreeIndex creates the private index of repo from a copy of the
// user's index (which keeps git's view of tracked files and its stat cache)
// and adds the working tree to it. The caller removes it.
func newWorktreeIndex(ctx context.Context, repo *git.Repo) (*worktreeIndex, error) {
	f, err := os.CreateTemp("", "codeforge-index-*")
	if err != nil {
		return nil, fmt.Errorf("create private index: %w", err)
	}
	idx := &worktreeIndex{path: f.Name()}
	idx.env = []string{"GIT_INDEX_FILE=" + idx.path}
	copyErr := copyIndex(filepath.Join(repo.GitDir, "index"), f)
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		idx.remove()
		return nil, copyErr
	}
	if _, err := repo.Run(ctx, idx.env, "add", "-A"); err != nil {
		idx.remove()
		return nil, err
	}
	return idx, nil
}

// copyIndex copies the user's index to dst; a repository without an index
// (nothing staged yet) starts from an empty one.
func copyIndex(src string, dst io.Writer) error {
	in, err := os.Open(src) //nolint:gosec // the workspace's index; git.OpenRepo checked it is a regular file
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
