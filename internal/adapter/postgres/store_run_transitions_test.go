package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// Run status writes follow the run's transitions (run.SourceStatuses); usage
// counters are updated without touching the status and never go down. A tool
// call's usage is added only while the run is running: once the worker
// reported its totals (gate, completion) or the run ended, the totals include
// the call (review 2, finding 1).

// runIn moves a fresh run to status through the store's own methods.
func (f *statusFixture) runIn(t *testing.T, status run.Status) *run.Run {
	t.Helper()
	switch status {
	case run.StatusPending, run.StatusRunning:
		return f.newRun(t, status)
	case run.StatusQualityGate:
		r := f.newRun(t, run.StatusRunning)
		if err := f.store.EnterQualityGate(f.ctx, &run.CompletionRequest{ID: r.ID, Status: run.StatusQualityGate}); err != nil {
			t.Fatalf("EnterQualityGate: %v", err)
		}
		return r
	default:
		r := f.newRun(t, run.StatusRunning)
		if err := f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: r.ID, Status: status}); err != nil {
			t.Fatalf("CompleteRun(%s): %v", status, err)
		}
		return r
	}
}

func TestStore_RunStatusTransitions(t *testing.T) {
	f := newStatusFixture(t)
	all := []run.Status{run.StatusPending, run.StatusRunning, run.StatusQualityGate,
		run.StatusCompleted, run.StatusFailed, run.StatusCancelled, run.StatusTimeout}

	write := func(id string, to run.Status) error {
		switch {
		case to == run.StatusQualityGate:
			return f.store.EnterQualityGate(f.ctx, &run.CompletionRequest{ID: id, Status: to})
		case to.IsTerminal():
			return f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: id, Status: to})
		default:
			return f.store.UpdateRunStatus(f.ctx, id, to, 0, 0, 0, 0)
		}
	}
	for _, from := range all {
		for _, to := range all {
			t.Run(string(from)+" to "+string(to), func(t *testing.T) {
				r := f.runIn(t, from)
				err := write(r.ID, to)
				want := from
				if run.CanTransition(from, to) {
					if err != nil {
						t.Fatalf("allowed transition: %v", err)
					}
					want = to
				} else if !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("refused transition: err = %v, want ErrConflict", err)
				}
				if got := f.runStatus(t, r.ID).Status; got != want {
					t.Errorf("status = %s, want %s", got, want)
				}
			})
		}
	}
}

func TestStore_CompleteRunRejectsActiveStatus(t *testing.T) {
	f := newStatusFixture(t)
	r := f.newRun(t, run.StatusRunning)
	for _, status := range []run.Status{run.StatusRunning, run.StatusQualityGate, run.StatusPending, ""} {
		if err := f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: r.ID, Status: status}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("CompleteRun(%q): err = %v, want ErrValidation", status, err)
		}
	}
}

func TestStore_EnterQualityGateKeepsTheOutcome(t *testing.T) {
	f := newStatusFixture(t)
	r := f.newRun(t, run.StatusRunning)
	req := &run.CompletionRequest{ID: r.ID, Status: run.StatusQualityGate, Output: "all done", Model: "model-x",
		CostUSD: 0.25, StepCount: 5, TokensIn: 100, TokensOut: 50}
	if err := f.store.EnterQualityGate(f.ctx, req); err != nil {
		t.Fatalf("EnterQualityGate: %v", err)
	}
	got := f.runStatus(t, r.ID)
	if got.Status != run.StatusQualityGate || got.Output != "all done" || got.Model != "model-x" ||
		got.CostUSD != 0.25 || got.StepCount != 5 || got.TokensIn != 100 || got.TokensOut != 50 || got.CompletedAt != nil {
		t.Errorf("run = %+v, want the worker's outcome in quality_gate, not completed", got)
	}
}

func TestStore_CountRunStep(t *testing.T) {
	f := newStatusFixture(t)

	r := f.newRun(t, run.StatusRunning)
	if _, err := f.store.AddRunUsage(f.ctx, r.ID, &run.Usage{CostUSD: 0.5, TokensIn: 10, TokensOut: 5}); err != nil {
		t.Fatalf("AddRunUsage: %v", err)
	}
	for range 2 {
		if err := f.store.CountRunStep(f.ctx, r.ID); err != nil {
			t.Fatalf("CountRunStep: %v", err)
		}
	}
	got := f.runStatus(t, r.ID)
	if got.StepCount != 2 || got.Status != run.StatusRunning || got.CostUSD != 0.5 || got.TokensIn != 10 {
		t.Errorf("run = %d steps %s cost %.2f tokens %d, want 2 steps running with the usage untouched", got.StepCount, got.Status, got.CostUSD, got.TokensIn)
	}

	for _, status := range []run.Status{run.StatusPending, run.StatusQualityGate, run.StatusCancelled, run.StatusCompleted} {
		r := f.runIn(t, status)
		if err := f.store.CountRunStep(f.ctx, r.ID); !errors.Is(err, domain.ErrConflict) {
			t.Errorf("CountRunStep on %s run: err = %v, want ErrConflict", status, err)
		}
		if got := f.runStatus(t, r.ID); got.StepCount != 0 || got.Status != status {
			t.Errorf("%s run changed: %d steps, status %s", status, got.StepCount, got.Status)
		}
	}

	otherTenant := ctxWithTenant(t, createTestTenant(t, f.store))
	for name, ctxID := range map[string]struct {
		ctx context.Context
		id  string
	}{"unknown run": {f.ctx, uuid.New().String()}, "other tenant's run": {otherTenant, r.ID}} {
		if err := f.store.CountRunStep(ctxID.ctx, ctxID.id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestStore_AddRunUsage(t *testing.T) {
	f := newStatusFixture(t)

	t.Run("running", func(t *testing.T) {
		r := f.newRun(t, run.StatusRunning)
		if _, err := f.store.AddRunUsage(f.ctx, r.ID, &run.Usage{Steps: 1, CostUSD: 0.25, TokensIn: 10, TokensOut: 4}); err != nil {
			t.Fatalf("AddRunUsage: %v", err)
		}
		got, err := f.store.AddRunUsage(f.ctx, r.ID, &run.Usage{CostUSD: 0.5, TokensIn: 5, TokensOut: 1})
		if err != nil {
			t.Fatalf("AddRunUsage: %v", err)
		}
		if got.Status != run.StatusRunning || got.StepCount != 1 || got.CostUSD != 0.75 || got.TokensIn != 15 || got.TokensOut != 5 {
			t.Errorf("returned run = %s %d steps %.2f %d/%d, want running 1 step 0.75 15/5", got.Status, got.StepCount, got.CostUSD, got.TokensIn, got.TokensOut)
		}
		if stored := f.runStatus(t, r.ID); stored.CostUSD != 0.75 || stored.Status != run.StatusRunning {
			t.Errorf("stored run = %s %.2f, want running 0.75", stored.Status, stored.CostUSD)
		}
	})

	// The worker's totals of a gated or ended run include the call already.
	for _, status := range []run.Status{run.StatusPending, run.StatusQualityGate, run.StatusCompleted, run.StatusCancelled, run.StatusTimeout} {
		t.Run(string(status)+" is refused", func(t *testing.T) {
			r := f.runIn(t, status)
			if _, err := f.store.AddRunUsage(f.ctx, r.ID, &run.Usage{Steps: 1, CostUSD: 0.25, TokensIn: 10, TokensOut: 4}); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("AddRunUsage: err = %v, want ErrConflict", err)
			}
			if got := f.runStatus(t, r.ID); got.Status != status || got.StepCount != 0 || got.CostUSD != 0 || got.TokensIn != 0 || got.TokensOut != 0 {
				t.Errorf("run = %s %d steps %.2f %d/%d, want %s without usage", got.Status, got.StepCount, got.CostUSD, got.TokensIn, got.TokensOut, status)
			}
		})
	}

	t.Run("concurrent calls all count", func(t *testing.T) {
		r := f.newRun(t, run.StatusRunning)
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := f.store.AddRunUsage(f.ctx, r.ID, &run.Usage{CostUSD: 0.125, TokensIn: 3, TokensOut: 1}); err != nil {
					t.Errorf("AddRunUsage: %v", err)
				}
			}()
		}
		wg.Wait()
		if got := f.runStatus(t, r.ID); got.CostUSD != 2.5 || got.TokensIn != 60 || got.TokensOut != 20 {
			t.Errorf("usage = %.3f %d/%d, want 2.500 60/20", got.CostUSD, got.TokensIn, got.TokensOut)
		}
	})

	t.Run("missing or foreign run", func(t *testing.T) {
		r := f.newRun(t, run.StatusRunning)
		otherTenant := ctxWithTenant(t, createTestTenant(t, f.store))
		if _, err := f.store.AddRunUsage(f.ctx, uuid.New().String(), &run.Usage{CostUSD: 1}); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown run: err = %v, want ErrNotFound", err)
		}
		if _, err := f.store.AddRunUsage(otherTenant, r.ID, &run.Usage{CostUSD: 1}); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("other tenant's run: err = %v, want ErrNotFound", err)
		}
		if got := f.runStatus(t, r.ID); got.CostUSD != 0 {
			t.Errorf("other tenant changed the usage: %.2f", got.CostUSD)
		}
	})
}

// TestStore_CompletionsNeverLowerTheUsage: the gate transition and the
// completion keep counters that are higher than the reported ones (usage a
// concurrent writer added, or worker totals recorded while the control plane
// stopped the run); higher reported totals replace them.
func TestStore_CompletionsNeverLowerTheUsage(t *testing.T) {
	f := newStatusFixture(t)
	lower := run.Usage{Steps: 1, CostUSD: 0.5, TokensIn: 10, TokensOut: 5}
	higher := run.Usage{Steps: 7, CostUSD: 2.5, TokensIn: 300, TokensOut: 90}
	stored := run.Usage{Steps: 3, CostUSD: 1.0, TokensIn: 100, TokensOut: 50}

	write := map[string]func(id string, u run.Usage) error{
		"quality gate": func(id string, u run.Usage) error {
			return f.store.EnterQualityGate(f.ctx, &run.CompletionRequest{ID: id, Status: run.StatusQualityGate,
				CostUSD: u.CostUSD, StepCount: u.Steps, TokensIn: u.TokensIn, TokensOut: u.TokensOut})
		},
		"completion": func(id string, u run.Usage) error {
			return f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: id, Status: run.StatusCancelled,
				CostUSD: u.CostUSD, StepCount: u.Steps, TokensIn: u.TokensIn, TokensOut: u.TokensOut})
		},
	}
	for name, fn := range write {
		for reported, want := range map[string][2]run.Usage{"lower": {lower, stored}, "higher": {higher, higher}} {
			t.Run(name+" with "+reported+" totals", func(t *testing.T) {
				r := f.newRun(t, run.StatusRunning)
				if _, err := f.store.AddRunUsage(f.ctx, r.ID, &stored); err != nil {
					t.Fatalf("AddRunUsage: %v", err)
				}
				if err := fn(r.ID, want[0]); err != nil {
					t.Fatalf("write: %v", err)
				}
				got := f.runStatus(t, r.ID)
				w := want[1]
				if got.StepCount != w.Steps || got.CostUSD != w.CostUSD || got.TokensIn != w.TokensIn || got.TokensOut != w.TokensOut {
					t.Errorf("usage = %d steps %.2f %d/%d, want %d steps %.2f %d/%d",
						got.StepCount, got.CostUSD, got.TokensIn, got.TokensOut, w.Steps, w.CostUSD, w.TokensIn, w.TokensOut)
				}
			})
		}
	}
}

func TestStore_RaiseRunUsage(t *testing.T) {
	f := newStatusFixture(t)
	r := f.newRun(t, run.StatusRunning)
	if err := f.store.CompleteRun(f.ctx, &run.CompletionRequest{ID: r.ID, Status: run.StatusCancelled, Error: "cancelled by user",
		CostUSD: 1.0, StepCount: 3, TokensIn: 100, TokensOut: 50}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	// The worker's totals: higher cost and input tokens, fewer steps and output tokens.
	if err := f.store.RaiseRunUsage(f.ctx, r.ID, &run.Usage{Steps: 2, CostUSD: 1.4, TokensIn: 150, TokensOut: 40}); err != nil {
		t.Fatalf("RaiseRunUsage: %v", err)
	}
	got := f.runStatus(t, r.ID)
	if got.Status != run.StatusCancelled || got.Error != "cancelled by user" ||
		got.StepCount != 3 || got.CostUSD != 1.4 || got.TokensIn != 150 || got.TokensOut != 50 {
		t.Errorf("run = %s %q %d steps %.2f %d/%d, want cancelled with 3 steps 1.40 150/50", got.Status, got.Error, got.StepCount, got.CostUSD, got.TokensIn, got.TokensOut)
	}

	otherTenant := ctxWithTenant(t, createTestTenant(t, f.store))
	if err := f.store.RaiseRunUsage(otherTenant, r.ID, &run.Usage{CostUSD: 9}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("other tenant's run: err = %v, want ErrNotFound", err)
	}
	if err := f.store.RaiseRunUsage(f.ctx, uuid.New().String(), &run.Usage{CostUSD: 9}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown run: err = %v, want ErrNotFound", err)
	}
}
