package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/proctemp"
)

// privateIndex is an index file of a workspace repository outside the
// repository. Git commands run with its env read and write it instead of the
// user's index, so a snapshot or diff of the working tree neither stages
// anything nor moves HEAD.
type privateIndex struct {
	env  []string
	path string
	dir  string // removed with the index; "" when the owner keeps the directory
	// renormalize: the index was copied from the user's index in a
	// repository with filter attributes; its next addWorktree re-reads every
	// tracked file (see renormalizeIfFiltered).
	renormalize bool
}

// newPrivateIndex returns a private index of repo in a new temporary
// directory, seeded with a copy of the user's index.
func newPrivateIndex(repo *git.Repo) (*privateIndex, error) {
	dir, err := proctemp.MkdirTemp("index-*")
	if err != nil {
		return nil, fmt.Errorf("create private index: %w", err)
	}
	idx := indexAt(filepath.Join(dir, "index"))
	idx.dir = dir
	if err := seedIndex(repo, idx.path); err != nil {
		idx.remove()
		return nil, err
	}
	return idx, nil
}

func indexAt(path string) *privateIndex {
	return &privateIndex{path: path, env: []string{"GIT_INDEX_FILE=" + path}}
}

// newWorktreeIndex returns a private index holding the working tree as it
// is: every change, including untracked files that are not ignored. The
// caller removes it.
func newWorktreeIndex(ctx context.Context, repo *git.Repo) (*privateIndex, error) {
	idx, err := newPrivateIndex(repo)
	if err != nil {
		return nil, err
	}
	idx.renormalizeIfFiltered(ctx, repo)
	if err := idx.addWorktree(ctx, repo); err != nil {
		idx.remove()
		return nil, err
	}
	return idx, nil
}

// renormalizeIfFiltered marks an index copied from the user's index for
// renormalization when the repository has filter attributes (security
// review P3). The user's git stored filtered files (git-lfs, git-crypt) in
// their clean form - an LFS pointer, ciphertext - while the working tree
// holds their content. The Go Core runs no filter programs, so a copied
// entry of an unchanged filtered file would keep the clean form, and
// rollback would write it into the working tree. Re-reading the tracked
// files once makes every entry hold the working tree's content.
func (i *privateIndex) renormalizeIfFiltered(ctx context.Context, repo *git.Repo) {
	if hasFilterAttributes(ctx, repo) {
		i.renormalize = true
	}
}

// addWorktree updates the index to the working tree.
func (i *privateIndex) addWorktree(ctx context.Context, repo *git.Repo) error {
	if i.renormalize {
		if _, err := repo.Run(ctx, i.env, "add", "--renormalize", "--", "."); err != nil {
			return err
		}
		i.renormalize = false
	}
	_, err := repo.Run(ctx, i.env, "add", "-A")
	return err
}

// hasFilterAttributes reports whether an attributes file of the repository
// (a .gitattributes of the working tree, .git/info/attributes) mentions a
// filter. Symlinked attributes files are skipped, as git does.
func hasFilterAttributes(ctx context.Context, repo *git.Repo) bool {
	out, err := repo.Run(ctx, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard",
		"--", ":(glob)**/.gitattributes")
	if err != nil {
		return false
	}
	files := []string{filepath.Join(repo.GitDir, "info", "attributes")}
	for _, name := range strings.Split(out, "\x00") {
		if name != "" {
			files = append(files, filepath.Join(repo.Dir, filepath.FromSlash(name)))
		}
	}
	for _, f := range files {
		if info, err := os.Lstat(f); err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(f) //nolint:gosec // an attributes file of the workspace, read as data
		if err == nil && strings.Contains(string(data), "filter") {
			return true
		}
	}
	return false
}

// filteredPaths returns the paths git would pass through a filter driver.
func filteredPaths(ctx context.Context, repo *git.Repo, paths []string) ([]string, error) {
	const batch = 200
	var filtered []string
	for start := 0; start < len(paths); start += batch {
		end := min(start+batch, len(paths))
		out, err := repo.Run(ctx, nil, append([]string{"check-attr", "-z", "filter", "--"}, paths[start:end]...)...)
		if err != nil {
			return nil, fmt.Errorf("check attributes: %w", err)
		}
		fields := strings.Split(out, "\x00")
		for i := 0; i+2 < len(fields); i += 3 {
			if v := fields[i+2]; v != "unspecified" && v != "unset" {
				filtered = append(filtered, fields[i])
			}
		}
	}
	return filtered, nil
}

// writeTree writes the index as a tree object and returns it.
func (i *privateIndex) writeTree(ctx context.Context, repo *git.Repo) (string, error) {
	out, err := repo.Run(ctx, i.env, "write-tree")
	if err != nil {
		return "", err
	}
	return trimLine(out), nil
}

func (i *privateIndex) remove() {
	if i.dir != "" {
		_ = os.RemoveAll(i.dir)
		return
	}
	_ = os.Remove(i.path)
}

// seedIndex copies the user's index to dst, which must not exist; keeping
// git's view of tracked files and its stat cache makes `add -A` cheap. A
// repository without an index (nothing staged yet) leaves dst absent: git
// reads a missing index file as an empty index, but refuses an empty file.
func seedIndex(repo *git.Repo, dst string) error {
	in, err := os.Open(filepath.Join(repo.GitDir, "index")) //nolint:gosec // the workspace's index; git.OpenRepo checked it is a regular file
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // dst is in a directory the Go Core created
	if err != nil {
		return fmt.Errorf("create private index: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copy index: %w", copyErr)
	}
	return nil
}
