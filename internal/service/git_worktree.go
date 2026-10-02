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
	"github.com/Strob0t/CodeForge/internal/workspacefs"
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

// addWorktree updates the index to the working tree. `add -A` comes first:
// it drops the entries of tracked files missing from the working tree,
// which `add --renormalize` cannot stat (S3-F review C1); renormalizing
// then re-reads the tracked files that exist (it adds no untracked ones).
func (i *privateIndex) addWorktree(ctx context.Context, repo *git.Repo) error {
	// `add` runs git's submodule dirty check - a git process in the nested
	// repository, with its config - for every gitlink of the index. OpenRepo
	// refused nested repositories; a gitlink that got into the index since
	// (copied from the user's index, or added by `add -A` for a repository
	// created meanwhile) is refused before any further add (KI-77).
	if err := repo.RefuseGitlinks(ctx, i.env); err != nil {
		i.dropUnnormalized()
		return err
	}
	if _, err := repo.Run(ctx, i.env, "add", "-A"); err != nil {
		i.dropUnnormalized()
		return err
	}
	if err := repo.RefuseGitlinks(ctx, i.env); err != nil {
		i.dropUnnormalized()
		return err
	}
	if i.renormalize {
		if _, err := repo.Run(ctx, i.env, "add", "--renormalize", "--", "."); err != nil {
			i.dropUnnormalized()
			return err
		}
		i.renormalize = false
	}
	return nil
}

// dropUnnormalized removes an index still waiting for its renormalization:
// the decision is made when an index is seeded, so a kept file would skip
// it on the next call (S3-F review C2). The next call seeds it again.
func (i *privateIndex) dropUnnormalized() {
	if i.renormalize {
		_ = os.Remove(i.path)
	}
}

// maxAttributesSize caps an attributes file read for the filter check; a
// larger one counts as mentioning a filter (renormalizing is the safe side).
const maxAttributesSize = 1 << 20

// hasFilterAttributes reports whether an attributes file of the repository
// (a .gitattributes of the working tree, .git/info/attributes) mentions a
// filter. Symlinked attributes files are skipped, as git does.
func hasFilterAttributes(ctx context.Context, repo *git.Repo) bool {
	out, err := repo.Run(ctx, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard",
		"--", ":(glob)**/.gitattributes")
	if err != nil {
		return false
	}
	gitDir, err := workspacefs.Open(repo.GitDir)
	if err != nil {
		return false
	}
	defer func() { _ = gitDir.Close() }()
	if attributesMentionFilter(gitDir, "info/attributes") {
		return true
	}
	ws, err := workspacefs.Open(repo.Dir)
	if err != nil {
		return false
	}
	defer func() { _ = ws.Close() }()
	for _, name := range strings.Split(out, "\x00") {
		if name != "" && attributesMentionFilter(ws, name) {
			return true
		}
	}
	return false
}

// attributesMentionFilter reports whether the attributes file name mentions
// a filter. It is read through workspacefs (KI-95): a symlink swapped in
// after the Lstat never leads out of the workspace and a FIFO never blocks.
func attributesMentionFilter(root *workspacefs.Root, name string) bool {
	if info, err := root.Lstat(name); err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, _, err := root.ReadFile(name, maxAttributesSize)
	if errors.Is(err, workspacefs.ErrTooLarge) {
		return true
	}
	return err == nil && strings.Contains(string(data), "filter")
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
	// git.OpenRepo checked that the index is a regular file; it is opened
	// through workspacefs, so a swap since then (a symlink, a FIFO) is never
	// followed or waited on (KI-95).
	gitDir, err := workspacefs.Open(repo.GitDir)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = gitDir.Close() }()
	in, _, err := gitDir.OpenFile("index")
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
	if copyErr == nil {
		copyErr = keepIndexTime(in, dst)
	}
	if copyErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copy index: %w", copyErr)
	}
	return nil
}

// keepIndexTime gives the copy the source index's modification time. git
// re-reads the content of an entry whose file is not older than the index
// ("racily clean"): a file written in the same second as the index has
// unchanged stat data even when its content changed. A copy with a newer
// time would make git trust such entries, so a same-size change made right
// after a checkpoint or commit was missed (TestDeliver_Patch flake).
func keepIndexTime(src *os.File, dst string) error {
	info, err := src.Stat()
	if err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
