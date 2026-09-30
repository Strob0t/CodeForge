package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
)

// retentionBatchSize limits the rows one statement changes, so the job never
// holds long locks; a backlog is worked off in several statements.
const retentionBatchSize = 1000

// retentionStore is the part of the store the retention job uses. Each method
// spans all tenants (the policy is instance-wide) and changes at most
// batchSize rows per call.
type retentionStore interface {
	DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error)
	DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error)
	AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

// RetentionService enforces the data retention policy (GDPR Art. 5(1)(e),
// docs/data-retention.md): it deletes data older than the configured periods
// and anonymizes the IP addresses of old audit entries. The policy is one
// configuration for the whole instance, so a sweep covers all tenants.
type RetentionService struct {
	store  retentionStore
	config config.Retention
	now    func() time.Time
}

// NewRetentionService creates a retention service with the given store and policy.
func NewRetentionService(store retentionStore, cfg config.Retention) *RetentionService {
	return &RetentionService{store: store, config: cfg, now: time.Now}
}

// retentionCategory is one kind of data with its retention period.
type retentionCategory struct {
	name   string
	action string // what happens to expired rows: "deleted" or "anonymized"
	maxAge time.Duration
	apply  func(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

func (s *RetentionService) categories() []retentionCategory {
	return []retentionCategory{
		{"sessions", "deleted", s.config.Sessions, s.store.DeleteExpiredSessions},
		{"conversations", "deleted", s.config.Conversations, s.store.DeleteExpiredConversations},
		{"runs", "deleted", s.config.CostRecords, s.store.DeleteExpiredRuns},
		{"audit_entries", "deleted", s.config.AuditEntries, s.store.DeleteExpiredAuditEntries},
		{"audit_ip_addresses", "anonymized", s.config.AuditIPAddresses, s.store.AnonymizeExpiredIPAddresses},
	}
}

// RunCleanup applies every category with a positive period once. A failing
// category is logged and does not stop the others; a cancelled context ends
// the sweep. The logs carry only categories, row counts and cutoffs.
func (s *RetentionService) RunCleanup(ctx context.Context) {
	now := s.now().UTC()
	for _, c := range s.categories() {
		if c.maxAge <= 0 {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		before := now.Add(-c.maxAge)
		n, err := applyInBatches(ctx, before, c.apply)
		if n > 0 {
			slog.Info("retention: purged expired data",
				"category", c.name, "action", c.action, "rows", n, "older_than", before.Format(time.RFC3339))
		}
		if err != nil && ctx.Err() == nil {
			slog.Error("retention: purge failed", "category", c.name, "error", err)
		}
	}
}

// applyInBatches calls apply until a batch comes back short and returns the
// rows changed in total.
func applyInBatches(
	ctx context.Context,
	before time.Time,
	apply func(ctx context.Context, before time.Time, batchSize int) (int64, error),
) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := apply(ctx, before, retentionBatchSize)
		total += n
		if err != nil || n < retentionBatchSize {
			return total, err
		}
	}
}

// Start sweeps now and then every interval until ctx ends or the returned
// stop is called; stop cancels a sweep under way and waits for it to end.
// The first sweep runs at start so that restarts cannot postpone the purge.
func (s *RetentionService) Start(ctx context.Context, interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			s.RunCleanup(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	return func() {
		cancel()
		wg.Wait()
	}
}
