package service_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	benchTenantA = "aaaaaaaa-0000-0000-0000-00000000000a"
	benchTenantB = "bbbbbbbb-0000-0000-0000-00000000000b"
)

// tenantBenchStore scopes benchmark runs to their tenant, like the store.
type tenantBenchStore struct {
	*benchMockStore
	tenantOf  map[string]string
	updatedIn []string
}

func (s *tenantBenchStore) GetBenchmarkRun(ctx context.Context, id string) (*benchmark.Run, error) {
	if s.tenantOf[id] != tenantctx.FromContext(ctx) {
		return nil, domain.ErrNotFound
	}
	return s.benchMockStore.GetBenchmarkRun(ctx, id)
}

func (s *tenantBenchStore) UpdateBenchmarkRun(ctx context.Context, r *benchmark.Run) error {
	s.updatedIn = append(s.updatedIn, tenantctx.FromContext(ctx))
	return s.benchMockStore.UpdateBenchmarkRun(ctx, r)
}

// TestBenchmarkDeadLetteredRequest_FailsTheRun (S2-G fix, f3): a benchmark
// run request the worker dead-lettered (invalid, or never accepted) left
// its run running until the watchdog's timeout. The Go Core reads
// benchmark.run.request.dlq and fails the run in its tenant.
func TestBenchmarkDeadLetteredRequest_FailsTheRun(t *testing.T) {
	store := &tenantBenchStore{benchMockStore: newBenchMockStore(), tenantOf: map[string]string{}}
	add := func(id, tenant string, status benchmark.RunStatus) {
		store.benchRuns[id] = &benchmark.Run{ID: id, TenantID: tenant, Status: status}
		store.tenantOf[id] = tenant
	}
	add("run-lost", benchTenantA, benchmark.StatusRunning)
	add("run-done", benchTenantA, benchmark.StatusCompleted)
	add("run-b", benchTenantB, benchmark.StatusRunning)

	suiteSvc := service.NewBenchmarkSuiteService(store, "")
	svc := service.NewBenchmarkService(suiteSvc, service.NewBenchmarkRunManager(store, suiteSvc),
		service.NewBenchmarkResultAggregator(store), service.NewBenchmarkWatchdog(store))
	deliver := func(runID, tenant string) {
		t.Helper()
		data, err := json.Marshal(messagequeue.BenchmarkRunRequestPayload{RunID: runID, TenantID: tenant, DatasetPath: "/d.yaml", Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.HandleDeadLetteredRunRequest(context.Background(), messagequeue.SubjectBenchmarkRunRequest+".dlq", data); err != nil {
			t.Fatalf("HandleDeadLetteredRunRequest(%s): %v", runID, err)
		}
	}

	deliver("run-lost", benchTenantA)
	if r := store.benchRuns["run-lost"]; r.Status != benchmark.StatusFailed || !strings.Contains(r.ErrorMessage, "dead-lettered") {
		t.Fatalf("run = %s %q, want failed as dead-lettered", r.Status, r.ErrorMessage)
	}
	if !slices.Equal(store.updatedIn, []string{benchTenantA}) {
		t.Fatalf("updated in tenants %v, want tenant A", store.updatedIn)
	}

	// An ended run, another tenant's run named with the wrong tenant, an
	// unknown run and an unreadable message change nothing.
	deliver("run-done", benchTenantA)
	deliver("run-b", benchTenantA)
	deliver("run-unknown", benchTenantA)
	if err := svc.HandleDeadLetteredRunRequest(context.Background(), messagequeue.SubjectBenchmarkRunRequest+".dlq", []byte("{")); err != nil {
		t.Fatalf("HandleDeadLetteredRunRequest(unreadable) = %v, want nil", err)
	}
	if store.benchRuns["run-done"].Status != benchmark.StatusCompleted || store.benchRuns["run-b"].Status != benchmark.StatusRunning {
		t.Fatalf("runs = %s, %s; want unchanged", store.benchRuns["run-done"].Status, store.benchRuns["run-b"].Status)
	}
	if len(store.updatedIn) != 1 {
		t.Fatalf("updates = %v, want only the lost run's", store.updatedIn)
	}
}

func TestBenchmarkService_SubscribesToDeadLetteredRequests(t *testing.T) {
	queue := &subscribingQueue{}
	store := newBenchMockStore()
	svc := newTestBenchmarkService(store)
	svc.SetQueue(queue)
	cancel, err := svc.StartResultSubscriber(context.Background())
	if err != nil {
		t.Fatalf("StartResultSubscriber: %v", err)
	}
	defer cancel()
	if !slices.Contains(queue.subjects, messagequeue.SubjectBenchmarkRunRequest+".dlq") {
		t.Fatalf("subscriptions = %v, want benchmark.run.request.dlq", queue.subjects)
	}
}
