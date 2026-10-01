package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Workspace git (KI-77). Agents write to project workspaces, including .git
// (Bash can always, file tools are denied it), and the Go Core shares the
// workspace volume and UID with the worker. Repository config, attributes and
// hooks are therefore attacker-controlled: an fsmonitor, a filter, diff or
// merge driver, a hook, a credential helper or an ssh command configured in
// the workspace would run in the Go Core with its secrets. Every git command
// the Go Core runs in a workspace goes through Repo:
//
//   - the environment ignores system and global config and attributes, never
//     prompts and carries no inherited GIT_* variables (except the operator's
//     TLS and ssh settings);
//   - command-line config (GIT_CONFIG_COUNT, which also reaches git processes
//     started by gh) disables fsmonitor, hooks, credential helpers, signing,
//     automatic gc and submodule recursion, and neutralises every filter
//     driver the repository config defines, so LFS repositories keep working
//     with their files as plain content;
//   - OpenRepo reads the repository config without running anything and
//     refuses the repository (fail closed) for any key outside an allowlist of
//     data-only keys, for include/includeIf, core.worktree, a .git that is a
//     file or a symlink, commondir, alternates and symlinked git directories.
//     Keys that name a transport program (ssh command, proxy command,
//     remote programs, remote helpers) or make a remote a promisor (lazy
//     fetches from local commands) are refused in every repository. Keys
//     that only configure transports with data (proxies, URL rewrites,
//     protocol and http settings) are refused for network operations only
//     (RequireNetworkSafe). Lazy fetches are also disabled in the
//     environment (GIT_NO_LAZY_FETCH).
//
// A process of the agent that keeps running can still rewrite the config
// between OpenRepo's check and git's own read (a new filter driver name);
// closing that window needs separate UIDs for the worker's tools (KI-71).

// ErrNotRepository: the directory has no .git directory.
var ErrNotRepository = errors.New("not a git repository")

// ErrUnsafeRepository: git must not run in the repository (KI-77).
var ErrUnsafeRepository = errors.New("unsafe workspace repository")

// keptGitEnv are the GIT_* variables of the Go Core's own environment that
// workspace git keeps: the operator's TLS and ssh transport settings.
var keptGitEnv = []string{"GIT_SSL_CAINFO", "GIT_SSL_CAPATH", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT"}

// baseEnv is the environment of every git process of the Go Core.
func baseEnv() []string {
	env := make([]string, 0, len(os.Environ())+12)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "SSH_ASKPASS" || (strings.HasPrefix(name, "GIT_") && !slices.Contains(keptGitEnv, name)) {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"GIT_EDITOR=true",
		"GIT_SEQUENCE_EDITOR=true",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		// A promisor remote written into the config after OpenRepo's check
		// must not make a local command fetch (S3-F security review S1).
		"GIT_NO_LAZY_FETCH=1",
	)
}

// commonOverrides is the command-line config of every git process of the Go
// Core (the highest precedence, above the repository config).
var commonOverrides = [][2]string{
	{"core.fsmonitor", "false"},
	{"core.hooksPath", "/dev/null"},
	{"core.untrackedCache", "false"},
	{"core.splitIndex", "false"},
	{"core.attributesFile", "/dev/null"},
	{"credential.helper", ""}, // an empty value drops every helper configured before it
	{"commit.gpgSign", "false"},
	{"tag.gpgSign", "false"},
	{"push.gpgSign", "false"},
	{"gc.auto", "0"},
	{"maintenance.auto", "false"},
	{"submodule.recurse", "false"},
	{"fetch.recurseSubmodules", "false"},
	// A recursive push runs `git push` in nested repositories, whose config
	// OpenRepo never checks (S3-F security review S2).
	{"push.recurseSubmodules", "no"},
	// Signature checks would run gpg/ssh-keygen; submodule summaries and
	// log-style submodule diffs would run git in submodule repositories,
	// whose config OpenRepo does not inspect.
	{"log.showSignature", "false"},
	{"merge.verifySignatures", "false"},
	{"status.submoduleSummary", "false"},
	{"diff.submodule", "short"},
}

// DiffFormatArgs make git diff print the plain default format whatever the
// repository configures (prefixes, color, relative paths, external diff and
// textconv drivers), so a diff can be applied as a patch.
var DiffFormatArgs = []string{"--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--no-relative"}

// repoOverrides restrict the transports of workspace repositories: the remote
// URL comes from agent-writable config, so local paths (other tenants'
// workspaces), file:// and ext:: are refused.
var repoOverrides = [][2]string{
	{"protocol.allow", "never"},
	{"protocol.https.allow", "always"},
	{"protocol.http.allow", "always"},
	{"protocol.ssh.allow", "always"},
	{"protocol.git.allow", "always"},
}

// configEnv encodes overrides as GIT_CONFIG_COUNT/KEY/VALUE variables.
func configEnv(overrides [][2]string) []string {
	env := make([]string, 0, 2*len(overrides)+1)
	env = append(env, "GIT_CONFIG_COUNT="+strconv.Itoa(len(overrides)))
	for i, kv := range overrides {
		n := strconv.Itoa(i)
		env = append(env, "GIT_CONFIG_KEY_"+n+"="+kv[0], "GIT_CONFIG_VALUE_"+n+"="+kv[1])
	}
	return env
}

// Run runs git outside any workspace repository (init, clone, ls-remote) with
// the hardened environment and returns its standard output. dir is the
// working directory; "" runs it in the temp directory, never in the Go Core's
// own working directory.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	env := append(baseEnv(), configEnv(commonOverrides)...)
	return runGit(ctx, dir, env, args...)
}

// networkCommands reach a remote: they run only in a network-safe repository.
var networkCommands = []string{"fetch", "pull", "push", "ls-remote"}

// RunIn runs git in the workspace repository at dir (OpenRepo; a network
// command also RequireNetworkSafe) and returns its standard output. With dir
// "" it runs outside any repository (Run).
func RunIn(ctx context.Context, dir string, args ...string) (string, error) {
	if dir == "" {
		return Run(ctx, "", args...)
	}
	repo, err := OpenRepo(ctx, dir)
	if err != nil {
		return "", err
	}
	if len(args) > 0 && slices.Contains(networkCommands, args[0]) {
		if err := repo.RequireNetworkSafe(); err != nil {
			return "", err
		}
	}
	return repo.Run(ctx, nil, args...)
}

// Repo is a workspace repository that OpenRepo checked.
type Repo struct {
	// Dir is the worktree, GitDir its .git directory (both absolute).
	Dir    string
	GitDir string

	overrides     [][2]string         // command-line config of every git process
	config        map[string][]string // repository config, keys as git lists them
	networkUnsafe []string            // keys that refuse network operations
}

// OpenRepo checks the workspace repository at dir (see the package comment)
// and returns it. It returns ErrNotRepository when dir has no .git and an
// error wrapping ErrUnsafeRepository when git must not run in it.
func OpenRepo(ctx context.Context, dir string) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("workspace path: %w", err)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workspace %s: %w", abs, ErrNotRepository)
	}
	gitDir := filepath.Join(abs, ".git")
	if err := checkGitDir(gitDir); err != nil {
		return nil, err
	}

	r := &Repo{Dir: abs, GitDir: gitDir}
	if err := r.loadConfig(ctx); err != nil {
		return nil, err
	}

	r.overrides = slices.Concat(commonOverrides, repoOverrides, [][2]string{{"safe.directory", abs}})
	for _, name := range r.filterDrivers() {
		prefix := "filter." + name + "."
		r.overrides = append(r.overrides,
			[2]string{prefix + "clean", ""}, [2]string{prefix + "smudge", ""},
			[2]string{prefix + "process", ""}, [2]string{prefix + "required", "false"})
	}
	return r, nil
}

// env is the environment of a git process in the repository: the hardened
// base, the overrides plus extra ones, the repository and the worktree.
func (r *Repo) env(extra [][2]string) []string {
	return slices.Concat(baseEnv(), configEnv(slices.Concat(r.overrides, extra)),
		[]string{"GIT_DIR=" + r.GitDir, "GIT_WORK_TREE=" + r.Dir})
}

// checkGitDir refuses a .git that is not a plain directory of this worktree:
// a gitdir file or symlink (another repository), a shared commondir,
// alternates, a config that is no regular file, or symlinked directories git
// writes into (refs, logs, objects).
func checkGitDir(gitDir string) error {
	info, err := os.Lstat(gitDir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", filepath.Dir(gitDir), ErrNotRepository)
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", gitDir, err)
	}
	if !info.IsDir() {
		return unsafeRepo(".git is not a directory (a gitdir file or a symlink)")
	}
	for _, name := range []string{"commondir", "objects/info/alternates", "objects/info/http-alternates"} {
		if _, err := os.Lstat(filepath.Join(gitDir, name)); err == nil {
			return unsafeRepo(".git/" + name + " points into another repository")
		}
	}
	if info, err := os.Lstat(filepath.Join(gitDir, "config")); err != nil || !info.Mode().IsRegular() {
		return unsafeRepo(".git/config is not a regular file")
	}
	for _, name := range []string{"HEAD", "index", "packed-refs"} {
		if info, err := os.Lstat(filepath.Join(gitDir, name)); err == nil && !info.Mode().IsRegular() {
			return unsafeRepo(".git/" + name + " is not a regular file")
		}
	}
	if info, err := os.Lstat(filepath.Join(gitDir, "info")); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return unsafeRepo(".git/info is a symlink")
	}
	for _, name := range []string{"refs", "logs", "objects"} {
		if err := refuseSymlinkedDirs(gitDir, name); err != nil {
			return err
		}
	}
	return nil
}

// refuseSymlinkedDirs refuses a symlink anywhere in the directory tree
// .git/name (objects: the directory and its immediate entries only, loose
// objects and packs are only read).
func refuseSymlinkedDirs(gitDir, name string) error {
	root := filepath.Join(gitDir, name)
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(gitDir, path)
			return unsafeRepo(".git/" + filepath.ToSlash(rel) + " is a symlink")
		}
		if name == "objects" && d.IsDir() && path != root {
			return fs.SkipDir
		}
		return nil
	})
}

func unsafeRepo(reason string) error {
	return fmt.Errorf("%w: %s", ErrUnsafeRepository, reason)
}

// loadConfig reads the repository config file without includes (git runs
// nothing to list a file) and checks every key.
func (r *Repo) loadConfig(ctx context.Context) error {
	out, err := runGit(ctx, r.Dir, append(baseEnv(), "GIT_DIR="+r.GitDir),
		"config", "--file", filepath.Join(r.GitDir, "config"), "--list", "--null")
	if err != nil {
		return fmt.Errorf("read workspace git config: %w", err)
	}
	r.config = make(map[string][]string)
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		key, value, _ := strings.Cut(entry, "\n")
		r.config[key] = append(r.config[key], value)
		switch classifyKey(key, value) {
		case keyRefused:
			return unsafeRepo(fmt.Sprintf("config key %q is not allowed in a workspace repository: it can name a program, "+
				"a command or another repository, or it is not known to be inert (remove it from .git/config)", key))
		case keyNetworkOnly:
			r.networkUnsafe = append(r.networkUnsafe, key)
		}
	}
	return nil
}

// filterDrivers returns the names of the filter drivers the config defines.
func (r *Repo) filterDrivers() []string {
	var names []string
	for key := range r.config {
		section, sub, _ := splitKey(key)
		if section == "filter" && sub != "" && !slices.Contains(names, sub) {
			names = append(names, sub)
		}
	}
	slices.Sort(names)
	return names
}

// HasConfig reports whether the repository config sets key (as git lists
// it: section and variable lower case).
func (r *Repo) HasConfig(key string) bool {
	return len(r.config[key]) > 0
}

// RequireNetworkSafe refuses network operations (fetch, pull, push) in a
// repository whose config redirects or configures transports.
func (r *Repo) RequireNetworkSafe() error {
	if len(r.networkUnsafe) > 0 {
		return unsafeRepo(fmt.Sprintf("config key %q is not allowed for network operations (remove it from .git/config)", r.networkUnsafe[0]))
	}
	return nil
}

// Run runs git in the repository with the hardened environment plus
// extraEnv and returns its standard output.
func (r *Repo) Run(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	return runGit(ctx, r.Dir, slices.Concat(r.env(nil), extraEnv), args...)
}

// FetchFrom fetches the branches of url into refs/remotes/origin. url is the
// project's own clone URL, which the caller compared with the repository's
// origin: unlike a remote from the agent-writable config it may be a local
// path. Transport-configuring repositories are refused.
func (r *Repo) FetchFrom(ctx context.Context, url string) error {
	if err := r.RequireNetworkSafe(); err != nil {
		return err
	}
	_, err := runGit(ctx, r.Dir, r.env([][2]string{{"protocol.file.allow", "always"}}),
		"fetch", "--no-recurse-submodules", "--", url, "+refs/heads/*:refs/remotes/origin/*")
	return err
}

// Push pushes from a network-safe repository, never into nested
// repositories (submodules), whatever the config says.
func (r *Repo) Push(ctx context.Context, args ...string) error {
	if err := r.RequireNetworkSafe(); err != nil {
		return err
	}
	_, err := runGit(ctx, r.Dir, r.env(nil), slices.Concat([]string{"push", "--no-recurse-submodules"}, args)...)
	return err
}

// Command returns another program (gh) to run in the repository with the
// hardened environment; git processes it starts inherit the overrides.
func (r *Repo) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: program and arguments chosen by the Go Core
	cmd.Dir = r.Dir
	cmd.Env = r.env(nil)
	return cmd
}

// runGit runs git with env in dir and returns its standard output.
func runGit(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: arguments chosen by the Go Core, no shell
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		op := "git"
		if len(args) > 0 {
			op += " " + args[0]
		}
		// Standard output is returned as well: some commands report their
		// result with a non-zero exit (git merge-tree on conflicts).
		return stdout.String(), fmt.Errorf("%s: %s: %w", op, strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}
