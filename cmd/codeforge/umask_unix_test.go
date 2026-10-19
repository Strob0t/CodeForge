//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSetWorkspaceUmask_FilesStayGroupWritable(t *testing.T) {
	previous := setWorkspaceUmask()
	t.Cleanup(func() { syscall.Umask(previous) })

	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "d")
	if err := os.Mkdir(sub, 0o777); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{file: 0o664, sub: 0o775} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s: mode %o, want %o", filepath.Base(path), got, want)
		}
	}
}
