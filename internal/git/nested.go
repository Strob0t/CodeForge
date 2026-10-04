package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
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

// unsafeIndexEntry reports why an `ls-files -s` or `ls-tree` entry makes
// the repository unsafe, or "" if it does not. git treats every mode with
// the gitlink type bits as a gitlink (S_ISGITLINK: mode & 0170000 ==
// 0160000) and prints the mode stored in the index as is, so a hand-written
// index can hold 160755: the mode is parsed, not compared as text. A path
// with a .git component never belongs in an index (git refuses to add one)
// and would sit where the walk does not look.
func unsafeIndexEntry(meta, path string) string {
	modeField, _, _ := strings.Cut(meta, " ")
	mode, err := strconv.ParseUint(modeField, 8, 32)
	if err != nil {
		return fmt.Sprintf("unreadable entry mode %q at %s", modeField, path)
	}
	if mode&0o170000 == 0o160000 {
		return fmt.Sprintf("nested repository at %s is not supported (KI-77)", path)
	}
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(part, ".git") {
			return fmt.Sprintf("index path %s has a .git component (KI-77)", path)
		}
	}
	return ""
}

// refuseUnsafeEntries checks the entries of `ls-files -s -z` or
// `ls-tree -z` output ("<meta>\t<path>", NUL separated).
func refuseUnsafeEntries(out string) error {
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		meta, path, _ := strings.Cut(entry, "\t")
		if reason := unsafeIndexEntry(meta, path); reason != "" {
			return unsafeRepo(reason)
		}
	}
	return nil
}

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
		if err := refuseUnsafeEntries(tree); err != nil {
			return err
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
	return refuseUnsafeEntries(out)
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
