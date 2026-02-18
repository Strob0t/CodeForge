package service_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/service"
)

// deliverMockStore is a minimal Store mock for delivery tests.
type deliverMockStore struct {
	runtimeMockStore
	proj *project.Project
}

func (m *deliverMockStore) GetProject(_ context.Context, _ string) (*project.Project, error) {
	if m.proj != nil {
		return m.proj, nil
	}
	return nil, errMockNotFound
}

// initDeliverTestRepo creates a temporary git repo with one commit.
func initDeliverTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// git init + initial commit
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %s: %v", args, out, err)
		}
	}

	// Create a file and commit
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit setup %v: %s: %v", args, out, err)
		}
	}

	return dir
}

func TestDeliver_NoneMode(t *testing.T) {
	svc := service.NewDeliverService(nil, &config.Runtime{}, git.NewPool(5))
	r := &run.Run{ID: "run-12345678", DeliverMode: ""}

	result, err := svc.Deliver(context.Background(), r, "task title")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != run.DeliverModeNone {
		t.Errorf("expected none, got %q", result.Mode)
	}
}

func TestDeliver_Patch(t *testing.T) {
	dir := initDeliverTestRepo(t)

	// Make a change
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &deliverMockStore{
		proj: &project.Project{ID: "proj-1", WorkspacePath: dir},
	}
	svc := service.NewDeliverService(store, &config.Runtime{
		DeliveryCommitPrefix: "test:",
	}, git.NewPool(5))

	r := &run.Run{
		ID:          "run-abcd1234",
		ProjectID:   "proj-1",
		DeliverMode: run.DeliverModePatch,
	}

	result, err := svc.Deliver(context.Background(), r, "fix bug")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != run.DeliverModePatch {
		t.Errorf("expected patch, got %q", result.Mode)
	}
	if result.PatchPath == "" {
		t.Error("expected patch path to be set")
	}

	// Verify patch file exists and contains diff
	data, err := os.ReadFile(result.PatchPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("patch file is empty")
	}
}

func TestDeliver_CommitLocal(t *testing.T) {
	dir := initDeliverTestRepo(t)

	// Make a change
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("committed"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &deliverMockStore{
		proj: &project.Project{ID: "proj-1", WorkspacePath: dir},
	}
	svc := service.NewDeliverService(store, &config.Runtime{
		DeliveryCommitPrefix: "codeforge:",
	}, git.NewPool(5))

	r := &run.Run{
		ID:          "run-abcd1234",
		ProjectID:   "proj-1",
		DeliverMode: run.DeliverModeCommitLocal,
	}

	result, err := svc.Deliver(context.Background(), r, "add feature")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != run.DeliverModeCommitLocal {
		t.Errorf("expected commit-local, got %q", result.Mode)
	}
	if result.CommitHash == "" {
		t.Error("expected commit hash to be set")
	}

	// Verify commit message
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	msg := string(out)
	if msg == "" {
		t.Error("commit message is empty")
	}
}

func TestDeliver_Branch(t *testing.T) {
	dir := initDeliverTestRepo(t)

	// Make a change
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("branched"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := &deliverMockStore{
		proj: &project.Project{ID: "proj-1", WorkspacePath: dir},
	}
	svc := service.NewDeliverService(store, &config.Runtime{
		DeliveryCommitPrefix: "codeforge:",
	}, git.NewPool(5))

	r := &run.Run{
		ID:          "run-abcd1234",
		ProjectID:   "proj-1",
		DeliverMode: run.DeliverModeBranch,
	}

	result, err := svc.Deliver(context.Background(), r, "branch work")
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != run.DeliverModeBranch {
		t.Errorf("expected branch, got %q", result.Mode)
	}
	if result.BranchName == "" {
		t.Error("expected branch name to be set")
	}
	if result.CommitHash == "" {
		t.Error("expected commit hash from branch delivery")
	}

	// Verify branch exists
	cmd := exec.Command("git", "branch", "--list", result.BranchName)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Error("branch was not created")
	}
}

func TestDeliver_NoWorkspacePath(t *testing.T) {
	store := &deliverMockStore{
		proj: &project.Project{ID: "proj-1", WorkspacePath: ""},
	}
	svc := service.NewDeliverService(store, &config.Runtime{}, git.NewPool(5))

	r := &run.Run{
		ID:          "run-abcd1234",
		ProjectID:   "proj-1",
		DeliverMode: run.DeliverModePatch,
	}

	_, err := svc.Deliver(context.Background(), r, "task")
	if err == nil {
		t.Error("expected error for missing workspace_path")
	}
}
