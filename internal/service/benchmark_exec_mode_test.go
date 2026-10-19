package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// Agent benchmarks execute their tools in the worker exactly like runs, so the
// sandbox and hybrid modes must be rejected before anything is persisted (KI-13).
func TestBenchmarkRun_RejectsExecModesWithoutIsolation(t *testing.T) {
	type startFunc func(ctx context.Context, req *benchmark.CreateRunRequest) (*benchmark.Run, error)

	for _, mode := range []benchmark.ExecMode{benchmark.ExecModeSandbox, benchmark.ExecModeHybrid} {
		store := newBenchMockStore()
		svc := newTestBenchmarkService(store)
		q := &benchMockQueue{}
		svc.SetQueue(q)

		for name, start := range map[string]startFunc{"CreateRun": svc.CreateRun, "StartRun": svc.StartRun} {
			t.Run(string(mode)+"/"+name, func(t *testing.T) {
				r, err := start(context.Background(), &benchmark.CreateRunRequest{
					Dataset:       "basic-coding",
					Model:         "gpt-4",
					Metrics:       []string{"correctness"},
					BenchmarkType: benchmark.TypeAgent,
					ExecMode:      mode,
				})
				if !errors.Is(err, run.ErrExecModeUnavailable) || !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrExecModeUnavailable wrapping ErrValidation", err)
				}
				if r != nil {
					t.Fatalf("expected no run, got %+v", r)
				}
				if len(store.benchRuns) != 0 {
					t.Fatalf("expected no persisted run, got %d", len(store.benchRuns))
				}
				if len(q.published) != 0 {
					t.Fatalf("expected no NATS message, got %d", len(q.published))
				}
			})
		}
	}
}

func TestBenchmarkRun_MountExecModeIsAccepted(t *testing.T) {
	for _, mode := range []benchmark.ExecMode{"", benchmark.ExecModeMount} {
		t.Run("mode="+string(mode), func(t *testing.T) {
			store := newBenchMockStore()
			svc := newTestBenchmarkService(store)

			r, err := svc.CreateRun(context.Background(), &benchmark.CreateRunRequest{
				Dataset:       "basic-coding",
				Model:         "gpt-4",
				Metrics:       []string{"correctness"},
				BenchmarkType: benchmark.TypeAgent,
				ExecMode:      mode,
			})
			if err != nil {
				t.Fatalf("CreateRun: %v", err)
			}
			if r.ExecMode != mode {
				t.Fatalf("exec_mode = %q, want %q", r.ExecMode, mode)
			}
		})
	}
}
