package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// RunStateManager encapsulates the sync.Map fields that track ephemeral
// per-run state inside RuntimeService. Typed accessors replace raw
// Load/Store calls, improving readability and eliminating type assertions
// at every call site.
type RunStateManager struct {
	stallTrackers    sync.Map // map[runID]*run.StallTracker
	heartbeats       sync.Map // map[runID]time.Time
	runTimeouts      sync.Map // map[runID]context.CancelFunc
	budgetAlerts     sync.Map // map["runID:threshold"]bool
	pendingApprovals sync.Map // map["runID:callID"]chan string
	cancelledConvs   sync.Map // map[conversationID]bool
	bypassedConvs    sync.Map // map[conversationID]bool
	runSpans         sync.Map // map[runID]trace.Span
}

// NewRunStateManager creates a zero-value RunStateManager ready for use.
func NewRunStateManager() *RunStateManager {
	return &RunStateManager{}
}

// --- StallTracker ---

func (m *RunStateManager) SetStallTracker(runID string, t *run.StallTracker) {
	m.stallTrackers.Store(runID, t)
}

func (m *RunStateManager) GetStallTracker(runID string) (*run.StallTracker, bool) {
	v, ok := m.stallTrackers.Load(runID)
	if !ok {
		return nil, false
	}
	return v.(*run.StallTracker), true
}

func (m *RunStateManager) DeleteStallTracker(runID string) {
	m.stallTrackers.Delete(runID)
}

// --- Heartbeats ---

func (m *RunStateManager) SetHeartbeat(runID string, t time.Time) {
	m.heartbeats.Store(runID, t)
}

func (m *RunStateManager) GetHeartbeat(runID string) (time.Time, bool) {
	v, ok := m.heartbeats.Load(runID)
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}

func (m *RunStateManager) DeleteHeartbeat(runID string) {
	m.heartbeats.Delete(runID)
}

// --- Run Timeouts ---

func (m *RunStateManager) SetRunTimeout(runID string, cancel context.CancelFunc) {
	m.runTimeouts.Store(runID, cancel)
}

func (m *RunStateManager) LoadAndDeleteRunTimeout(runID string) (context.CancelFunc, bool) {
	v, ok := m.runTimeouts.LoadAndDelete(runID)
	if !ok {
		return nil, false
	}
	return v.(context.CancelFunc), true
}

// --- Budget Alerts ---

func (m *RunStateManager) StoreBudgetAlert(key string) (alreadySent bool) {
	_, alreadySent = m.budgetAlerts.LoadOrStore(key, true)
	return alreadySent
}

func (m *RunStateManager) DeleteBudgetAlert(key string) {
	m.budgetAlerts.Delete(key)
}

// --- Pending Approvals ---

func (m *RunStateManager) SetPendingApproval(key string, ch chan string) {
	m.pendingApprovals.Store(key, ch)
}

func (m *RunStateManager) DeletePendingApproval(key string) {
	m.pendingApprovals.Delete(key)
}

func (m *RunStateManager) LoadAndDeletePendingApproval(key string) (chan string, bool) {
	v, ok := m.pendingApprovals.LoadAndDelete(key)
	if !ok {
		return nil, false
	}
	ch, _ := v.(chan string)
	return ch, ch != nil
}

// RangePendingApprovals iterates over all pending approvals.
func (m *RunStateManager) RangePendingApprovals(fn func(key string, ch chan string) bool) {
	m.pendingApprovals.Range(func(k, v any) bool {
		key, _ := k.(string)
		ch, _ := v.(chan string)
		return fn(key, ch)
	})
}

// --- Cancelled Conversations ---

func (m *RunStateManager) SetCancelledConversation(convID string) {
	m.cancelledConvs.Store(convID, true)
}

func (m *RunStateManager) IsConversationCancelled(convID string) bool {
	_, ok := m.cancelledConvs.Load(convID)
	return ok
}

// --- Bypassed Conversations ---

func (m *RunStateManager) SetBypassedConversation(convID string) {
	m.bypassedConvs.Store(convID, true)
}

func (m *RunStateManager) IsConversationBypassed(convID string) bool {
	_, ok := m.bypassedConvs.Load(convID)
	return ok
}

// --- Run Spans ---

func (m *RunStateManager) SetRunSpan(runID string, span trace.Span) {
	m.runSpans.Store(runID, span)
}

func (m *RunStateManager) GetRunSpan(runID string) (trace.Span, bool) {
	v, ok := m.runSpans.Load(runID)
	if !ok {
		return nil, false
	}
	return v.(trace.Span), true
}

func (m *RunStateManager) LoadAndDeleteRunSpan(runID string) (trace.Span, bool) {
	v, ok := m.runSpans.LoadAndDelete(runID)
	if !ok {
		return nil, false
	}
	return v.(trace.Span), true
}

// --- Composite Operations ---

// CleanupRun removes all ephemeral state for a run. Pending approval channels
// receive a "deny" message to unblock waiting goroutines.
func (m *RunStateManager) CleanupRun(runID string) {
	m.DeleteHeartbeat(runID)
	m.DeleteStallTracker(runID)
	if cancel, ok := m.LoadAndDeleteRunTimeout(runID); ok {
		cancel()
	}
	if span, ok := m.LoadAndDeleteRunSpan(runID); ok {
		span.End()
	}
	m.DeleteBudgetAlert(fmt.Sprintf("%s:80", runID))
	m.DeleteBudgetAlert(fmt.Sprintf("%s:90", runID))
	// Drain and close pending approval channels for this run.
	m.RangePendingApprovals(func(key string, ch chan string) bool {
		if len(key) > len(runID) && key[:len(runID)] == runID && key[len(runID)] == ':' {
			if ch != nil {
				select {
				case ch <- "deny":
				default:
				}
			}
			m.DeletePendingApproval(key)
		}
		return true
	})
}
