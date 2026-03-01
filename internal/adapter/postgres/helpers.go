package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// scannable abstracts pgx.Row and pgx.Rows for shared scan helpers.
type scannable interface {
	Scan(dest ...any) error
}

// tenantFromCtx extracts the tenant ID from the request context.
// All tenant-scoped queries must use this to enforce isolation.
func tenantFromCtx(ctx context.Context) string {
	return middleware.TenantIDFromContext(ctx)
}

// nullIfEmpty returns nil for empty strings (for nullable UUID columns).
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullTime converts a zero time to nil for nullable DB columns.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// pgTextArray converts a string slice to a pgx-compatible text array.
// nil slices become empty arrays to avoid SQL NULL.
func pgTextArray(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// orEmpty returns items unchanged if non-nil, or an empty slice if nil.
// Useful to ensure JSON serialization produces [] instead of null.
func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// notFoundWrap checks whether err is pgx.ErrNoRows and, if so, wraps
// domain.ErrNotFound with the given message. Otherwise it wraps the
// original error.
func notFoundWrap(err error, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", msg, domain.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// execExpectOne verifies that an Exec affected exactly one row. If not
// (and err is nil), it returns domain.ErrNotFound with the given message.
func execExpectOne(tag pgconn.CommandTag, err error, format string, args ...any) error {
	if err != nil {
		return fmt.Errorf(fmt.Sprintf(format, args...)+": %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf(fmt.Sprintf(format, args...)+": %w", domain.ErrNotFound)
	}
	return nil
}
