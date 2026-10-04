package service_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
)

// S6-G re-review 3: the training exports never treat an evaluator error as a
// score. Old stored rows carry the error as a `<evaluator>_error` score key;
// new rows report it in evaluation_errors. A result with an evaluation error
// and no valid score has no reward and is left out of both exports; a
// partially scored result is scored on its valid dimensions only.

func errorsTestResult(taskID string, rollout int, best bool, scores string, evalErrors map[string]string) benchmark.Result {
	return benchmark.Result{
		RunID: "run-err", TaskID: taskID, TaskName: "task " + taskID, ActualOutput: "out " + taskID,
		Scores: json.RawMessage(scores), EvaluationErrors: evalErrors, RolloutID: rollout, IsBestRollout: best,
	}
}

func TestExportRLVRDataset_EvaluationErrors(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	store.benchRuns["run-err"] = &benchmark.Run{ID: "run-err", Model: "gpt-4"}
	store.benchResult["run-err"] = []benchmark.Result{
		errorsTestResult("old-unscored", 0, false, `{"llm_judge_error":0.0}`, nil),
		errorsTestResult("old-partial", 0, false, `{"correctness":0.6,"llm_judge_error":0.0}`, nil),
		errorsTestResult("new-unscored", 0, false, `{}`, map[string]string{"llm_judge": "proxy down"}),
		errorsTestResult("new-partial", 0, false, `{"functional_test":1.0}`, map[string]string{"llm_judge": "proxy down"}),
		errorsTestResult("not-evaluated", 0, false, `{}`, nil),
	}

	entries, err := svc.ExportRLVRDataset(context.Background(), "run-err")
	if err != nil {
		t.Fatal(err)
	}
	rewards := map[string]float64{}
	for _, e := range entries {
		rewards[e.Metadata["task_id"]] = e.Reward
	}
	want := map[string]float64{"old-partial": 0.6, "new-partial": 1.0, "not-evaluated": 0.0}
	if len(rewards) != len(want) {
		t.Fatalf("exported tasks = %v, want %v (results without a valid score left out)", rewards, want)
	}
	for task, reward := range want {
		got, ok := rewards[task]
		if !ok || math.Abs(got-reward) > 1e-9 {
			t.Errorf("reward of %s = %v (exported %v), want %v", task, got, ok, reward)
		}
	}
}

func TestExportTrainingPairs_EvaluationErrors(t *testing.T) {
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	store.benchRuns["run-err"] = &benchmark.Run{ID: "run-err", Model: "gpt-4"}
	store.benchResult["run-err"] = []benchmark.Result{
		errorsTestResult("t1", 0, true, `{"correctness":0.9}`, nil),
		errorsTestResult("t1", 1, false, `{"correctness":0.4,"llm_judge_error":0.0}`, nil),
		errorsTestResult("t1", 2, false, `{"llm_judge_error":0.0}`, nil),
		errorsTestResult("t1", 3, false, `{}`, map[string]string{"llm_judge": "proxy down"}),
		// The best rollout of t2 has no valid score: no pair for t2.
		errorsTestResult("t2", 0, true, `{}`, map[string]string{"llm_judge": "proxy down"}),
		errorsTestResult("t2", 1, false, `{"correctness":0.2}`, nil),
	}

	pairs, err := svc.ExportTrainingPairs(context.Background(), "run-err")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 {
		t.Fatalf("pairs = %+v, want one (t1 best vs. its partially scored rollout)", pairs)
	}
	p := pairs[0]
	if p.TaskID != "t1" || p.Rejected.RolloutID != 1 {
		t.Fatalf("pair = %s chosen %d rejected %d", p.TaskID, p.Chosen.RolloutID, p.Rejected.RolloutID)
	}
	if math.Abs(p.Rejected.AvgScore-0.4) > 1e-9 || math.Abs(p.ScoreGap-0.5) > 1e-9 {
		t.Errorf("rejected avg %v gap %v, want 0.4 and 0.5 (error markers left out)", p.Rejected.AvgScore, p.ScoreGap)
	}
	if _, ok := p.Rejected.Scores["llm_judge_error"]; ok {
		t.Errorf("rejected scores = %v, want no error marker", p.Rejected.Scores)
	}
}
