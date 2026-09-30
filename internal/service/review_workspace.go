package service

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Strob0t/CodeForge/internal/git"
)

// The review pipeline measures its refactoring step against a baseline: the
// workspace as it was when the pipeline started (its first three steps only
// read). The baseline is a base commit like a run's base checkpoint
// (checkpoint_chain.go): its tree is the working tree, its parent the commit
// checked out, and its trailers record where HEAD pointed and what the user's
// index held. It is kept under reviewBaselineRef until the refactoring is
// decided; recording it touches neither HEAD, the user's index nor a branch.

// reviewBaselineRef is the ref that keeps a review plan's baseline. Like the
// checkpoint refs it is outside refs/heads and refs/tags.
func reviewBaselineRef(planID string) string {
	return "refs/codeforge/review/" + planID
}

// checkPlanID limits plan IDs to what is safe in a ref name.
func checkPlanID(planID string) error {
	if !runIDPattern.MatchString(planID) {
		return fmt.Errorf("invalid plan id %q for a review baseline", planID)
	}
	return nil
}

// snapshotWorkspace records the workspace (working tree with untracked files
// that are not ignored, HEAD, the user's index) as the plan's baseline. A
// baseline recorded before is kept.
func snapshotWorkspace(ctx context.Context, repo *git.Repo, planID string) error {
	if err := checkPlanID(planID); err != nil {
		return err
	}
	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return fmt.Errorf("baseline tree: %w", err)
	}
	defer idx.remove()
	tree, err := idx.writeTree(ctx, repo)
	if err != nil {
		return fmt.Errorf("baseline tree: %w", err)
	}
	commit, err := writeBaseCommit(ctx, repo, "codeforge-review-baseline: "+planID, tree)
	if err != nil {
		return err
	}
	if _, err := repo.Run(ctx, nil, "update-ref", "-m", "codeforge review baseline", reviewBaselineRef(planID), commit, ""); err != nil {
		return fmt.Errorf("baseline ref: %w", err)
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

// deleteBaseline removes the plan's baseline ref; a missing ref is not an
// error.
func deleteBaseline(ctx context.Context, repo *git.Repo, planID string) error {
	if checkPlanID(planID) != nil {
		return nil
	}
	_, err := repo.Run(ctx, nil, "update-ref", "-d", reviewBaselineRef(planID))
	return err
}

// restoreWorkspace makes the workspace what it was at the baseline, like
// CheckpointService.RewindToFirst: HEAD where it pointed, the working tree
// as recorded (files added since are removed, ignored files left alone) and
// the user's index.
func restoreWorkspace(ctx context.Context, repo *git.Repo, baseline string) error {
	base, err := resolveBase(ctx, repo, baseline)
	if err != nil {
		return err
	}
	idx, err := newPrivateIndex(repo)
	if err != nil {
		return err
	}
	defer idx.remove()
	if err := restoreHead(ctx, repo, base); err != nil {
		return fmt.Errorf("restore HEAD: %w", err)
	}
	if err := restoreWorktree(ctx, repo, idx, base.commit); err != nil {
		return fmt.Errorf("restore working tree: %w", err)
	}
	if err := restoreUserIndex(ctx, repo, base); err != nil {
		return fmt.Errorf("restore index: %w", err)
	}
	return nil
}

// workspaceChange is the change of a working tree since a baseline.
type workspaceChange struct {
	Stats DiffStats
	Paths []string // changed paths; both sides of a rename
}

// changeSince measures the working tree against the baseline commit. Added,
// deleted, renamed or copied files make the change structural; binary files
// count as changed files without lines.
func changeSince(ctx context.Context, repo *git.Repo, baseline string) (*workspaceChange, error) {
	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return nil, err
	}
	defer idx.remove()

	numstat, err := repo.Run(ctx, idx.env, "diff", "--cached", "--numstat", "-M", baseline)
	if err != nil {
		return nil, err
	}
	nameStatus, err := repo.Run(ctx, idx.env, "diff", "--cached", "--name-status", "-M", baseline)
	if err != nil {
		return nil, err
	}

	change := &workspaceChange{}
	for _, line := range strings.Split(strings.TrimSpace(numstat), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		change.Stats.FilesChanged++
		change.Stats.LinesAdded += atoiOrZero(fields[0]) // "-" for a binary file
		change.Stats.LinesRemoved += atoiOrZero(fields[1])
	}
	for _, line := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 || fields[0] == "" {
			continue
		}
		if !strings.HasPrefix(fields[0], "M") && !strings.HasPrefix(fields[0], "T") {
			change.Stats.Structural = true // A, D, R, C: a file was added, deleted, renamed or copied
		}
		for _, p := range fields[1:] {
			if !slices.Contains(change.Paths, p) {
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
