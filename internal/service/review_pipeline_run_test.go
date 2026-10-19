package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S6-F 1: the pipeline presets (the review pipelines among them) run
// through every step. Each step's run
// completes through the runtime's completion path, which validates the
// artifact its mode requires; a mode whose artifact type is unknown fails
// its run and with it the plan.

// sampleArtifacts is a valid output for each artifact type a mode requires.
var sampleArtifacts = map[string]string{
	"":                   "done",
	"PLAN.md":            "## Plan\nStep one: read the code. Step two: change it. Step three: test it thoroughly before delivery.",
	"DIFF":               "+ added line\n- removed line",
	"REVIEW.md":          "# Review\nOne finding: the handler ignores errors; suggestion: return them.",
	"TEST_REPORT":        `{"status": "passed"}`,
	"AUDIT_REPORT":       "# Audit\nNo vulnerability found; the remaining risk is low.",
	"DECISION.md":        "Decision: keep the cache. Rationale: it halves the latency.",
	"BOUNDARIES.json":    "```json\n[{\"path\": \"api/user.proto\", \"type\": \"api\"}]\n```",
	"CONTRACT_REVIEW.md": "# Contract review\nProducers and consumers agree.",
	"PROPOSAL.md":        "Use the existing cache.",
	"SYNTHESIS.md":       "Both proposals agree on the cache.",
}

func TestPipelinePresets_RunThroughEveryStep(t *testing.T) {
	modes := service.NewModeService()
	pipelines := service.NewPipelineService(modes)
	templates := pipelines.List()
	seen := map[string]bool{}
	for i := range templates {
		seen[templates[i].ID] = true
	}
	for _, id := range []string{"boundary-analysis", "review-refactor"} {
		if !seen[id] {
			t.Fatalf("preset %s missing", id)
		}
	}
	for i := range templates {
		tmpl := templates[i]
		t.Run(tmpl.ID, func(t *testing.T) {
			store := newOrchStore()
			store.tasks = newPendingTasks("t1", "t2", "t3", "t4", "t5", "t6")
			bc := &runtimeMockBroadcaster{}
			es := &runtimeMockEventStore{}
			rt := service.NewRuntimeService(store, &runtimeMockQueue{}, bc, es,
				service.NewPolicyService("plan-readonly", nil), &config.Runtime{StallThreshold: 5})
			rt.SetModeService(modes)
			orch := service.NewOrchestratorService(store, bc, es, rt, &config.Orchestrator{MaxParallel: 6, PingPongMaxRounds: 3})
			rt.SetOnRunComplete(orch.HandleRunCompleted)
			ctx := context.Background()

			bindings := make([]pipeline.StepBinding, len(tmpl.Steps))
			for i := range tmpl.Steps {
				bindings[i] = pipeline.StepBinding{TaskID: store.tasks[i].ID, AgentID: "a1"}
			}
			req, err := pipelines.Instantiate(ctx, tmpl.ID, pipeline.InstantiateRequest{ProjectID: "proj-1", Bindings: bindings})
			if err != nil {
				t.Fatalf("Instantiate: %v", err)
			}
			p, err := orch.CreatePlan(ctx, req)
			if err != nil {
				t.Fatalf("CreatePlan: %v", err)
			}
			if _, err := orch.StartPlan(ctx, p.ID); err != nil {
				t.Fatalf("StartPlan: %v", err)
			}

			// Complete every running step with a valid output for its mode's
			// artifact until the plan ends; each pass must make progress.
			for pass := 0; pass <= len(tmpl.Steps); pass++ {
				state := planState(t, store, p.ID)
				if state.Status != plan.StatusRunning {
					break
				}
				progressed := false
				for i := range state.Steps {
					step := state.Steps[i]
					if step.Status != plan.StepStatusRunning {
						continue
					}
					m, err := modes.Get(step.ModeID)
					if err != nil {
						t.Fatalf("mode %s: %v", step.ModeID, err)
					}
					output, ok := sampleArtifacts[m.RequiredArtifact]
					if !ok {
						t.Fatalf("mode %s requires artifact %q without a sample output", m.ID, m.RequiredArtifact)
					}
					if err := rt.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
						RunID: step.RunID, TaskID: step.TaskID, ProjectID: "proj-1", Status: string(run.StatusCompleted), Output: output,
					}); err != nil {
						t.Fatalf("HandleRunComplete step %d: %v", i, err)
					}
					if got := planState(t, store, p.ID).Steps[i]; got.Status != plan.StepStatusCompleted {
						t.Fatalf("step %d (%s, artifact %q) = %s (%s), want completed", i, step.ModeID, m.RequiredArtifact, got.Status, got.Error)
					}
					progressed = true
				}
				if !progressed {
					t.Fatalf("pass %d: no running step in a running plan", pass)
				}
			}
			got := planState(t, store, p.ID)
			if got.Status != plan.StatusCompleted {
				t.Fatalf("plan = %s, want completed", got.Status)
			}
			for i := range got.Steps {
				if got.Steps[i].Status != plan.StepStatusCompleted {
					t.Fatalf("step %d = %s, want completed", i, got.Steps[i].Status)
				}
			}
		})
	}
}
