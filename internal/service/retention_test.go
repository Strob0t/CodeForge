package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// KI-52: the retention job applies the instance-wide policy to every
// category, works off a backlog in bounded batches, keeps going when one
// category fails, and runs on a ticker that stops with the server.

// retentionCategories in sweep order.
var retentionCategories = []string{
	"sessions", "conversations", "runs", "audit_entries", "audit_ip_addresses", "consent_ip_addresses",
}

type retentionCall struct {
	category     string
	before       time.Time
	batchSize    int
	messageBatch int // conversations only
}

// fakeRetentionStore is the store and the purger of the sweep; it records
// every purge call. results holds the rows each call of a category reports,
// in order (then 0); errs fails a category.
type fakeRetentionStore struct {
	mu      sync.Mutex
	calls   []retentionCall
	results map[string][]int64
	errs    map[string]error
	// onCall, if set, runs inside each call (e.g. to cancel or block).
	onCall   func(ctx context.Context, category string) error
	lockHeld bool  // another replica holds the retention lock
	lockErr  error // taking the retention lock fails
	// oauthDeletes counts DeleteExpiredOAuthStates calls (a system step).
	oauthDeletes int
}

func (f *fakeRetentionStore) DeleteExpiredOAuthStates(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.oauthDeletes++
	return 0, f.errs["oauth_states"]
}

// S3-F review C8: expired OAuth states are deleted by the retention job as
// a system step - every sweep, whatever the retention periods - not on the
// request path; a failure does not stop the other categories.
func TestRetention_DeletesExpiredOAuthStatesEverySweep(t *testing.T) {
	store := &fakeRetentionStore{errs: map[string]error{"oauth_states": errors.New("db down")}}
	newTestRetentionService(store, config.Retention{Interval: time.Hour}, time.Now()).RunCleanup(context.Background())
	if store.oauthDeletes != 1 {
		t.Fatalf("expired OAuth state deletions = %d, want 1 (periods are 0)", store.oauthDeletes)
	}

	store = &fakeRetentionStore{errs: map[string]error{"oauth_states": errors.New("db down")}}
	newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())
	if store.oauthDeletes != 1 || len(store.calls) != len(retentionCategories) {
		t.Fatalf("deletions %d, category calls %d; want 1 and every category", store.oauthDeletes, len(store.calls))
	}

	held := &fakeRetentionStore{lockHeld: true}
	newTestRetentionService(held, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())
	if held.oauthDeletes != 0 {
		t.Fatal("expired OAuth states deleted without the retention lock")
	}
}

// Only one replica sweeps at a time: without the retention lock (held by
// another replica, or not obtainable) the sweep does not run.
func TestRetention_SweepsOnlyWithTheLock(t *testing.T) {
	tests := []struct {
		name      string
		store     *fakeRetentionStore
		wantCalls int
	}{
		{"lock taken", &fakeRetentionStore{}, len(retentionCategories)},
		{"another replica sweeps", &fakeRetentionStore{lockHeld: true}, 0},
		{"lock fails", &fakeRetentionStore{lockErr: errors.New("db down")}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newTestRetentionService(tt.store, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())
			if len(tt.store.calls) != tt.wantCalls {
				t.Fatalf("%d store calls, want %d", len(tt.store.calls), tt.wantCalls)
			}
		})
	}
}

func (f *fakeRetentionStore) record(ctx context.Context, call retentionCall) (int64, error) {
	category := call.category
	f.mu.Lock()
	f.calls = append(f.calls, call)
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

// WithRetentionLock runs the sweep with the fake as purger unless lockHeld
// (another replica sweeps) or lockErr is set.
func (f *fakeRetentionStore) WithRetentionLock(ctx context.Context, sweep func(context.Context, database.RetentionPurger)) (bool, error) {
	if f.lockErr != nil {
		return false, f.lockErr
	}
	if f.lockHeld {
		return false, nil
	}
	sweep(ctx, f)
	return true, nil
}

func (f *fakeRetentionStore) DeleteExpiredSessions(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, retentionCall{category: "sessions", before: before, batchSize: batchSize})
}

func (f *fakeRetentionStore) DeleteExpiredConversations(ctx context.Context, before time.Time, batchSize, messageBatch int) (int64, error) {
	return f.record(ctx, retentionCall{category: "conversations", before: before, batchSize: batchSize, messageBatch: messageBatch})
}

func (f *fakeRetentionStore) DeleteExpiredRuns(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, retentionCall{category: "runs", before: before, batchSize: batchSize})
}

func (f *fakeRetentionStore) DeleteExpiredAuditEntries(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, retentionCall{category: "audit_entries", before: before, batchSize: batchSize})
}

func (f *fakeRetentionStore) AnonymizeExpiredIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, retentionCall{category: "audit_ip_addresses", before: before, batchSize: batchSize})
}

func (f *fakeRetentionStore) AnonymizeExpiredConsentIPAddresses(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	return f.record(ctx, retentionCall{category: "consent_ip_addresses", before: before, batchSize: batchSize})
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
		Interval:           day,
		Sessions:           30 * day,
		Conversations:      365 * day,
		CostRecords:        400 * day,
		AuditEntries:       7 * 365 * day,
		AuditIPAddresses:   180 * day,
		ConsentIPAddresses: 90 * day,
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

	var order []string
	for _, c := range store.calls {
		order = append(order, c.category)
	}
	if strings.Join(order, ",") != strings.Join(retentionCategories, ",") {
		t.Fatalf("sweep order = %v, want %v", order, retentionCategories)
	}

	// Whole 365-day years count as calendar years (conversations: 1 year,
	// audit entries: 7 years, over two leap days); other periods are durations.
	want := map[string]time.Time{
		"sessions":             now.Add(-policy.Sessions),
		"conversations":        time.Date(2025, 9, 30, 12, 0, 0, 0, time.UTC),
		"runs":                 now.Add(-policy.CostRecords),
		"audit_entries":        time.Date(2019, 9, 30, 12, 0, 0, 0, time.UTC),
		"audit_ip_addresses":   now.Add(-policy.AuditIPAddresses),
		"consent_ip_addresses": now.Add(-policy.ConsentIPAddresses),
	}
	for _, category := range retentionCategories {
		calls := store.callsOf(category)
		if len(calls) != 1 {
			t.Fatalf("%s: %d calls, want 1", category, len(calls))
		}
		if wantBefore := want[category]; !calls[0].before.Equal(wantBefore) {
			t.Errorf("%s: cutoff %v, want %v", category, calls[0].before, wantBefore)
		}
		wantBatch, wantMessages := retentionBatchSize, 0
		if category == "conversations" {
			// Conversations go in small batches with all their messages, the
			// messages in statements of the usual batch size.
			wantBatch, wantMessages = retentionConversationBatch, retentionBatchSize
		}
		if calls[0].batchSize != wantBatch || calls[0].messageBatch != wantMessages {
			t.Errorf("%s: batch size %d (messages %d), want %d (%d)",
				category, calls[0].batchSize, calls[0].messageBatch, wantBatch, wantMessages)
		}
	}
}

// A period of whole 365-day years (8760h, 61320h) keeps data for that many
// calendar years on every day, leap days included, and not a day longer; any
// other period is a plain duration.
func TestRetentionCutoff(t *testing.T) {
	const day = 24 * time.Hour
	for d := time.Date(2024, 1, 1, 6, 0, 0, 0, time.UTC); d.Year() < 2032; d = d.AddDate(0, 0, 1) {
		for years := 1; years <= 7; years += 6 {
			if got, want := retentionCutoff(d, time.Duration(years)*365*day), d.AddDate(-years, 0, 0); !got.Equal(want) {
				t.Fatalf("%d year(s) on %s: cutoff %s, want %s", years, d.Format(time.DateOnly), got, want)
			}
		}
	}
	now := time.Date(2024, 3, 1, 6, 0, 0, 0, time.UTC)
	for _, period := range []time.Duration{30 * day, 180 * day, 366 * day, 400 * day, 36 * time.Hour} {
		if got := retentionCutoff(now, period); !got.Equal(now.Add(-period)) {
			t.Errorf("period %v: cutoff %s, want %s", period, got, now.Add(-period))
		}
	}
}

func TestRetention_ZeroPeriodKeepsCategory(t *testing.T) {
	tests := []struct {
		setting  string
		modify   func(*config.Retention)
		disabled []string
	}{
		{"sessions", func(p *config.Retention) { p.Sessions = 0 }, []string{"sessions"}},
		{"conversations", func(p *config.Retention) { p.Conversations = 0 }, []string{"conversations"}},
		{"cost_records", func(p *config.Retention) { p.CostRecords = -time.Hour }, []string{"runs"}},
		{"audit_entries", func(p *config.Retention) { p.AuditEntries = 0 }, []string{"audit_entries"}},
		{"audit_ip_addresses", func(p *config.Retention) { p.AuditIPAddresses = 0 }, []string{"audit_ip_addresses"}},
		{"consent_ip_addresses", func(p *config.Retention) { p.ConsentIPAddresses = 0 }, []string{"consent_ip_addresses"}},
	}
	for _, tt := range tests {
		t.Run(tt.setting, func(t *testing.T) {
			policy := testRetentionPolicy()
			tt.modify(&policy)
			store := &fakeRetentionStore{}
			newTestRetentionService(store, policy, time.Now()).RunCleanup(context.Background())
			for _, category := range retentionCategories {
				wantCalls := 1
				if slices.Contains(tt.disabled, category) {
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
		"sessions":             {retentionBatchSize, retentionBatchSize, 7},
		"conversations":        {retentionConversationBatch, 0},
		"runs":                 {retentionBatchSize - 1},
		"audit_ip_addresses":   {retentionBatchSize, retentionBatchSize, retentionBatchSize, 1},
		"consent_ip_addresses": {retentionBatchSize, 5},
	}}
	newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(context.Background())

	for category, want := range map[string]int{
		"sessions": 3, "conversations": 2, "runs": 1, "audit_entries": 1, "audit_ip_addresses": 4, "consent_ip_addresses": 2,
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

// Shutdown during a sweep is a normal stop, not an error.
func TestRetention_CancelIsNotAnError(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, store := range []*fakeRetentionStore{
		{lockErr: fmt.Errorf("acquire retention lock connection: %w", context.Canceled)},
		{errs: map[string]error{"sessions": fmt.Errorf("delete expired sessions: %w", context.Canceled)}},
	} {
		newTestRetentionService(store, testRetentionPolicy(), time.Now()).RunCleanup(ctx)
	}
	if strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Fatalf("cancelled sweep logged an error:\n%s", buf.String())
	}
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
	purged := map[string]int64{}
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(line, &keys); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		for key := range keys {
			if !allowed[key] {
				t.Errorf("retention log has attribute %q (%s); only counts and cutoffs are logged", key, line)
			}
		}
		var rec struct {
			Msg      string `json:"msg"`
			Category string `json:"category"`
			Rows     int64  `json:"rows"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if rec.Msg == "retention: purged expired data" {
			purged[rec.Category] = rec.Rows
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
	policy := testRetentionPolicy()
	policy.Interval = 20 * time.Millisecond
	stop := NewRetentionService(store, policy).Start(context.Background())
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
	stop := NewRetentionService(store, testRetentionPolicy()).Start(context.Background()) // interval: one day

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

// retention.interval 0 disables the job: Start runs nothing.
func TestRetention_ZeroIntervalDisablesJob(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Hour} {
		store := &fakeRetentionStore{}
		policy := testRetentionPolicy()
		policy.Interval = interval
		stop := NewRetentionService(store, policy).Start(context.Background())
		stop()
		if len(store.calls) != 0 {
			t.Fatalf("interval %v: %d store calls, want none", interval, len(store.calls))
		}
	}
}
