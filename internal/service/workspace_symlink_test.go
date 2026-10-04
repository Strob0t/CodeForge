//go:build unix

package service

// In-process workspace readers and writers of the Go Core never follow a
// symlink out of the workspace and never block on a FIFO (KI-95). Agents
// write the workspaces; the Go Core reads them with its own rights.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

const outsideSecret = "outside secret roadmap auth jwt"

// symlinkWorkspace builds a workspace with symlinks into and out of it, a
// FIFO, and a directory outside it that holds a secret.
func symlinkWorkspace(t *testing.T) (ws, out string) {
	t.Helper()
	base := t.TempDir()
	ws = filepath.Join(base, "ws")
	out = filepath.Join(base, "out")
	for _, dir := range []string{filepath.Join(ws, "src"), filepath.Join(ws, "docs"), out} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(out, "secret.txt"), outsideSecret)
	writeTestFile(t, filepath.Join(ws, "src", "a.go"), "package a\n")
	links := map[string]string{
		"leak.txt":  "../out/secret.txt",
		"outdir":    "../out",
		"alias.go":  "src/a.go",
		"srclink":   "src",
		"abs.txt":   filepath.Join(out, "secret.txt"),
		"notes.txt": "../out/secret.txt",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(ws, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ws, out
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// within fails the test when fn blocks (a FIFO opened without O_NONBLOCK).
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

func assertOutsideUnchanged(t *testing.T, out string) {
	t.Helper()
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outside directory changed: %v %v", entries, err)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "secret.txt")); string(got) != outsideSecret { //nolint:gosec // test file
		t.Fatal("outside file changed")
	}
}

func TestFileService_StaysInsideTheWorkspace(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	svc := newTestFileService(ws)
	ctx := context.Background()

	for _, name := range []string{"leak.txt", "abs.txt", "outdir/secret.txt", "../out/secret.txt", "srclink/../../out/secret.txt"} {
		_, err := svc.ReadFile(ctx, "p1", name)
		if !errors.Is(err, workspacefs.ErrLeavesWorkspace) || !errors.Is(err, domain.ErrValidation) {
			t.Errorf("ReadFile(%s) = %v, want a validation error that the path leaves the workspace", name, err)
		}
	}
	for _, name := range []string{"alias.go", "srclink/a.go", "/src/a.go"} {
		content, err := svc.ReadFile(ctx, "p1", name)
		if err != nil || content.Content != "package a\n" {
			t.Errorf("ReadFile(%s) = %v, %v", name, content, err)
		}
	}
	if err := within(t, func() error { _, err := svc.ReadFile(ctx, "p1", "pipe"); return err }); !errors.Is(err, workspacefs.ErrNotRegular) {
		t.Errorf("ReadFile(pipe) = %v, want ErrNotRegular", err)
	}

	if _, err := svc.ListDirectory(ctx, "p1", "outdir"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Errorf("ListDirectory(outdir) = %v", err)
	}
	tree, err := svc.ListTree(ctx, "p1", 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tree {
		if strings.HasPrefix(e.Path, "outdir/") || strings.HasPrefix(e.Path, "srclink/") {
			t.Errorf("ListTree descended into a symlink: %s", e.Path)
		}
	}

	for _, name := range []string{"leak.txt", "outdir/new.txt", "outdir/x/y.txt", "../escape.txt"} {
		if err := svc.WriteFile(ctx, "p1", name, "pwned"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
			t.Errorf("WriteFile(%s) = %v", name, err)
		}
	}
	if err := within(t, func() error { return svc.WriteFile(ctx, "p1", "pipe", "x") }); !errors.Is(err, workspacefs.ErrNotRegular) {
		t.Errorf("WriteFile(pipe) = %v", err)
	}
	if err := svc.RenameFile(ctx, "p1", "src/a.go", "../moved.go"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Errorf("RenameFile out = %v", err)
	}
	if err := svc.RenameFile(ctx, "p1", "alias.go", "renamed.go"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(ws, "renamed.go")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("RenameFile must move the symlink itself")
	}
	if err := svc.DeleteFile(ctx, "p1", "outdir"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteFile(ctx, "p1", "."); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("DeleteFile(.) = %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "src", "a.go")); err != nil {
		t.Error("deleting or renaming a symlink touched its target")
	}
	assertOutsideUnchanged(t, out)
}

func TestFileService_RefusesASymlinkedWorkspace(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	swapped := filepath.Join(filepath.Dir(ws), "swapped")
	if err := os.Symlink(out, swapped); err != nil {
		t.Fatal(err)
	}
	svc := newTestFileService(swapped)
	if _, err := svc.ReadFile(context.Background(), "p1", "secret.txt"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Fatalf("ReadFile in a symlinked workspace = %v", err)
	}
}

func TestGoalDiscovery_StaysInsideTheWorkspace(t *testing.T) {
	ws, _ := symlinkWorkspace(t)
	for name, target := range map[string]string{"CLAUDE.md": "../out/secret.txt", ".cursorrules": "leak.txt"} {
		if err := os.Symlink(target, filepath.Join(ws, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "README.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ws, "docs", "contributing.md"), "Be kind.")
	if err := os.Symlink("docs/contributing.md", filepath.Join(ws, "CONTRIBUTING.md")); err != nil {
		t.Fatal(err)
	}

	goals := within(t, func() []DetectedGoal { return testGoalSvc().detectGoalFiles(context.Background(), ws) })
	var sources []string
	for i := range goals {
		if strings.Contains(goals[i].Content, "outside secret") {
			t.Fatalf("goal %q holds outside content", goals[i].Title)
		}
		sources = append(sources, goals[i].Source)
	}
	if strings.Join(sources, ",") != "contributing" {
		t.Fatalf("sources = %v, want only the inside symlink (contributing)", sources)
	}
}

func TestContextScoring_StaysInsideTheWorkspace(t *testing.T) {
	ws, _ := symlinkWorkspace(t)
	writeTestFile(t, filepath.Join(ws, "src", "auth.go"), "package auth // jwt auth roadmap")
	svc := NewContextOptimizerService(&mockStore{}, &config.Orchestrator{},
		&config.Limits{MaxFiles: 50, MaxFileSize: 32768})

	entries := within(t, func() []string {
		var paths []string
		for _, e := range svc.scanWorkspaceFiles(context.Background(), ws, "outside secret roadmap auth jwt") {
			if strings.Contains(e.Content, "outside secret") {
				t.Errorf("context entry %s holds outside content", e.Path)
			}
			paths = append(paths, e.Path)
		}
		return paths
	})
	if strings.Join(entries, ",") != "src/auth.go" {
		t.Fatalf("entries = %v, want only src/auth.go", entries)
	}
}

func TestStackDetection_StaysInsideTheWorkspace(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	writeTestFile(t, filepath.Join(out, "package.json"), `{"dependencies":{"react":"18"}}`)
	if err := os.Symlink("../out/package.json", filepath.Join(ws, "package.json")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "go.mod"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := within(t, func() *project.StackDetectionResult {
		res, err := project.ScanWorkspace(ws)
		if err != nil {
			t.Error(err)
		}
		return res
	})
	for _, lang := range result.Languages {
		if len(lang.Frameworks) > 0 {
			t.Fatalf("frameworks read through a symlink out of the workspace: %v", lang)
		}
	}
	cmds := within(t, func() project.GateCommands {
		return detectedGateCommands(&project.Project{ID: "p1", WorkspacePath: ws})
	})
	_ = cmds // reaching here means the FIFO did not block the gate detection

	// detect-stack by path: no symlink on the way leads out of the tenant area.
	svc := NewProjectService(&mockStore{}, filepath.Dir(ws))
	ctx := tenantctx.WithTenant(context.Background(), "ws")
	if _, err := svc.DetectStackByPath(ctx, filepath.Join(ws, "outdir"), false); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Fatalf("DetectStackByPath through a symlink out = %v", err)
	}
	if _, err := svc.DetectStackByPath(ctx, filepath.Join(ws, "srclink"), false); err != nil {
		t.Fatalf("DetectStackByPath through a symlink inside = %v", err)
	}
}

func TestWorkspaceHealth_DoesNotWalkOutside(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	writeTestFile(t, filepath.Join(out, "big.bin"), strings.Repeat("x", 1<<20))
	svc := NewProjectService(&mockStore{projects: []project.Project{{ID: "p1", WorkspacePath: ws}}}, "")
	info, err := svc.WorkspaceHealth(context.Background(), "p1")
	if err != nil || !info.Exists {
		t.Fatalf("WorkspaceHealth = %v, %v", info, err)
	}
	if info.DiskUsageBytes >= 1<<20 {
		t.Fatalf("disk usage %d counts files outside the workspace", info.DiskUsageBytes)
	}
}

// roadmapStore returns an empty roadmap so SyncToSpecFile renders one.
type roadmapStore struct {
	mockStore
}

func (m *roadmapStore) GetRoadmapByProject(_ context.Context, projectID string) (*roadmap.Roadmap, error) {
	return &roadmap.Roadmap{ID: "r1", ProjectID: projectID, Title: "Plan"}, nil
}

func TestRoadmap_StaysInsideTheWorkspace(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	if err := os.Symlink("../out/secret.txt", filepath.Join(ws, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "docs", "plan.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &roadmapStore{mockStore{projects: []project.Project{{ID: "p1", WorkspacePath: ws}}}}
	svc := NewRoadmapService(store, nil, nil, nil)

	result := within(t, func() *roadmap.DetectionResult {
		res, err := svc.AutoDetect(context.Background(), "p1")
		if err != nil {
			t.Error(err)
		}
		return res
	})
	for _, marker := range result.FileMarkers {
		if marker == "ROADMAP.md" {
			t.Fatal("a symlink out of the workspace counted as a roadmap file")
		}
	}

	err := svc.SyncToSpecFile(context.Background(), "p1")
	if !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Fatalf("SyncToSpecFile through a symlink out = %v", err)
	}
	assertOutsideUnchanged(t, out)
}

func TestCheckpointSnapshots_StayInsideTheWorkspace(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	svc := NewCheckpointService(git.NewPool(1))

	if err := svc.Store("r1", "c0", ws, "leak.txt"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Fatalf("Store(leak.txt) = %v", err)
	}
	if err := within(t, func() error { return svc.Store("r1", "c0", ws, "pipe") }); !errors.Is(err, workspacefs.ErrNotRegular) {
		t.Fatalf("Store(pipe) = %v", err)
	}
	if err := svc.Store("r1", "c1", ws, "src/a.go"); err != nil {
		t.Fatal(err)
	}
	// The agent swaps the file for a symlink out of the workspace before the revert.
	target := filepath.Join(ws, "src", "a.go")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../out/secret.txt", target); err != nil {
		t.Fatal(err)
	}
	if err := svc.Revert("r1", "c1"); !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
		t.Fatalf("Revert through a symlink out = %v", err)
	}
	assertOutsideUnchanged(t, out)
}

func TestGitIndexAndAttributes_NeverBlockOrLeave(t *testing.T) {
	ws, _ := symlinkWorkspace(t)
	gitDir := filepath.Join(ws, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(gitDir, "index"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "private.index")
	err := within(t, func() error { return seedIndex(&git.Repo{Dir: ws, GitDir: gitDir}, dst) })
	if !errors.Is(err, workspacefs.ErrNotRegular) {
		t.Fatalf("seedIndex with a FIFO index = %v", err)
	}

	root, err := workspacefs.Open(ws)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	writeTestFile(t, filepath.Join(filepath.Dir(ws), "out", "attrs"), "* filter=lfs")
	if err := os.Symlink("../out/attrs", filepath.Join(ws, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "src", ".gitattributes"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ws, "docs", ".gitattributes"), "*.bin filter=crypt")
	for name, want := range map[string]bool{".gitattributes": false, "src/.gitattributes": false, "docs/.gitattributes": true} {
		if got := within(t, func() bool { return attributesMentionFilter(root, name) }); got != want {
			t.Errorf("attributesMentionFilter(%s) = %v, want %v", name, got, want)
		}
	}
}
