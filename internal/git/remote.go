package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Strob0t/CodeForge/internal/proctemp"
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
// the repository the caller names (the project's repository URL, KI-188),
// with the full refspec refs/heads/<b>:refs/heads/<b>. The refspec has no
// "+": a branch that moved on the remote is never overwritten.
//
// The push runs from a private bare repository of the Go Core (pushCommit)
// that borrows the workspace's objects and holds only the branch's commit,
// so no workspace config takes part - not even one a concurrent run writes
// after OpenRepo's check: no remote, URL rewrite (url.<x>.insteadOf, which
// git applies to a URL on the command line too), push option, tag
// following or submodule recursion of the agent reaches the push.
// Network-unsafe repositories and remotes named like url are still refused.
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
	out, err := r.Run(ctx, nil, "rev-parse", "--verify", "-q", ref+"^{commit}")
	if err != nil {
		return fmt.Errorf("push: resolve %s: %w", ref, err)
	}
	return r.pushCommit(ctx, url, ref, strings.TrimSpace(out))
}

// pushOverrides fix the keys that change what a push sends or where, on top
// of commonOverrides (signing, submodule recursion) and repoOverrides
// (transports). The private repository sets none of them; the overrides
// keep it that way whatever git's defaults become.
var pushOverrides = [][2]string{
	{"push.pushOption", ""}, // the empty value clears the list
	{"push.followTags", "false"},
	{"push.negotiate", "false"},
	{"push.default", "nothing"},
	{"push.autoSetupRemote", "false"},
}

// objectFormatPattern matches the object formats git names (sha1, sha256).
var objectFormatPattern = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// pushCommit pushes commit to ref at url from a private bare repository in
// the Go Core's temporary directory, removed afterwards: its
// objects/info/alternates names the workspace's object directory, and it
// holds ref and nothing else. Loose objects and packs of the workspace are
// still read by name; a FIFO among them ends at the git deadline.
func (r *Repo) pushCommit(ctx context.Context, url, ref, commit string) error {
	objects := filepath.Join(r.GitDir, "objects")
	if strings.ContainsAny(objects, "\n\r") || strings.HasPrefix(objects, "\"") {
		return fmt.Errorf("push: workspace path %q cannot be named in an alternates file", r.Dir)
	}
	// Alternates added after OpenRepo would chain the push to another
	// repository's objects (a narrow window remains until git reads it).
	if _, err := os.Lstat(filepath.Join(objects, "info", "alternates")); err == nil {
		return unsafeRepo(".git/objects/info/alternates points into another repository")
	}
	tmp, err := proctemp.MkdirTemp("git-push-*")
	if err != nil {
		return fmt.Errorf("push: private repository: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			slog.Warn("remove private push repository", "dir", tmp, "error", err)
		}
	}()

	gitDir := filepath.Join(tmp, "push.git")
	initArgs := []string{"init", "-q", "--bare", "--template="}
	if values := r.config["extensions.objectformat"]; len(values) > 0 {
		format := strings.ToLower(values[len(values)-1])
		if !objectFormatPattern.MatchString(format) {
			return fmt.Errorf("push: unknown object format %q", format)
		}
		initArgs = append(initArgs, "--object-format="+format)
	}
	if _, err := Run(ctx, tmp, append(initArgs, gitDir)...); err != nil {
		return fmt.Errorf("push: private repository: %w", err)
	}
	info := filepath.Join(gitDir, "objects", "info")
	if err := os.MkdirAll(info, 0o700); err != nil {
		return fmt.Errorf("push: private repository: %w", err)
	}
	if err := os.WriteFile(filepath.Join(info, "alternates"), []byte(objects+"\n"), 0o600); err != nil {
		return fmt.Errorf("push: private repository: %w", err)
	}

	env := slices.Concat(baseEnv(), configEnv(slices.Concat(commonOverrides, repoOverrides, pushOverrides)),
		[]string{"GIT_DIR=" + gitDir})
	if _, err := runGit(ctx, tmp, env, "update-ref", ref, commit); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	_, err = runGit(ctx, tmp, env, "push", "--no-recurse-submodules", "--no-verify", "--no-follow-tags", "--", url, ref+":"+ref)
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
