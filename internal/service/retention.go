package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// retentionBatchSize limits the number of rows deleted per batch to avoid
// long-running transactions and lock contention.
const retentionBatchSize = 1000

// RetentionService enforces data retention policies by periodically removing
// records older than configured durations (GDPR Article 5(1)(e) — storage limitation).
type RetentionService struct {
	store  database.Store
	config config.Retention
}

// NewRetentionService creates a retention service with the given store and config.
func NewRetentionService(store database.Store, cfg config.Retention) *RetentionService {
	return &RetentionService{store: store, config: cfg}
}

// RunCleanup deletes expired records across all retention categories for every tenant.
// Each category is processed independently so a failure in one does not
// block the others. Deletions are batched (LIMIT 1000) to avoid long locks.
func (s *RetentionService) RunCleanup(ctx context.Context) error {
	tenants, err := s.store.ListTenants(ctx)
	if err != nil {
		slog.Error("retention: failed to list tenants", "error", err)
		// Fall back to default tenant so cleanup still runs for single-tenant deployments.
		tenants = nil
	}

	// Build list of tenant IDs to iterate. Always include the default tenant
	// in case data exists outside explicitly-created tenants.
	tenantIDs := make([]string, 0, len(tenants)+1)
	seen := make(map[string]struct{}, len(tenants)+1)
	for i := range tenants {
		if _, ok := seen[tenants[i].ID]; !ok {
			tenantIDs = append(tenantIDs, tenants[i].ID)
			seen[tenants[i].ID] = struct{}{}
		}
	}
	if _, ok := seen[tenantctx.DefaultTenantID]; !ok {
		tenantIDs = append(tenantIDs, tenantctx.DefaultTenantID)
	}

	now := time.Now().UTC()
	for _, tid := range tenantIDs {
		tctx := tenantctx.WithTenant(ctx, tid)
		s.cleanupSessions(tctx, now)
		s.cleanupConversations(tctx, now)
		s.cleanupRuns(tctx, now)
		s.cleanupAuditEntries(tctx, now)
	}

	return nil
}

func (s *RetentionService) cleanupSessions(ctx context.Context, now time.Time) {
	if s.config.Sessions <= 0 {
		return
	}
	before := now.Add(-s.config.Sessions)
	total := s.deleteBatched(ctx, "sessions", before, s.store.DeleteExpiredSessions)
	if total > 0 {
		slog.Info("retention: cleaned up expired sessions", "deleted", total, "older_than", before.Format(time.RFC3339))
	}
}

func (s *RetentionService) cleanupConversations(ctx context.Context, now time.Time) {
	if s.config.Conversations <= 0 {
		return
	}
	before := now.Add(-s.config.Conversations)
	total := s.deleteBatched(ctx, "conversations", before, s.store.DeleteExpiredConversations)
	if total > 0 {
		slog.Info("retention: cleaned up expired conversations", "deleted", total, "older_than", before.Format(time.RFC3339))
	}
}

func (s *RetentionService) cleanupRuns(ctx context.Context, now time.Time) {
	if s.config.CostRecords <= 0 {
		return
	}
	before := now.Add(-s.config.CostRecords)
	total := s.deleteBatched(ctx, "runs", before, s.store.DeleteExpiredRuns)
	if total > 0 {
		slog.Info("retention: cleaned up expired runs", "deleted", total, "older_than", before.Format(time.RFC3339))
	}
}

func (s *RetentionService) cleanupAuditEntries(ctx context.Context, now time.Time) {
	if s.config.AuditEntries <= 0 {
		return
	}
	before := now.Add(-s.config.AuditEntries)
	total := s.deleteBatched(ctx, "audit_entries", before, s.store.DeleteExpiredAuditEntries)
	if total > 0 {
		slog.Info("retention: cleaned up expired audit entries", "deleted", total, "older_than", before.Format(time.RFC3339))
	}
}

// deleteBatched repeatedly calls the delete function in batches until no more
// rows are affected or the context is cancelled.
func (s *RetentionService) deleteBatched(
	ctx context.Context,
	table string,
	before time.Time,
	deleteFn func(ctx context.Context, before time.Time, batchSize int) (int64, error),
) int64 {
	var total int64
	for {
		if ctx.Err() != nil {
			slog.Warn("retention: context cancelled during cleanup", "table", table, "deleted_so_far", total)
			break
		}
		n, err := deleteFn(ctx, before, retentionBatchSize)
		if err != nil {
			slog.Error("retention: batch delete failed", "table", table, "error", err)
			break
		}
		total += n
		if n < int64(retentionBatchSize) {
			break
		}
	}
	return total
}
