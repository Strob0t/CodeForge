package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// DeliveryResult holds the outcome of a delivery operation.
type DeliveryResult struct {
	Mode       run.DeliverMode `json:"mode"`
	PatchPath  string          `json:"patch_path,omitempty"`
	CommitHash string          `json:"commit_hash,omitempty"`
	BranchName string          `json:"branch_name,omitempty"`
	PRURL      string          `json:"pr_url,omitempty"`
	PushError  string          `json:"push_error,omitempty"` // P2-5: propagate push failure
	// PRError says why PR delivery opened no pull request (it then stays a
	// branch delivery).
	PRError string `json:"pr_error,omitempty"`
}

// DeliverService executes delivery strategies after a successful run. All
// git runs through git.OpenRepo's hardened repository (KI-77): the
// workspace is agent-writable, so its hooks, filters, drivers and transport
// settings must not run in the Go Core. Pull requests are opened through the
// provider's REST API (KI-117), never by a CLI in the workspace.
type DeliverService struct {
	store database.Store
	cfg   *config.Runtime
	pool  *git.Pool
	// githubToken is the operator's GitHub token (github.token); it opens
	// pull requests of github.com repositories in the default tenant only.
	githubToken string
	// pullRequests builds the git provider that opens a pull request.
	pullRequests func(name string, cfg map[string]string) (gitprovider.PullRequestCreator, error)
	// protection evaluates the project's branch protection rules (KI-205,
	// deliver_protection.go); profiles tells which quality gate a run passed.
	protection *BranchProtectionService
	profiles   gateProfiles
	// pushURL maps the project's repository URL to the URL branch delivery
	// pushes to (the URL itself; tests point it at a local server).
	pushURL func(repoURL string) string
}

// NewDeliverService creates a new DeliverService with a shared git pool.
func NewDeliverService(store database.Store, cfg *config.Runtime, pool *git.Pool) *DeliverService {
	return &DeliverService{store: store, cfg: cfg, pool: pool, pullRequests: pullRequestProvider,
		protection: NewBranchProtectionService(store),
		pushURL:    func(repoURL string) string { return repoURL }}
}

// SetOperatorGitHubToken sets the operator's GitHub token (github.token).
func (s *DeliverService) SetOperatorGitHubToken(token string) {
	s.githubToken = token
}

// pullRequestProvider builds the registered git provider name with cfg and
// returns it when it opens pull requests.
func pullRequestProvider(name string, cfg map[string]string) (gitprovider.PullRequestCreator, error) {
	provider, err := gitprovider.New(name, cfg)
	if err != nil {
		return nil, err
	}
	creator, ok := provider.(gitprovider.PullRequestCreator)
	if !ok {
		return nil, fmt.Errorf("git provider %q does not open pull requests", name)
	}
	return creator, nil
}

// deliveryIdentity is the author of delivery commits in a repository that
// configures none (the Go Core ignores global git config).
var deliveryIdentity = []string{
	"GIT_AUTHOR_NAME=CodeForge",
	"GIT_AUTHOR_EMAIL=codeforge@codeforge.invalid",
	"GIT_COMMITTER_NAME=CodeForge",
	"GIT_COMMITTER_EMAIL=codeforge@codeforge.invalid",
}

// Deliver executes the delivery strategy for the given run.
func (s *DeliverService) Deliver(ctx context.Context, r *run.Run, taskTitle string) (*DeliveryResult, error) {
	if r.DeliverMode == "" || r.DeliverMode == run.DeliverModeNone {
		return &DeliveryResult{Mode: run.DeliverModeNone}, nil
	}

	// Look up workspace path from project
	proj, err := s.store.GetProject(ctx, r.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("get project for delivery: %w", err)
	}
	dir := proj.WorkspacePath
	if dir == "" {
		return nil, fmt.Errorf("project %s has no workspace_path", r.ProjectID)
	}

	shortID := r.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if err := s.checkDeliveryPush(ctx, r, shortID); err != nil {
		return nil, fmt.Errorf("%s delivery: %w", r.DeliverMode, err)
	}

	switch r.DeliverMode {
	case run.DeliverModePatch:
		return s.deliverPatch(ctx, dir, r)
	case run.DeliverModeCommitLocal:
		return s.deliverCommitLocal(ctx, dir, r, shortID, taskTitle)
	case run.DeliverModeBranch:
		return s.deliverBranch(ctx, proj, r, shortID, taskTitle)
	case run.DeliverModePR:
		return s.deliverPR(ctx, proj, r, shortID, taskTitle)
	default:
		return nil, fmt.Errorf("unsupported deliver mode %q", r.DeliverMode)
	}
}

// deliverPatch writes the run's change as a patch: the working tree
// (including new files) against the run's base checkpoint, the working tree
// before its first change, so the user's earlier uncommitted work and files
// of earlier deliveries are not part of it. The diff is taken from a private
// index (the user's index is not touched) before the checkpoints are cleaned
// up, and the patch is written inside the repository's .git directory, where
// git does not see it as a change.
func (s *DeliverService) deliverPatch(ctx context.Context, dir string, r *run.Run) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return fmt.Errorf("patch delivery: %w", err)
		}
		diff, err := runChange(ctx, repo, r.ID)
		if err != nil {
			return fmt.Errorf("patch delivery: %w", err)
		}
		patchFile, err := writePatch(repo, r.ID, diff)
		if err != nil {
			return fmt.Errorf("write patch: %w", err)
		}

		slog.Info("patch delivered", "run_id", r.ID, "path", patchFile)
		result = &DeliveryResult{
			Mode:      run.DeliverModePatch,
			PatchPath: patchFile,
		}
		return nil
	})
	return result, err
}

// runChange returns the run's change as a binary diff: its base checkpoint
// against the working tree now.
func runChange(ctx context.Context, repo *git.Repo, runID string) (string, error) {
	if err := checkRunID(runID); err != nil {
		return "", err
	}
	tip, _, err := readCheckpointRef(ctx, repo, runID)
	if err != nil {
		return "", err
	}
	if tip == "" {
		return "", fmt.Errorf("run %s: %w: its change is unknown", runID, ErrNoCheckpoints)
	}
	base, err := resolveBase(ctx, repo, tip)
	if err != nil {
		return "", err
	}
	idx, err := newWorktreeIndex(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("patch index: %w", err)
	}
	defer idx.remove()
	args := append([]string{"diff", "--cached", "--binary"}, git.DiffFormatArgs...)
	diff, err := repo.Run(ctx, idx.env, append(args, base.commit, "--")...)
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return diff, nil
}

// patchDir is where patches are written, relative to the .git directory.
const patchDir = "codeforge/patches"

// writePatch writes the patch to .git/codeforge/patches/<run>.patch and
// returns its path. The directory is agent-writable: all access goes through
// workspacefs on .git (os.Root, never blocking: a FIFO swapped in for .git
// after git.OpenRepo looked at it would otherwise hold a slot of the shared
// git pool), which refuses symlinks leading out of it; the directories must
// not be symlinks at all, and an existing file (or symlink) of that name is
// replaced, never written through.
func writePatch(repo *git.Repo, runID, diff string) (string, error) {
	root, err := workspacefs.Open(repo.GitDir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	for _, d := range []string{"codeforge", patchDir} {
		if err := root.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		info, err := root.Lstat(d)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf(".git/%s is not a directory", d)
		}
	}
	name := patchDir + "/" + runID + ".patch"
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := root.CreateExclusive(name, []byte(diff), 0o600); err != nil {
		return "", err
	}
	return filepath.Join(repo.GitDir, filepath.FromSlash(name)), nil
}

func (s *DeliverService) deliverCommitLocal(ctx context.Context, dir string, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, dir)
		if err != nil {
			return fmt.Errorf("commit-local delivery: %w", err)
		}
		rc, err := buildRunCommit(ctx, repo, r.ID, s.commitMessage(shortID, taskTitle))
		if err != nil {
			return fmt.Errorf("commit-local delivery: %w", err)
		}
		if err := s.checkCommitLocal(ctx, r, shortID, rc.headRef); err != nil {
			return fmt.Errorf("commit-local delivery: %w", err)
		}
		if err := rc.advanceHead(ctx, repo); err != nil {
			return fmt.Errorf("commit-local delivery: %w", err)
		}
		rc.syncIndex(ctx, repo)
		slog.Info("commit-local delivered", "run_id", r.ID, "hash", rc.commit)
		result = &DeliveryResult{
			Mode:       run.DeliverModeCommitLocal,
			CommitHash: rc.commit,
		}
		return nil
	})
	return result, err
}

func (s *DeliverService) commitMessage(shortID, taskTitle string) string {
	return fmt.Sprintf("%s %s [run %s]", s.cfg.DeliveryCommitPrefix, taskTitle, shortID)
}

// deliverBranch commits the run's change on the branch codeforge/<run> and
// pushes it to the project's repository URL (KI-188), never to the remote
// the agent-writable workspace config names. A project without a
// repository URL keeps the branch local and reports why.
func (s *DeliverService) deliverBranch(ctx context.Context, proj *project.Project, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	var result *DeliveryResult
	err := s.pool.Run(ctx, func() error {
		repo, err := git.OpenRepo(ctx, proj.WorkspacePath)
		if err != nil {
			return fmt.Errorf("branch delivery: %w", err)
		}
		branchName := deliveryBranch(shortID)

		rc, err := buildRunCommit(ctx, repo, r.ID, s.commitMessage(shortID, taskTitle))
		if err != nil {
			return fmt.Errorf("branch delivery: %w", err)
		}
		if err := rc.checkoutNewBranch(ctx, repo, "refs/heads/"+branchName); err != nil {
			return fmt.Errorf("branch delivery: %w", err)
		}
		rc.syncIndex(ctx, repo)
		commitHash := rc.commit

		// The transport settings come from agent-writable config: a
		// repository that configures transports is not pushed from.
		pushErr := errors.New("the project has no repository URL to push the branch to (set its repo_url)")
		if proj.RepoURL != "" {
			pushErr = repo.PushBranch(ctx, s.pushURL(proj.RepoURL), branchName)
		}
		var pushError string
		if pushErr != nil {
			pushError = pushErr.Error()
			slog.Warn("git push failed (branch delivery)", "run_id", r.ID, "error", pushErr)
		}

		slog.Info("branch delivered", "run_id", r.ID, "branch", branchName)
		result = &DeliveryResult{
			Mode:       run.DeliverModeBranch,
			BranchName: branchName,
			CommitHash: commitHash,
			PushError:  pushError,
		}
		return nil
	})
	return result, err
}

func (s *DeliverService) deliverPR(ctx context.Context, proj *project.Project, r *run.Run, shortID, taskTitle string) (*DeliveryResult, error) {
	// First create branch (already uses pool internally)
	branchResult, err := s.deliverBranch(ctx, proj, r, shortID, taskTitle)
	if err != nil {
		return nil, fmt.Errorf("branch for PR: %w", err)
	}

	// P2-5: If push failed, skip PR creation — can't create PR without remote branch.
	if branchResult.PushError != "" {
		slog.Warn("skipping PR creation due to push failure", "run_id", r.ID, "push_error", branchResult.PushError)
		return branchResult, nil
	}

	prURL, err := s.openPullRequest(ctx, proj, &gitprovider.PullRequest{
		Head:  branchResult.BranchName,
		Title: fmt.Sprintf("%s %s", s.cfg.DeliveryCommitPrefix, taskTitle),
		Body:  fmt.Sprintf("Automated delivery from CodeForge run %s", r.ID),
	})
	if err != nil {
		slog.Warn("pull request not opened, falling back to branch-only", "run_id", r.ID, "error", err)
		branchResult.PRError = err.Error()
		return branchResult, nil
	}

	slog.Info("PR delivered", "run_id", r.ID, "url", prURL)
	return &DeliveryResult{
		Mode:       run.DeliverModePR,
		BranchName: branchResult.BranchName,
		CommitHash: branchResult.CommitHash,
		PRURL:      prURL,
	}, nil
}

// openPullRequest opens pr in proj's repository (the project's repository
// URL, never the agent-writable remote) through the provider's REST API:
//   - with a github-api project's own token, at its API (base_url);
//   - otherwise, for a github.com repository, with the operator's GitHub
//     token (github.token), which serves only the default tenant (KI-85).
func (s *DeliverService) openPullRequest(ctx context.Context, proj *project.Project, pr *gitprovider.PullRequest) (string, error) {
	parsed, err := project.ParseRepoURL(proj.RepoURL)
	if err != nil {
		return "", fmt.Errorf("the project's repository URL names no repository to open a pull request in: %w", err)
	}
	var cfg map[string]string
	switch {
	case proj.Provider == "github-api" && proj.Config["token"] != "":
		cfg = gitProviderConfig(proj)
	case !strings.EqualFold(parsed.Host, "github.com"):
		return "", fmt.Errorf("pull requests are opened on github.com, or through a project's github-api provider with its own token; %s is neither", parsed.Host)
	case !operatorCredentialsServe(ctx):
		return "", errors.New("no GitHub token: set the project's github-api provider token (github.token serves only the default tenant)")
	case s.githubToken == "":
		return "", errors.New("no GitHub token: set github.token (CODEFORGE_GITHUB_TOKEN) or the project's github-api provider token")
	default:
		cfg = map[string]string{"token": s.githubToken}
	}
	creator, err := s.pullRequests("github-api", cfg)
	if err != nil {
		return "", err
	}
	pr.Repo = parsed.Owner + "/" + parsed.Repo
	return creator.CreatePullRequest(ctx, pr)
}
