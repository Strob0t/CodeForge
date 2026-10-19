package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	bp "github.com/Strob0t/CodeForge/internal/domain/branchprotection"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// Branch protection on delivery (KI-205). Delivery is where the Go Core puts
// an agent's work on a branch, so it is where a project's branch protection
// rules are evaluated. A rule protects the branches its pattern matches
// ("*" within one segment, "**" across segments); a branch no enabled rule
// matches is not protected:
//   - branch and PR delivery push the new branch codeforge/<run>: a push
//     (CheckBranch) of that branch, checked before anything is created and
//     refused only by a rule that matches it;
//   - commit-local delivery moves the checked-out branch with a commit nobody
//     reviewed: a merge without review into that branch (CheckMerge), whose
//     tests and lint passed only if the run's quality gate required them (a
//     run delivers only once its gate passed). Its refusal points to branch
//     and PR delivery. A detached HEAD moves no branch.
//
// Patch delivery changes no branch. Rollbacks and review undo move a branch
// back to where it was before the run, and the user's own checkout and pull
// take the remote's state: none of them is checked. The rules are read in
// the run's tenant; a failure to read them refuses the delivery.

// ErrBranchProtected is the error of a delivery the project's branch
// protection rules refuse.
var ErrBranchProtected = errors.New("refused by branch protection")

// gateProfiles looks up the policy profile a run ran under.
type gateProfiles interface {
	GetProfile(ctx context.Context, name string) (policy.PolicyProfile, bool)
}

// SetPolicyProfiles sets where the quality gate of a run's policy profile is
// looked up. Without it, a commit-local delivery never counts as tested or
// linted.
func (s *DeliverService) SetPolicyProfiles(p gateProfiles) {
	s.profiles = p
}

// deliveryBranch is the branch that branch and PR delivery create and push.
func deliveryBranch(shortID string) string {
	return "codeforge/" + shortID
}

// checkDeliveryPush refuses a branch or PR delivery whose push of the
// delivery branch the project's rules deny.
func (s *DeliverService) checkDeliveryPush(ctx context.Context, r *run.Run, shortID string) error {
	if r.DeliverMode != run.DeliverModeBranch && r.DeliverMode != run.DeliverModePR {
		return nil
	}
	branch := deliveryBranch(shortID)
	res, err := s.protection.CheckBranch(ctx, r.ProjectID, bp.PushAction{Branch: branch, HasChanges: true})
	if err != nil {
		return fmt.Errorf("branch protection rules: %w", err)
	}
	return protectionRefusal(res, "pushing branch "+branch)
}

// checkCommitLocal refuses a commit-local delivery that would move the
// branch headRef (the checked-out branch, "" for a detached HEAD) against
// the project's rules.
func (s *DeliverService) checkCommitLocal(ctx context.Context, r *run.Run, shortID, headRef string) error {
	branch, ok := strings.CutPrefix(headRef, "refs/heads/")
	if !ok {
		return nil
	}
	var gate policy.QualityGate
	if s.profiles != nil {
		if profile, found := s.profiles.GetProfile(ctx, r.PolicyProfile); found {
			gate = profile.QualityGate
		}
	}
	res, err := s.protection.CheckMerge(ctx, r.ProjectID, bp.MergeAction{
		TargetBranch: branch,
		TestsPassed:  gate.RequireTestsPass,
		LintPassed:   gate.RequireLintPass,
	})
	if err != nil {
		return fmt.Errorf("branch protection rules: %w", err)
	}
	if err := protectionRefusal(res, "committing to "+branch+" without review"); err != nil {
		return fmt.Errorf("%w; use deliver mode %q or %q instead, which pushes the branch %s for review",
			err, run.DeliverModeBranch, run.DeliverModePR, deliveryBranch(shortID))
	}
	return nil
}

// protectionRefusal is the error of a denied evaluation, nil when it is allowed.
func protectionRefusal(res *bp.EvalResult, action string) error {
	if res.Allowed {
		return nil
	}
	if res.Rule != "" {
		return fmt.Errorf("%w: %s: %s (rule %q)", ErrBranchProtected, action, res.Reason, res.Rule)
	}
	return fmt.Errorf("%w: %s: %s", ErrBranchProtected, action, res.Reason)
}
