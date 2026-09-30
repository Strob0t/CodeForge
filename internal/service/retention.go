package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// retentionBatchSize limits the rows one statement changes, so the job never
// holds long locks; a backlog is worked off in several statements.
const retentionBatchSize = 1000

// RetentionService enforces the data retention policy (GDPR Art. 5(1)(e),
// docs/data-retention.md): it deletes data older than the configured periods
// and anonymizes the IP addresses of old audit entries and the IP addresses and
// user agents of old consent records. The policy is one configuration for the
// whole instance, so a sweep covers all tenants. Agent events and benchmark
// results are not purged: their retention needs a decision about trajectories.
type RetentionService struct {
	store  database.RetentionStore
	config config.Retention
	now    func() time.Time
}

// NewRetentionService creates a retention service with the given store and policy.
func NewRetentionService(store database.RetentionStore, cfg config.Retention) *RetentionService {
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
		// Messages first, in bounded batches: deleting a conversation cascades to them.
		{"conversation_messages", "deleted", s.config.Conversations, s.store.DeleteExpiredConversationMessages},
		{"conversations", "deleted", s.config.Conversations, s.store.DeleteExpiredConversations},
		{"runs", "deleted", s.config.CostRecords, s.store.DeleteExpiredRuns},
		{"audit_entries", "deleted", s.config.AuditEntries, s.store.DeleteExpiredAuditEntries},
		{"audit_ip_addresses", "anonymized", s.config.AuditIPAddresses, s.store.AnonymizeExpiredIPAddresses},
		{"consent_ip_addresses", "anonymized", s.config.ConsentIPAddresses, s.store.AnonymizeExpiredConsentIPAddresses},
	}
}

// RunCleanup sweeps once if this replica gets the retention lock; while
// another replica (or blue-green color) sweeps, it skips.
func (s *RetentionService) RunCleanup(ctx context.Context) {
	acquired, err := s.store.WithRetentionLock(ctx, s.sweep)
	switch {
	case err != nil:
		slog.Error("retention: sweep skipped, lock failed", "error", err)
	case !acquired:
		slog.Info("retention: sweep skipped, another replica is sweeping")
	}
}

// sweep applies every category with a positive period once. A failing
// category is logged and does not stop the others; a cancelled context ends
// the sweep. The logs carry only categories, row counts and cutoffs.
func (s *RetentionService) sweep(ctx context.Context) {
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

// Start sweeps now and then every retention.interval until ctx ends or the
// returned stop is called; stop cancels a sweep under way and waits for it to
// end. The first sweep runs at start so that restarts cannot postpone the
// purge. An interval of 0 disables the job.
func (s *RetentionService) Start(ctx context.Context) (stop func()) {
	if s.config.Interval <= 0 {
		slog.Warn("retention.interval is 0: the retention job is disabled and expired data is kept")
		return func() {}
	}
	slog.Info("retention job started",
		"interval", s.config.Interval,
		"sessions", s.config.Sessions,
		"conversations", s.config.Conversations,
		"cost_records", s.config.CostRecords,
		"audit_entries", s.config.AuditEntries,
		"audit_ip_addresses", s.config.AuditIPAddresses,
		"consent_ip_addresses", s.config.ConsentIPAddresses,
	)
	return startPeriodic(ctx, s.config.Interval, true, s.RunCleanup)
}
