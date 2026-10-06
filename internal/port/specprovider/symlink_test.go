//go:build unix

package specprovider_test

// Spec providers read workspace files through workspacefs (KI-95): a spec
// file or directory that is a symlink out of the workspace is neither listed
// nor read, and a FIFO never blocks a detection or an import. (The roadmap
// service writes spec files, through workspacefs too: KI-203.)

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/Strob0t/CodeForge/internal/adapter/autospec"
	_ "github.com/Strob0t/CodeForge/internal/adapter/markdownspec"
	_ "github.com/Strob0t/CodeForge/internal/adapter/openspec"
	_ "github.com/Strob0t/CodeForge/internal/adapter/speckit"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

const outsideTitle = "OUTSIDE SECRET"

func within[T any](t *testing.T, fn func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- fn() }()
	select {
	case v := <-done:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("the call blocked")
		var zero T
		return zero
	}
}

func TestSpecProviders_StayInsideTheWorkspace(t *testing.T) {
	cases := []struct {
		provider string
		dir      string // spec directory ("" for files in the workspace root)
		inside   string // a regular spec file
		ext      string
		content  string
	}{
		{"openspec", "openspec", "openspec/real.yaml", ".yaml", "title: Inside\n"},
		{"speckit", ".specify", ".specify/real.md", ".md", "# Inside\n"},
		{"autospec", "specs", "specs/spec.yaml", ".yaml", "title: Inside\n"},
		{"markdown", "docs", "docs/TODO.md", ".md", "# Inside\n- [ ] item\n"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			base := t.TempDir()
			ws := filepath.Join(base, "ws")
			out := filepath.Join(base, "out")
			for _, d := range []string{filepath.Join(ws, tc.dir), out} {
				if err := os.MkdirAll(d, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			secret := filepath.Join(out, "secret"+tc.ext)
			if err := os.WriteFile(secret, []byte("title: "+outsideTitle+"\n# "+outsideTitle+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(ws, tc.inside), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			leak := filepath.Join(tc.dir, "leak"+tc.ext)
			pipe := filepath.Join(tc.dir, "pipe"+tc.ext)
			if tc.provider == "markdown" { // candidates are fixed names
				leak, pipe = "ROADMAP.md", "TODO.md"
			}
			rel, _ := filepath.Rel(filepath.Dir(filepath.Join(ws, leak)), secret)
			if err := os.Symlink(rel, filepath.Join(ws, leak)); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(filepath.Join(ws, pipe), 0o600); err != nil {
				t.Fatal(err)
			}
			p, err := specprovider.New(tc.provider, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()

			specs := within(t, func() []specprovider.Spec {
				s, listErr := p.ListSpecs(ctx, ws)
				if listErr != nil {
					t.Error(listErr)
				}
				return s
			})
			var paths []string
			for _, s := range specs {
				if strings.Contains(s.Title, outsideTitle) {
					t.Errorf("spec %s titled from outside the workspace", s.Path)
				}
				paths = append(paths, s.Path)
			}
			if strings.Join(paths, ",") != tc.inside {
				t.Errorf("ListSpecs = %v, want only %s", paths, tc.inside)
			}
			if found := within(t, func() bool { ok, _ := p.Detect(ctx, ws); return ok }); !found {
				t.Error("Detect = false")
			}

			for _, name := range []string{leak, "../out/secret" + tc.ext, secret} {
				if _, readErr := p.ReadSpec(ctx, ws, name); !errors.Is(readErr, workspacefs.ErrLeavesWorkspace) {
					t.Errorf("ReadSpec(%s) = %v, want ErrLeavesWorkspace", name, readErr)
				}
			}
			if readErr := within(t, func() error { _, e := p.ReadSpec(ctx, ws, pipe); return e }); !errors.Is(readErr, workspacefs.ErrNotRegular) {
				t.Errorf("ReadSpec(%s) = %v, want ErrNotRegular", pipe, readErr)
			}
		})
	}
}

func TestSpecProviders_SpecDirectorySymlinkedOut(t *testing.T) {
	for provider, dir := range map[string]string{"openspec": "openspec", "speckit": ".specify", "autospec": "specs"} {
		t.Run(provider, func(t *testing.T) {
			base := t.TempDir()
			ws := filepath.Join(base, "ws")
			out := filepath.Join(base, "out")
			for _, d := range []string{ws, out} {
				if err := os.MkdirAll(d, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"spec.yaml", "x.md"} {
				if err := os.WriteFile(filepath.Join(out, name), []byte("title: "+outsideTitle), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink("../out", filepath.Join(ws, dir)); err != nil {
				t.Fatal(err)
			}
			p, err := specprovider.New(provider, nil)
			if err != nil {
				t.Fatal(err)
			}
			if ok, _ := p.Detect(context.Background(), ws); ok {
				t.Error("Detect followed a spec directory out of the workspace")
			}
			if specs, _ := p.ListSpecs(context.Background(), ws); len(specs) != 0 {
				t.Errorf("ListSpecs = %v", specs)
			}
		})
	}
}
