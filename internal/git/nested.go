package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Nested repositories (security review of the S3-F fix round). For a
// gitlink (mode 160000) in the index whose directory holds a repository,
// `git add` and `git status` run git's submodule dirty check: a child git
// in the nested repository that reads its own config and attributes
// (filter drivers, fsmonitor, ...), which OpenRepo never inspected. No git
// option suppresses that check for `add` (it overrides the submodule
// config). The Go Core therefore does not run git on a workspace whose
// HEAD or index holds a gitlink, or whose working tree holds a nested
// .git (directory or file) outside ignored directories.

// Bounds of the walk for nested .git entries; reaching one refuses the
// workspace (fail closed). Variables for tests.
var (
	maxNestedWalkEntries = 200_000
	maxNestedWalkDepth   = 64
)

const gitlinkMode = "160000"

func nestedRepo(path string) error {
	return unsafeRepo(fmt.Sprintf("nested repository at %s is not supported (KI-77)", path))
}

// checkNoNestedRepositories refuses a workspace with a gitlink in HEAD or
// the user's index, or a nested .git in its working tree. Nothing it runs
// enters a nested repository: ls-files and ls-tree read the index and the
// object database, the listing of ignored directories reads the working
// tree without starting other processes.
func (r *Repo) checkNoNestedRepositories(ctx context.Context) error {
	if err := r.RefuseGitlinks(ctx, nil); err != nil {
		return err
	}
	if out, err := r.Run(ctx, nil, "rev-parse", "--verify", "-q", "HEAD^{tree}"); err == nil && strings.TrimSpace(out) != "" {
		tree, err := r.Run(ctx, nil, "ls-tree", "-r", "-z", "--full-tree", strings.TrimSpace(out))
		if err != nil {
			return fmt.Errorf("list HEAD: %w", err)
		}
		for _, entry := range strings.Split(tree, "\x00") {
			meta, path, _ := strings.Cut(entry, "\t")
			if strings.HasPrefix(meta, gitlinkMode+" ") {
				return nestedRepo(path)
			}
		}
	}
	ignored, err := r.ignoredDirs(ctx)
	if err != nil {
		return err
	}
	return walkForNestedGit(r.Dir, ignored)
}

// RefuseGitlinks refuses an index that holds a gitlink: the user's index
// (indexEnv nil) or a private one (GIT_INDEX_FILE in indexEnv). Run it
// before every `git add` on an index: add runs the dirty check for its
// gitlinks.
func (r *Repo) RefuseGitlinks(ctx context.Context, indexEnv []string) error {
	out, err := r.Run(ctx, indexEnv, "ls-files", "-s", "-z")
	if err != nil {
		return fmt.Errorf("list index: %w", err)
	}
	for _, entry := range strings.Split(out, "\x00") {
		meta, path, _ := strings.Cut(entry, "\t")
		if strings.HasPrefix(meta, gitlinkMode+" ") {
			return nestedRepo(path)
		}
	}
	return nil
}

// ignoredDirs returns the ignored directories of the working tree (as git
// lists them, relative with a trailing slash): `git add` never enters them,
// so the walk skips them (node_modules and the like).
func (r *Repo) ignoredDirs(ctx context.Context) (map[string]bool, error) {
	out, err := r.Run(ctx, nil, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, fmt.Errorf("list ignored directories: %w", err)
	}
	dirs := make(map[string]bool)
	for _, p := range strings.Split(out, "\x00") {
		if strings.HasSuffix(p, "/") {
			dirs[p] = true
		}
	}
	return dirs, nil
}

var errWalkBound = errors.New("walk bound reached")

// walkForNestedGit walks the working tree below root, without following
// symbolic links and skipping the root's .git and the ignored directories,
// and refuses the first nested .git (directory, file or link).
func walkForNestedGit(root string, ignored map[string]bool) error {
	entries := 0
	var nested string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // unreadable: fail closed
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" {
			return filepath.SkipDir
		}
		if d.Name() == ".git" {
			nested = filepath.ToSlash(filepath.Dir(rel))
			return fs.SkipAll
		}
		entries++
		if entries > maxNestedWalkEntries || strings.Count(rel, "/") >= maxNestedWalkDepth {
			return errWalkBound
		}
		if d.IsDir() && ignored[rel+"/"] {
			return filepath.SkipDir
		}
		return nil
	})
	switch {
	case nested != "":
		return nestedRepo(nested)
	case errors.Is(err, errWalkBound):
		return unsafeRepo(fmt.Sprintf("the working tree has more than %d entries or deeper than %d directories outside ignored ones; "+
			"it cannot be checked for nested repositories (KI-77)", maxNestedWalkEntries, maxNestedWalkDepth))
	case err != nil:
		return fmt.Errorf("check the working tree for nested repositories: %w", err)
	}
	return nil
}
