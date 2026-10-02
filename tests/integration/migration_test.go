//go:build integration

package integration_test

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
)

// migrationsDir is the embedded migrations source, relative to this package.
const migrationsDir = "../../internal/adapter/postgres/migrations"

// TestMigrationUpDown applies all migrations, rolls them all back, then re-applies.
// This verifies that every migration's Down section works correctly.
func TestMigrationUpDown(t *testing.T) {
	dsn := scratchDatabase(t)
	ctx := context.Background()
	totalMigrations, migrationCount := latestMigrationVersion(t)

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
	if err := postgres.RollbackMigrations(ctx, dsn, migrationCount); err != nil {
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

// latestMigrationVersion returns the highest version prefix (NNN_name.sql) of
// the migration files and how many there are (versions may leave gaps:
// numbers reserved for work in progress), so the test follows new
// migrations automatically.
func latestMigrationVersion(t *testing.T) (latest int64, count int) {
	t.Helper()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("migration %s: version prefix: %v", e.Name(), err)
		}
		latest = max(latest, v)
		count++
	}
	if latest == 0 {
		t.Fatalf("no migrations found in %s", migrationsDir)
	}
	return latest, count
}

// scratchDatabase creates an empty database next to the test database and
// returns its DSN; it is dropped when the test ends. Rolling back every
// migration drops all tables, which must never happen in the shared database.
func scratchDatabase(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	u, err := url.Parse(testDSN())
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatalf("DATABASE_URL must be a postgres:// URL, got %q (parse error: %v)", testDSN(), err)
	}
	name := "codeforge_migtest_" + strings.ReplaceAll(uuid.NewString()[:13], "-", "")
	quoted := pgx.Identifier{name}.Sanitize()

	if _, err := testPool.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatalf("create scratch database %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("drop scratch database %s: %v", name, err)
		}
	})

	u.Path = "/" + name
	return u.String()
}
