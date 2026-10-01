package service_test

import (
	"context"
	"slices"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Worker heartbeats of runtimeMockStore (KI-65), mirroring the Postgres store:
// a heartbeat counts only for work that still waits for its worker, and the
// lists hold only work that had a heartbeat.

type runBeat struct {
	at     time.Time
	tenant string
}

type turnBeat struct {
	conversation, turn, tenant string
}

type activeTurn struct {
	turn   string
	tenant string
	beat   time.Time // zero until the turn's first heartbeat
}

func (m *runtimeMockStore) TouchRunHeartbeat(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID == id && m.runs[i].Status == run.StatusRunning {
			if m.runBeats == nil {
				m.runBeats = map[string]runBeat{}
			}
			m.runBeats[id] = runBeat{at: time.Now(), tenant: tenantctx.FromContext(ctx)}
		}
	}
	return nil
}

func (m *runtimeMockStore) ListRunsWithStaleHeartbeat(_ context.Context, idleFor time.Duration, limit int) ([]run.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-idleFor)
	var stale []run.Run
	for i := range m.runs {
		beat, ok := m.runBeats[m.runs[i].ID]
		if ok && m.runs[i].Status == run.StatusRunning && beat.at.Before(cutoff) {
			stale = append(stale, m.runs[i])
		}
	}
	slices.SortFunc(stale, func(a, b run.Run) int { return m.runBeats[a.ID].at.Compare(m.runBeats[b.ID].at) })
	if len(stale) > limit {
		stale = stale[:limit]
	}
	return stale, nil
}

func (m *runtimeMockStore) BeginConversationTurn(ctx context.Context, conversationID, turnID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.turns == nil {
		m.turns = map[string]activeTurn{}
	}
	m.turns[conversationID] = activeTurn{turn: turnID, tenant: tenantctx.FromContext(ctx)}
	return nil
}

func (m *runtimeMockStore) EndConversationTurn(_ context.Context, conversationID, turnID string) (bool, error) {
	if hook := m.endTurnHook; hook != nil {
		hook(conversationID, turnID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if at, ok := m.turns[conversationID]; ok && (turnID == "" || at.turn == turnID) {
		delete(m.turns, conversationID)
		return true, nil
	}
	return false, nil
}

func (m *runtimeMockStore) TouchConversationTurnHeartbeat(ctx context.Context, conversationID, turnID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnBeats = append(m.turnBeats, turnBeat{conversation: conversationID, turn: turnID, tenant: tenantctx.FromContext(ctx)})
	if at, ok := m.turns[conversationID]; ok && at.turn == turnID {
		at.beat = time.Now()
		m.turns[conversationID] = at
	}
	return nil
}

func (m *runtimeMockStore) ListConversationTurnsWithStaleHeartbeat(_ context.Context, idleFor time.Duration, limit int) ([]conversation.ActiveTurn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-idleFor)
	var stale []conversation.ActiveTurn
	for id, at := range m.turns {
		if !at.beat.IsZero() && at.beat.Before(cutoff) {
			stale = append(stale, conversation.ActiveTurn{ConversationID: id, TenantID: at.tenant, TurnID: at.turn})
		}
	}
	slices.SortFunc(stale, func(a, b conversation.ActiveTurn) int {
		return m.turns[a.ConversationID].beat.Compare(m.turns[b.ConversationID].beat)
	})
	if len(stale) > limit {
		stale = stale[:limit]
	}
	return stale, nil
}

// activeTurnOf returns the conversation's active turn ("" when none).
func (m *runtimeMockStore) activeTurnOf(conversationID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.turns[conversationID].turn
}

// setTurnHeartbeat stores the time of the last heartbeat of the conversation's active turn.
func (m *runtimeMockStore) setTurnHeartbeat(conversationID string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.turns[conversationID]; ok {
		t.beat = at
		m.turns[conversationID] = t
	}
}

func (m *runtimeMockStore) TouchTaskHeartbeat(ctx context.Context, id, dispatchID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tasks {
		active := m.tasks[i].Status == task.StatusQueued || m.tasks[i].Status == task.StatusRunning
		if m.tasks[i].ID == id && active && m.tasks[i].DispatchID == dispatchID {
			if m.taskBeats == nil {
				m.taskBeats = map[string]runBeat{}
			}
			m.taskBeats[id] = runBeat{at: time.Now(), tenant: tenantctx.FromContext(ctx)}
		}
	}
	return nil
}

func (m *runtimeMockStore) ListTasksWithStaleHeartbeat(_ context.Context, idleFor time.Duration, limit int) ([]task.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-idleFor)
	var stale []task.Task
	for i := range m.tasks {
		beat, ok := m.taskBeats[m.tasks[i].ID]
		active := m.tasks[i].Status == task.StatusQueued || m.tasks[i].Status == task.StatusRunning
		if ok && active && beat.at.Before(cutoff) {
			stale = append(stale, m.tasks[i])
		}
	}
	if len(stale) > limit {
		stale = stale[:limit]
	}
	return stale, nil
}

func (m *runtimeMockStore) QueueTask(_ context.Context, id, agentID, dispatchID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tasks {
		if m.tasks[i].ID != id {
			continue
		}
		if m.tasks[i].Status == task.StatusQueued || m.tasks[i].Status == task.StatusRunning {
			return errMockRunTransition
		}
		m.tasks[i].Status = task.StatusQueued
		m.tasks[i].AgentID = agentID
		m.tasks[i].DispatchID = dispatchID
		delete(m.taskBeats, id) // a heartbeat names its dispatch
		return nil
	}
	return errMockNotFound
}

// EndTaskDispatch applies the store's predicate: the task's current dispatch,
// still queued or running.
func (m *runtimeMockStore) EndTaskDispatch(_ context.Context, id, dispatchID string, status task.Status, result task.Result) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tasks {
		if m.tasks[i].ID != id {
			continue
		}
		active := m.tasks[i].Status == task.StatusQueued || m.tasks[i].Status == task.StatusRunning
		if !active || m.tasks[i].DispatchID != dispatchID {
			return errMockRunTransition
		}
		m.tasks[i].Status = status
		r := result
		m.tasks[i].Result = &r
		return nil
	}
	return errMockNotFound
}

// ListTasksNeverAccepted lists the queued tasks whose dispatch has no heartbeat.
func (m *runtimeMockStore) ListTasksNeverAccepted(_ context.Context, _ time.Duration, limit int) ([]task.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var waiting []task.Task
	for i := range m.tasks {
		if _, beat := m.taskBeats[m.tasks[i].ID]; !beat && m.tasks[i].Status == task.StatusQueued && m.tasks[i].DispatchID != "" {
			waiting = append(waiting, m.tasks[i])
		}
	}
	if len(waiting) > limit {
		waiting = waiting[:limit]
	}
	return waiting, nil
}
