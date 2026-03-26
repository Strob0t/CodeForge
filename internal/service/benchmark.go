package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// BenchmarkService is a thin orchestrator that composes focused sub-services
// for suites, runs, results, and watchdog. It owns the NATS message handlers
// and WebSocket broadcast logic that span multiple sub-services.
type BenchmarkService struct {
	Suites   *BenchmarkSuiteService
	Runs     *BenchmarkRunManager
	Results  *BenchmarkResultAggregator
	Watchdog *BenchmarkWatchdog

	store database.Store
	queue messagequeue.Queue
	hub   broadcast.Broadcaster
}

// NewBenchmarkService creates a benchmark orchestrator composing the given sub-services.
func NewBenchmarkService(
	suites *BenchmarkSuiteService,
	runs *BenchmarkRunManager,
	results *BenchmarkResultAggregator,
	watchdog *BenchmarkWatchdog,
) *BenchmarkService {
	return &BenchmarkService{
		Suites:   suites,
		Runs:     runs,
		Results:  results,
		Watchdog: watchdog,
		store:    suites.store, // shared store reference for NATS handlers
	}
}

// SetQueue sets the NATS queue for publishing benchmark requests and subscribing to results.
func (s *BenchmarkService) SetQueue(q messagequeue.Queue) {
	s.queue = q
	s.Runs.SetQueue(q)
}

// SetHub sets the WebSocket hub for broadcasting benchmark progress events.
func (s *BenchmarkService) SetHub(hub broadcast.Broadcaster) { s.hub = hub }

// SetRoutingService sets the routing service for benchmark -> routing integration.
func (s *BenchmarkService) SetRoutingService(routingSvc *RoutingService) {
	s.Runs.SetRoutingService(routingSvc)
}

// --- Delegation methods (preserve existing API for handlers) ---

// SeedDefaultSuites delegates to BenchmarkSuiteService.
func (s *BenchmarkService) SeedDefaultSuites(ctx context.Context) { s.Suites.SeedDefaultSuites(ctx) }

// RegisterSuite delegates to BenchmarkSuiteService.
func (s *BenchmarkService) RegisterSuite(ctx context.Context, req *benchmark.CreateSuiteRequest) (*benchmark.Suite, error) {
	return s.Suites.RegisterSuite(ctx, req)
}

// GetSuite delegates to BenchmarkSuiteService.
func (s *BenchmarkService) GetSuite(ctx context.Context, id string) (*benchmark.Suite, error) {
	return s.Suites.GetSuite(ctx, id)
}

// ListSuites delegates to BenchmarkSuiteService.
func (s *BenchmarkService) ListSuites(ctx context.Context) ([]benchmark.Suite, error) {
	return s.Suites.ListSuites(ctx)
}

// UpdateSuite delegates to BenchmarkSuiteService.
func (s *BenchmarkService) UpdateSuite(ctx context.Context, suite *benchmark.Suite) error {
	return s.Suites.UpdateSuite(ctx, suite)
}

// DeleteSuite delegates to BenchmarkSuiteService.
func (s *BenchmarkService) DeleteSuite(ctx context.Context, id string) error {
	return s.Suites.DeleteSuite(ctx, id)
}

// ListDatasets delegates to BenchmarkSuiteService.
func (s *BenchmarkService) ListDatasets() ([]benchmark.DatasetInfo, error) {
	return s.Suites.ListDatasets()
}

// CreateRun delegates to BenchmarkRunManager.
func (s *BenchmarkService) CreateRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	return s.Runs.CreateRun(ctx, req)
}

// StartRun delegates to BenchmarkRunManager.
func (s *BenchmarkService) StartRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	return s.Runs.StartRun(ctx, req)
}

// GetRun delegates to BenchmarkRunManager.
func (s *BenchmarkService) GetRun(ctx context.Context, id string) (*benchmark.Run, error) {
	return s.Runs.GetRun(ctx, id)
}

// ListRuns delegates to BenchmarkRunManager.
func (s *BenchmarkService) ListRuns(ctx context.Context) ([]benchmark.Run, error) {
	return s.Runs.ListRuns(ctx)
}

// ListRunsFiltered delegates to BenchmarkRunManager.
func (s *BenchmarkService) ListRunsFiltered(ctx context.Context, filter *benchmark.RunFilter) ([]benchmark.Run, error) {
	return s.Runs.ListRunsFiltered(ctx, filter)
}

// UpdateRun delegates to BenchmarkRunManager.
func (s *BenchmarkService) UpdateRun(ctx context.Context, r *benchmark.Run) error {
	return s.Runs.UpdateRun(ctx, r)
}

// DeleteRun delegates to BenchmarkRunManager.
func (s *BenchmarkService) DeleteRun(ctx context.Context, id string) error {
	return s.Runs.DeleteRun(ctx, id)
}

// ListResults delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) ListResults(ctx context.Context, runID string) ([]benchmark.Result, error) {
	return s.Results.ListResults(ctx, runID)
}

// Compare delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) Compare(ctx context.Context, idA, idB string) (*benchmark.CompareResult, error) {
	return s.Results.Compare(ctx, idA, idB)
}

// CompareMulti delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) CompareMulti(ctx context.Context, runIDs []string) ([]benchmark.MultiCompareEntry, error) {
	return s.Results.CompareMulti(ctx, runIDs)
}

// CostAnalysis delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) CostAnalysis(ctx context.Context, runID string) (*benchmark.CostAnalysis, error) {
	return s.Results.CostAnalysis(ctx, runID)
}

// Leaderboard delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) Leaderboard(ctx context.Context, suiteID string) ([]benchmark.LeaderboardEntry, error) {
	return s.Results.Leaderboard(ctx, suiteID)
}

// ExportTrainingPairs delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) ExportTrainingPairs(ctx context.Context, runID string) ([]benchmark.TrainingPair, error) {
	return s.Results.ExportTrainingPairs(ctx, runID)
}

// ExportRLVRDataset delegates to BenchmarkResultAggregator.
func (s *BenchmarkService) ExportRLVRDataset(ctx context.Context, runID string) ([]benchmark.RLVREntry, error) {
	return s.Results.ExportRLVRDataset(ctx, runID)
}

// RunWatchdogOnce delegates to BenchmarkWatchdog.
func (s *BenchmarkService) RunWatchdogOnce(ctx context.Context, timeout time.Duration) {
	s.Watchdog.RunWatchdogOnce(ctx, timeout)
}

// StartWatchdog delegates to BenchmarkWatchdog.
func (s *BenchmarkService) StartWatchdog(interval, timeout time.Duration) context.CancelFunc {
	return s.Watchdog.StartWatchdog(interval, timeout)
}

// --- NATS handlers (orchestrator-owned: they span store + hub + run updates) ---

// HandleBenchmarkRunResult processes a benchmark.run.result message from Python.
// It stores individual task results, updates the run status, and broadcasts WS events.
func (s *BenchmarkService) HandleBenchmarkRunResult(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.BenchmarkRunResultPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal benchmark run result: %w", err)
	}

	// Inject tenant context from NATS payload (background consumer has no tenant).
	if payload.TenantID != "" {
		ctx = tenantctx.WithTenant(ctx, payload.TenantID)
	}

	slog.Info("benchmark run result received",
		"run_id", payload.RunID,
		"status", payload.Status,
		"results", len(payload.Results),
		"total_cost", payload.TotalCost,
	)

	// Idempotency / stale message guard: skip if run doesn't exist or is already terminal.
	existing, err := s.store.GetBenchmarkRun(ctx, payload.RunID)
	if err != nil {
		slog.Warn("benchmark run not found, skipping stale result", "run_id", payload.RunID)
		return nil
	}

	// Verify tenant_id from NATS payload matches the run's actual tenant.
	if payload.TenantID != "" && existing.TenantID != "" && payload.TenantID != existing.TenantID {
		slog.Warn("NATS payload tenant_id mismatch, using run's tenant_id",
			"run_id", payload.RunID,
			"payload_tenant", payload.TenantID,
			"run_tenant", existing.TenantID,
		)
		ctx = tenantctx.WithTenant(ctx, existing.TenantID)
	}

	if existing.Status == benchmark.StatusCompleted || existing.Status == benchmark.StatusFailed {
		slog.Info("benchmark run result already processed, skipping", "run_id", payload.RunID)
		return nil
	}

	// Store each task result.
	for i := range payload.Results {
		tr := &payload.Results[i]

		scoresJSON, _ := json.Marshal(tr.Scores)              //nolint:errcheck // best effort
		toolCallsJSON, _ := json.Marshal(tr.ToolCalls)        //nolint:errcheck // best effort
		evalScoresJSON, _ := json.Marshal(tr.EvaluatorScores) //nolint:errcheck // best effort

		result := &benchmark.Result{
			ID:                   uuid.New().String(),
			RunID:                payload.RunID,
			TaskID:               tr.TaskID,
			TaskName:             tr.TaskName,
			Scores:               scoresJSON,
			ActualOutput:         tr.ActualOutput,
			ExpectedOutput:       tr.ExpectedOutput,
			ToolCalls:            toolCallsJSON,
			CostUSD:              tr.CostUSD,
			TokensIn:             tr.TokensIn,
			TokensOut:            tr.TokensOut,
			DurationMs:           tr.DurationMs,
			EvaluatorScores:      evalScoresJSON,
			FilesChanged:         tr.FilesChanged,
			FunctionalTestOutput: tr.FunctionalTestOutput,
			RolloutID:            tr.RolloutID,
			RolloutCount:         tr.RolloutCount,
			IsBestRollout:        tr.IsBestRollout,
			DiversityScore:       tr.DiversityScore,
			SelectedModel:        tr.SelectedModel,
			RoutingReason:        tr.RoutingReason,
			FallbackChain:        tr.FallbackChain,
			FallbackCount:        tr.FallbackCount,
			ProviderErrors:       tr.ProviderErrors,
		}

		if err := s.store.CreateBenchmarkResult(ctx, result); err != nil {
			slog.Error("failed to store benchmark result", "run_id", payload.RunID, "task_id", tr.TaskID, "error", err)
		}

		// Broadcast per-task completion event.
		if s.hub != nil {
			s.hub.BroadcastEvent(ctx, "benchmark.task.completed", BenchmarkTaskCompletedPayload{
				RunID:    payload.RunID,
				TaskID:   tr.TaskID,
				TaskName: tr.TaskName,
				Score:    avgFromMap(tr.Scores),
				CostUSD:  tr.CostUSD,
				Index:    i + 1,
				Total:    len(payload.Results),
			})
		}
	}

	// Update the run status.
	run, err := s.store.GetBenchmarkRun(ctx, payload.RunID)
	if err != nil {
		return fmt.Errorf("get run for update: %w", err)
	}

	now := time.Now().UTC()
	run.CompletedAt = &now
	run.TotalCost = payload.TotalCost
	if run.TotalCost == 0 {
		run.TotalCost = payload.Summary.TotalCostUSD
	}
	run.TotalTokens = payload.TotalTokens
	if run.TotalTokens == 0 {
		run.TotalTokens = payload.Summary.TotalTokensIn + payload.Summary.TotalTokensOut
	}
	run.TotalDurationMs = payload.TotalDurationMs
	if run.TotalDurationMs == 0 {
		run.TotalDurationMs = payload.Summary.ElapsedMs
	}

	if payload.Status == "completed" {
		run.Status = benchmark.StatusCompleted
	} else {
		run.Status = benchmark.StatusFailed
		if payload.Error != "" {
			run.ErrorMessage = payload.Error
		}
	}

	// Store summary scores if provided.
	if payload.Summary.TaskCount > 0 {
		summaryJSON, _ := json.Marshal(payload.Summary) //nolint:errcheck // best effort
		run.SummaryScores = summaryJSON
	}

	if err := s.Runs.UpdateRun(ctx, run); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}

	// Broadcast run progress / completion event.
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, "benchmark.run.progress", BenchmarkRunProgressPayload{
			RunID:          payload.RunID,
			Status:         string(run.Status),
			CompletedTasks: len(payload.Results),
			TotalTasks:     len(payload.Results),
			AvgScore:       payload.Summary.AvgScore,
			TotalCostUSD:   payload.TotalCost,
		})
	}

	return nil
}

// HandleBenchmarkTaskStarted processes a benchmark.task.started message from Python
// and broadcasts it to the frontend via WebSocket.
func (s *BenchmarkService) HandleBenchmarkTaskStarted(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.BenchmarkTaskStartedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal benchmark task started: %w", err)
	}

	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, "benchmark.task.started", BenchmarkTaskCompletedPayload{
			RunID:    payload.RunID,
			TaskID:   payload.TaskID,
			TaskName: payload.TaskName,
			Index:    payload.Index,
			Total:    payload.Total,
		})
	}

	return nil
}

// HandleBenchmarkTaskProgress processes a benchmark.task.progress message from Python
// and broadcasts task completion + run progress events to the frontend.
func (s *BenchmarkService) HandleBenchmarkTaskProgress(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.BenchmarkTaskProgressPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal benchmark task progress: %w", err)
	}

	if s.hub != nil {
		// Per-task completion event (for the feature list).
		s.hub.BroadcastEvent(ctx, "benchmark.task.completed", BenchmarkTaskCompletedPayload{
			RunID:    payload.RunID,
			TaskID:   payload.TaskID,
			TaskName: payload.TaskName,
			Score:    payload.Score,
			CostUSD:  payload.CostUSD,
			Index:    payload.CompletedTasks,
			Total:    payload.TotalTasks,
		})

		// Running progress event (for the progress bar).
		s.hub.BroadcastEvent(ctx, "benchmark.run.progress", BenchmarkRunProgressPayload{
			RunID:          payload.RunID,
			Status:         "running",
			CompletedTasks: payload.CompletedTasks,
			TotalTasks:     payload.TotalTasks,
			AvgScore:       payload.AvgScore,
			TotalCostUSD:   payload.TotalCostUSD,
		})
	}

	return nil
}

// StartResultSubscriber subscribes to benchmark NATS subjects.
// Returns a cancel function to stop all subscriptions.
func (s *BenchmarkService) StartResultSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}

	cancelResult, err := s.queue.Subscribe(ctx, messagequeue.SubjectBenchmarkRunResult, s.HandleBenchmarkRunResult)
	if err != nil {
		return func() {}, fmt.Errorf("subscribe benchmark result: %w", err)
	}

	cancelStarted, err := s.queue.Subscribe(ctx, messagequeue.SubjectBenchmarkTaskStarted, s.HandleBenchmarkTaskStarted)
	if err != nil {
		cancelResult()
		return func() {}, fmt.Errorf("subscribe benchmark task started: %w", err)
	}

	cancelProgress, err := s.queue.Subscribe(ctx, messagequeue.SubjectBenchmarkTaskProgress, s.HandleBenchmarkTaskProgress)
	if err != nil {
		cancelResult()
		cancelStarted()
		return func() {}, fmt.Errorf("subscribe benchmark task progress: %w", err)
	}

	return func() {
		cancelResult()
		cancelStarted()
		cancelProgress()
	}, nil
}

// --- Shared types and helper functions used across sub-services ---

// defaultSuites defines built-in benchmark suites seeded on startup.
var defaultSuites = []benchmark.CreateSuiteRequest{
	{Name: "Basic Coding", Type: benchmark.TypeSimple, ProviderName: "codeforge_simple"},
	{Name: "Agent Coding", Type: benchmark.TypeAgent, ProviderName: "codeforge_agent"},
	{Name: "Tool Use Basic", Type: benchmark.TypeToolUse, ProviderName: "codeforge_tool_use"},
	{Name: "HumanEval", Type: benchmark.TypeSimple, ProviderName: "humaneval"},
	{Name: "MBPP", Type: benchmark.TypeSimple, ProviderName: "mbpp"},
	{Name: "SWE-bench", Type: benchmark.TypeAgent, ProviderName: "swebench"},
	{Name: "BigCodeBench", Type: benchmark.TypeSimple, ProviderName: "bigcodebench"},
	{Name: "CRUXEval", Type: benchmark.TypeSimple, ProviderName: "cruxeval"},
	{Name: "LiveCodeBench", Type: benchmark.TypeSimple, ProviderName: "livecodebench"},
	{Name: "SPARCBench", Type: benchmark.TypeAgent, ProviderName: "sparcbench"},
	{Name: "Aider Polyglot", Type: benchmark.TypeAgent, ProviderName: "aider_polyglot"},
	{Name: "DPAI Arena", Type: benchmark.TypeSimple, ProviderName: "dpai_arena"},
	{Name: "Terminal-Bench", Type: benchmark.TypeAgent, ProviderName: "terminal_bench"},
}

// datasetFile is the YAML structure of a benchmark dataset file.
type datasetFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tasks       []struct {
		ID             string `yaml:"id"`
		Name           string `yaml:"name"`
		Input          string `yaml:"input"`
		ExpectedOutput string `yaml:"expected_output"`
	} `yaml:"tasks"`
}

// BenchmarkTaskCompletedPayload is broadcast when a single benchmark task finishes.
type BenchmarkTaskCompletedPayload struct {
	RunID    string  `json:"run_id"`
	TaskID   string  `json:"task_id"`
	TaskName string  `json:"task_name"`
	Score    float64 `json:"score"`
	CostUSD  float64 `json:"cost_usd"`
	Index    int     `json:"index"`
	Total    int     `json:"total"`
}

// BenchmarkRunProgressPayload is broadcast when a benchmark run progresses or completes.
type BenchmarkRunProgressPayload struct {
	RunID          string  `json:"run_id"`
	Status         string  `json:"status"`
	CompletedTasks int     `json:"completed_tasks"`
	TotalTasks     int     `json:"total_tasks"`
	AvgScore       float64 `json:"avg_score"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
}

// mergeProviderConfig merges suite-level config with request-level overrides.
func mergeProviderConfig(suiteConfig, requestConfig json.RawMessage) json.RawMessage {
	if len(requestConfig) == 0 || string(requestConfig) == "null" {
		return suiteConfig
	}
	if len(suiteConfig) == 0 || string(suiteConfig) == "null" {
		return requestConfig
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(suiteConfig, &base); err != nil {
		return requestConfig
	}
	var override map[string]json.RawMessage
	if err := json.Unmarshal(requestConfig, &override); err != nil {
		return suiteConfig
	}
	for k, v := range override {
		base[k] = v
	}
	merged, _ := json.Marshal(base) //nolint:errcheck // best effort
	return merged
}

// watchdogTimeoutForType returns a per-benchmark-type timeout. Simple runs
// timeout faster (30 min) while agent runs get more time (4h). The
// globalDefault is returned for unknown or empty types.
func watchdogTimeoutForType(bt benchmark.BenchmarkType, globalDefault time.Duration) time.Duration {
	switch bt {
	case benchmark.TypeSimple:
		return 30 * time.Minute
	case benchmark.TypeToolUse:
		return 60 * time.Minute
	case benchmark.TypeAgent:
		return 4 * time.Hour
	default:
		return globalDefault
	}
}

// sortLeaderboard sorts entries by AvgScore descending.
func sortLeaderboard(entries []benchmark.LeaderboardEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].AvgScore > entries[j-1].AvgScore; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// ParseScores unmarshals a JSON-encoded score map. Returns an empty map on any error.
func ParseScores(raw json.RawMessage) map[string]float64 {
	scores := make(map[string]float64)
	_ = json.Unmarshal(raw, &scores) //nolint:errcheck // best effort
	return scores
}

func resultToTrainingEntry(r *benchmark.Result, avgScore float64) benchmark.TrainingEntry {
	scores := ParseScores(r.Scores)

	return benchmark.TrainingEntry{
		RolloutID:   r.RolloutID,
		TaskID:      r.TaskID,
		TaskName:    r.TaskName,
		Output:      r.ActualOutput,
		Scores:      scores,
		AvgScore:    avgScore,
		CostUSD:     r.CostUSD,
		TokensTotal: r.TokensIn + r.TokensOut,
	}
}

// ComputeRLVRReward computes an RLVR reward from evaluation scores.
// Strategy: weighted average where functional_test scores get 2x weight.
// All other scores get 1x weight. Result is clamped to [0.0, 1.0].
func ComputeRLVRReward(scores map[string]float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}

	var totalWeighted, totalWeight float64
	for key, value := range scores {
		weight := 1.0
		if key == "functional_test" {
			weight = 2.0
		}
		totalWeighted += value * weight
		totalWeight += weight
	}

	if totalWeight == 0.0 {
		return 0.0
	}

	avg := totalWeighted / totalWeight

	// Clamp to [0.0, 1.0].
	if avg > 1.0 {
		return 1.0
	}
	if avg < 0.0 {
		return 0.0
	}
	return avg
}

// avgFromMap computes the average of a float64 map's values.
func avgFromMap(m map[string]float64) float64 {
	if len(m) == 0 {
		return 0
	}
	var total float64
	for _, v := range m {
		total += v
	}
	return total / float64(len(m))
}

// avgScoreFromJSON extracts the average score from a JSON scores map.
func avgScoreFromJSON(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var scores map[string]float64
	if err := json.Unmarshal(raw, &scores); err != nil {
		return 0
	}
	if len(scores) == 0 {
		return 0
	}
	var total float64
	for _, v := range scores {
		total += v
	}
	return total / float64(len(scores))
}
