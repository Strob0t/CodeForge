package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// BenchmarkService manages benchmark runs and results.
type BenchmarkService struct {
	store       database.Store
	datasetsDir string
	routingSvc  *RoutingService
	queue       messagequeue.Queue
	hub         broadcast.Broadcaster
}

// NewBenchmarkService creates a benchmark service.
func NewBenchmarkService(store database.Store, datasetsDir string) *BenchmarkService {
	return &BenchmarkService{store: store, datasetsDir: datasetsDir}
}

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
}

// SeedDefaultSuites creates built-in benchmark suites if they don't exist.
func (s *BenchmarkService) SeedDefaultSuites(ctx context.Context) {
	existing, err := s.store.ListBenchmarkSuites(ctx)
	if err != nil {
		slog.Warn("failed to list suites for seeding", "error", err)
		return
	}
	seen := make(map[string]bool, len(existing))
	for i := range existing {
		seen[existing[i].ProviderName] = true
	}
	for i := range defaultSuites {
		def := defaultSuites[i]
		if seen[def.ProviderName] {
			continue
		}
		if _, err := s.RegisterSuite(ctx, &def); err != nil {
			slog.Warn("failed to seed benchmark suite", "name", def.Name, "error", err)
		} else {
			slog.Info("seeded benchmark suite", "name", def.Name, "provider", def.ProviderName)
		}
	}
}

// SetRoutingService sets the routing service for benchmark → routing integration.
func (s *BenchmarkService) SetRoutingService(routingSvc *RoutingService) {
	s.routingSvc = routingSvc
}

// SetQueue sets the NATS queue for publishing benchmark requests.
func (s *BenchmarkService) SetQueue(q messagequeue.Queue) { s.queue = q }

// SetHub sets the WebSocket hub for broadcasting benchmark progress events.
func (s *BenchmarkService) SetHub(hub broadcast.Broadcaster) { s.hub = hub }

// RegisterSuite validates and persists a new benchmark suite.
func (s *BenchmarkService) RegisterSuite(ctx context.Context, req *benchmark.CreateSuiteRequest) (*benchmark.Suite, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	suite := &benchmark.Suite{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Description:  req.Description,
		Type:         req.Type,
		ProviderName: req.ProviderName,
		Config:       req.Config,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.CreateBenchmarkSuite(ctx, suite); err != nil {
		return nil, err
	}
	return suite, nil
}

// GetSuite retrieves a benchmark suite by ID.
func (s *BenchmarkService) GetSuite(ctx context.Context, id string) (*benchmark.Suite, error) {
	return s.store.GetBenchmarkSuite(ctx, id)
}

// ListSuites returns all registered benchmark suites.
func (s *BenchmarkService) ListSuites(ctx context.Context) ([]benchmark.Suite, error) {
	return s.store.ListBenchmarkSuites(ctx)
}

// UpdateSuite updates an existing benchmark suite.
func (s *BenchmarkService) UpdateSuite(ctx context.Context, suite *benchmark.Suite) error {
	return s.store.UpdateBenchmarkSuite(ctx, suite)
}

// DeleteSuite removes a benchmark suite by ID.
func (s *BenchmarkService) DeleteSuite(ctx context.Context, id string) error {
	return s.store.DeleteBenchmarkSuite(ctx, id)
}

// CreateRun validates and persists a new benchmark run.
func (s *BenchmarkService) CreateRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	rolloutCount := req.RolloutCount
	if rolloutCount < 1 {
		rolloutCount = 1
	}
	rolloutStrategy := req.RolloutStrategy
	if rolloutStrategy == "" {
		rolloutStrategy = "best"
	}
	r := &benchmark.Run{
		ID:                 uuid.New().String(),
		Dataset:            req.Dataset,
		Model:              req.Model,
		Metrics:            req.Metrics,
		Status:             benchmark.StatusRunning,
		SuiteID:            req.SuiteID,
		BenchmarkType:      req.BenchmarkType,
		ExecMode:           req.ExecMode,
		HybridVerification: req.HybridVerification,
		RolloutCount:       rolloutCount,
		RolloutStrategy:    rolloutStrategy,
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.store.CreateBenchmarkRun(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// StartRun creates a benchmark run in the database and publishes it to NATS
// for Python worker execution. Falls back to CreateRun (DB-only) if queue is nil.
func (s *BenchmarkService) StartRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	run, err := s.CreateRun(ctx, req)
	if err != nil {
		return nil, err
	}

	if s.queue == nil {
		slog.Warn("benchmark NATS queue not configured, run will stay in running state", "run_id", run.ID)
		return run, nil
	}

	// Resolve dataset name to absolute file path (e.g. "basic-coding" → "/workspaces/.../configs/benchmarks/basic-coding.yaml").
	datasetPath := run.Dataset
	if s.datasetsDir != "" && !filepath.IsAbs(datasetPath) {
		base := datasetPath
		if !strings.HasSuffix(base, ".yaml") {
			base += ".yaml"
		}
		candidate := filepath.Join(s.datasetsDir, base)
		absCandidate, _ := filepath.Abs(candidate)
		if _, statErr := os.Stat(absCandidate); statErr == nil {
			datasetPath = absCandidate
			slog.Info("resolved dataset path", "original", run.Dataset, "resolved", datasetPath)
		} else {
			slog.Warn("dataset path resolution failed", "original", run.Dataset, "candidate", absCandidate, "error", statErr)
		}
	}

	// Resolve provider info from suite (if suite-based run).
	var providerName string
	var providerConfig json.RawMessage
	if run.SuiteID != "" {
		suite, sErr := s.store.GetBenchmarkSuite(ctx, run.SuiteID)
		if sErr != nil {
			slog.Warn("failed to load suite for run, falling back to dataset path", "suite_id", run.SuiteID, "error", sErr)
		} else {
			providerName = suite.ProviderName
			providerConfig = mergeProviderConfig(suite.Config, req.ProviderConfig)
			if run.BenchmarkType == "" {
				run.BenchmarkType = suite.Type
			}
		}
	}

	payload := messagequeue.BenchmarkRunRequestPayload{
		RunID:              run.ID,
		TenantID:           tenantctx.FromContext(ctx),
		DatasetPath:        datasetPath,
		Model:              run.Model,
		Metrics:            run.Metrics,
		BenchmarkType:      string(run.BenchmarkType),
		SuiteID:            run.SuiteID,
		ExecMode:           string(run.ExecMode),
		Evaluators:         run.Metrics, // metrics double as evaluator names
		HybridVerification: run.HybridVerification,
		RolloutCount:       run.RolloutCount,
		RolloutStrategy:    run.RolloutStrategy,
		ProviderName:       providerName,
		ProviderConfig:     providerConfig,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark run request: %w", err)
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectBenchmarkRunRequest, data); err != nil {
		// Run is already saved — mark as failed if we can't dispatch.
		slog.Error("failed to publish benchmark run request", "run_id", run.ID, "error", err)
		run.Status = benchmark.StatusFailed
		_ = s.store.UpdateBenchmarkRun(ctx, run) //nolint:errcheck // best effort
		return nil, fmt.Errorf("publish benchmark run request: %w", err)
	}

	slog.Info("benchmark run dispatched to worker", "run_id", run.ID, "model", run.Model, "dataset", run.Dataset)
	return run, nil
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

// GetRun retrieves a benchmark run by ID.
func (s *BenchmarkService) GetRun(ctx context.Context, id string) (*benchmark.Run, error) {
	return s.store.GetBenchmarkRun(ctx, id)
}

// ListRuns returns all benchmark runs.
func (s *BenchmarkService) ListRuns(ctx context.Context) ([]benchmark.Run, error) {
	return s.store.ListBenchmarkRuns(ctx)
}

// ListRunsFiltered returns benchmark runs matching the given filter.
func (s *BenchmarkService) ListRunsFiltered(ctx context.Context, filter *benchmark.RunFilter) ([]benchmark.Run, error) {
	return s.store.ListBenchmarkRunsFiltered(ctx, filter)
}

// UpdateRun updates a benchmark run. When the run transitions to completed,
// its results are asynchronously seeded into the routing system for MAB learning.
func (s *BenchmarkService) UpdateRun(ctx context.Context, r *benchmark.Run) error {
	if err := s.store.UpdateBenchmarkRun(ctx, r); err != nil {
		return err
	}

	// Seed routing outcomes from completed benchmark runs.
	if r.Status == benchmark.StatusCompleted && s.routingSvc != nil {
		go func() {
			if _, err := s.routingSvc.SeedFromBenchmarkRun(ctx, r.ID); err != nil {
				slog.Warn("seed routing from benchmark run failed", "run_id", r.ID, "error", err)
			}
		}()
	}

	return nil
}

// DeleteRun deletes a benchmark run and its results.
func (s *BenchmarkService) DeleteRun(ctx context.Context, id string) error {
	return s.store.DeleteBenchmarkRun(ctx, id)
}

// ListResults returns all results for a benchmark run.
func (s *BenchmarkService) ListResults(ctx context.Context, runID string) ([]benchmark.Result, error) {
	return s.store.ListBenchmarkResults(ctx, runID)
}

// Compare loads two runs and their results for side-by-side comparison.
func (s *BenchmarkService) Compare(ctx context.Context, idA, idB string) (*benchmark.CompareResult, error) {
	runA, err := s.store.GetBenchmarkRun(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("run A: %w", err)
	}
	runB, err := s.store.GetBenchmarkRun(ctx, idB)
	if err != nil {
		return nil, fmt.Errorf("run B: %w", err)
	}
	resultsA, err := s.store.ListBenchmarkResults(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("results A: %w", err)
	}
	resultsB, err := s.store.ListBenchmarkResults(ctx, idB)
	if err != nil {
		return nil, fmt.Errorf("results B: %w", err)
	}
	return &benchmark.CompareResult{
		RunA:    runA,
		RunB:    runB,
		ResultA: resultsA,
		ResultB: resultsB,
	}, nil
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

// ListDatasets scans the datasets directory for YAML files and returns metadata.
func (s *BenchmarkService) ListDatasets() ([]benchmark.DatasetInfo, error) {
	if s.datasetsDir == "" {
		return nil, nil
	}

	var datasets []benchmark.DatasetInfo
	err := filepath.WalkDir(s.datasetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // path is from WalkDir within datasetsDir
		if err != nil {
			return nil // skip unreadable files
		}
		var df datasetFile
		if err := yaml.Unmarshal(data, &df); err != nil {
			return nil // skip invalid files
		}

		rel, _ := filepath.Rel(s.datasetsDir, path)
		datasets = append(datasets, benchmark.DatasetInfo{
			Name:        df.Name,
			Description: df.Description,
			TaskCount:   len(df.Tasks),
			Path:        rel,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk datasets dir: %w", err)
	}
	return datasets, nil
}

// CompareMulti loads N runs and their results for multi-way comparison.
func (s *BenchmarkService) CompareMulti(ctx context.Context, runIDs []string) ([]benchmark.MultiCompareEntry, error) {
	if len(runIDs) < 2 {
		return nil, fmt.Errorf("at least 2 run IDs are required for multi-comparison")
	}

	entries := make([]benchmark.MultiCompareEntry, len(runIDs))
	errs := make([]error, len(runIDs))
	var wg sync.WaitGroup

	for i, id := range runIDs {
		wg.Add(1)
		go func(idx int, runID string) {
			defer wg.Done()
			run, err := s.store.GetBenchmarkRun(ctx, runID)
			if err != nil {
				errs[idx] = fmt.Errorf("run %s: %w", runID, err)
				return
			}
			results, err := s.store.ListBenchmarkResults(ctx, runID)
			if err != nil {
				errs[idx] = fmt.Errorf("results for %s: %w", runID, err)
				return
			}
			entries[idx] = benchmark.MultiCompareEntry{Run: run, Results: results}
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// CostAnalysis computes cost breakdown and efficiency metrics for a benchmark run.
func (s *BenchmarkService) CostAnalysis(ctx context.Context, runID string) (*benchmark.CostAnalysis, error) {
	run, err := s.store.GetBenchmarkRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	results, err := s.store.ListBenchmarkResults(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("results: %w", err)
	}

	var totalCost float64
	var totalTokensIn, totalTokensOut int
	var totalScore float64
	var scoreCount int

	breakdown := make([]benchmark.CostBreakdown, 0, len(results))
	for i := range results {
		taskScore := avgScoreFromJSON(results[i].Scores)
		breakdown = append(breakdown, benchmark.CostBreakdown{
			TaskID:    results[i].TaskID,
			TaskName:  results[i].TaskName,
			CostUSD:   results[i].CostUSD,
			TokensIn:  results[i].TokensIn,
			TokensOut: results[i].TokensOut,
			Score:     taskScore,
		})
		totalCost += results[i].CostUSD
		totalTokensIn += results[i].TokensIn
		totalTokensOut += results[i].TokensOut
		if taskScore > 0 {
			totalScore += taskScore
			scoreCount++
		}
	}

	avgScore := 0.0
	if scoreCount > 0 {
		avgScore = totalScore / float64(scoreCount)
	}

	costPerPoint := 0.0
	if avgScore > 0 {
		costPerPoint = totalCost / avgScore
	}

	tokenEff := 0.0
	totalTokens := totalTokensIn + totalTokensOut
	if totalTokens > 0 && avgScore > 0 {
		tokenEff = avgScore / float64(totalTokens) * 1000 // score per 1K tokens
	}

	return &benchmark.CostAnalysis{
		RunID:             runID,
		Model:             run.Model,
		SuiteID:           run.SuiteID,
		TotalCostUSD:      totalCost,
		TotalTokensIn:     totalTokensIn,
		TotalTokensOut:    totalTokensOut,
		AvgScore:          avgScore,
		CostPerScorePoint: costPerPoint,
		TokenEfficiency:   tokenEff,
		TaskBreakdown:     breakdown,
	}, nil
}

// Leaderboard computes ranked model performance across runs for a given suite.
func (s *BenchmarkService) Leaderboard(ctx context.Context, suiteID string) ([]benchmark.LeaderboardEntry, error) {
	filter := &benchmark.RunFilter{SuiteID: suiteID}
	runs, err := s.store.ListBenchmarkRunsFiltered(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	if len(runs) == 0 {
		return []benchmark.LeaderboardEntry{}, nil
	}

	// Build one entry per run (each run = one model execution).
	entries := make([]benchmark.LeaderboardEntry, 0, len(runs))
	for i := range runs {
		run := &runs[i]
		if run.Status != benchmark.StatusCompleted {
			continue
		}

		results, err := s.store.ListBenchmarkResults(ctx, run.ID)
		if err != nil {
			continue // skip runs with missing results
		}

		var totalCost float64
		var totalTokensIn, totalTokensOut int
		var totalScore float64
		var scoreCount int

		for i := range results {
			taskScore := avgScoreFromJSON(results[i].Scores)
			totalCost += results[i].CostUSD
			totalTokensIn += results[i].TokensIn
			totalTokensOut += results[i].TokensOut
			if taskScore > 0 {
				totalScore += taskScore
				scoreCount++
			}
		}

		avgScore := 0.0
		if scoreCount > 0 {
			avgScore = totalScore / float64(scoreCount)
		}

		costPerPoint := 0.0
		if avgScore > 0 {
			costPerPoint = totalCost / avgScore
		}

		tokenEff := 0.0
		totalTokens := totalTokensIn + totalTokensOut
		if totalTokens > 0 && avgScore > 0 {
			tokenEff = avgScore / float64(totalTokens) * 1000
		}

		entries = append(entries, benchmark.LeaderboardEntry{
			Model:             run.Model,
			RunID:             run.ID,
			SuiteID:           run.SuiteID,
			AvgScore:          avgScore,
			TotalCostUSD:      totalCost,
			TotalTokensIn:     totalTokensIn,
			TotalTokensOut:    totalTokensOut,
			TaskCount:         len(results),
			CostPerScorePoint: costPerPoint,
			TokenEfficiency:   tokenEff,
			DurationMs:        run.TotalDurationMs,
		})
	}

	// Sort by avg_score descending (best first).
	sortLeaderboard(entries)
	return entries, nil
}

// sortLeaderboard sorts entries by AvgScore descending.
func sortLeaderboard(entries []benchmark.LeaderboardEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].AvgScore > entries[j-1].AvgScore; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// ExportTrainingPairs generates chosen/rejected DPO pairs from multi-rollout results.
func (s *BenchmarkService) ExportTrainingPairs(ctx context.Context, runID string) ([]benchmark.TrainingPair, error) {
	results, err := s.store.ListBenchmarkResults(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("results: %w", err)
	}

	// Group results by TaskID.
	grouped := make(map[string][]benchmark.Result)
	for i := range results {
		grouped[results[i].TaskID] = append(grouped[results[i].TaskID], results[i])
	}

	var pairs []benchmark.TrainingPair
	for taskID, taskResults := range grouped {
		if len(taskResults) < 2 {
			continue
		}

		// Find the best rollout.
		var best *benchmark.Result
		for i := range taskResults {
			if taskResults[i].IsBestRollout {
				best = &taskResults[i]
				break
			}
		}
		if best == nil {
			continue
		}

		chosenScore := avgScoreFromJSON(best.Scores)
		chosen := resultToTrainingEntry(best, chosenScore)

		for i := range taskResults {
			r := &taskResults[i]
			if r.IsBestRollout {
				continue
			}
			rejectedScore := avgScoreFromJSON(r.Scores)
			rejected := resultToTrainingEntry(r, rejectedScore)
			pairs = append(pairs, benchmark.TrainingPair{
				TaskID:   taskID,
				Prompt:   best.TaskName,
				Chosen:   chosen,
				Rejected: rejected,
				ScoreGap: chosenScore - rejectedScore,
			})
		}
	}

	return pairs, nil
}

func resultToTrainingEntry(r *benchmark.Result, avgScore float64) benchmark.TrainingEntry {
	scores := make(map[string]float64)
	_ = json.Unmarshal(r.Scores, &scores) //nolint:errcheck // best effort

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

// HandleBenchmarkRunResult processes a benchmark.run.result message from Python.
// It stores individual task results, updates the run status, and broadcasts WS events.
func (s *BenchmarkService) HandleBenchmarkRunResult(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.BenchmarkRunResultPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal benchmark run result: %w", err)
	}

	slog.Info("benchmark run result received",
		"run_id", payload.RunID,
		"status", payload.Status,
		"results", len(payload.Results),
		"total_cost", payload.TotalCost,
	)

	// Idempotency: skip if run is already completed/failed (NATS redelivery).
	existing, err := s.store.GetBenchmarkRun(ctx, payload.RunID)
	if err == nil && (existing.Status == benchmark.StatusCompleted || existing.Status == benchmark.StatusFailed) {
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
	}

	// Store summary scores if provided.
	if payload.Summary.TaskCount > 0 {
		summaryJSON, _ := json.Marshal(payload.Summary) //nolint:errcheck // best effort
		run.SummaryScores = summaryJSON
	}

	if err := s.UpdateRun(ctx, run); err != nil {
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
