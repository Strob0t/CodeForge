package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
)

// KI-52: the retention job applies the instance-wide policy to every
// category, works off a backlog in bounded batches, keeps going when one
// category fails, and runs on a ticker that stops with the server.

var retentionCategories = []string{"sessions", "conversations", "runs", "audit_entries", "audit_ip_addresses"}

type retentionCall struct {
	category  string
	before    time.Time
	batchSize int
}

// fakeRetentionStore records every call. results holds the rows each call
// of a category reports, in order (then 0); errs fails a category.
type fakeRetentionStore struct {
	mu      sync.Mutex
	calls   []retentionCall
	results map[string][]int64
	errs    map[string]error
	// onCall, if set, runs inside each call (e.g. to cancel or block).
	onCall func(ctx context.Context, category string) error
}

func (f *fakeRetentionStore) record(ctx context.Context, category string, before time.Time, batchSize int) (int64, error) {
	f.mu.Lock()
	f.calls = append(f.calls, retentionCall{category, before, batchSize})
	var n int64
	if rs := f.results[category]; len(rs) > 0 {
		n, f.results[category] = rs[0], rs[1:]
	}
	err := f.errs[category]
	onCall := f.onCall
	f.mu.Unlock()
	if onCall != nil {
		if cbErr := onCall(ctx, category); cbErr != nil {
			return 0, cbErr
		}
	}
	return n, err
}

func (f *fakeRetentionStore) DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, "sessions", before, batchSize)
}

func (f *fakeRetentionStore) DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, "conversations", before, batchSize)
}

func (f *fakeRetentionStore) DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, "runs", before, batchSize)
}

func (f *fakeRetentionStore) DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, "audit_entries", before, batchSize)
}

func (f *fakeRetentionStore) AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, "audit_ip_addresses", before, batchSize)
}

func (f *fakeRetentionStore) callsOf(category string) []retentionCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []retentionCall
	for _, c := range f.calls {
		if c.category == category {
			out = append(out, c)
		}
	}
	return out
}

func testRetentionPolicy() config.Retention {
	const day = 24 * time.Hour
	return config.Retention{
		Interval:         day,
		Sessions:         30 * day,
		Conversations:    365 * day,
		CostRecords:      400 * day,
		AuditEntries:     7 * 365 * day,
		AuditIPAddresses: 180 * day,
	}
}

func newTestRetentionService(store *fakeRetentionStore, policy config.Retention, now time.Time) *RetentionService {
	svc := NewRetentionService(store, policy)
	svc.now = func() time.Time { return now }
	return svc
}

func TestRetention_CutoffPerCategory(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	policy := testRetentionPolicy()
	store := &fakeRetentionStore{}
	newTestRetentionService(store, policy, now).RunCleanup(context.Background())

	want := map[string]time.Duration{
		"sessions":           policy.Sessions,
		"conversations":      policy.Conversations,
		"runs":               policy.CostRecords,
		"audit_entries":      policy.AuditEntries,
		"audit_ip_addresses": policy.AuditIPAddresses,
	}
	for _, category := range retentionCategories {
		calls := store.callsOf(category)
		if len(calls) != 1 {
			t.Fatalf("%s: %d calls, want 1", category, len(calls))
		}
		if wantBefore := now.Add(-want[category]); !calls[0].before.Equal(wantBefore) {
			t.Errorf("%s: cutoff %v, want %v", category, calls[0].before, wantBefore)
		}
		if calls[0].batchSize != retentionBatchSize {
			t.Errorf("%s: batch size %d, want %d", category, calls[0].batchSize, retentionBatchSize)
		}
	}
}

func TestRetention_ZeroPeriodKeepsCategory(t *testing.T) {
	disable := map[string]func(*config.Retention){
		"sessions":           func(p *config.Retention) { p.Sessions = 0 },
		"conversations":      func(p *config.Retention) { p.Conversations = 0 },
		"runs":               func(p *config.Retention) { p.CostRecords = -time.Hour },
		"audit_entries":      func(p *config.Retention) { p.AuditEntries = 0 },
		"audit_ip_addresses": func(p *config.Retention) { p.AuditIPAddresses = 0 },
	}
	for disabled, modify := range disable {
		t.Run(disabled, func(t *testing.T) {
			policy := testRetentionPolicy()
			modify(&policy)
			store := &fakeRetentionStore{}
			newTestRetentionService(store, policy, time.Now()).RunCleanup(context.Background())
			for _, category := range retentionCategories {
				wantCalls := 1
				if category == disabled {
					wantCalls = 0
				}
				if got := len(store.callsOf(category)); got != wantCalls {
					t.Errorf("%s: %d calls, want %d", category, got, wantCalls)
				}
			}
		})
	}
}

func TestRetention_WorksOffBacklogInBatches(t *testing.T) {
	store := &fakeRetentionStore{results: map[string][]int64{
		"sessions":           {retentionBatchSize, retentionBatchSize, 7},
		"conversations":      {retentionBatchSize, 0},
		"runs":               {retentionBatchSize - 1},
		"audit_ip_addresses": {retentionBatchSize, retentionBatchSize, retentionBatchSize, 1},
	}}
	newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())

	for category, want := range map[string]int{
		"sessions": 3, "conversations": 2, "runs": 1, "audit_entries": 1, "audit_ip_addresses": 4,
	} {
		if got := len(store.callsOf(category)); got != want {
			t.Errorf("%s: %d batches, want %d", category, got, want)
		}
	}
}

func TestRetention_FailedCategoryDoesNotStopOthers(t *testing.T) {
	store := &fakeRetentionStore{
		results: map[string][]int64{"sessions": {retentionBatchSize, retentionBatchSize}},
		errs:    map[string]error{"sessions": errors.New("db down")},
	}
	newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())

	if got := len(store.callsOf("sessions")); got != 1 {
		t.Errorf("sessions: %d batches after an error, want 1", got)
	}
	for _, category := range retentionCategories[1:] {
		if got := len(store.callsOf(category)); got != 1 {
			t.Errorf("%s: %d calls, want 1 despite the sessions failure", category, got)
		}
	}
}

func TestRetention_CancelledContextStopsSweep(t *testing.T) {
	t.Run("cancelled before the sweep", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		store := &fakeRetentionStore{}
		newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(ctx)
		if len(store.calls) != 0 {
			t.Fatalf("calls = %v, want none", store.calls)
		}
	})
	t.Run("cancelled during a batch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		store := &fakeRetentionStore{
			results: map[string][]int64{"sessions": {retentionBatchSize, retentionBatchSize}},
			onCall: func(context.Context, string) error {
				cancel()
				return nil
			},
		}
		newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(ctx)
		if len(store.calls) != 1 {
			t.Fatalf("calls = %v, want only the first sessions batch", store.calls)
		}
	})
}

func TestRetention_LogsCountsOnly(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	store := &fakeRetentionStore{
		results: map[string][]int64{"sessions": {3}, "audit_ip_addresses": {2}},
		errs:    map[string]error{"runs": errors.New("db down")},
	}
	newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())

	allowed := map[string]bool{"time": true, "level": true, "msg": true, "category": true, "action": true, "rows": true, "older_than": true, "error": true}
	purged := map[string]float64{}
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		for key := range rec {
			if !allowed[key] {
				t.Errorf("retention log has attribute %q (%s); only counts and cutoffs are logged", key, line)
			}
		}
		if rec["msg"] == "retention: purged expired data" {
			purged[rec["category"].(string)] = rec["rows"].(float64)
		}
	}
	if purged["sessions"] != 3 || purged["audit_ip_addresses"] != 2 || len(purged) != 2 {
		t.Fatalf("purge log = %v, want sessions 3 and audit_ip_addresses 2", purged)
	}
}

func TestRetention_StartSweepsAtOnceAndOnEveryTick(t *testing.T) {
	sweeps := make(chan struct{}, 16)
	store := &fakeRetentionStore{onCall: func(_ context.Context, category string) error {
		if category == "sessions" {
			sweeps <- struct{}{}
		}
		return nil
	}}
	stop := NewRetentionService(store, testRetentionPolicy()).Start(context.Background(), 20*time.Millisecond)
	defer stop()

	for i := range 3 {
		select {
		case <-sweeps:
		case <-time.After(5 * time.Second):
			t.Fatalf("sweep %d did not run", i+1)
		}
	}
}

func TestRetention_StopWaitsForRunningSweep(t *testing.T) {
	started := make(chan struct{})
	var mu sync.Mutex
	finished := false
	store := &fakeRetentionStore{onCall: func(ctx context.Context, category string) error {
		if category != "sessions" {
			return nil
		}
		close(started)
		<-ctx.Done() // a long statement that ends when the sweep is cancelled
		mu.Lock()
		finished = true
		mu.Unlock()
		return ctx.Err()
	}}
	stop := NewRetentionService(store, testRetentionPolicy()).Start(context.Background(), time.Hour)

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first sweep did not start")
	}
	stop()
	mu.Lock()
	defer mu.Unlock()
	if !finished {
		t.Fatal("stop returned before the running sweep ended")
	}
	if got := len(store.callsOf("conversations")); got != 0 {
		t.Fatalf("sweep continued after stop: %d conversations calls", got)
	}
}
