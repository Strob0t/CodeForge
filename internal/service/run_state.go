package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Strob0t/CodeForge/internal/domain/event"
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
	pendingRequests  sync.Map // map["runID:callID"]pendingApprovalRequest
	toolResults      sync.Map // map["runID:callID"]bool: results of running runs already handled
	bypassedConvs    sync.Map // map[conversationID]bool
	runSpans         sync.Map // map[runID]trace.Span

	stopsMu      sync.Mutex
	stops        map[string]int               // runID -> control-plane stops of the run under way
	stopDeferred map[string][]deferredMessage // runID -> worker messages received during its stops, in arrival order

	convMu   sync.Mutex
	convRuns map[string]*convRunState // conversationID -> run state (see Conversation Runs)
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

// pendingApprovalRequest is what a pending approval asks, and for which
// tenant (the approval page shows it).
type pendingApprovalRequest struct {
	tenantID string
	req      event.AGUIPermissionRequestEvent
}

// SetPendingApprovalRequest records what the pending approval of key asks.
func (m *RunStateManager) SetPendingApprovalRequest(key, tenantID string, req *event.AGUIPermissionRequestEvent) {
	m.pendingRequests.Store(key, pendingApprovalRequest{tenantID: tenantID, req: *req})
}

// PendingApprovalRequest returns what the pending approval of key asks.
func (m *RunStateManager) PendingApprovalRequest(key string) (tenantID string, req event.AGUIPermissionRequestEvent, ok bool) {
	v, found := m.pendingRequests.Load(key)
	if !found {
		return "", event.AGUIPermissionRequestEvent{}, false
	}
	p, _ := v.(pendingApprovalRequest)
	return p.tenantID, p.req, true
}

func (m *RunStateManager) DeletePendingApproval(key string) {
	m.pendingApprovals.Delete(key)
	m.pendingRequests.Delete(key)
}

func (m *RunStateManager) LoadAndDeletePendingApproval(key string) (chan string, bool) {
	m.pendingRequests.Delete(key)
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

// --- Tool Results ---

// FirstToolResult records that the result of a call of a running run is being
// handled and reports whether it is the first delivery: runs.toolcall.result
// is delivered at least once, and a redelivered result must not count its
// usage twice. Entries live until the run's state is cleaned up.
func (m *RunStateManager) FirstToolResult(runID, callID string) bool {
	_, seen := m.toolResults.LoadOrStore(runID+":"+callID, true)
	return !seen
}

// ForgetToolResult drops the record of a call's result: its run was no
// longer running, so the result was not counted.
func (m *RunStateManager) ForgetToolResult(runID, callID string) {
	m.toolResults.Delete(runID + ":" + callID)
}

// --- Stops ---

// BeginStop records that the control plane is stopping the run (cancel,
// timeout, limits): the stop records the run's end, and the worker's own
// completion, which the stop triggers, must not end the run first.
func (m *RunStateManager) BeginStop(runID string) {
	m.stopsMu.Lock()
	defer m.stopsMu.Unlock()
	if m.stops == nil {
		m.stops = make(map[string]int)
	}
	m.stops[runID]++
}

// Kinds of worker messages a stop defers.
const (
	deferredCompletion = "completion"
	deferredGateResult = "gate_result"
)

// deferredMessage is the handling of a worker message kept during a stop.
type deferredMessage struct {
	kind   string
	handle func(ctx context.Context) error
}

// DeferIfStopping keeps handle, the handling of a worker message of kind
// (its completion, its quality gate result) that ends the run, while the
// run is being stopped, and reports whether it did; the last EndStop of the
// run returns the kept messages. Seeing the stop and keeping the message is
// one step: a stop that ends in between would otherwise never see the
// message, and a run whose stop failed would stay running (or waiting for
// its gate). Each kind is kept (S2-G fix 2, 5): a message of another kind
// does not overwrite it, and a repeated message of a kind (a redelivery)
// replaces its earlier copy in place.
func (m *RunStateManager) DeferIfStopping(runID, kind string, handle func(ctx context.Context) error) bool {
	m.stopsMu.Lock()
	defer m.stopsMu.Unlock()
	if m.stops[runID] == 0 {
		return false
	}
	if m.stopDeferred == nil {
		m.stopDeferred = make(map[string][]deferredMessage)
	}
	kept := m.stopDeferred[runID]
	for i := range kept {
		if kept[i].kind == kind {
			kept[i].handle = handle
			return true
		}
	}
	m.stopDeferred[runID] = append(kept, deferredMessage{kind: kind, handle: handle})
	return true
}

// EndStop records that a stop of the run is over. The last one returns the
// handling of the worker messages that arrived during the stops, which
// handles them in arrival order and joins their errors; nil if none.
func (m *RunStateManager) EndStop(runID string) func(ctx context.Context) error {
	m.stopsMu.Lock()
	defer m.stopsMu.Unlock()
	if m.stops[runID] > 1 {
		m.stops[runID]--
		return nil
	}
	delete(m.stops, runID)
	kept := m.stopDeferred[runID]
	delete(m.stopDeferred, runID)
	if len(kept) == 0 {
		return nil
	}
	return func(ctx context.Context) error {
		var errs []error
		for _, msg := range kept {
			if err := msg.handle(ctx); err != nil {
				errs = append(errs, fmt.Errorf("deferred %s: %w", msg.kind, err))
			}
		}
		return errors.Join(errs...)
	}
}

// --- Conversation Runs ---
//
// Conversation runs reuse the conversation ID as run ID; the turn of each run
// start tells them apart. A conversation has at most one active run: the run
// whose start is being dispatched or was published and has not ended or been
// stopped. A stop cancels the conversation: tool calls that are not calls of
// the active run are rejected until the next run's start is published or the
// stopped run reports its end (KI-24). An entry exists only while a run is
// active or a stop's mark holds, so the state does not grow with the number
// of conversations.

// convRunState is the run state of one conversation; convMu guards it.
type convRunState struct {
	active    string // turn of the active run, "" when none
	stopped   string // turn of the run a stop ended, until it reports its end
	cancelled bool   // calls that are not of the active run are rejected
}

// convRun returns the conversation's state, creating it; the caller holds convMu.
func (m *RunStateManager) convRun(convID string) *convRunState {
	if m.convRuns == nil {
		m.convRuns = make(map[string]*convRunState)
	}
	st, ok := m.convRuns[convID]
	if !ok {
		st = &convRunState{}
		m.convRuns[convID] = st
	}
	return st
}

// dropIdleConvRun removes a conversation's state that holds nothing; the
// caller holds convMu.
func (m *RunStateManager) dropIdleConvRun(convID string) {
	if st, ok := m.convRuns[convID]; ok && st.active == "" && st.stopped == "" && !st.cancelled {
		delete(m.convRuns, convID)
	}
}

// BeginConversationRun makes turnID the conversation's active run before its
// start is dispatched, so the run's first tool calls are recognized however
// fast they come. It reports false, and changes nothing, while another run of
// the conversation is active.
func (m *RunStateManager) BeginConversationRun(convID, turnID string) bool {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	st := m.convRun(convID)
	if st.active != "" {
		return false
	}
	st.active = turnID
	return true
}

// ConversationRunDispatched records that the start of run turnID was
// published: a stop's mark no longer holds. A stop that ended the run while
// its start was dispatched keeps its mark.
func (m *RunStateManager) ConversationRunDispatched(convID, turnID string) {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	st, ok := m.convRuns[convID]
	if !ok || st.active != turnID {
		return
	}
	st.cancelled = false
	st.stopped = ""
}

// AbortConversationRun releases run turnID, whose start was not published:
// no run of the conversation is active, and a stop's mark stays.
func (m *RunStateManager) AbortConversationRun(convID, turnID string) {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	if st, ok := m.convRuns[convID]; ok && st.active == turnID {
		st.active = ""
		m.dropIdleConvRun(convID)
	}
}

// EndConversationRun records the reported end of run turnID: the active run
// ends, or the mark of the stop that ended it no longer holds (its calls are
// over). A report without turn (a worker that does not send turns) ends the
// active run.
func (m *RunStateManager) EndConversationRun(convID, turnID string) {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	st, ok := m.convRuns[convID]
	if !ok {
		return
	}
	switch turnID {
	case "", st.active:
		st.active = ""
	case st.stopped:
		st.stopped = ""
		st.cancelled = false
	}
	m.dropIdleConvRun(convID)
}

// ActiveConversationRun returns the turn of the conversation's active run
// ("" when none).
func (m *RunStateManager) ActiveConversationRun(convID string) string {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	if st, ok := m.convRuns[convID]; ok {
		return st.active
	}
	return ""
}

// IsActiveConversationRun reports whether turnID is the conversation's active run.
func (m *RunStateManager) IsActiveConversationRun(convID, turnID string) bool {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	st, ok := m.convRuns[convID]
	return ok && turnID != "" && st.active == turnID
}

// SetCancelledConversation records a stop of the conversation's run: the
// active run ends and tool calls of the conversation are rejected until the
// next run's start is published or the stopped run reports its end. It
// returns the turn of the run it stopped ("" when none was active).
func (m *RunStateManager) SetCancelledConversation(convID string) string {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	st := m.convRun(convID)
	st.cancelled = true
	stopped := st.active
	if stopped != "" {
		st.stopped = stopped
		st.active = ""
	}
	return stopped
}

// ClearCancelledConversation forgets the cancel mark of a conversation.
func (m *RunStateManager) ClearCancelledConversation(convID string) {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	if st, ok := m.convRuns[convID]; ok {
		st.cancelled = false
		st.stopped = ""
		m.dropIdleConvRun(convID)
	}
}

// ActiveConversationTurn returns the turn of the conversation's active run,
// if this process dispatched one.
func (m *RunStateManager) ActiveConversationTurn(convID string) (string, bool) {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	if st, ok := m.convRuns[convID]; ok && st.active != "" {
		return st.active, true
	}
	return "", false
}

func (m *RunStateManager) IsConversationCancelled(convID string) bool {
	m.convMu.Lock()
	defer m.convMu.Unlock()
	st, ok := m.convRuns[convID]
	return ok && st.cancelled
}

// ForgetConversation drops all state of a deleted conversation.
func (m *RunStateManager) ForgetConversation(convID string) {
	m.convMu.Lock()
	delete(m.convRuns, convID)
	m.convMu.Unlock()
	m.bypassedConvs.Delete(convID)
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

// isRunKey reports whether key is a "runID:..." key of runID.
func isRunKey(key, runID string) bool {
	return len(key) > len(runID) && key[:len(runID)] == runID && key[len(runID)] == ':'
}

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
	m.toolResults.Range(func(k, _ any) bool {
		if key, _ := k.(string); isRunKey(key, runID) {
			m.toolResults.Delete(key)
		}
		return true
	})
	// Drain and close pending approval channels for this run.
	m.RangePendingApprovals(func(key string, ch chan string) bool {
		if isRunKey(key, runID) {
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
