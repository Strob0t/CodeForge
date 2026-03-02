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
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// BenchmarkService manages benchmark runs and results.
type BenchmarkService struct {
	store       database.Store
	datasetsDir string
	routingSvc  *RoutingService
}

// NewBenchmarkService creates a benchmark service.
func NewBenchmarkService(store database.Store, datasetsDir string) *BenchmarkService {
	return &BenchmarkService{store: store, datasetsDir: datasetsDir}
}

// SetRoutingService sets the routing service for benchmark → routing integration.
func (s *BenchmarkService) SetRoutingService(routingSvc *RoutingService) {
	s.routingSvc = routingSvc
}

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

// DeleteSuite removes a benchmark suite by ID.
func (s *BenchmarkService) DeleteSuite(ctx context.Context, id string) error {
	return s.store.DeleteBenchmarkSuite(ctx, id)
}

// CreateRun validates and persists a new benchmark run.
func (s *BenchmarkService) CreateRun(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	r := &benchmark.Run{
		ID:            uuid.New().String(),
		Dataset:       req.Dataset,
		Model:         req.Model,
		Metrics:       req.Metrics,
		Status:        benchmark.StatusRunning,
		SuiteID:       req.SuiteID,
		BenchmarkType: req.BenchmarkType,
		ExecMode:      req.ExecMode,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.store.CreateBenchmarkRun(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
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
func (s *BenchmarkService) ListRunsFiltered(ctx context.Context, filter benchmark.RunFilter) ([]benchmark.Run, error) {
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
	entries := make([]benchmark.MultiCompareEntry, 0, len(runIDs))
	for _, id := range runIDs {
		run, err := s.store.GetBenchmarkRun(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("run %s: %w", id, err)
		}
		results, err := s.store.ListBenchmarkResults(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("results for %s: %w", id, err)
		}
		entries = append(entries, benchmark.MultiCompareEntry{
			Run:     run,
			Results: results,
		})
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
	filter := benchmark.RunFilter{SuiteID: suiteID}
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
