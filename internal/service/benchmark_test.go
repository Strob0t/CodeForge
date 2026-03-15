package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// benchMockStore embeds the runtimeMockStore and adds benchmark-specific storage.
type benchMockStore struct {
	runtimeMockStore
	suites      map[string]*benchmark.Suite
	benchRuns   map[string]*benchmark.Run
	benchResult map[string][]benchmark.Result
}

func newBenchMockStore() *benchMockStore {
	return &benchMockStore{
		suites:      make(map[string]*benchmark.Suite),
		benchRuns:   make(map[string]*benchmark.Run),
		benchResult: make(map[string][]benchmark.Result),
	}
}

func (m *benchMockStore) CreateBenchmarkSuite(_ context.Context, s *benchmark.Suite) error {
	m.suites[s.ID] = s
	return nil
}
func (m *benchMockStore) GetBenchmarkSuite(_ context.Context, id string) (*benchmark.Suite, error) {
	s, ok := m.suites[id]
	if !ok {
		return nil, fmt.Errorf("suite not found: %s", id)
	}
	return s, nil
}
func (m *benchMockStore) ListBenchmarkSuites(_ context.Context) ([]benchmark.Suite, error) {
	var out []benchmark.Suite
	for _, s := range m.suites {
		out = append(out, *s)
	}
	return out, nil
}
func (m *benchMockStore) DeleteBenchmarkSuite(_ context.Context, id string) error {
	delete(m.suites, id)
	return nil
}
func (m *benchMockStore) UpdateBenchmarkSuite(_ context.Context, _ *benchmark.Suite) error {
	return nil
}
func (m *benchMockStore) CreateBenchmarkRun(_ context.Context, r *benchmark.Run) error {
	m.benchRuns[r.ID] = r
	return nil
}
func (m *benchMockStore) GetBenchmarkRun(_ context.Context, id string) (*benchmark.Run, error) {
	r, ok := m.benchRuns[id]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", id)
	}
	return r, nil
}
func (m *benchMockStore) ListBenchmarkRuns(_ context.Context) ([]benchmark.Run, error) {
	var out []benchmark.Run
	for _, r := range m.benchRuns {
		out = append(out, *r)
	}
	return out, nil
}
func (m *benchMockStore) UpdateBenchmarkRun(_ context.Context, r *benchmark.Run) error {
	m.benchRuns[r.ID] = r
	return nil
}
func (m *benchMockStore) DeleteBenchmarkRun(_ context.Context, id string) error {
	delete(m.benchRuns, id)
	delete(m.benchResult, id)
	return nil
}
func (m *benchMockStore) ListBenchmarkRunsFiltered(_ context.Context, f *benchmark.RunFilter) ([]benchmark.Run, error) {
	var out []benchmark.Run
	for _, r := range m.benchRuns {
		if f.SuiteID != "" && r.SuiteID != f.SuiteID {
			continue
		}
		if f.BenchmarkType != "" && r.BenchmarkType != f.BenchmarkType {
			continue
		}
		if f.Model != "" && r.Model != f.Model {
			continue
		}
		if f.Status != "" && r.Status != f.Status {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}
func (m *benchMockStore) CreateBenchmarkResult(_ context.Context, r *benchmark.Result) error {
	m.benchResult[r.RunID] = append(m.benchResult[r.RunID], *r)
	return nil
}
func (m *benchMockStore) ListBenchmarkResults(_ context.Context, runID string) ([]benchmark.Result, error) {
	return m.benchResult[runID], nil
}

// --- Tests ---

func TestBenchmarkService_CompareMulti(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")
	ctx := context.Background()

	for _, id := range []string{"run-1", "run-2", "run-3"} {
		store.benchRuns[id] = &benchmark.Run{
			ID: id, Model: "gpt-4", Status: benchmark.StatusCompleted,
			CreatedAt: time.Now(),
		}
		store.benchResult[id] = []benchmark.Result{
			{ID: "r-" + id, RunID: id, TaskID: "t1", TaskName: "Task1",
				Scores: json.RawMessage(`{"correctness":0.9}`)},
		}
	}

	entries, err := svc.CompareMulti(ctx, []string{"run-1", "run-2", "run-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Run == nil {
			t.Error("entry.Run is nil")
		}
		if len(e.Results) != 1 {
			t.Errorf("expected 1 result per entry, got %d", len(e.Results))
		}
	}
}

func TestBenchmarkService_CompareMulti_TooFewRuns(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")

	_, err := svc.CompareMulti(context.Background(), []string{"run-1"})
	if err == nil {
		t.Error("expected error for < 2 run IDs")
	}
}

func TestBenchmarkService_CompareMulti_MissingRun(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")

	store.benchRuns["run-1"] = &benchmark.Run{ID: "run-1", Model: "gpt-4"}

	_, err := svc.CompareMulti(context.Background(), []string{"run-1", "run-missing"})
	if err == nil {
		t.Error("expected error for missing run")
	}
}

func TestBenchmarkService_CostAnalysis(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")
	ctx := context.Background()

	store.benchRuns["run-1"] = &benchmark.Run{
		ID: "run-1", Model: "gpt-4", SuiteID: "suite-1",
		Status: benchmark.StatusCompleted,
	}
	store.benchResult["run-1"] = []benchmark.Result{
		{RunID: "run-1", TaskID: "t1", TaskName: "FizzBuzz",
			CostUSD: 0.05, TokensIn: 2000, TokensOut: 800,
			Scores: json.RawMessage(`{"correctness":0.9}`)},
		{RunID: "run-1", TaskID: "t2", TaskName: "Sort",
			CostUSD: 0.075, TokensIn: 3000, TokensOut: 1200,
			Scores: json.RawMessage(`{"correctness":0.8}`)},
	}

	analysis, err := svc.CostAnalysis(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}

	if analysis.RunID != "run-1" {
		t.Errorf("RunID = %q, want run-1", analysis.RunID)
	}
	if analysis.Model != "gpt-4" {
		t.Errorf("Model = %q, want gpt-4", analysis.Model)
	}
	if analysis.TotalCostUSD != 0.125 {
		t.Errorf("TotalCostUSD = %f, want 0.125", analysis.TotalCostUSD)
	}
	if analysis.TotalTokensIn != 5000 {
		t.Errorf("TotalTokensIn = %d, want 5000", analysis.TotalTokensIn)
	}
	if analysis.TotalTokensOut != 2000 {
		t.Errorf("TotalTokensOut = %d, want 2000", analysis.TotalTokensOut)
	}
	if diff := analysis.AvgScore - 0.85; diff > 0.001 || diff < -0.001 {
		t.Errorf("AvgScore = %f, want ~0.85", analysis.AvgScore)
	}
	if len(analysis.TaskBreakdown) != 2 {
		t.Errorf("TaskBreakdown length = %d, want 2", len(analysis.TaskBreakdown))
	}
	if analysis.CostPerScorePoint <= 0 {
		t.Error("CostPerScorePoint should be > 0")
	}
	if analysis.TokenEfficiency <= 0 {
		t.Error("TokenEfficiency should be > 0")
	}
}

func TestBenchmarkService_CostAnalysis_NoResults(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")

	store.benchRuns["run-1"] = &benchmark.Run{ID: "run-1", Model: "gpt-4"}

	analysis, err := svc.CostAnalysis(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if analysis.AvgScore != 0 {
		t.Errorf("AvgScore should be 0 for no results, got %f", analysis.AvgScore)
	}
	if analysis.CostPerScorePoint != 0 {
		t.Errorf("CostPerScorePoint should be 0 for no results")
	}
}

func TestBenchmarkService_CostAnalysis_MissingRun(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")

	_, err := svc.CostAnalysis(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing run")
	}
}

func TestBenchmarkService_Leaderboard(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")
	ctx := context.Background()

	store.benchRuns["run-1"] = &benchmark.Run{
		ID: "run-1", Model: "gpt-4", SuiteID: "suite-1",
		Status: benchmark.StatusCompleted, TotalDurationMs: 10000,
	}
	store.benchRuns["run-2"] = &benchmark.Run{
		ID: "run-2", Model: "claude-3", SuiteID: "suite-1",
		Status: benchmark.StatusCompleted, TotalDurationMs: 8000,
	}
	// Running run should be excluded
	store.benchRuns["run-3"] = &benchmark.Run{
		ID: "run-3", Model: "gemini", SuiteID: "suite-1",
		Status: benchmark.StatusRunning,
	}

	store.benchResult["run-1"] = []benchmark.Result{
		{RunID: "run-1", TaskID: "t1", CostUSD: 0.1, TokensIn: 1000, TokensOut: 500,
			Scores: json.RawMessage(`{"correctness":0.7}`)},
	}
	store.benchResult["run-2"] = []benchmark.Result{
		{RunID: "run-2", TaskID: "t1", CostUSD: 0.2, TokensIn: 2000, TokensOut: 1000,
			Scores: json.RawMessage(`{"correctness":0.95}`)},
	}

	entries, err := svc.Leaderboard(ctx, "suite-1")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (running excluded), got %d", len(entries))
	}

	// Sorted by score descending: claude-3 (0.95) > gpt-4 (0.7)
	if entries[0].Model != "claude-3" {
		t.Errorf("first entry should be claude-3, got %s", entries[0].Model)
	}
	if entries[1].Model != "gpt-4" {
		t.Errorf("second entry should be gpt-4, got %s", entries[1].Model)
	}
}

func TestBenchmarkService_Leaderboard_EmptySuite(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")

	entries, err := svc.Leaderboard(context.Background(), "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestBenchmarkService_Leaderboard_NoSuiteFilter(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")

	store.benchRuns["run-1"] = &benchmark.Run{
		ID: "run-1", Model: "gpt-4", Status: benchmark.StatusCompleted,
	}
	store.benchResult["run-1"] = []benchmark.Result{
		{RunID: "run-1", TaskID: "t1", Scores: json.RawMessage(`{"correctness":0.8}`)},
	}

	entries, err := svc.Leaderboard(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

// benchMockQueue is a minimal NATS queue mock for benchmark tests.
type benchMockQueue struct {
	published []benchPublishedMsg
}

type benchPublishedMsg struct {
	Subject string
	Data    []byte
}

func (q *benchMockQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.published = append(q.published, benchPublishedMsg{Subject: subject, Data: data})
	return nil
}
func (q *benchMockQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, _ string) error {
	return q.Publish(ctx, subject, data)
}
func (q *benchMockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *benchMockQueue) Drain() error      { return nil }
func (q *benchMockQueue) Close() error      { return nil }
func (q *benchMockQueue) IsConnected() bool { return true }

func TestStartRun_NonExistentDataset_NoSuite_ReturnsError(t *testing.T) {
	store := newBenchMockStore()
	// Use /tmp as datasetsDir — it exists but won't contain "nonexistent-dataset.yaml".
	svc := service.NewBenchmarkService(store, "/tmp")
	q := &benchMockQueue{}
	svc.SetQueue(q)
	ctx := context.Background()

	req := &benchmark.CreateRunRequest{
		Dataset: "nonexistent-dataset",
		Model:   "gpt-4",
		Metrics: []string{"correctness"},
		// SuiteID is empty — no fallback.
	}

	run, err := svc.StartRun(ctx, req)
	if err == nil {
		t.Fatal("expected error for non-existent dataset with empty SuiteID, got nil")
	}
	if run != nil {
		t.Errorf("expected nil run on error, got %+v", run)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %s", err.Error())
	}

	// No message should have been published to NATS.
	if len(q.published) != 0 {
		t.Errorf("expected 0 NATS messages, got %d", len(q.published))
	}

	// Verify the run was persisted with failed status.
	// The run was created by CreateRun internally, so there should be exactly one run in the store.
	if len(store.benchRuns) != 1 {
		t.Fatalf("expected 1 run in store, got %d", len(store.benchRuns))
	}
	for _, r := range store.benchRuns {
		if r.Status != benchmark.StatusFailed {
			t.Errorf("expected run status %q, got %q", benchmark.StatusFailed, r.Status)
		}
		if r.ErrorMessage == "" {
			t.Error("expected non-empty ErrorMessage on failed run")
		}
		if !strings.Contains(r.ErrorMessage, "nonexistent-dataset") {
			t.Errorf("ErrorMessage should reference dataset name, got: %s", r.ErrorMessage)
		}
	}
}

func TestWatchdog_MarksStaleRunAsFailed(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")
	ctx := context.Background()

	// Run created 20 minutes ago, still running — should be marked failed.
	store.benchRuns["stale-run"] = &benchmark.Run{
		ID:        "stale-run",
		Model:     "gpt-4",
		Status:    benchmark.StatusRunning,
		CreatedAt: time.Now().Add(-20 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 15*time.Minute)

	r := store.benchRuns["stale-run"]
	if r.Status != benchmark.StatusFailed {
		t.Errorf("expected status %q, got %q", benchmark.StatusFailed, r.Status)
	}
	if r.ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage on watchdog-failed run")
	}
	if !strings.Contains(r.ErrorMessage, "watchdog timeout") {
		t.Errorf("ErrorMessage should contain 'watchdog timeout', got: %s", r.ErrorMessage)
	}
}

func TestWatchdog_DoesNotMarkYoungRuns(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")
	ctx := context.Background()

	// Run created 5 minutes ago, still running — should stay running.
	store.benchRuns["young-run"] = &benchmark.Run{
		ID:        "young-run",
		Model:     "gpt-4",
		Status:    benchmark.StatusRunning,
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 15*time.Minute)

	r := store.benchRuns["young-run"]
	if r.Status != benchmark.StatusRunning {
		t.Errorf("expected status %q, got %q", benchmark.StatusRunning, r.Status)
	}
	if r.ErrorMessage != "" {
		t.Errorf("expected empty ErrorMessage, got: %s", r.ErrorMessage)
	}
}

func TestWatchdog_SkipsCompletedRuns(t *testing.T) {
	store := newBenchMockStore()
	svc := service.NewBenchmarkService(store, "")
	ctx := context.Background()

	// Run created 20 minutes ago but already completed — should stay completed.
	store.benchRuns["completed-run"] = &benchmark.Run{
		ID:        "completed-run",
		Model:     "gpt-4",
		Status:    benchmark.StatusCompleted,
		CreatedAt: time.Now().Add(-20 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 15*time.Minute)

	r := store.benchRuns["completed-run"]
	if r.Status != benchmark.StatusCompleted {
		t.Errorf("expected status %q, got %q", benchmark.StatusCompleted, r.Status)
	}
	if r.ErrorMessage != "" {
		t.Errorf("expected empty ErrorMessage, got: %s", r.ErrorMessage)
	}
}

func TestStartRun_NonExistentDataset_WithSuite_Continues(t *testing.T) {
	store := newBenchMockStore()
	// Use /tmp as datasetsDir — it exists but won't contain "nonexistent-dataset.yaml".
	svc := service.NewBenchmarkService(store, "/tmp")
	q := &benchMockQueue{}
	svc.SetQueue(q)
	ctx := context.Background()

	// Create a suite so the suite lookup succeeds.
	store.suites["suite-1"] = &benchmark.Suite{
		ID:           "suite-1",
		Name:         "Test Suite",
		Type:         benchmark.TypeSimple,
		ProviderName: "test_provider",
	}

	req := &benchmark.CreateRunRequest{
		Dataset: "nonexistent-dataset",
		Model:   "gpt-4",
		Metrics: []string{"correctness"},
		SuiteID: "suite-1",
	}

	run, err := svc.StartRun(ctx, req)
	if err != nil {
		t.Fatalf("expected no error when SuiteID is set (suite fallback), got: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}

	// The run should still be in running status (not failed).
	if run.Status != benchmark.StatusRunning {
		t.Errorf("expected run status %q, got %q", benchmark.StatusRunning, run.Status)
	}

	// A message should have been published to NATS.
	if len(q.published) != 1 {
		t.Errorf("expected 1 NATS message, got %d", len(q.published))
	}
}
