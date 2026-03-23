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

// newTestBenchmarkService creates a BenchmarkService with sub-services wired for testing.
func newTestBenchmarkService(store *benchMockStore) *service.BenchmarkService {
	return newTestBenchmarkServiceWithDir(store, "")
}

// newTestBenchmarkServiceWithDir creates a BenchmarkService with a custom datasets directory.
func newTestBenchmarkServiceWithDir(store *benchMockStore, datasetsDir string) *service.BenchmarkService {
	suiteSvc := service.NewBenchmarkSuiteService(store, datasetsDir)
	runMgr := service.NewBenchmarkRunManager(store, suiteSvc)
	resultAgg := service.NewBenchmarkResultAggregator(store)
	watchdog := service.NewBenchmarkWatchdog(store)
	return service.NewBenchmarkService(suiteSvc, runMgr, resultAgg, watchdog)
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
	svc := newTestBenchmarkService(store)
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
	svc := newTestBenchmarkService(store)

	_, err := svc.CompareMulti(context.Background(), []string{"run-1"})
	if err == nil {
		t.Error("expected error for < 2 run IDs")
	}
}

func TestBenchmarkService_CompareMulti_MissingRun(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)

	store.benchRuns["run-1"] = &benchmark.Run{ID: "run-1", Model: "gpt-4"}

	_, err := svc.CompareMulti(context.Background(), []string{"run-1", "run-missing"})
	if err == nil {
		t.Error("expected error for missing run")
	}
}

func TestBenchmarkService_CostAnalysis(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
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
	svc := newTestBenchmarkService(store)

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
	svc := newTestBenchmarkService(store)

	_, err := svc.CostAnalysis(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing run")
	}
}

func TestBenchmarkService_Leaderboard(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
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
	svc := newTestBenchmarkService(store)

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
	svc := newTestBenchmarkService(store)

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

func TestRegisterSuite_AutoDerivesTypeFromProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		wantType     benchmark.BenchmarkType
	}{
		{"humaneval derives simple", "humaneval", benchmark.TypeSimple},
		{"swebench derives agent", "swebench", benchmark.TypeAgent},
		{"codeforge_tool_use derives tool_use", "codeforge_tool_use", benchmark.TypeToolUse},
		{"mbpp derives simple", "mbpp", benchmark.TypeSimple},
		{"terminal_bench derives agent", "terminal_bench", benchmark.TypeAgent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newBenchMockStore()
			svc := newTestBenchmarkService(store)
			ctx := context.Background()

			req := &benchmark.CreateSuiteRequest{
				Name:         "Test Suite",
				ProviderName: tt.providerName,
				// Type intentionally omitted.
			}

			suite, err := svc.RegisterSuite(ctx, req)
			if err != nil {
				t.Fatalf("RegisterSuite() error = %v", err)
			}
			if suite.Type != tt.wantType {
				t.Errorf("suite.Type = %q, want %q", suite.Type, tt.wantType)
			}
		})
	}
}

func TestRegisterSuite_ExplicitTypeNotOverridden(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	req := &benchmark.CreateSuiteRequest{
		Name:         "Custom Suite",
		Type:         benchmark.TypeAgent,
		ProviderName: "humaneval", // Default would be "simple", but explicit "agent" wins.
	}

	suite, err := svc.RegisterSuite(ctx, req)
	if err != nil {
		t.Fatalf("RegisterSuite() error = %v", err)
	}
	if suite.Type != benchmark.TypeAgent {
		t.Errorf("suite.Type = %q, want %q (explicit type should not be overridden)", suite.Type, benchmark.TypeAgent)
	}
}

func TestRegisterSuite_UnknownProviderWithoutTypeFailsValidation(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	req := &benchmark.CreateSuiteRequest{
		Name:         "Unknown Provider Suite",
		ProviderName: "totally_unknown",
		// Type omitted — unknown provider cannot auto-derive.
	}

	_, err := svc.RegisterSuite(ctx, req)
	if err == nil {
		t.Fatal("expected validation error for unknown provider without explicit type")
	}
	if !strings.Contains(err.Error(), "invalid benchmark type") {
		t.Errorf("expected 'invalid benchmark type' error, got: %v", err)
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
	svc := newTestBenchmarkServiceWithDir(store, "/tmp")
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
	svc := newTestBenchmarkService(store)
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
	svc := newTestBenchmarkService(store)
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
	svc := newTestBenchmarkService(store)
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

func TestWatchdog_PerTypeTimeout_SimpleRunTimesOutFaster(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	// A simple run created 45 minutes ago should be marked failed even with a
	// 2-hour global timeout because simple runs have a 30-minute per-type timeout.
	store.benchRuns["simple-old"] = &benchmark.Run{
		ID:            "simple-old",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeSimple,
		CreatedAt:     time.Now().Add(-45 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 2*time.Hour) // global timeout = 2h

	r := store.benchRuns["simple-old"]
	if r.Status != benchmark.StatusFailed {
		t.Errorf("expected simple run to be failed after 45 min, got %q", r.Status)
	}
	if !strings.Contains(r.ErrorMessage, "30m") {
		t.Errorf("ErrorMessage should reference 30m timeout, got: %s", r.ErrorMessage)
	}
}

func TestWatchdog_PerTypeTimeout_AgentRunSurvivesLonger(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	// An agent run created 3 hours ago should NOT be marked failed because
	// agent runs have a 4-hour per-type timeout.
	store.benchRuns["agent-running"] = &benchmark.Run{
		ID:            "agent-running",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeAgent,
		CreatedAt:     time.Now().Add(-3 * time.Hour),
	}

	svc.RunWatchdogOnce(ctx, 2*time.Hour) // global timeout = 2h

	r := store.benchRuns["agent-running"]
	if r.Status != benchmark.StatusRunning {
		t.Errorf("expected agent run to still be running after 3h, got %q", r.Status)
	}
	if r.ErrorMessage != "" {
		t.Errorf("expected empty ErrorMessage, got: %s", r.ErrorMessage)
	}
}

func TestWatchdog_PerTypeTimeout_AgentRunTimesOutAt4h(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	// An agent run created 5 hours ago should be marked failed (exceeds 4h).
	store.benchRuns["agent-stale"] = &benchmark.Run{
		ID:            "agent-stale",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeAgent,
		CreatedAt:     time.Now().Add(-5 * time.Hour),
	}

	svc.RunWatchdogOnce(ctx, 2*time.Hour)

	r := store.benchRuns["agent-stale"]
	if r.Status != benchmark.StatusFailed {
		t.Errorf("expected agent run to be failed after 5h, got %q", r.Status)
	}
	if !strings.Contains(r.ErrorMessage, "4h") {
		t.Errorf("ErrorMessage should reference 4h timeout, got: %s", r.ErrorMessage)
	}
}

func TestWatchdog_PerTypeTimeout_ToolUseRunTimesOutAt1h(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	// A tool_use run created 75 minutes ago should be marked failed (exceeds 60 min).
	store.benchRuns["tooluse-stale"] = &benchmark.Run{
		ID:            "tooluse-stale",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeToolUse,
		CreatedAt:     time.Now().Add(-75 * time.Minute),
	}
	// A tool_use run created 45 minutes ago should still be running.
	store.benchRuns["tooluse-young"] = &benchmark.Run{
		ID:            "tooluse-young",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeToolUse,
		CreatedAt:     time.Now().Add(-45 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 2*time.Hour)

	stale := store.benchRuns["tooluse-stale"]
	if stale.Status != benchmark.StatusFailed {
		t.Errorf("expected tool_use run to be failed after 75 min, got %q", stale.Status)
	}
	if !strings.Contains(stale.ErrorMessage, "1h") {
		t.Errorf("ErrorMessage should reference 1h timeout, got: %s", stale.ErrorMessage)
	}

	young := store.benchRuns["tooluse-young"]
	if young.Status != benchmark.StatusRunning {
		t.Errorf("expected tool_use run to still be running after 45 min, got %q", young.Status)
	}
}

func TestWatchdog_PerTypeTimeout_EmptyTypeFallsBackToGlobal(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	// Run with empty BenchmarkType created 90 minutes ago.
	// With a 1-hour global timeout, it should be marked failed.
	store.benchRuns["no-type-old"] = &benchmark.Run{
		ID:        "no-type-old",
		Model:     "gpt-4",
		Status:    benchmark.StatusRunning,
		CreatedAt: time.Now().Add(-90 * time.Minute),
	}
	// Run with empty BenchmarkType created 30 minutes ago should stay running.
	store.benchRuns["no-type-young"] = &benchmark.Run{
		ID:        "no-type-young",
		Model:     "gpt-4",
		Status:    benchmark.StatusRunning,
		CreatedAt: time.Now().Add(-30 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 1*time.Hour) // global = 1h

	old := store.benchRuns["no-type-old"]
	if old.Status != benchmark.StatusFailed {
		t.Errorf("expected empty-type run to be failed after 90 min with 1h global timeout, got %q", old.Status)
	}

	young := store.benchRuns["no-type-young"]
	if young.Status != benchmark.StatusRunning {
		t.Errorf("expected empty-type run to still be running after 30 min, got %q", young.Status)
	}
}

func TestWatchdog_PerTypeTimeout_MixedTypes(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	ctx := context.Background()

	// All runs created 45 minutes ago. With 2h global timeout:
	// - simple (30m limit) -> should FAIL
	// - tool_use (60m limit) -> should SURVIVE
	// - agent (4h limit) -> should SURVIVE
	// - empty type (2h global) -> should SURVIVE
	store.benchRuns["mix-simple"] = &benchmark.Run{
		ID:            "mix-simple",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeSimple,
		CreatedAt:     time.Now().Add(-45 * time.Minute),
	}
	store.benchRuns["mix-tooluse"] = &benchmark.Run{
		ID:            "mix-tooluse",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeToolUse,
		CreatedAt:     time.Now().Add(-45 * time.Minute),
	}
	store.benchRuns["mix-agent"] = &benchmark.Run{
		ID:            "mix-agent",
		Model:         "gpt-4",
		Status:        benchmark.StatusRunning,
		BenchmarkType: benchmark.TypeAgent,
		CreatedAt:     time.Now().Add(-45 * time.Minute),
	}
	store.benchRuns["mix-empty"] = &benchmark.Run{
		ID:        "mix-empty",
		Model:     "gpt-4",
		Status:    benchmark.StatusRunning,
		CreatedAt: time.Now().Add(-45 * time.Minute),
	}

	svc.RunWatchdogOnce(ctx, 2*time.Hour)

	if store.benchRuns["mix-simple"].Status != benchmark.StatusFailed {
		t.Error("expected simple run to be failed")
	}
	if store.benchRuns["mix-tooluse"].Status != benchmark.StatusRunning {
		t.Error("expected tool_use run to still be running")
	}
	if store.benchRuns["mix-agent"].Status != benchmark.StatusRunning {
		t.Error("expected agent run to still be running")
	}
	if store.benchRuns["mix-empty"].Status != benchmark.StatusRunning {
		t.Error("expected empty-type run to still be running")
	}
}

func TestStartRun_NonExistentDataset_WithSuite_Continues(t *testing.T) {
	store := newBenchMockStore()
	// Use /tmp as datasetsDir — it exists but won't contain "nonexistent-dataset.yaml".
	svc := newTestBenchmarkServiceWithDir(store, "/tmp")
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
