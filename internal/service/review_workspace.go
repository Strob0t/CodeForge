package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/git"
)

// The review pipeline measures its refactoring step against a baseline: the
// workspace as it was when the refactorer step started (S6-F 2), so edits the
// user made while the reports were written are not part of it. The baseline
// is a base commit like a run's base checkpoint (checkpoint_chain.go): its
// tree is the working tree, its parent the commit checked out, and its
// trailers record where HEAD pointed and what the user's index held. When the
// refactoring is measured, the workspace is recorded the same way as the
// result. The Go DB keeps both commits (review.Pipeline); the refs below only
// keep them from git's garbage collection until the refactoring is decided.
// Recording them touches neither HEAD, the user's index nor a branch.

// reviewBaselineRef keeps a review plan's baseline. Like the checkpoint refs
// it is outside refs/heads and refs/tags.
func reviewBaselineRef(planID string) string {
	return "refs/codeforge/review/" + planID
}

// reviewResultRef keeps a review plan's measured result.
func reviewResultRef(planID string) string {
	return "refs/codeforge/review-result/" + planID
}

// checkPlanID limits plan IDs to what is safe in a ref name.
func checkPlanID(planID string) error {
	if !runIDPattern.MatchString(planID) {
		return fmt.Errorf("invalid plan id %q for a review baseline", planID)
	}
	return nil
}

// snapshotWorkspace records the workspace (working tree with untracked files
// that are not ignored, HEAD, the user's index) as a base commit under ref
// and returns the commit; a ref left from an earlier attempt is replaced. The
// caller stores the commit in the Go DB, which is what counts.
func snapshotWorkspace(ctx context.Context, repo *git.Repo, planID, ref, subject string) (string, error) {
	if err := checkPlanID(planID); err != nil {
		return "", err
	}
	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("workspace tree: %w", err)
	}
	defer idx.remove()
	tree, err := idx.writeTree(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("workspace tree: %w", err)
	}
	commit, err := writeBaseCommit(ctx, repo, subject+": "+planID, tree)
	if err != nil {
		return "", err
	}
	if _, err := repo.Run(ctx, nil, "update-ref", "-m", "codeforge review", ref, commit); err != nil {
		return "", fmt.Errorf("review ref: %w", err)
	}
	return commit, nil
}

// errBaselineTampered: the workspace's baseline does not match the recorded
// one (the ref was deleted or moved, or the commit is gone).
var errBaselineTampered = errors.New("the workspace baseline does not match the recorded one")

// checkBaseline verifies the workspace against the recorded baseline commit:
// the ref must still point to it and the commit must exist. The ref is
// agent-writable, so a mismatch means the change cannot be measured.
func checkBaseline(ctx context.Context, repo *git.Repo, planID, recorded string) error {
	if !objectIDPattern.MatchString(recorded) {
		return fmt.Errorf("%w: no baseline commit recorded", errBaselineTampered)
	}
	current, err := readBaseline(ctx, repo, planID)
	if err != nil {
		return err
	}
	switch current {
	case "":
		return fmt.Errorf("%w: the baseline ref was deleted", errBaselineTampered)
	case recorded:
	default:
		return fmt.Errorf("%w: the baseline ref was moved to %s", errBaselineTampered, current)
	}
	return requireCommit(ctx, repo, recorded)
}

// requireCommit fails unless the recorded commit exists in the repository.
func requireCommit(ctx context.Context, repo *git.Repo, commit string) error {
	if !objectIDPattern.MatchString(commit) {
		return fmt.Errorf("%w: no commit recorded", errBaselineTampered)
	}
	if _, err := repo.Run(ctx, nil, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return fmt.Errorf("%w: the recorded commit %s is gone", errBaselineTampered, commit)
	}
	return nil
}

// readBaseline returns the plan's baseline commit, "" when it has none.
func readBaseline(ctx context.Context, repo *git.Repo, planID string) (string, error) {
	if checkPlanID(planID) != nil {
		return "", nil // no baseline is recorded under an invalid plan ID
	}
	ref := reviewBaselineRef(planID)
	out, err := repo.Run(ctx, nil, "for-each-ref", "--format=%(refname) %(objectname)", ref)
	if err != nil {
		return "", fmt.Errorf("read baseline ref: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == ref {
			return fields[1], nil
		}
	}
	return "", nil
}

// deleteReviewRefs removes the plan's baseline and result refs; missing
// refs are not an error.
func deleteReviewRefs(ctx context.Context, repo *git.Repo, planID string) error {
	if checkPlanID(planID) != nil {
		return nil
	}
	for _, ref := range []string{reviewBaselineRef(planID), reviewResultRef(planID)} {
		if _, err := repo.Run(ctx, nil, "update-ref", "-d", ref); err != nil {
			return err
		}
	}
	return nil
}

// workspaceChange is the change of a working tree since a baseline.
type workspaceChange struct {
	Stats DiffStats
	Paths []string // changed paths; both sides of a rename
}

// changeBetween measures the change from one recorded workspace to another.
// Added, deleted, renamed or copied files make the change structural; binary
// files count as changed files without lines. The output is read
// NUL-separated (-z): otherwise git quotes paths with non-ASCII bytes,
// quotes or control characters, and they would match no boundary (KI-94).
func changeBetween(ctx context.Context, repo *git.Repo, from, to string) (*workspaceChange, error) {
	numstat, err := repo.Run(ctx, nil, "diff", "--numstat", "-z", "-M", from, to)
	if err != nil {
		return nil, err
	}
	nameStatus, err := repo.Run(ctx, nil, "diff", "--name-status", "-z", "-M", from, to)
	if err != nil {
		return nil, err
	}

	change := &workspaceChange{}
	// "added\tremoved\tpath\0", or for a rename or copy
	// "added\tremoved\t\0old\0new\0".
	records := strings.Split(numstat, "\x00")
	for i := 0; i < len(records); i++ {
		fields := strings.SplitN(records[i], "\t", 3)
		if len(fields) < 3 {
			continue
		}
		if fields[2] == "" {
			i += 2 // the old and the new path follow
		}
		change.Stats.FilesChanged++
		change.Stats.LinesAdded += atoiOrZero(fields[0]) // "-" for a binary file
		change.Stats.LinesRemoved += atoiOrZero(fields[1])
	}
	// "status\0path\0", or for a rename or copy "R100\0old\0new\0".
	tokens := strings.Split(nameStatus, "\x00")
	for i := 0; i+1 < len(tokens); {
		status := tokens[i]
		if status == "" {
			break
		}
		paths := tokens[i+1 : i+2]
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			paths = tokens[i+1 : min(i+3, len(tokens))]
		}
		i += 1 + len(paths)
		if !strings.HasPrefix(status, "M") && !strings.HasPrefix(status, "T") {
			change.Stats.Structural = true // A, D, R, C: a file was added, deleted, renamed or copied
		}
		for _, p := range paths {
			if p != "" && !slices.Contains(change.Paths, p) {
				change.Paths = append(change.Paths, p)
			}
		}
	}
	return change, nil
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// undoOutcome is what undoing a refactoring did.
type undoOutcome struct {
	Restored     []string // paths set back to their baseline content
	HeadRestored bool     // HEAD moved back to the commit checked out at the baseline
	HeadNote     string   // why HEAD was left where it is; "" when it did not need to move or moved back
}

// undoRefactoring reverts what the refactoring changed (baseline -> result)
// in the workspace as it is now, path-scoped (S6-F 2): a three-way merge of
// the working tree with the baseline, based on the result, so changes the
// user made since (in other paths, or other lines) stay. Changes that overlap
// the refactoring in the same lines fail, naming the files, and nothing
// changes. The index entries of the restored paths are set to what the
// user's index held at the baseline. HEAD is moved back only with a
// compare-and-swap, when it still points where the refactoring left it
// (restoreRefactoredHead); otherwise it stays and HeadNote says so.
func undoRefactoring(ctx context.Context, repo *git.Repo, baseline, result string) (*undoOutcome, error) {
	if err := requireCommit(ctx, repo, baseline); err != nil {
		return nil, err
	}
	if err := requireCommit(ctx, repo, result); err != nil {
		return nil, err
	}
	base, err := resolveBase(ctx, repo, baseline)
	if err != nil {
		return nil, err
	}
	res, err := resolveBase(ctx, repo, result)
	if err != nil {
		return nil, err
	}

	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("working tree index: %w", err)
	}
	current, err := idx.writeTree(ctx, repo)
	idx.remove()
	if err != nil {
		return nil, fmt.Errorf("working tree: %w", err)
	}
	currentCommit, err := scratchCommit(ctx, repo, current)
	if err != nil {
		return nil, err
	}
	tree, conflicts, err := mergeTrees(ctx, repo, result, currentCommit, baseline)
	if err != nil {
		return nil, fmt.Errorf("revert the refactoring: %w", err)
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%w: the refactoring overlaps changes made after it in %s; undo it by hand",
			domain.ErrValidation, strings.Join(conflicts, ", "))
	}
	changed, err := repo.Run(ctx, nil, "diff-tree", "-r", "-z", "--name-only", "--no-renames", current, tree)
	if err != nil {
		return nil, fmt.Errorf("restored paths: %w", err)
	}
	out := &undoOutcome{Restored: strings.FieldsFunc(changed, func(r rune) bool { return r == 0 })}

	if len(out.Restored) > 0 {
		pidx, err := newPrivateIndex(repo)
		if err != nil {
			return nil, err
		}
		err = restoreWorktree(ctx, repo, pidx, tree)
		pidx.remove()
		if err != nil {
			return nil, fmt.Errorf("restore working tree: %w", err)
		}
	}
	if out.HeadRestored, out.HeadNote, err = restoreRefactoredHead(ctx, repo, base, res); err != nil {
		return nil, err
	}
	if source := orValue(base.indexTree, base.headCommit); len(out.Restored) > 0 && source != "" {
		if err := resetPaths(ctx, repo, source, out.Restored); err != nil {
			slog.Warn("review undo: index entries of the restored paths not reset; git status shows them as staged",
				"workspace", repo.Dir, "error", err)
		}
	}
	return out, nil
}

// restoreRefactoredHead moves HEAD back to where it pointed at the baseline
// if the refactoring moved it (it committed) and HEAD still points where the
// refactoring left it: a compare-and-swap on the branch (or a detached
// HEAD). Otherwise HEAD stays and the note says why.
func restoreRefactoredHead(ctx context.Context, repo *git.Repo, base, res *checkpointBase) (moved bool, note string, err error) {
	if base.headRef == res.headRef && base.headCommit == res.headCommit {
		return false, "", nil
	}
	curRef, curCommit, err := headState(ctx, repo)
	if err != nil {
		return false, "", err
	}
	leftAt := "HEAD was left at " + describeHead(curRef, curCommit) + ": "
	switch {
	case curRef != res.headRef || curCommit != res.headCommit:
		return false, leftAt + "it no longer points where the refactoring left it (" + describeHead(res.headRef, res.headCommit) +
			"); only the files were restored", nil
	case base.headRef != res.headRef:
		return false, leftAt + "the refactoring switched from " + describeHead(base.headRef, base.headCommit) +
			"; only the files were restored", nil
	}
	const reason = "codeforge review undo"
	switch {
	case res.headRef == "":
		_, err = repo.Run(ctx, nil, "update-ref", "--no-deref", "-m", reason, "HEAD", base.headCommit, res.headCommit)
	case base.headCommit == "":
		_, err = repo.Run(ctx, nil, "update-ref", "-m", reason, "-d", res.headRef, res.headCommit)
	default:
		_, err = repo.Run(ctx, nil, "update-ref", "-m", reason, res.headRef, base.headCommit, res.headCommit)
	}
	if err != nil { // the compare-and-swap lost: HEAD moved meanwhile
		slog.Info("review undo: HEAD not moved back", "workspace", repo.Dir, "error", err)
		return false, leftAt + "it moved while the refactoring was undone; only the files were restored", nil
	}
	return true, "", nil
}

func describeHead(ref, commit string) string {
	short := commit
	if len(short) > 12 {
		short = short[:12]
	}
	switch {
	case commit == "":
		return "unborn " + strings.TrimPrefix(ref, "refs/heads/")
	case ref == "":
		return "detached " + short
	}
	return strings.TrimPrefix(ref, "refs/heads/") + " at " + short
}
