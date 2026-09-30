// Package svn implements the gitprovider.Provider interface for SVN repositories using the svn CLI.
//
// Working copies are agent-writable (S3 follow-up 1c), so svn runs hardened:
//   - every invocation is non-interactive, caches no credentials and reads
//     its client configuration from a private, empty directory (no diff,
//     merge or editor commands, tunnels or auth stores of the operator's or
//     anyone else's ~/.subversion); SVN_* variables, EDITOR and VISUAL are
//     dropped from its environment;
//   - svn has no client-side hooks; externals are never fetched
//     (--ignore-externals on checkout, update and switch), since the agent
//     can define them in the working copy;
//   - a working copy is used only when .svn is a real directory of the given
//     path (svn would otherwise look for a working copy in parent
//     directories) with a regular wc.db and no symlinked pristine store, so
//     svn cannot be pointed at another tenant's working copy;
//   - the repository URL of a working copy comes from its agent-writable
//     wc.db: operations that contact the repository (update, switch, log,
//     ls) are refused unless its scheme is http, https, svn or svn+ssh, so
//     the agent cannot redirect them to a local (file://) repository, e.g.
//     another tenant's. file:// repositories are allowed only when the
//     operator enables them (config key allow_file_urls).
package svn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

const providerName = "svn"

// Provider interacts with SVN repositories via the svn CLI.
type Provider struct {
	pool        *git.Pool
	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
	username    string
	password    string
	// allowFileURLs lets working copies of local (file://) repositories be
	// updated; off by default (see the package comment).
	allowFileURLs bool

	configOnce sync.Once
	configDir  string
	configErr  error
}

// NewProvider creates an SVN provider that limits concurrent operations via pool.
func NewProvider(pool *git.Pool) *Provider {
	return &Provider{pool: pool, execCommand: exec.CommandContext}
}

// Name returns "svn".
func (p *Provider) Name() string { return providerName }

// Capabilities returns what the SVN provider supports.
func (p *Provider) Capabilities() gitprovider.Capabilities {
	return gitprovider.Capabilities{
		Clone:       true,
		Push:        false,
		PullRequest: false,
		Webhook:     false,
		Issues:      false,
	}
}

// CloneURL returns the URL as-is for SVN operations.
func (p *Provider) CloneURL(_ context.Context, repo string) (string, error) {
	return repo, nil
}

// ListRepos is not supported for SVN.
func (p *Provider) ListRepos(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("svn: ListRepos not supported")
}

// errUnsafeWorkingCopy: svn is not run on the working copy.
var errUnsafeWorkingCopy = errors.New("unsafe SVN working copy")

// Clone checks out an SVN repository to the given local path.
// If Branch is set via CloneOption, the URL is adjusted to point to the branch
// directory following SVN's standard trunk/branches layout.
// If the destination already exists and is an SVN working copy with a matching URL,
// it runs svn update instead of failing.
func (p *Provider) Clone(ctx context.Context, url, destPath string, opts ...gitprovider.CloneOption) error {
	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("svn: resolve path: %w", err)
	}

	o := gitprovider.ApplyCloneOptions(opts)
	checkoutURL := resolveBranchURL(url, o.Branch)
	if err := p.checkRepositoryURL(checkoutURL); err != nil {
		return err
	}

	return p.pool.Run(ctx, func() error {
		// Handle existing directory.
		if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
			return p.reclone(ctx, checkoutURL, absPath)
		}

		if _, execErr := p.runSVN(ctx, "", "checkout", "--ignore-externals", checkoutURL, absPath); execErr != nil {
			return fmt.Errorf("svn: checkout: %w", execErr)
		}
		return nil
	})
}

// reclone handles re-checkout when the destination directory already exists.
// If it's an SVN working copy with the same URL, runs svn update.
// Otherwise removes the directory and does a fresh checkout. A working copy
// whose metadata is unsafe is reported, not removed.
func (p *Provider) reclone(ctx context.Context, url, absPath string) error {
	wcErr := checkWorkingCopy(absPath)
	if errors.Is(wcErr, errUnsafeWorkingCopy) {
		return wcErr
	}
	if wcErr == nil {
		wcURL, err := p.runSVN(ctx, absPath, "info", "--show-item", "url")
		if err == nil && strings.TrimSpace(wcURL) == url {
			// Same URL as the project's: update from it.
			if _, updErr := p.runSVN(ctx, absPath, "update", "--ignore-externals"); updErr != nil {
				return fmt.Errorf("svn: update: %w", updErr)
			}
			return nil
		}
	}

	// Not an SVN working copy or different URL — remove and re-checkout.
	if rmErr := os.RemoveAll(absPath); rmErr != nil {
		return fmt.Errorf("svn: remove existing directory: %w", rmErr)
	}
	if _, coErr := p.runSVN(ctx, "", "checkout", "--ignore-externals", url, absPath); coErr != nil {
		return fmt.Errorf("svn: checkout: %w", coErr)
	}
	return nil
}

// resolveBranchURL maps a branch name to the SVN URL convention.
// Empty branch or "trunk" returns the URL as-is.
// Other branch names replace a trailing /trunk with /branches/<name>,
// or append /branches/<name> if no /trunk suffix is present.
func resolveBranchURL(url, branch string) string {
	if branch == "" || branch == "trunk" {
		return url
	}
	if base, ok := strings.CutSuffix(url, "/trunk"); ok {
		return base + "/branches/" + branch
	}
	return url + "/branches/" + branch
}

// Status returns the status of an SVN working copy.
func (p *Provider) Status(ctx context.Context, repoPath string) (*project.GitStatus, error) {
	var status *project.GitStatus
	err := p.pool.Run(ctx, func() error {
		if err := checkWorkingCopy(repoPath); err != nil {
			return err
		}
		status = &project.GitStatus{}

		// Get SVN info for current revision
		info, err := p.runSVN(ctx, repoPath, "info", "--show-item", "revision")
		if err != nil {
			return fmt.Errorf("svn: info: %w", err)
		}
		status.CommitHash = strings.TrimSpace(info)

		// Get the last log entry; svn log contacts the repository.
		if _, err := p.repositoryRoot(ctx, repoPath); err == nil {
			logOut, err := p.runSVN(ctx, repoPath, "log", "-l", "1")
			if err == nil {
				lines := strings.Split(strings.TrimSpace(logOut), "\n")
				// SVN log format: separator, metadata, blank, message, separator
				if len(lines) >= 4 {
					status.CommitMessage = strings.TrimSpace(lines[3])
				}
			}
		}

		// Get URL as branch name
		urlOut, err := p.runSVN(ctx, repoPath, "info", "--show-item", "relative-url")
		if err == nil {
			relURL := strings.TrimSpace(urlOut)
			status.Branch = relURL
		}

		// Check for modified/untracked files
		st, err := p.runSVN(ctx, repoPath, "status")
		if err != nil {
			return fmt.Errorf("svn: status: %w", err)
		}
		for _, line := range strings.Split(st, "\n") {
			if len(line) < 2 {
				continue
			}
			indicator := line[0]
			file := strings.TrimSpace(line[1:])
			if file == "" {
				continue
			}
			switch indicator {
			case '?':
				status.Untracked = append(status.Untracked, file)
			case 'M', 'A', 'D', 'C', 'R':
				status.Modified = append(status.Modified, file)
			}
		}
		status.Dirty = len(status.Modified) > 0 || len(status.Untracked) > 0

		return nil
	})
	return status, err
}

// Pull updates an SVN working copy (svn update).
func (p *Provider) Pull(ctx context.Context, repoPath string) error {
	return p.pool.Run(ctx, func() error {
		if err := checkWorkingCopy(repoPath); err != nil {
			return err
		}
		if _, err := p.repositoryRoot(ctx, repoPath); err != nil {
			return err
		}
		if _, err := p.runSVN(ctx, repoPath, "update", "--ignore-externals"); err != nil {
			return fmt.Errorf("svn: update: %w", err)
		}
		return nil
	})
}

// ListBranches lists SVN branches by listing the branches/ directory.
func (p *Provider) ListBranches(ctx context.Context, repoPath string) ([]project.Branch, error) {
	var branches []project.Branch
	err := p.pool.Run(ctx, func() error {
		if err := checkWorkingCopy(repoPath); err != nil {
			return err
		}
		rootURL, err := p.repositoryRoot(ctx, repoPath)
		if err != nil {
			return err
		}

		// List branches
		branchesURL := rootURL + "/branches"
		out, err := p.runSVN(ctx, "", "ls", branchesURL)
		if err != nil {
			// No branches directory -- return trunk only
			branches = append(branches, project.Branch{Name: "trunk", Current: true})
			return nil
		}

		// Get current relative URL
		curURL, _ := p.runSVN(ctx, repoPath, "info", "--show-item", "relative-url")
		curURL = strings.TrimSpace(curURL)

		branches = append(branches, project.Branch{
			Name:    "trunk",
			Current: strings.Contains(curURL, "trunk"),
		})

		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSuffix(strings.TrimSpace(line), "/")
			if line == "" {
				continue
			}
			branches = append(branches, project.Branch{
				Name:    line,
				Current: strings.Contains(curURL, "branches/"+line),
			})
		}

		return nil
	})
	return branches, err
}

// Checkout switches to a different SVN branch by doing svn switch.
func (p *Provider) Checkout(ctx context.Context, repoPath, branch string) error {
	return p.pool.Run(ctx, func() error {
		if err := checkWorkingCopy(repoPath); err != nil {
			return err
		}
		rootURL, err := p.repositoryRoot(ctx, repoPath)
		if err != nil {
			return err
		}

		var targetURL string
		if branch == "trunk" {
			targetURL = rootURL + "/trunk"
		} else {
			targetURL = rootURL + "/branches/" + branch
		}

		if _, err := p.runSVN(ctx, repoPath, "switch", "--ignore-externals", targetURL); err != nil {
			return fmt.Errorf("svn: switch to %s: %w", branch, err)
		}
		return nil
	})
}

// checkWorkingCopy checks that dir is itself the root of an SVN working copy
// whose metadata svn may read (see the package comment).
func checkWorkingCopy(dir string) error {
	meta := filepath.Join(dir, ".svn")
	info, err := os.Lstat(meta)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("svn: %s is not an SVN working copy (no .svn directory)", dir)
	}
	if err != nil {
		return fmt.Errorf("svn: inspect %s: %w", meta, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("svn: %s: .svn is not a directory: %w", dir, errUnsafeWorkingCopy)
	}
	if info, err := os.Lstat(filepath.Join(meta, "wc.db")); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("svn: %s: .svn/wc.db is not a regular file: %w", dir, errUnsafeWorkingCopy)
	}
	if info, err := os.Lstat(filepath.Join(meta, "pristine")); err == nil && !info.IsDir() {
		return fmt.Errorf("svn: %s: .svn/pristine is not a directory: %w", dir, errUnsafeWorkingCopy)
	}
	return nil
}

// repositoryRoot returns the repository root URL recorded in the working
// copy, refusing one svn must not contact (checkRepositoryURL).
func (p *Provider) repositoryRoot(ctx context.Context, repoPath string) (string, error) {
	out, err := p.runSVN(ctx, repoPath, "info", "--show-item", "repos-root-url")
	if err != nil {
		return "", fmt.Errorf("svn: get repo root: %w", err)
	}
	root := strings.TrimSpace(out)
	if err := p.checkRepositoryURL(root); err != nil {
		return "", err
	}
	return root, nil
}

// allowedSchemes are the repository URL schemes svn may contact.
var allowedSchemes = map[string]bool{"http": true, "https": true, "svn": true, "svn+ssh": true}

func (p *Provider) checkRepositoryURL(raw string) error {
	u, err := neturl.Parse(raw)
	if err != nil {
		return fmt.Errorf("svn: repository URL %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if allowedSchemes[scheme] || (scheme == "file" && p.allowFileURLs) {
		return nil
	}
	return fmt.Errorf("svn: repository URL %q: scheme %q is not allowed (http, https, svn, svn+ssh; file only when allow_file_urls is set)", raw, u.Scheme)
}

// config returns the private, empty client configuration directory.
func (p *Provider) config() (string, error) {
	p.configOnce.Do(func() {
		p.configDir, p.configErr = os.MkdirTemp("", "codeforge-svn-config-*")
	})
	return p.configDir, p.configErr
}

// environment is the environment of svn: the Go Core's, without the
// variables that make svn run other programs (editors, merge tools, ssh
// replacements) or read other configuration.
func (p *Provider) environment() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "SVN_") || name == "EDITOR" || name == "VISUAL" {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// runSVN executes an svn command and returns stdout.
// Authentication flags are prepended when username/password are configured.
func (p *Provider) runSVN(ctx context.Context, dir string, args ...string) (string, error) {
	configDir, err := p.config()
	if err != nil {
		return "", fmt.Errorf("svn: private config directory: %w", err)
	}
	global := []string{"--non-interactive", "--no-auth-cache", "--config-dir", configDir}
	if p.username != "" {
		global = append(global, "--username", p.username)
		if p.password != "" {
			global = append(global, "--password", p.password)
		}
	}
	args = append(global, args...)

	cmd := p.execCommand(ctx, "svn", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = p.environment()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}
