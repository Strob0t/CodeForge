package benchmark_test

import (
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

func TestCostAnalysis_JSONRoundTrip(t *testing.T) {
	ca := benchmark.CostAnalysis{
		RunID:             "run-1",
		Model:             "gpt-4",
		SuiteID:           "suite-1",
		TotalCostUSD:      0.125,
		TotalTokensIn:     5000,
		TotalTokensOut:    2000,
		AvgScore:          0.85,
		CostPerScorePoint: 0.147,
		TokenEfficiency:   0.121,
		TaskBreakdown: []benchmark.CostBreakdown{
			{TaskID: "t1", TaskName: "FizzBuzz", CostUSD: 0.05, TokensIn: 2000, TokensOut: 800, Score: 0.9},
			{TaskID: "t2", TaskName: "Sort", CostUSD: 0.075, TokensIn: 3000, TokensOut: 1200, Score: 0.8},
		},
	}

	data, err := json.Marshal(ca)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.CostAnalysis
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.RunID != ca.RunID {
		t.Errorf("RunID mismatch: %s != %s", got.RunID, ca.RunID)
	}
	if got.Model != ca.Model {
		t.Errorf("Model mismatch: %s != %s", got.Model, ca.Model)
	}
	if len(got.TaskBreakdown) != 2 {
		t.Errorf("expected 2 task breakdowns, got %d", len(got.TaskBreakdown))
	}
	if got.TotalCostUSD != ca.TotalCostUSD {
		t.Errorf("TotalCostUSD mismatch: %f != %f", got.TotalCostUSD, ca.TotalCostUSD)
	}
}

func TestLeaderboardEntry_JSONRoundTrip(t *testing.T) {
	entry := benchmark.LeaderboardEntry{
		Model:             "claude-3",
		RunID:             "run-2",
		SuiteID:           "suite-1",
		AvgScore:          0.92,
		TotalCostUSD:      0.5,
		TotalTokensIn:     10000,
		TotalTokensOut:    4000,
		TaskCount:         50,
		CostPerScorePoint: 0.543,
		TokenEfficiency:   0.066,
		DurationMs:        30000,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.LeaderboardEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.Model != entry.Model {
		t.Errorf("Model mismatch: %s != %s", got.Model, entry.Model)
	}
	if got.AvgScore != entry.AvgScore {
		t.Errorf("AvgScore mismatch: %f != %f", got.AvgScore, entry.AvgScore)
	}
	if got.TaskCount != entry.TaskCount {
		t.Errorf("TaskCount mismatch: %d != %d", got.TaskCount, entry.TaskCount)
	}
}

func TestCostBreakdown_ZeroValues(t *testing.T) {
	cb := benchmark.CostBreakdown{}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.CostBreakdown
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.CostUSD != 0 || got.Score != 0 || got.TokensIn != 0 {
		t.Errorf("zero-value CostBreakdown should have all zeroes")
	}
}

func TestMultiCompareRequest_Empty(t *testing.T) {
	req := benchmark.MultiCompareRequest{RunIDs: []string{}}
	if len(req.RunIDs) != 0 {
		t.Errorf("expected empty run IDs")
	}
}

func TestLeaderboardEntry_EmptySuiteID(t *testing.T) {
	entry := benchmark.LeaderboardEntry{
		Model:    "gpt-4",
		RunID:    "run-1",
		AvgScore: 0.75,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}

	var got benchmark.LeaderboardEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.SuiteID != "" {
		t.Errorf("SuiteID should be empty: %q", got.SuiteID)
	}
}
