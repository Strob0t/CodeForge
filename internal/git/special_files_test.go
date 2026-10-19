package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/git"
)

// KI-187: git opens the workspace's ignore and attributes files, the files of
// .git/info and the core.excludesFile by name with a blocking open(). OpenRepo
// refuses a FIFO, socket or device (or a symbolic link to one) at these
// places before git could block on it - the reviewer's `mkfifo .gitignore` -
// and the nested walk refuses the ignore and attributes files of every
// directory it enters, which later commands (add, checkout) read.

// assertSpecialRefused checks that OpenRepo refuses dir at once (the
// deadlines are long: the check must end it, not the deadline) and names
// want.
func assertSpecialRefused(t *testing.T, dir, want string) {
	t.Helper()
	setTimeouts(t, time.Minute, time.Minute)
	err := openWithin(context.Background(), t, dir)
	if !errors.Is(err, git.ErrUnsafeRepository) || !strings.Contains(err.Error(), want) {
		t.Fatalf("OpenRepo = %v, want ErrUnsafeRepository naming %s", err, want)
	}
}

func mksock(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mknod(path, syscall.S_IFSOCK|0o644, 0); err != nil {
		t.Skipf("mknod socket: %v", err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRepo_RefusesSpecialFilesGitOpens(t *testing.T) {
	tests := []struct {
		name  string
		plant func(t *testing.T, dir string)
		want  string
	}{
		{"FIFO .gitignore (the reviewer's reproduction)", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".gitignore"))
		}, ".gitignore"},
		{"FIFO .gitattributes", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".gitattributes"))
		}, ".gitattributes"},
		{"FIFO .mailmap", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".mailmap"))
		}, ".mailmap"},
		{"FIFO .gitmodules", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".gitmodules"))
		}, ".gitmodules"},
		{"socket .gitignore", func(t *testing.T, dir string) {
			mksock(t, filepath.Join(dir, ".gitignore"))
		}, ".gitignore"},
		{".gitignore linked to a FIFO outside", func(t *testing.T, dir string) {
			fifo := filepath.Join(t.TempDir(), "fifo")
			mkfifo(t, fifo)
			symlink(t, fifo, filepath.Join(dir, ".gitignore"))
		}, ".gitignore"},
		{"FIFO .git/info/exclude", func(t *testing.T, dir string) {
			_ = os.Remove(filepath.Join(dir, ".git", "info", "exclude"))
			mkfifo(t, filepath.Join(dir, ".git", "info", "exclude"))
		}, ".git/info/exclude"},
		{"FIFO .git/info/attributes", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".git", "info", "attributes"))
		}, ".git/info/attributes"},
		{".git/info/exclude linked to a FIFO outside", func(t *testing.T, dir string) {
			fifo := filepath.Join(t.TempDir(), "fifo")
			mkfifo(t, fifo)
			_ = os.Remove(filepath.Join(dir, ".git", "info", "exclude"))
			symlink(t, fifo, filepath.Join(dir, ".git", "info", "exclude"))
		}, ".git/info/exclude"},
		{"FIFO .git/FETCH_HEAD", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".git", "FETCH_HEAD"))
		}, ".git/FETCH_HEAD"},
		{"FIFO loose ref", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".git", "refs", "heads", "evil"))
		}, ".git/refs/heads/evil"},
		{"FIFO reflog", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".git", "logs", "refs", "heads", "evil"))
		}, ".git/logs/refs/heads/evil"},
		{"FIFO pack index", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, ".git", "objects", "pack", "pack-evil.idx"))
		}, ".git/objects/pack/pack-evil.idx"},
		{"FIFO .gitattributes in a tracked directory", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "src", "main.go"), "package main\n", 0o644)
			plainGit(t, dir, "add", "src")
			plainGit(t, dir, "commit", "-q", "-m", "src")
			mkfifo(t, filepath.Join(dir, "src", ".gitattributes"))
		}, "src/.gitattributes"},
		{"FIFO .gitattributes in a new directory", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, "new", "deeper", ".gitattributes"))
		}, "new/deeper/.gitattributes"},
		{"FIFO core.excludesFile inside the worktree", func(t *testing.T, dir string) {
			mkfifo(t, filepath.Join(dir, "ignore-list"))
			plainGit(t, dir, "config", "core.excludesFile", "ignore-list")
		}, "core.excludesfile"},
		{"core.excludesFile outside the worktree", func(t *testing.T, dir string) {
			outside := filepath.Join(t.TempDir(), "ignore")
			writeFile(t, outside, "*.log\n", 0o644)
			plainGit(t, dir, "config", "core.excludesFile", outside)
		}, "core.excludesfile"},
		{"core.excludesFile in the Go Core's home", func(t *testing.T, dir string) {
			plainGit(t, dir, "config", "core.excludesFile", "~/.gitignore_global")
		}, "core.excludesfile"},
		{"core.excludesFile leaving the worktree through ..", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(filepath.Dir(dir), "ignore"), "*.log\n", 0o644)
			plainGit(t, dir, "config", "core.excludesFile", "../ignore")
		}, "core.excludesfile"},
		{"core.excludesFile through a directory link out of the worktree", func(t *testing.T, dir string) {
			outside := t.TempDir()
			writeFile(t, filepath.Join(outside, "ignore"), "*.log\n", 0o644)
			symlink(t, outside, filepath.Join(dir, "out"))
			plainGit(t, dir, "config", "core.excludesFile", "out/ignore")
		}, "core.excludesfile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newRepo(t)
			tt.plant(t, dir)
			assertSpecialRefused(t, dir, tt.want)
		})
	}
}

// Regular files and links git does not follow keep working.
func TestOpenRepo_AllowsRegularIgnoreAndAttributesFiles(t *testing.T) {
	tests := []struct {
		name  string
		plant func(t *testing.T, dir string)
	}{
		{"regular files everywhere", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\n", 0o644)
			writeFile(t, filepath.Join(dir, ".gitattributes"), "*.txt text\n", 0o644)
			writeFile(t, filepath.Join(dir, ".mailmap"), "A <a@example.invalid>\n", 0o644)
			writeFile(t, filepath.Join(dir, "sub", ".gitignore"), "build/\n", 0o644)
			writeFile(t, filepath.Join(dir, "sub", ".gitattributes"), "*.bin binary\n", 0o644)
			writeFile(t, filepath.Join(dir, ".git", "info", "attributes"), "*.md text\n", 0o644)
		}},
		{"core.excludesFile inside the worktree", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "conf", "ignore"), "*.log\n", 0o644)
			plainGit(t, dir, "config", "core.excludesFile", "conf/ignore")
		}},
		{"absolute core.excludesFile inside the worktree", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, ".git", "info", "more"), "*.log\n", 0o644)
			plainGit(t, dir, "config", "core.excludesFile", filepath.Join(dir, ".git", "info", "more"))
		}},
		{"missing core.excludesFile", func(t *testing.T, dir string) {
			plainGit(t, dir, "config", "core.excludesFile", "no-such-file")
		}},
		{".gitignore linked to a regular file", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "ignore-rules"), "*.log\n", 0o644)
			symlink(t, "ignore-rules", filepath.Join(dir, "sub", ".gitignore"))
		}},
		{"FIFO in an ignored directory", func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, ".gitignore"), "node_modules/\n", 0o644)
			mkfifo(t, filepath.Join(dir, "node_modules", "pkg", ".gitattributes"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newRepo(t)
			tt.plant(t, dir)
			setTimeouts(t, time.Minute, time.Minute)
			if err := openWithin(context.Background(), t, dir); err != nil {
				t.Fatalf("OpenRepo: %v", err)
			}
		})
	}
}
