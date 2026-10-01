package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/proctemp"
)

// runCommit is the commit of a run's change, built on top of HEAD without
// touching HEAD, the user's index or the working tree (S3 follow-up 1a).
type runCommit struct {
	commit  string   // the new commit
	parent  string   // the commit HEAD pointed to, "" on an unborn branch
	headRef string   // the branch HEAD points to, "" when detached
	changed []string // paths the commit changes relative to its parent
	// preIndex is the user's index before the run as a tree ("" when it
	// had unmerged entries): syncIndex keeps the changes staged there.
	preIndex string
}

// buildRunCommit builds the commit of the run's change: a three-way merge of
// the run's base checkpoint (the working tree before the run), HEAD's tree
// and the working tree now. The result is HEAD plus exactly what the run
// changed; the user's uncommitted pre-run work (in the base and the working
// tree alike) is not part of it. A run change that overlaps pre-run work in
// the same lines cannot be separated from it and fails, naming the files.
func buildRunCommit(ctx context.Context, repo *git.Repo, runID, message string) (*runCommit, error) {
	if err := checkRunID(runID); err != nil {
		return nil, err
	}
	tip, _, err := readCheckpointRef(ctx, repo, runID)
	if err != nil {
		return nil, err
	}
	if tip == "" {
		return nil, fmt.Errorf("run %s: %w: its change is unknown", runID, ErrNoCheckpoints)
	}
	base, err := resolveBase(ctx, repo, tip)
	if err != nil {
		return nil, err
	}
	headRef, head, err := headState(ctx, repo)
	if err != nil {
		return nil, err
	}
	ours, oursTree, err := headCommitOrEmpty(ctx, repo, head)
	if err != nil {
		return nil, err
	}
	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("working tree index: %w", err)
	}
	final, err := idx.writeTree(ctx, repo)
	idx.remove()
	if err != nil {
		return nil, fmt.Errorf("working tree: %w", err)
	}
	if err := refuseFilteredChanges(ctx, repo, base.commit, final); err != nil {
		return nil, err
	}
	theirs, err := scratchCommit(ctx, repo, final)
	if err != nil {
		return nil, err
	}

	tree, err := mergeRunChange(ctx, repo, base.commit, ours, theirs)
	if err != nil {
		return nil, err
	}
	if tree == oursTree {
		return nil, errors.New("nothing to commit: the run did not change the workspace")
	}

	args := []string{"commit-tree", "--no-gpg-sign", "-m", message}
	if head != "" {
		args = append(args, "-p", head)
	}
	var identity []string
	if !repo.HasConfig("user.name") || !repo.HasConfig("user.email") {
		identity = deliveryIdentity
	}
	out, err := repo.Run(ctx, identity, append(args, tree)...)
	if err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	changed, err := repo.Run(ctx, nil, "diff-tree", "-r", "-z", "--name-only", "--no-renames", oursTree, tree)
	if err != nil {
		return nil, fmt.Errorf("changed paths: %w", err)
	}
	return &runCommit{
		commit:   trimLine(out),
		parent:   head,
		headRef:  headRef,
		changed:  strings.FieldsFunc(changed, func(r rune) bool { return r == 0 }),
		preIndex: base.indexTree,
	}, nil
}

// refuseFilteredChanges refuses a commit of run changes to files with a
// filter attribute (security review P3). The Go Core runs no filter
// programs: it would commit such a file as its working tree content - an
// LFS object as a plain blob, the plaintext of a git-crypt file - instead of
// the form the repository stores. Patch delivery holds the content change,
// which the user's git cleans again when the patch is applied and added.
func refuseFilteredChanges(ctx context.Context, repo *git.Repo, base, final string) error {
	out, err := repo.Run(ctx, nil, "diff-tree", "-r", "-z", "--name-only", "--no-renames", base, final)
	if err != nil {
		return fmt.Errorf("changed paths: %w", err)
	}
	changed := strings.FieldsFunc(out, func(r rune) bool { return r == 0 })
	filtered, err := filteredPaths(ctx, repo, changed)
	if err != nil {
		return err
	}
	if len(filtered) > 0 {
		return fmt.Errorf("the run changed files that git passes through a filter (git-lfs, git-crypt, ...): %s; "+
			"CodeForge runs no filter programs and cannot commit them in their stored form - use patch delivery or commit them yourself",
			strings.Join(filtered, ", "))
	}
	return nil
}

// headCommitOrEmpty returns the commit to merge the run's change onto and
// its tree: HEAD's commit, or on an unborn branch a scratch commit of the
// empty tree (git merge-tree takes commits).
func headCommitOrEmpty(ctx context.Context, repo *git.Repo, head string) (commit, tree string, err error) {
	if head == "" {
		out, err := repo.Run(ctx, nil, "mktree") // no input: the empty tree
		if err != nil {
			return "", "", fmt.Errorf("empty tree: %w", err)
		}
		tree = trimLine(out)
		commit, err = scratchCommit(ctx, repo, tree)
		return commit, tree, err
	}
	out, err := repo.Run(ctx, nil, "rev-parse", "--verify", "-q", head+"^{tree}")
	if err != nil {
		return "", "", fmt.Errorf("tree of HEAD: %w", err)
	}
	return head, trimLine(out), nil
}

// scratchCommit wraps tree in a commit that no ref keeps; git's gc removes it.
func scratchCommit(ctx context.Context, repo *git.Repo, tree string) (string, error) {
	out, err := repo.Run(ctx, checkpointIdentity, "commit-tree", "--no-gpg-sign", "-m", "codeforge-scratch", tree)
	if err != nil {
		return "", fmt.Errorf("scratch commit: %w", err)
	}
	return trimLine(out), nil
}

// mergeRunChange merges the changes base..theirs into ours and returns the
// resulting tree; overlapping changes fail with the files named. git
// merge-tree exits 1 on conflicts (its output starts with the tree) but also
// on some errors (no output), so a conflict is recognized by its output.
func mergeRunChange(ctx context.Context, repo *git.Repo, base, ours, theirs string) (string, error) {
	out, err := repo.Run(ctx, nil, "merge-tree", "--write-tree", "--no-messages", "--name-only", "-z",
		"--merge-base="+base, ours, theirs)
	fields := strings.Split(out, "\x00")
	if code, ran := exitCode(err); ran && code == 1 && objectIDPattern.MatchString(fields[0]) {
		var files []string
		for _, f := range fields[1:] {
			if f != "" && !slices.Contains(files, f) {
				files = append(files, f)
			}
		}
		return "", fmt.Errorf("the run's change overlaps uncommitted changes made before the run in %s; commit or stash them and deliver again",
			strings.Join(files, ", "))
	}
	if err != nil {
		return "", fmt.Errorf("merge the run's change onto HEAD: %w", err)
	}
	if !objectIDPattern.MatchString(fields[0]) {
		return "", fmt.Errorf("merge the run's change onto HEAD: unexpected output %q", out)
	}
	return fields[0], nil
}

const deliveryReflog = "codeforge delivery"

// advanceHead moves the checked-out branch (or a detached HEAD) to the
// commit, compare-and-swap on the commit it was built on.
func (rc *runCommit) advanceHead(ctx context.Context, repo *git.Repo) error {
	var err error
	if rc.headRef == "" {
		_, err = repo.Run(ctx, nil, "update-ref", "--no-deref", "-m", deliveryReflog, "HEAD", rc.commit, rc.parent)
	} else {
		_, err = repo.Run(ctx, nil, "update-ref", "-m", deliveryReflog, rc.headRef, rc.commit, rc.parent)
	}
	if err != nil {
		return fmt.Errorf("move HEAD to the delivered commit: %w", err)
	}
	return nil
}

// checkoutNewBranch creates branch at the commit (it must not exist) and
// points HEAD at it; the working tree already holds the commit's change.
func (rc *runCommit) checkoutNewBranch(ctx context.Context, repo *git.Repo, branch string) error {
	if _, err := repo.Run(ctx, nil, "check-ref-format", branch); err != nil {
		return fmt.Errorf("branch %s: %w", branch, err)
	}
	if _, err := repo.Run(ctx, nil, "update-ref", "-m", deliveryReflog, branch, rc.commit, ""); err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	if _, err := repo.Run(ctx, nil, "symbolic-ref", "-m", deliveryReflog, "HEAD", branch); err != nil {
		return fmt.Errorf("check out branch %s: %w", branch, err)
	}
	return nil
}

// syncIndex sets the user's index entries of the paths the commit changed to
// the commit's content, so the delivered change is not shown as staged or
// unstaged; the user's other entries, staged ones included, are kept. A
// path the user had staged a change of before the run keeps that change
// staged on top of the commit's content (S3-F review C6): a three-way merge
// of the staged blob, the parent's and the commit's. When they do not merge,
// the entry takes the commit's content and a warning names the path. The
// commit is delivered either way: a failure (a concurrent git holding the
// index lock) is logged.
func (rc *runCommit) syncIndex(ctx context.Context, repo *git.Repo) {
	if len(rc.changed) == 0 {
		return
	}
	reset := rc.changed
	var kept []indexEntry
	if rc.preIndex != "" {
		reset = nil
		for _, path := range rc.changed {
			entry, ok := rc.keepStaged(ctx, repo, path)
			if ok {
				kept = append(kept, entry)
			} else {
				reset = append(reset, path)
			}
		}
	}
	if len(reset) > 0 {
		if err := resetPaths(ctx, repo, rc.commit, reset); err != nil {
			slog.Warn("delivery: index not updated for the delivered paths; git status shows them as changed",
				"workspace", repo.Dir, "commit", rc.commit, "error", err)
		}
	}
	for _, e := range kept {
		if _, err := repo.Run(ctx, nil, "update-index", "--cacheinfo", e.mode+","+e.oid+","+e.path); err != nil {
			slog.Warn("delivery: the user's staged change could not be kept in the index",
				"workspace", repo.Dir, "path", e.path, "error", err)
		}
	}
}

// indexEntry is a path's mode and blob in a tree or index.
type indexEntry struct{ mode, oid, path string }

// keepStaged returns the index entry that keeps the user's pre-run staged
// change of path on top of the commit's content, or false when the user had
// staged nothing there (or the change does not merge: logged).
func (rc *runCommit) keepStaged(ctx context.Context, repo *git.Repo, path string) (indexEntry, bool) {
	staged, ok := treeEntry(ctx, repo, rc.preIndex, path)
	if !ok {
		return indexEntry{}, false
	}
	parent, inParent := indexEntry{}, false
	if rc.parent != "" {
		parent, inParent = treeEntry(ctx, repo, rc.parent, path)
	}
	if inParent && parent.oid == staged.oid && parent.mode == staged.mode {
		return indexEntry{}, false // nothing staged
	}
	unmerged := func(reason string) (indexEntry, bool) {
		slog.Warn("delivery: the user's staged change does not merge with the delivered content; the index takes the delivered content",
			"workspace", repo.Dir, "path", path, "reason", reason)
		return indexEntry{}, false
	}
	delivered, ok := treeEntry(ctx, repo, rc.commit, path)
	if !ok {
		return unmerged("the run deleted the file")
	}
	if !isRegularFileMode(staged.mode) || !isRegularFileMode(delivered.mode) || (inParent && !isRegularFileMode(parent.mode)) {
		return unmerged("not a regular file")
	}
	baseOID := parent.oid
	if !inParent {
		out, err := repo.Run(ctx, nil, "hash-object", "-w", "--no-filters", os.DevNull)
		if err != nil {
			return unmerged("empty blob: " + err.Error())
		}
		baseOID = trimLine(out)
	}
	out, err := repo.Run(ctx, nil, "merge-file", "--object-id", staged.oid, baseOID, delivered.oid)
	if err != nil {
		return unmerged(err.Error())
	}
	return indexEntry{mode: staged.mode, oid: trimLine(out), path: path}, true
}

func isRegularFileMode(mode string) bool { return mode == "100644" || mode == "100755" }

// treeEntry returns path's entry in the tree (or commit) tree-ish.
func treeEntry(ctx context.Context, repo *git.Repo, treeish, path string) (indexEntry, bool) {
	out, err := repo.Run(ctx, []string{"GIT_LITERAL_PATHSPECS=1"}, "ls-tree", "-z", "--full-tree", treeish, "--", path)
	if err != nil {
		return indexEntry{}, false
	}
	meta, name, found := strings.Cut(strings.TrimSuffix(out, "\x00"), "\t")
	fields := strings.Fields(meta)
	if !found || name != path || len(fields) != 3 || fields[1] != "blob" {
		return indexEntry{}, false
	}
	return indexEntry{mode: fields[0], oid: fields[2], path: path}, true
}

func resetPaths(ctx context.Context, repo *git.Repo, commit string, paths []string) error {
	dir, err := proctemp.MkdirTemp("paths-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	list := filepath.Join(dir, "paths")
	if err := os.WriteFile(list, []byte(strings.Join(paths, "\x00")+"\x00"), 0o600); err != nil {
		return err
	}
	_, err = repo.Run(ctx, []string{"GIT_LITERAL_PATHSPECS=1"}, "reset", "-q", commit,
		"--pathspec-from-file="+list, "--pathspec-file-nul")
	return err
}
