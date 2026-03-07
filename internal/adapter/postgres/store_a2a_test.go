package postgres_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// --------------------------------------------------------------------------
// TestStore_A2ATask_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_A2ATask_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	// Create a task under tenant A.
	task := a2adomain.NewA2ATask(uuid.New().String())
	if err := store.CreateA2ATask(ctxA, task); err != nil {
		t.Fatalf("CreateA2ATask: %v", err)
	}

	t.Run("Get_SameTenant", func(t *testing.T) {
		got, err := store.GetA2ATask(ctxA, task.ID)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if got.ID != task.ID {
			t.Fatalf("expected task %s, got %s", task.ID, got.ID)
		}
	})

	t.Run("Get_WrongTenant", func(t *testing.T) {
		_, err := store.GetA2ATask(ctxB, task.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update_WrongTenant", func(t *testing.T) {
		// Get the task from the correct tenant first.
		got, _ := store.GetA2ATask(ctxA, task.ID)
		got.State = a2adomain.TaskStateWorking
		err := store.UpdateA2ATask(ctxB, got)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
	})

	t.Run("Delete_WrongTenant", func(t *testing.T) {
		err := store.DeleteA2ATask(ctxB, task.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete_SameTenant", func(t *testing.T) {
		if err := store.DeleteA2ATask(ctxA, task.ID); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_ListA2ATasks_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_ListA2ATasks_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	// Create a task in each tenant.
	taskA := a2adomain.NewA2ATask(uuid.New().String())
	if err := store.CreateA2ATask(ctxA, taskA); err != nil {
		t.Fatalf("CreateA2ATask A: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteA2ATask(ctxA, taskA.ID) })

	taskB := a2adomain.NewA2ATask(uuid.New().String())
	if err := store.CreateA2ATask(ctxB, taskB); err != nil {
		t.Fatalf("CreateA2ATask B: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteA2ATask(ctxB, taskB.ID) })

	t.Run("TenantA_SeesOnlyOwn", func(t *testing.T) {
		tasks, _, err := store.ListA2ATasks(ctxA, &database.A2ATaskFilter{Limit: 100})
		if err != nil {
			t.Fatalf("ListA2ATasks: %v", err)
		}
		for _, tk := range tasks {
			if tk.TenantID != tenantA {
				t.Fatalf("tenant A saw task from tenant %s", tk.TenantID)
			}
		}
	})

	t.Run("TenantB_SeesOnlyOwn", func(t *testing.T) {
		tasks, _, err := store.ListA2ATasks(ctxB, &database.A2ATaskFilter{Limit: 100})
		if err != nil {
			t.Fatalf("ListA2ATasks: %v", err)
		}
		for _, tk := range tasks {
			if tk.TenantID != tenantB {
				t.Fatalf("tenant B saw task from tenant %s", tk.TenantID)
			}
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_ListA2ATasks_LimitParameterized
// --------------------------------------------------------------------------

func TestStore_ListA2ATasks_LimitParameterized(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	// Create 3 tasks.
	for i := 0; i < 3; i++ {
		task := a2adomain.NewA2ATask(uuid.New().String())
		if err := store.CreateA2ATask(ctx, task); err != nil {
			t.Fatalf("CreateA2ATask %d: %v", i, err)
		}
		t.Cleanup(func() { _ = store.DeleteA2ATask(ctx, task.ID) })
	}

	// List with limit 2.
	tasks, _, err := store.ListA2ATasks(ctx, &database.A2ATaskFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListA2ATasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

// --------------------------------------------------------------------------
// TestStore_RemoteAgent_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_RemoteAgent_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	// Create agent under tenant A.
	agent := a2adomain.NewRemoteAgent("test-agent", "https://agent.example.com")
	if err := store.CreateRemoteAgent(ctxA, agent); err != nil {
		t.Fatalf("CreateRemoteAgent: %v", err)
	}

	t.Run("Get_SameTenant", func(t *testing.T) {
		got, err := store.GetRemoteAgent(ctxA, agent.ID)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if got.ID != agent.ID {
			t.Fatalf("expected agent %s, got %s", agent.ID, got.ID)
		}
	})

	t.Run("Get_WrongTenant", func(t *testing.T) {
		_, err := store.GetRemoteAgent(ctxB, agent.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update_WrongTenant", func(t *testing.T) {
		got, _ := store.GetRemoteAgent(ctxA, agent.ID)
		got.Description = "hacked"
		err := store.UpdateRemoteAgent(ctxB, got)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete_WrongTenant", func(t *testing.T) {
		err := store.DeleteRemoteAgent(ctxB, agent.ID)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete_SameTenant", func(t *testing.T) {
		if err := store.DeleteRemoteAgent(ctxA, agent.ID); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// TestStore_ListRemoteAgents_TenantIsolation
// --------------------------------------------------------------------------

func TestStore_ListRemoteAgents_TenantIsolation(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	agentA := a2adomain.NewRemoteAgent("agent-a", "https://a.example.com")
	if err := store.CreateRemoteAgent(ctxA, agentA); err != nil {
		t.Fatalf("CreateRemoteAgent A: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRemoteAgent(ctxA, agentA.ID) })

	agentB := a2adomain.NewRemoteAgent("agent-b", "https://b.example.com")
	if err := store.CreateRemoteAgent(ctxB, agentB); err != nil {
		t.Fatalf("CreateRemoteAgent B: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteRemoteAgent(ctxB, agentB.ID) })

	t.Run("TenantA_SeesOnlyOwn", func(t *testing.T) {
		agents, err := store.ListRemoteAgents(ctxA, "", false)
		if err != nil {
			t.Fatalf("ListRemoteAgents: %v", err)
		}
		for _, a := range agents {
			if a.TenantID != tenantA {
				t.Fatalf("tenant A saw agent from tenant %s", a.TenantID)
			}
		}
	})

	t.Run("TenantB_SeesOnlyOwn", func(t *testing.T) {
		agents, err := store.ListRemoteAgents(ctxB, "", false)
		if err != nil {
			t.Fatalf("ListRemoteAgents: %v", err)
		}
		for _, a := range agents {
			if a.TenantID != tenantB {
				t.Fatalf("tenant B saw agent from tenant %s", a.TenantID)
			}
		}
	})
}
