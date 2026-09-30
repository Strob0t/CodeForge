package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Strob0t/CodeForge/internal/git"
)

// The checkpoint chain of a run lives in the workspace repository under
// checkpointRef and is the durable record of the run's starting state: it
// survives a restart of the Go Core and is shared by its replicas. Its first
// commit, the base, holds the working tree before the run's first change,
// has the commit checked out then as its parent and records in trailers
// where HEAD pointed and what the user's index held. Every later checkpoint
// names the base in a trailer, so the base is found without walking the
// chain.

const (
	trailerHeadRef    = "CodeForge-Head-Ref"
	trailerHeadCommit = "CodeForge-Head-Commit"
	trailerIndexTree  = "CodeForge-Index-Tree"
	trailerBase       = "CodeForge-Base"

	detachedHead = "detached"
	noValue      = "none"
)

// ErrNoCheckpoints: the workspace holds no checkpoint of the run; no
// checkpointed tool call of the run changed it.
var ErrNoCheckpoints = errors.New("no checkpoints")

// errBrokenCheckpoints: the run's checkpoint ref exists but does not lead to
// a base checkpoint (the ref or its commits were changed from outside).
var errBrokenCheckpoints = errors.New("checkpoint chain is broken")

// runIDPattern limits run IDs to what is safe in a ref name and a file name.
var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

var objectIDPattern = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

func checkRunID(runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("invalid run id %q for checkpoints", runID)
	}
	return nil
}

// checkpointRef is the ref that keeps a run's checkpoints reachable. It is
// outside refs/heads and refs/tags, so no branch, push or clone carries it.
func checkpointRef(runID string) string {
	return "refs/codeforge/checkpoints/" + runID
}

// checkpointIdentity is the author of checkpoint commits: the workspace may
// have no git identity configured, and checkpoints are not the user's commits.
var checkpointIdentity = []string{
	"GIT_AUTHOR_NAME=CodeForge Checkpoint",
	"GIT_AUTHOR_EMAIL=checkpoint@codeforge.invalid",
	"GIT_COMMITTER_NAME=CodeForge Checkpoint",
	"GIT_COMMITTER_EMAIL=checkpoint@codeforge.invalid",
}

// checkpointBase is the run's starting state as its base checkpoint records
// it.
type checkpointBase struct {
	commit     string // the base checkpoint; its tree is the working tree before the run
	headRef    string // the branch HEAD pointed to, "" when HEAD was detached
	headCommit string // the commit checked out, "" on an unborn branch
	indexTree  string // the user's index as a tree, "" when it had unmerged entries
}

func trimLine(s string) string { return strings.TrimSpace(s) }

// exitCode returns the exit code of a git process that ran and failed.
func exitCode(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

// readCheckpointRef returns the tip of the run's checkpoint chain and its
// tree; tip is "" when the run has no checkpoints.
func readCheckpointRef(ctx context.Context, repo *git.Repo, runID string) (tip, tree string, err error) {
	ref := checkpointRef(runID)
	out, err := repo.Run(ctx, nil, "for-each-ref", "--format=%(refname) %(objectname) %(tree)", ref)
	if err != nil {
		return "", "", fmt.Errorf("read checkpoint ref: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ref {
			if len(fields) == 3 {
				tree = fields[2]
			}
			return fields[1], tree, nil
		}
	}
	return "", "", nil
}

// headState returns where HEAD points: a branch ("" when detached) and the
// commit checked out ("" on an unborn branch).
func headState(ctx context.Context, repo *git.Repo) (branch, commit string, err error) {
	out, err := repo.Run(ctx, nil, "symbolic-ref", "-q", "HEAD")
	switch code, ran := exitCode(err); {
	case err == nil:
		branch = trimLine(out)
	case ran && code == 1: // detached HEAD
	default:
		return "", "", fmt.Errorf("read HEAD: %w", err)
	}
	out, err = repo.Run(ctx, nil, "rev-parse", "--verify", "-q", "HEAD^{commit}")
	switch code, ran := exitCode(err); {
	case err == nil:
		commit = trimLine(out)
	case ran && code == 1 && branch != "": // a branch without commits
	default:
		return "", "", fmt.Errorf("read HEAD commit: %w", err)
	}
	return branch, commit, nil
}

// userIndexTree writes the user's index as a tree, from a copy so the index
// itself is not rewritten. An index with unmerged entries (a conflict in
// progress) has no tree; it is reported as "".
func userIndexTree(ctx context.Context, repo *git.Repo) (string, error) {
	idx, err := newPrivateIndex(repo)
	if err != nil {
		return "", err
	}
	defer idx.remove()
	tree, err := idx.writeTree(ctx, repo)
	if err != nil {
		slog.Warn("checkpoint: the user's index has no tree (unmerged entries?); rollback resets it to the pre-run HEAD",
			"workspace", repo.Dir, "error", err)
		return "", nil
	}
	return tree, nil
}

// checkpointSubject is the first line of a checkpoint commit message.
func checkpointSubject(callID string) string {
	clean := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return ' '
		}
		return r
	}, callID)
	return "codeforge-checkpoint: " + clean
}

func orValue(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// createBaseCheckpoint records the run's starting state and the working
// tree as the run's first checkpoint. The ref is created only if it does not
// exist: a base another process wrote first is kept.
func createBaseCheckpoint(ctx context.Context, repo *git.Repo, runID, tree, callID string) (string, error) {
	headRef, headCommit, err := headState(ctx, repo)
	if err != nil {
		return "", err
	}
	indexTree, err := userIndexTree(ctx, repo)
	if err != nil {
		return "", err
	}
	msg := checkpointSubject(callID) + "\n\n" +
		trailerHeadRef + ": " + orValue(headRef, detachedHead) + "\n" +
		trailerHeadCommit + ": " + orValue(headCommit, noValue) + "\n" +
		trailerIndexTree + ": " + orValue(indexTree, noValue) + "\n"
	args := []string{"commit-tree", "--no-gpg-sign", "-m", msg}
	if headCommit != "" {
		args = append(args, "-p", headCommit)
	}
	out, err := repo.Run(ctx, checkpointIdentity, append(args, tree)...)
	if err != nil {
		return "", fmt.Errorf("checkpoint commit: %w", err)
	}
	hash := trimLine(out)
	if _, err := repo.Run(ctx, nil, "update-ref", "-m", "codeforge checkpoint", checkpointRef(runID), hash, ""); err != nil {
		return "", fmt.Errorf("checkpoint ref: %w", err)
	}
	return hash, nil
}

// appendCheckpoint adds tree to the chain whose tip is tip, unless the tip
// already holds it, and returns the new tip.
func appendCheckpoint(ctx context.Context, repo *git.Repo, runID, tip, tipTree, tree, callID string) (string, error) {
	if tree == tipTree {
		return tip, nil
	}
	base, err := resolveBase(ctx, repo, tip)
	if err != nil {
		return "", err
	}
	msg := checkpointSubject(callID) + "\n\n" + trailerBase + ": " + base.commit + "\n"
	out, err := repo.Run(ctx, checkpointIdentity, "commit-tree", "--no-gpg-sign", "-p", tip, "-m", msg, tree)
	if err != nil {
		return "", fmt.Errorf("checkpoint commit: %w", err)
	}
	hash := trimLine(out)
	// Compare-and-swap: a chain another process moved meanwhile is not lost.
	if _, err := repo.Run(ctx, nil, "update-ref", "-m", "codeforge checkpoint", checkpointRef(runID), hash, tip); err != nil {
		return "", fmt.Errorf("checkpoint ref: %w", err)
	}
	return hash, nil
}

// commitTrailers returns the "Key: value" lines of a commit's message body.
func commitTrailers(ctx context.Context, repo *git.Repo, commit string) (map[string]string, error) {
	out, err := repo.Run(ctx, nil, "cat-file", "commit", commit)
	if err != nil {
		return nil, err
	}
	_, msg, ok := strings.Cut(out, "\n\n")
	if !ok {
		return nil, fmt.Errorf("commit %s has no message", commit)
	}
	trailers := make(map[string]string)
	for _, line := range strings.Split(msg, "\n")[1:] {
		key, value, ok := strings.Cut(line, ": ")
		if ok {
			if _, seen := trailers[key]; !seen {
				trailers[key] = strings.TrimSpace(value)
			}
		}
	}
	return trailers, nil
}

// resolveBase finds and parses the base checkpoint of the chain ending at
// tip.
func resolveBase(ctx context.Context, repo *git.Repo, tip string) (*checkpointBase, error) {
	trailers, err := commitTrailers(ctx, repo, tip)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBrokenCheckpoints, err)
	}
	commit := tip
	if _, isBase := trailers[trailerHeadRef]; !isBase {
		commit = trailers[trailerBase]
		if !objectIDPattern.MatchString(commit) {
			return nil, fmt.Errorf("%w: %s names no base checkpoint", errBrokenCheckpoints, tip)
		}
		if trailers, err = commitTrailers(ctx, repo, commit); err != nil {
			return nil, fmt.Errorf("%w: %w", errBrokenCheckpoints, err)
		}
	}
	return parseBase(ctx, repo, commit, trailers)
}

func parseBase(ctx context.Context, repo *git.Repo, commit string, trailers map[string]string) (*checkpointBase, error) {
	broken := func(what string) error {
		return fmt.Errorf("%w: base checkpoint %s: %s", errBrokenCheckpoints, commit, what)
	}
	b := &checkpointBase{commit: commit}
	switch ref := trailers[trailerHeadRef]; {
	case ref == detachedHead:
	case strings.HasPrefix(ref, "refs/heads/"):
		if _, err := repo.Run(ctx, nil, "check-ref-format", ref); err != nil {
			return nil, broken("invalid branch " + ref)
		}
		b.headRef = ref
	default:
		return nil, broken("no HEAD ref")
	}
	switch c := trailers[trailerHeadCommit]; {
	case c == noValue:
	case objectIDPattern.MatchString(c):
		b.headCommit = c
	default:
		return nil, broken("no HEAD commit")
	}
	switch t := trailers[trailerIndexTree]; {
	case t == noValue:
	case objectIDPattern.MatchString(t):
		b.indexTree = t
	default:
		return nil, broken("no index tree")
	}
	if b.headRef == "" && b.headCommit == "" {
		return nil, broken("detached HEAD without a commit")
	}
	return b, nil
}

// restoreHead points HEAD where it pointed when the run began: the same
// branch at the same commit (an unborn branch again), or the same detached
// commit. The index and the working tree are not touched.
func restoreHead(ctx context.Context, repo *git.Repo, b *checkpointBase) error {
	const reason = "codeforge rollback"
	if b.headRef == "" {
		_, err := repo.Run(ctx, nil, "update-ref", "--no-deref", "-m", reason, "HEAD", b.headCommit)
		return err
	}
	var err error
	if b.headCommit == "" {
		_, err = repo.Run(ctx, nil, "update-ref", "-m", reason, "-d", b.headRef)
	} else {
		_, err = repo.Run(ctx, nil, "update-ref", "-m", reason, b.headRef, b.headCommit)
	}
	if err != nil {
		return err
	}
	_, err = repo.Run(ctx, nil, "symbolic-ref", "-m", reason, "HEAD", b.headRef)
	return err
}

// restoreUserIndex sets the user's index to what it held when the run began
// (stat data of unchanged entries is kept). An index that had unmerged
// entries is reset to the pre-run HEAD.
func restoreUserIndex(ctx context.Context, repo *git.Repo, b *checkpointBase) error {
	var err error
	switch {
	case b.indexTree != "":
		_, err = repo.Run(ctx, nil, "read-tree", "--reset", b.indexTree)
	case b.headCommit != "":
		_, err = repo.Run(ctx, nil, "read-tree", "--reset", b.headCommit)
	default:
		_, err = repo.Run(ctx, nil, "read-tree", "--empty")
	}
	return err
}

// restoreWorktree makes the working tree match the checkpoint's tree: idx,
// updated to the current working tree, is moved to the checkpoint with
// read-tree -u, which rewrites changed files and deletes the files the
// checkpoint does not have. Ignored files the index does not hold are left
// alone.
func restoreWorktree(ctx context.Context, repo *git.Repo, idx *privateIndex, checkpoint string) error {
	if err := idx.addWorktree(ctx, repo); err != nil {
		return err
	}
	_, err := repo.Run(ctx, idx.env, "read-tree", "-u", "--reset", checkpoint)
	return err
}
