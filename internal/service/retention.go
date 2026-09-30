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

// retentionConversationBatch limits the conversations one transaction deletes
// with all their messages (at most retentionBatchSize per statement).
const retentionConversationBatch = 100

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
	batch  int // rows (conversations: conversations) per call
	apply  func(ctx context.Context, before time.Time, batchSize int) (int64, error)
}

func (s *RetentionService) categories(purge database.RetentionPurger) []retentionCategory {
	deleteConversations := func(ctx context.Context, before time.Time, batchSize int) (int64, error) {
		return purge.DeleteExpiredConversations(ctx, before, batchSize, retentionBatchSize)
	}
	return []retentionCategory{
		{"sessions", "deleted", s.config.Sessions, retentionBatchSize, purge.DeleteExpiredSessions},
		{"conversations", "deleted", s.config.Conversations, retentionConversationBatch, deleteConversations},
		{"runs", "deleted", s.config.CostRecords, retentionBatchSize, purge.DeleteExpiredRuns},
		{"audit_entries", "deleted", s.config.AuditEntries, retentionBatchSize, purge.DeleteExpiredAuditEntries},
		{"audit_ip_addresses", "anonymized", s.config.AuditIPAddresses, retentionBatchSize, purge.AnonymizeExpiredIPAddresses},
		{"consent_ip_addresses", "anonymized", s.config.ConsentIPAddresses, retentionBatchSize, purge.AnonymizeExpiredConsentIPAddresses},
	}
}

// RunCleanup sweeps once if this replica gets the retention lock; while
// another replica (or blue-green color) sweeps, it skips. A cancelled context
// (shutdown) ends it without an error.
func (s *RetentionService) RunCleanup(ctx context.Context) {
	acquired, err := s.store.WithRetentionLock(ctx, s.sweep)
	switch {
	case err != nil && ctx.Err() != nil:
		slog.Info("retention: sweep stopped", "error", err)
	case err != nil:
		slog.Error("retention: sweep skipped, lock failed", "error", err)
	case !acquired:
		slog.Info("retention: sweep skipped, another replica is sweeping")
	}
}

// sweep applies every category with a positive period once. A failing
// category is logged and does not stop the others; a cancelled context ends
// the sweep. The logs carry only categories, row counts and cutoffs.
func (s *RetentionService) sweep(ctx context.Context, purge database.RetentionPurger) {
	now := s.now().UTC()
	for _, c := range s.categories(purge) {
		if c.maxAge <= 0 {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		before := retentionCutoff(now, c.maxAge)
		n, err := applyInBatches(ctx, before, c.batch, c.apply)
		if n > 0 {
			slog.Info("retention: purged expired data",
				"category", c.name, "action", c.action, "rows", n, "older_than", before.Format(time.RFC3339))
		}
		if err != nil && ctx.Err() == nil {
			slog.Error("retention: purge failed", "category", c.name, "error", err)
		}
	}
}

// retentionYear is the 365-day year in which the configuration states
// periods of years (8760h = 1 year, 61320h = 7 years).
const retentionYear = 365 * 24 * time.Hour

// retentionCutoff is the time before which data of the given maximum age has
// expired. A period of whole 365-day years counts calendar years - the same
// date that many years back - so the data is kept exactly that long, leap
// days included; any other period is subtracted as a duration.
func retentionCutoff(now time.Time, maxAge time.Duration) time.Time {
	if maxAge%retentionYear == 0 {
		return now.AddDate(-int(maxAge/retentionYear), 0, 0)
	}
	return now.Add(-maxAge)
}

// applyInBatches calls apply until a batch comes back short and returns the
// rows changed in total.
func applyInBatches(
	ctx context.Context,
	before time.Time,
	batchSize int,
	apply func(ctx context.Context, before time.Time, batchSize int) (int64, error),
) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := apply(ctx, before, batchSize)
		total += n
		if err != nil || n < int64(batchSize) {
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
