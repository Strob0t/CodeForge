//go:build unix

package workspacefs

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

const outsideText = "outside-only content"

// tree builds a workspace next to a directory outside it that holds a secret.
func tree(t *testing.T) (ws, out string) {
	t.Helper()
	base := t.TempDir()
	ws = filepath.Join(base, "ws")
	out = filepath.Join(base, "out")
	mustMkdir(t, filepath.Join(ws, "src", "sub"))
	mustMkdir(t, out)
	mustWrite(t, filepath.Join(out, "secret.txt"), outsideText)
	mustWrite(t, filepath.Join(ws, "src", "a.go"), "package a\n")
	mustWrite(t, filepath.Join(ws, "src", "sub", "b.go"), "package sub\n")
	return ws, out
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func mustOpen(t *testing.T, path string) *Root {
	t.Helper()
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// noBlock fails the test when fn does not return in time (a FIFO opened blocking).
func noBlock(t *testing.T, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the call blocked")
		return nil
	}
}

func TestReadFile(t *testing.T) {
	ws, out := tree(t)
	mustSymlink(t, "src/a.go", filepath.Join(ws, "alias.go"))
	mustSymlink(t, "src", filepath.Join(ws, "srclink"))
	mustSymlink(t, "..", filepath.Join(ws, "src", "up"))
	mustSymlink(t, "../out/secret.txt", filepath.Join(ws, "leak.txt"))
	mustSymlink(t, filepath.Join(out, "secret.txt"), filepath.Join(ws, "leak_abs.txt"))
	mustSymlink(t, "../out", filepath.Join(ws, "outdir"))
	mustSymlink(t, filepath.Join(ws, "src"), filepath.Join(ws, "abs_inside"))
	r := mustOpen(t, ws)

	for _, name := range []string{"src/a.go", "alias.go", "srclink/a.go", "src/sub/../a.go", "src/up/src/a.go", "./src//a.go"} {
		t.Run("inside "+name, func(t *testing.T) {
			data, info, err := r.ReadFile(name, 1024)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "package a\n" || info.Size() != int64(len(data)) {
				t.Fatalf("got %q (%d)", data, info.Size())
			}
		})
	}
	for _, name := range []string{
		"leak.txt", "leak_abs.txt", "outdir/secret.txt", "../out/secret.txt", "src/../../out/secret.txt",
		"src/up/../out/secret.txt", filepath.Join(out, "secret.txt"), filepath.Join(ws, "src", "a.go"),
		// An absolute symlink leaves the workspace even when it names a place inside (os.Root).
		"abs_inside/a.go",
	} {
		t.Run("outside "+name, func(t *testing.T) {
			data, _, err := r.ReadFile(name, 1024)
			if !errors.Is(err, ErrLeavesWorkspace) {
				t.Fatalf("err = %v, want ErrLeavesWorkspace", err)
			}
			if !strings.Contains(err.Error(), "path leaves the workspace") {
				t.Fatalf("unclear error %q", err)
			}
			if strings.Contains(string(data), outsideText) {
				t.Fatal("outside content returned")
			}
		})
	}
}

func TestReadFileErrors(t *testing.T) {
	ws, _ := tree(t)
	r := mustOpen(t, ws)

	if _, _, err := r.ReadFile("missing.go", 1024); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if _, _, err := r.ReadFile("src", 1024); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory: %v", err)
	}
	if _, _, err := r.ReadFile("src/a.go", 3); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size cap: %v", err)
	}
	if _, _, err := r.ReadFile("src/a.go", 10); err != nil {
		t.Fatalf("exactly at the cap: %v", err)
	}
	if _, _, err := r.ReadFile("src/a.go/x", 1024); err == nil {
		t.Fatal("a file used as a directory must fail")
	}
}

func TestSymlinkLimits(t *testing.T) {
	ws, _ := tree(t)
	previous := "src/a.go"
	for i := range 8 {
		name := filepath.Join(ws, "l"+string(rune('0'+i)))
		mustSymlink(t, previous, name)
		previous = filepath.Base(name)
	}
	mustSymlink(t, previous, filepath.Join(ws, "ninth"))
	mustSymlink(t, "y", filepath.Join(ws, "x"))
	mustSymlink(t, "x", filepath.Join(ws, "y"))
	r := mustOpen(t, ws)

	if _, _, err := r.ReadFile("l7", 1024); err != nil {
		t.Fatalf("8 links: %v", err)
	}
	for _, name := range []string{"ninth", "x"} {
		if _, _, err := r.ReadFile(name, 1024); !errors.Is(err, syscall.ELOOP) {
			t.Fatalf("%s: %v, want ELOOP", name, err)
		}
	}
}

func TestSpecialFilesNeverBlock(t *testing.T) {
	ws, _ := tree(t)
	if err := syscall.Mkfifo(filepath.Join(ws, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "pipe", filepath.Join(ws, "pipelink"))
	sock := filepath.Join(ws, "s.sock")
	if len(sock) < 100 {
		l, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
	}
	r := mustOpen(t, ws)

	for _, name := range []string{"pipe", "pipelink", "s.sock"} {
		if name == "s.sock" && len(sock) >= 100 {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if err := noBlock(t, func() error { _, _, err := r.ReadFile(name, 1024); return err }); !errors.Is(err, ErrNotRegular) {
				t.Fatalf("read: %v", err)
			}
			if err := noBlock(t, func() error { return r.WriteFile(name, []byte("x"), 0o664) }); !errors.Is(err, ErrNotRegular) {
				t.Fatalf("write: %v", err)
			}
			if err := noBlock(t, func() error { _, err := r.ReadDir(name); return err }); err == nil {
				t.Fatal("ReadDir of a special file must fail")
			}
			if err := noBlock(t, func() error { _, err := fs.ReadFile(r.FS(), name); return err }); !errors.Is(err, ErrNotRegular) {
				t.Fatalf("FS read: %v", err)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	ws, out := tree(t)
	mustSymlink(t, "src/a.go", filepath.Join(ws, "alias.go"))
	mustSymlink(t, "../out/secret.txt", filepath.Join(ws, "leak.txt"))
	mustSymlink(t, "../out", filepath.Join(ws, "outdir"))
	mustSymlink(t, "../out/new.txt", filepath.Join(ws, "dangling_out"))
	r := mustOpen(t, ws)

	if err := r.WriteFile("new.txt", []byte("one"), 0o664); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteFile("new.txt", []byte("2"), 0o664); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(ws, "new.txt")); string(got) != "2" { //nolint:gosec // test file
		t.Fatalf("overwrite: %q", got)
	}
	if err := r.WriteFile("alias.go", []byte("changed"), 0o664); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(ws, "src", "a.go")); string(got) != "changed" { //nolint:gosec // test file
		t.Fatalf("write through an inside symlink: %q", got)
	}
	for _, name := range []string{"leak.txt", "outdir/secret.txt", "outdir/created.txt", "dangling_out", "../escape.txt"} {
		if err := r.WriteFile(name, []byte("pwned"), 0o664); !errors.Is(err, ErrLeavesWorkspace) {
			t.Fatalf("%s: %v, want ErrLeavesWorkspace", name, err)
		}
	}
	if err := r.MkdirAll("outdir/x/y", 0o770); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("MkdirAll through an outside symlink: %v", err)
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 1 {
		t.Fatalf("outside directory changed: %v", entries)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "secret.txt")); string(got) != outsideText { //nolint:gosec // test file
		t.Fatal("outside file changed")
	}
	if err := r.WriteFile("src", []byte("x"), 0o664); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory: %v", err)
	}
}

func TestWorkspaceDirectoryMustNotBeASymlink(t *testing.T) {
	ws, out := tree(t)
	link := filepath.Join(filepath.Dir(ws), "swapped")
	mustSymlink(t, out, link)
	for _, path := range []string{link, link + "/"} {
		if _, err := Open(path); !errors.Is(err, ErrLeavesWorkspace) {
			t.Fatalf("Open(%s) = %v, want ErrLeavesWorkspace", path, err)
		}
	}
	if _, err := Open(filepath.Join(ws, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing workspace: %v", err)
	}
}

// The tool user can put a FIFO in its workspace's place: opening the
// workspace must fail at once, not block the Go Core.
func TestOpenNeverBlocksOnAFIFOWorkspace(t *testing.T) {
	base := t.TempDir()
	fifo := filepath.Join(base, "ws")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := noBlock(t, func() error { _, err := Open(fifo); return err }); err == nil {
		t.Fatal("Open of a FIFO must fail")
	}
	if err := noBlock(t, func() error { _, err := OpenBelow(base, fifo); return err }); err == nil {
		t.Fatal("OpenBelow of a FIFO must fail")
	}
	mustMkdir(t, filepath.Join(base, "real", "sub"))
	mustSymlink(t, "real", filepath.Join(base, "link"))
	r, err := OpenBelow(base, filepath.Join(base, "link", "sub"))
	if err != nil {
		t.Fatalf("OpenBelow through a symlink inside the base: %v", err)
	}
	_ = r.Close()
	if _, err := OpenBelow(base, filepath.Dir(base)); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("OpenBelow above the base: %v", err)
	}
}

func TestStatReadDirAndWalk(t *testing.T) {
	ws, _ := tree(t)
	mustSymlink(t, "../out", filepath.Join(ws, "outdir"))
	mustSymlink(t, "src", filepath.Join(ws, "srclink"))
	mustSymlink(t, ".", filepath.Join(ws, "loop"))
	r := mustOpen(t, ws)

	if info, err := r.Stat("srclink"); err != nil || !info.IsDir() {
		t.Fatalf("Stat of an inside symlink: %v %v", info, err)
	}
	if _, err := r.Stat("outdir"); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("Stat outside: %v", err)
	}
	if info, err := r.Lstat("outdir"); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("Lstat: %v %v", info, err)
	}
	if _, err := r.ReadDir("outdir"); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("ReadDir outside: %v", err)
	}
	entries, err := r.ReadDir("srclink")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"a.go", "sub"}) {
		t.Fatalf("ReadDir(srclink) = %v", names)
	}

	var walked []string
	err = r.WalkDir(".", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		walked = append(walked, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".", "loop", "outdir", "src", "src/a.go", "src/sub", "src/sub/b.go", "srclink"}
	if !slices.Equal(walked, want) {
		t.Fatalf("WalkDir = %v, want %v (symlinks listed, never descended)", walked, want)
	}
}

func TestRemoveAndRenameNeverFollow(t *testing.T) {
	ws, out := tree(t)
	mustSymlink(t, "../out", filepath.Join(ws, "outdir"))
	mustSymlink(t, "src/a.go", filepath.Join(ws, "alias.go"))
	r := mustOpen(t, ws)

	if err := r.RemoveAll("outdir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "secret.txt")); err != nil {
		t.Fatal("removing a symlink removed its target")
	}
	if err := r.Rename("alias.go", "renamed.go"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(ws, "renamed.go")); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Fatal("rename must move the link itself")
	}
	if err := r.Rename("src/a.go", "../moved.go"); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("rename out: %v", err)
	}
	if err := r.RemoveAll("../out"); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("remove outside: %v", err)
	}
}

// TestSwapRaceConstruction shows why a swap cannot redirect a resolution:
// once a directory is opened, the rest of the path is resolved relative to
// its descriptor, so replacing it by a symlink afterwards changes nothing.
func TestSwapRaceConstruction(t *testing.T) {
	ws, out := tree(t)
	mustWrite(t, filepath.Join(out, "a.go"), outsideText)
	r := mustOpen(t, ws)

	sub, err := r.root.OpenRoot("src") // the walk holds src open
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Close() }()
	if err := os.Rename(filepath.Join(ws, "src"), filepath.Join(ws, "src-real")); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "../out", filepath.Join(ws, "src"))

	data, err := sub.ReadFile("a.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package a\n" {
		t.Fatalf("read %q through the swapped directory", data)
	}
	// A fresh resolution sees the symlink and refuses it.
	if _, _, err := r.ReadFile("src/a.go", 1024); !errors.Is(err, ErrLeavesWorkspace) {
		t.Fatalf("after the swap: %v", err)
	}
}

// The escape error os.Root returns is unexported; Root recognises it by its
// text. This pins the text against Go upgrades.
func TestEscapeErrorIsRecognised(t *testing.T) {
	ws, _ := tree(t)
	root, err := os.OpenRoot(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	_, err = root.Stat("../x")
	if !isEscape(err) {
		t.Fatalf("os.Root escape error %q is not recognised", err)
	}
}
