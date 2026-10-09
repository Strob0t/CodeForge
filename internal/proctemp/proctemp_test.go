package proctemp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S3 follow-up 1e: the Go Core's private temporary files (per-run checkpoint
// indexes, the svn client config) live under one directory per process;
// at startup the directories of processes that no longer run are removed.

func TestDirFor_OnePrivateDirectoryPerProcess(t *testing.T) {
	base := t.TempDir()
	dir, err := dirFor(base)
	if err != nil {
		t.Fatalf("dirFor: %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("%s: %v %v, want a 0700 directory", dir, info, err)
	}
	name := filepath.Base(dir)
	if !strings.HasPrefix(name, fmt.Sprintf("%s%d-", prefix, os.Getpid())) {
		t.Fatalf("name %s does not carry the process", name)
	}
	again, err := dirFor(base)
	if err != nil || again != dir {
		t.Fatalf("second dirFor = %s, %v; want the same directory", again, err)
	}
}

func TestRemoveStale_RemovesOnlyDirectoriesOfEndedProcesses(t *testing.T) {
	base := t.TempDir()
	own, err := dirFor(base)
	if err != nil {
		t.Fatal(err)
	}
	mkdir := func(name string) string {
		p := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Join(p, "checkpoints-1"), 0o700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	start, ok := processStart(os.Getpid())
	if !ok {
		t.Skip("no /proc on this system")
	}
	ended := mkdir(prefix + "999999999-12345")                           // no such process
	reused := mkdir(fmt.Sprintf("%s%d-%s1", prefix, os.Getpid(), start)) // our PID, another start: an earlier process
	other := mkdir("codeforge-checkpoints-123")                          // not ours
	malformed := mkdir(prefix + "abc")                                   // not a process directory
	outside := t.TempDir()
	link := filepath.Join(base, prefix+"999999998-1")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	var foreign string
	if os.Getuid() == 0 {
		foreign = mkdir(prefix + "999999997-1")
		if err := os.Chown(foreign, 4242, 4242); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := RemoveStale(base)
	if err != nil {
		t.Fatalf("RemoveStale: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d directories, want 2", removed)
	}
	for _, gone := range []string{ended, reused} {
		if _, err := os.Lstat(gone); !os.IsNotExist(err) {
			t.Errorf("%s not removed", gone)
		}
	}
	for _, kept := range []string{own, other, malformed, link, outside} {
		if _, err := os.Lstat(kept); err != nil {
			t.Errorf("%s removed: %v", kept, err)
		}
	}
	if foreign != "" {
		if _, err := os.Lstat(foreign); err != nil {
			t.Errorf("another user's directory removed: %v", err)
		}
	}
}
