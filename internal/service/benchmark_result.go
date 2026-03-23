package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// BenchmarkResultAggregator handles benchmark results, comparison, leaderboard, and export.
type BenchmarkResultAggregator struct {
	store database.Store
}

// NewBenchmarkResultAggregator creates a result aggregator.
func NewBenchmarkResultAggregator(store database.Store) *BenchmarkResultAggregator {
	return &BenchmarkResultAggregator{store: store}
}

// ListResults returns all results for a benchmark run.
func (a *BenchmarkResultAggregator) ListResults(ctx context.Context, runID string) ([]benchmark.Result, error) {
	return a.store.ListBenchmarkResults(ctx, runID)
}

// Compare loads two runs and their results for side-by-side comparison.
func (a *BenchmarkResultAggregator) Compare(ctx context.Context, idA, idB string) (*benchmark.CompareResult, error) {
	runA, err := a.store.GetBenchmarkRun(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("run A: %w", err)
	}
	runB, err := a.store.GetBenchmarkRun(ctx, idB)
	if err != nil {
		return nil, fmt.Errorf("run B: %w", err)
	}
	resultsA, err := a.store.ListBenchmarkResults(ctx, idA)
	if err != nil {
		return nil, fmt.Errorf("results A: %w", err)
	}
	resultsB, err := a.store.ListBenchmarkResults(ctx, idB)
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

// CompareMulti loads N runs and their results for multi-way comparison.
func (a *BenchmarkResultAggregator) CompareMulti(ctx context.Context, runIDs []string) ([]benchmark.MultiCompareEntry, error) {
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
			run, err := a.store.GetBenchmarkRun(ctx, runID)
			if err != nil {
				errs[idx] = fmt.Errorf("run %s: %w", runID, err)
				return
			}
			results, err := a.store.ListBenchmarkResults(ctx, runID)
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
func (a *BenchmarkResultAggregator) CostAnalysis(ctx context.Context, runID string) (*benchmark.CostAnalysis, error) {
	run, err := a.store.GetBenchmarkRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	results, err := a.store.ListBenchmarkResults(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("results: %w", err)
	}

	var totalCost float64
	var totalTokensIn, totalTokensOut int64
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
func (a *BenchmarkResultAggregator) Leaderboard(ctx context.Context, suiteID string) ([]benchmark.LeaderboardEntry, error) {
	filter := &benchmark.RunFilter{SuiteID: suiteID}
	runs, err := a.store.ListBenchmarkRunsFiltered(ctx, filter)
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

		results, err := a.store.ListBenchmarkResults(ctx, run.ID)
		if err != nil {
			continue // skip runs with missing results
		}

		var totalCost float64
		var totalTokensIn, totalTokensOut int64
		var totalScore float64
		var scoreCount int

		for j := range results {
			taskScore := avgScoreFromJSON(results[j].Scores)
			totalCost += results[j].CostUSD
			totalTokensIn += results[j].TokensIn
			totalTokensOut += results[j].TokensOut
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

// ExportTrainingPairs generates chosen/rejected DPO pairs from multi-rollout results.
func (a *BenchmarkResultAggregator) ExportTrainingPairs(ctx context.Context, runID string) ([]benchmark.TrainingPair, error) {
	results, err := a.store.ListBenchmarkResults(ctx, runID)
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

// ExportRLVRDataset generates RLVR training entries from a benchmark run.
// Each result becomes one entry with prompt, response, scalar reward, and metadata.
func (a *BenchmarkResultAggregator) ExportRLVRDataset(ctx context.Context, runID string) ([]benchmark.RLVREntry, error) {
	run, err := a.store.GetBenchmarkRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	results, err := a.store.ListBenchmarkResults(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("results: %w", err)
	}

	entries := make([]benchmark.RLVREntry, 0, len(results))
	for i := range results {
		scores := make(map[string]float64)
		_ = json.Unmarshal(results[i].Scores, &scores) //nolint:errcheck // best effort

		entries = append(entries, benchmark.RLVREntry{
			Prompt:   results[i].TaskName,
			Response: results[i].ActualOutput,
			Reward:   ComputeRLVRReward(scores),
			Metadata: map[string]string{
				"task_id": results[i].TaskID,
				"model":   run.Model,
				"run_id":  runID,
			},
		})
	}

	return entries, nil
}
