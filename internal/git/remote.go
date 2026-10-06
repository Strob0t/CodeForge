package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidBranchName: a branch name git would not accept as one, or that it
// could read as an option or a pathspec.
var ErrInvalidBranchName = errors.New("invalid branch name")

// CheckBranchName refuses name unless it is a valid branch name (git
// check-ref-format --branch, without @{-n} shorthands): a name that starts
// with "-" or is a path such as "." never reaches a git command line.
func CheckBranchName(ctx context.Context, name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return fmt.Errorf("%w: %q", ErrInvalidBranchName, name)
	}
	out, err := Run(ctx, "", "check-ref-format", "--branch", name)
	if err != nil || strings.TrimSpace(out) != name {
		return fmt.Errorf("%w: %q", ErrInvalidBranchName, name)
	}
	return nil
}

// PushBranch pushes the local branch to the branch of the same name at url,
// the repository the caller names (the project's repository URL, KI-188):
// `git push -- <url> refs/heads/<b>:refs/heads/<b>`. The URL and the full
// refspec leave the agent-writable remote config out (remote.*.url,
// pushurl and push refspecs, push.default, branch upstreams); a remote named
// like url, through which git would still resolve it, is refused. The
// refspec has no "+": a branch that moved on the remote is never
// overwritten. Network-unsafe repositories are refused, and nested
// repositories (submodules) are never pushed, whatever the config says.
func (r *Repo) PushBranch(ctx context.Context, url, branch string) error {
	if err := r.RequireNetworkSafe(); err != nil {
		return err
	}
	if url == "" || strings.HasPrefix(url, "-") {
		return fmt.Errorf("push: invalid repository URL %q", url)
	}
	if err := CheckBranchName(ctx, branch); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	if err := r.refuseRemoteNamed(url); err != nil {
		return err
	}
	ref := "refs/heads/" + branch
	_, err := runGit(ctx, r.Dir, r.env(nil), "push", "--no-recurse-submodules", "--no-verify", "--", url, ref+":"+ref)
	return err
}

// refuseRemoteNamed refuses a network operation on url when the repository
// defines a remote named url: git resolves a URL argument through a remote
// of that name - remote.<url>.url or pushurl in the config, or, for a name
// without "/", a legacy .git/remotes or .git/branches file - so the agent
// could redirect the operation to another repository (KI-188).
func (r *Repo) refuseRemoteNamed(url string) error {
	for key := range r.config {
		if section, subsection, _ := splitKey(key); section == "remote" && subsection == url {
			return unsafeRepo(fmt.Sprintf("config key %q defines a remote named like the repository URL, "+
				"which would redirect the network operation (remove it from .git/config)", key))
		}
	}
	if strings.ContainsRune(url, '/') || url == "." || url == ".." {
		return nil
	}
	for _, dir := range []string{"remotes", "branches"} {
		if _, err := os.Lstat(filepath.Join(r.GitDir, dir, url)); err == nil {
			return unsafeRepo(fmt.Sprintf(".git/%s/%s defines a remote named like the repository URL", dir, url))
		}
	}
	return nil
}
