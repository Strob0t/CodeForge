// Package postgres provides the PostgreSQL connection pool and migration runner.
package postgres

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver for database/sql (needed by goose)
	"github.com/pressly/goose/v3"

	"github.com/Strob0t/CodeForge/internal/config"
)

//go:embed migrations/*.sql
var migrations embed.FS

// NewPool creates a pgxpool connection pool from a config.Postgres struct.
func NewPool(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = cfg.HealthCheck

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}

// RunMigrations applies all pending goose migrations from the embedded SQL files.
func RunMigrations(ctx context.Context, dsn string) error {
	goose.SetBaseFS(migrations)

	db, err := goose.OpenDBWithDriver("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// RollbackMigrations rolls back the last N migrations.
func RollbackMigrations(ctx context.Context, dsn string, steps int) error {
	goose.SetBaseFS(migrations)

	db, err := goose.OpenDBWithDriver("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db for rollback: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	for range steps {
		if err := goose.DownContext(ctx, db, "migrations"); err != nil {
			return fmt.Errorf("rollback: %w", err)
		}
	}

	return nil
}

// MigrationVersion returns the current migration version.
func MigrationVersion(ctx context.Context, dsn string) (int64, error) {
	goose.SetBaseFS(migrations)

	db, err := goose.OpenDBWithDriver("pgx", dsn)
	if err != nil {
		return 0, fmt.Errorf("open db for version: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return 0, fmt.Errorf("set dialect: %w", err)
	}

	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("get version: %w", err)
	}

	return version, nil
}
