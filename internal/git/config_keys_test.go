package git_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/git"
)

// Security review of KI-77: the fail-closed allowlist refused whole
// repositories for common, inert settings. Real-world configs open; keys that
// name a program, command or another repository still refuse the repository,
// and the error names the key.

func TestOpenRepo_AcceptsCommonRealWorldConfig(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	for _, kv := range [][2]string{
		{"core.excludesFile", "/home/dev/.gitignore_global"},
		{"core.attributesFile", "/home/dev/.gitattributes_global"},
		{"core.preloadIndex", "true"},
		{"core.longpaths", "true"},
		{"core.autocrlf", "input"},
		{"core.sharedRepository", "group"},
		{"core.commitGraph", "true"},
		{"commit.template", ".gitmessage"},
		{"commit.verbose", "true"},
		{"commit.cleanup", "scissors"},
		{"rerere.enabled", "true"},
		{"rerere.autoUpdate", "true"},
		{"log.date", "iso"},
		{"log.decorate", "short"},
		{"log.showSignature", "true"},
		{"tag.sort", "version:refname"},
		{"versionsort.suffix", "-rc"},
		{"format.pretty", "oneline"},
		{"format.signOff", "true"},
		{"pretty.changelog", "format:%s"},
		{"apply.whitespace", "fix"},
		{"gpg.format", "ssh"},
		{"color.ui", "auto"},
		{"color.diff.meta", "yellow"},
		{"pull.rebase", "true"},
		{"pull.ff", "only"},
		{"fetch.prune", "true"},
		{"fetch.parallel", "4"},
		{"fetch.writeCommitGraph", "true"},
		{"advice.detachedHead", "false"},
		{"i18n.commitEncoding", "utf-8"},
		{"diff.colorMoved", "zebra"},
		{"diff.algorithm", "histogram"},
		{"diff.renames", "copies"},
		{"diff.context", "5"},
		{"diff.noprefix", "true"},
		{"diff.submodule", "log"},
		{"diff.lfs.cachetextconv", "true"},
		{"merge.conflictStyle", "zdiff3"},
		{"merge.ff", "false"},
		{"merge.renames", "true"},
		{"merge.log", "true"},
		{"merge.ours.name", "keep ours"},
		{"init.defaultBranch", "main"},
		{"lfs.repositoryformatversion", "0"},
		{"status.submoduleSummary", "true"},
		{"blame.ignoreRevsFile", ".git-blame-ignore-revs"},
		{"rebase.autoSquash", "true"},
		{"rebase.updateRefs", "true"},
		{"branch.autoSetupRebase", "always"},
		{"help.autocorrect", "prompt"},
		{"transfer.fsckObjects", "true"},
		{"checkout.defaultRemote", "origin"},
		{"index.threads", "true"},
		{"safe.directory", "*"},
	} {
		plainGit(t, dir, "config", "--add", kv[0], kv[1])
	}

	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatalf("OpenRepo with a common real-world config: %v", err)
	}
	if err := repo.RequireNetworkSafe(); err != nil {
		t.Fatalf("RequireNetworkSafe: %v", err)
	}
	// Still no program runs: log would verify signatures with gpg.
	if _, err := repo.Run(ctx, nil, "log", "-1"); err != nil {
		t.Fatalf("log: %v", err)
	}
}

func TestOpenRepo_RefusesKeysThatNameProgramsAndSaysWhich(t *testing.T) {
	ctx := context.Background()
	for _, kv := range [][2]string{
		{"include.path", "/tmp/other.config"},
		{"includeIf.gitdir:/x/.path", "/tmp/other.config"},
		{"core.worktree", "/tmp/elsewhere"},
		{"core.askPass", "/tmp/evil"},
		{"extensions.worktreeConfig", "true"},
		{"diff.external", "/tmp/evil"},
		{"diff.tool", "evil"},
		{"diff.guitool", "evil"},
		{"diff.x.command", "/tmp/evil"},
		{"diff.x.textconv", "/tmp/evil"},
		{"merge.tool", "evil"},
		{"merge.x.driver", "/tmp/evil %A"},
		{"pull.twohead", "evil"},
		{"pull.octopus", "evil"},
		{"gpg.program", "/tmp/evil"},
		{"gpg.ssh.program", "/tmp/evil"},
		{"sequence.editor", "/tmp/evil"},
		{"interactive.diffFilter", "/tmp/evil"},
		{"submodule.x.update", "!/tmp/evil"},
		{"uploadpack.packObjectsHook", "/tmp/evil"},
		{"init.templateDir", "/tmp/templates"},
		{"sendemail.smtpServer", "/tmp/evil"},
		{"web.browser", "evil"},
		{"browser.evil.cmd", "/tmp/evil"},
		{"trace2.eventTarget", "/tmp/trace"},
		{"something.new", "x"},
	} {
		t.Run(kv[0], func(t *testing.T) {
			dir := newRepo(t)
			plainGit(t, dir, "config", kv[0], kv[1])
			_, err := git.OpenRepo(ctx, dir)
			if !errors.Is(err, git.ErrUnsafeRepository) {
				t.Fatalf("OpenRepo with %s = %v, want ErrUnsafeRepository", kv[0], err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(kv[0])) {
				t.Fatalf("error %q does not name the key %s", err, kv[0])
			}
		})
	}
}

func TestOpenRepo_NetworkOnlyFetchKeys(t *testing.T) {
	dir := newRepo(t)
	plainGit(t, dir, "config", "fetch.bundleURI", "https://evil.example/bundle")
	repo, err := git.OpenRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if err := repo.RequireNetworkSafe(); !errors.Is(err, git.ErrUnsafeRepository) || !strings.Contains(err.Error(), "fetch.bundleuri") {
		t.Fatalf("RequireNetworkSafe = %v, want fetch.bundleuri refused for network use", err)
	}
}

func TestDiffOutput_IgnoresPrefixAndColorConfig(t *testing.T) {
	ctx := context.Background()
	dir := newRepo(t)
	for _, kv := range [][2]string{{"diff.noprefix", "true"}, {"color.ui", "always"}, {"diff.relative", "true"}} {
		plainGit(t, dir, "config", kv[0], kv[1])
	}
	writeFile(t, dir+"/a.txt", "changed\n", 0o644)
	repo, err := git.OpenRepo(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	out, err := repo.Run(ctx, nil, append([]string{"diff"}, git.DiffFormatArgs...)...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--- a/a.txt") || strings.Contains(out, "\x1b[") {
		t.Fatalf("diff output not in the default format:\n%q", out)
	}
}
