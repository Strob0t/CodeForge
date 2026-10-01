package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// S3-F review C3: GetProjectByRepoName matched a substring of the
// repository URL with LIMIT 1 and no order, so a webhook for acme/app could
// hit acme/app-private (and the PM webhook then answered 404).
// FindProjectByRepo matches host and full path exactly, in the caller's
// tenant.
func TestStore_FindProjectByRepo_ExactHostAndPath(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)
	owner := "acme-" + uuid.New().String()[:8]

	create := func(ctx context.Context, name, url string) *project.Project {
		t.Helper()
		p, err := store.CreateProject(ctx, &project.CreateRequest{Name: name, RepoURL: url, Provider: "github"})
		if err != nil {
			t.Fatalf("CreateProject %s: %v", name, err)
		}
		t.Cleanup(func() { _ = store.DeleteProject(ctx, p.ID) })
		return p
	}
	// The private repository is created first: a substring match with
	// LIMIT 1 found it for acme/app.
	create(ctxA, "private", "https://github.com/"+owner+"/app-private.git")
	app := create(ctxA, "app", "https://github.com/"+owner+"/app.git")
	create(ctxA, "enterprise", "https://ghe.example.com/"+owner+"/app.git")
	gl := create(ctxA, "gitlab", "git@gitlab.example.com:"+owner+"/sub/app.git")

	got, err := store.FindProjectByRepo(ctxA, "github.com", owner+"/app")
	if err != nil || got.ID != app.ID {
		t.Fatalf("FindProjectByRepo(github.com, %s/app) = %v, %v; want project %s", owner, got, err, app.ID)
	}
	got, err = store.FindProjectByRepo(ctxA, "GitLab.example.com", owner+"/sub/app")
	if err != nil || got.ID != gl.ID {
		t.Fatalf("FindProjectByRepo(gitlab) = %v, %v; want project %s", got, err, gl.ID)
	}
	for _, tc := range [][2]string{
		{"github.com", owner + "/ap"},
		{"github.com", owner + "/app-priv"},
		{"gitlab.example.com", owner + "/app"},
		{"other.example.com", owner + "/app"},
	} {
		if _, err := store.FindProjectByRepo(ctxA, tc[0], tc[1]); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("FindProjectByRepo(%s, %s) = %v, want ErrNotFound", tc[0], tc[1], err)
		}
	}
	// Another tenant does not see tenant A's projects.
	if _, err := store.FindProjectByRepo(ctxB, "github.com", owner+"/app"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tenant B found tenant A's project: %v", err)
	}
}
