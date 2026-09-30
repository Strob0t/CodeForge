package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// Plan events have no agent or task, task results arrive without an agent:
// the event store keeps them with NULL agent_id/task_id instead of rejecting
// them (KI-32).

// setupEventStore returns the store, an event store and a pool on the
// migrated test database.
func setupEventStore(t *testing.T) (*postgres.Store, *postgres.EventStore, *pgxpool.Pool) {
	t.Helper()
	store := setupStore(t) // skips without DATABASE_URL and runs the migrations
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return store, postgres.NewEventStore(pool), pool
}

func TestEventStore_AppendWithoutAgentOrTask(t *testing.T) {
	store, events, pool := setupEventStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)
	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "optional-ids", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	tests := []struct {
		name          string
		agentID       string
		taskID        string
		runID         string
		wantNullAgent bool
		wantNullTask  bool
	}{
		{name: "plan event: no agent, no task, no run", wantNullAgent: true, wantNullTask: true},
		{name: "task result: task without agent", taskID: uuid.New().String(), wantNullAgent: true},
		{name: "run event without agent and task", runID: uuid.New().String(), wantNullAgent: true, wantNullTask: true},
		{name: "all ids", agentID: uuid.New().String(), taskID: uuid.New().String(), runID: uuid.New().String()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := &event.AgentEvent{
				AgentID: tc.agentID, TaskID: tc.taskID, RunID: tc.runID, ProjectID: proj.ID,
				Type: event.TypePlanStarted, Payload: []byte(`{"case":"` + tc.name + `"}`), Version: 1,
			}
			if err := events.Append(ctx, ev); err != nil {
				t.Fatalf("Append: %v", err)
			}

			var nullAgent, nullTask bool
			err := pool.QueryRow(context.Background(),
				`SELECT agent_id IS NULL, task_id IS NULL FROM agent_events WHERE sequence_number = $1 AND tenant_id = $2`,
				ev.SequenceNumber, tenantID).Scan(&nullAgent, &nullTask)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if nullAgent != tc.wantNullAgent || nullTask != tc.wantNullTask {
				t.Errorf("agent_id NULL = %v, task_id NULL = %v, want %v, %v", nullAgent, nullTask, tc.wantNullAgent, tc.wantNullTask)
			}
		})
	}

	t.Run("loads events with NULL ids back as empty strings", func(t *testing.T) {
		taskID, runID := uuid.New().String(), uuid.New().String()
		for _, ev := range []*event.AgentEvent{
			{TaskID: taskID, ProjectID: proj.ID, Type: event.TypeAgentFinished, Payload: []byte(`{}`), Version: 1},
			{RunID: runID, ProjectID: proj.ID, Type: event.TypeRunCompleted, Payload: []byte(`{}`), Version: 1},
		} {
			if err := events.Append(ctx, ev); err != nil {
				t.Fatalf("Append: %v", err)
			}
		}

		byTask, err := events.LoadByTask(ctx, taskID)
		if err != nil {
			t.Fatalf("LoadByTask: %v", err)
		}
		if len(byTask) != 1 || byTask[0].AgentID != "" || byTask[0].TaskID != taskID {
			t.Errorf("LoadByTask = %+v, want one event of the task without agent", byTask)
		}
		byRun, err := events.LoadByRun(ctx, runID)
		if err != nil {
			t.Fatalf("LoadByRun: %v", err)
		}
		if len(byRun) != 1 || byRun[0].AgentID != "" || byRun[0].TaskID != "" {
			t.Errorf("LoadByRun = %+v, want one event without agent and task", byRun)
		}
		page, err := events.LoadTrajectory(ctx, runID, eventstore.TrajectoryFilter{}, "", 10)
		if err != nil {
			t.Fatalf("LoadTrajectory: %v", err)
		}
		if len(page.Events) != 1 {
			t.Errorf("LoadTrajectory returned %d events, want 1", len(page.Events))
		}

		otherTenant := ctxWithTenant(t, createTestTenant(t, store))
		if other, err := events.LoadByTask(otherTenant, taskID); err != nil || len(other) != 0 {
			t.Errorf("other tenant LoadByTask = %d events (err %v), want none", len(other), err)
		}
	})

	t.Run("an event without project is still rejected", func(t *testing.T) {
		ev := &event.AgentEvent{Type: event.TypePlanStarted, Payload: []byte(`{}`), Version: 1}
		if err := events.Append(ctx, ev); err == nil {
			t.Error("Append without project: want an error")
		}
	})
}
