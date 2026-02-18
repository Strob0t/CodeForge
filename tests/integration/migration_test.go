//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
)

// TestMigrationUpDown applies all migrations, rolls them all back, then re-applies.
// This verifies that every migration's Down section works correctly.
func TestMigrationUpDown(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=disable"
	}

	ctx := context.Background()
	const totalMigrations = 15

	// Step 1: Apply all migrations (up to latest)
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("RunMigrations (up): %v", err)
	}

	v, err := postgres.MigrationVersion(ctx, dsn)
	if err != nil {
		t.Fatalf("MigrationVersion after up: %v", err)
	}
	if v != totalMigrations {
		t.Fatalf("expected version %d after up, got %d", totalMigrations, v)
	}

	// Step 2: Roll back all migrations
	if err := postgres.RollbackMigrations(ctx, dsn, totalMigrations); err != nil {
		t.Fatalf("RollbackMigrations (down all): %v", err)
	}

	v, err = postgres.MigrationVersion(ctx, dsn)
	if err != nil {
		t.Fatalf("MigrationVersion after rollback: %v", err)
	}
	if v != 0 {
		t.Fatalf("expected version 0 after full rollback, got %d", v)
	}

	// Step 3: Re-apply all (idempotency check)
	if err := postgres.RunMigrations(ctx, dsn); err != nil {
		t.Fatalf("RunMigrations (re-up): %v", err)
	}

	v, err = postgres.MigrationVersion(ctx, dsn)
	if err != nil {
		t.Fatalf("MigrationVersion after re-up: %v", err)
	}
	if v != totalMigrations {
		t.Fatalf("expected version %d after re-up, got %d", totalMigrations, v)
	}
}
