package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// MaxPromptLength is the maximum allowed prompt length for inbound A2A tasks (100KB).
const MaxPromptLength = 100_000

// cancelResult is the typed payload published to NATS when canceling a task.
type cancelResult struct {
	TaskID string `json:"task_id"`
}

// Screener checks an inbound message before it is published (the
// quarantine service): ScreenMessage returns the verdict and the stored
// message's ID; Withdraw takes a held message back when its task's caller
// cancels the task.
type Screener interface {
	ScreenMessage(ctx context.Context, ann *trust.Annotation, subject string, payload []byte, projectID string) (quarantine.Verdict, string, error)
	Withdraw(ctx context.Context, id, reason string) error
}

// Executor implements a2asrv.AgentExecutor for inbound A2A tasks.
type Executor struct {
	store    database.Store
	queue    messagequeue.Queue
	hub      broadcast.Broadcaster
	modes    []string
	screener Screener
}

// NewExecutor creates an Executor.
func NewExecutor(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, modes []string) *Executor {
	return &Executor{store: store, queue: queue, hub: hub, modes: modes}
}

// SetScreener makes every inbound prompt pass the screener (quarantine)
// before it is published (KI-15).
func (e *Executor) SetScreener(s Screener) { e.screener = s }

// Execute handles an inbound A2A task (implements a2asrv.AgentExecutor).
func (e *Executor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, eq eventqueue.Queue) error {
	// Extract text from first message part.
	prompt := ""
	if reqCtx.Message != nil {
		for _, p := range reqCtx.Message.Parts {
			if tp, ok := p.(sdka2a.TextPart); ok {
				prompt = tp.Text
				break
			}
		}
	}

	if len(prompt) > MaxPromptLength {
		return fmt.Errorf("prompt exceeds maximum length (%d > %d)", len(prompt), MaxPromptLength)
	}

	// The caller's trust comes from its authentication (A2AAuth): partial
	// for an A2A API key, untrusted otherwise.
	taskID := fmt.Sprintf("a2a-%s", reqCtx.TaskID)
	ann := &trust.Annotation{
		Origin:     "a2a",
		TrustLevel: trust.Level(middleware.A2ATrustFromContext(ctx)),
		SourceID:   "a2a:" + string(reqCtx.TaskID),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	payload, marshalErr := json.Marshal(messagequeue.A2ATaskCreatedPayload{
		TaskID:   taskID,
		TenantID: tenantctx.FromContext(ctx),
		SkillID:  "",
		Prompt:   prompt,
	})
	if marshalErr != nil {
		slog.Error("a2a: failed to marshal task created payload", "task_id", taskID, "error", marshalErr)
		return fmt.Errorf("marshal a2a task payload: %w", marshalErr)
	}

	// The task exists before its prompt is screened (S2-G fix, 9): a held
	// prompt's quarantine message always has its task, which records the
	// message so that the caller's cancel can withdraw it.
	dt := a2adomain.NewA2ATask(taskID)
	dt.State = a2adomain.TaskStateSubmitted
	dt.Direction = a2adomain.DirectionInbound
	dt.TrustOrigin = ann.Origin
	dt.TrustLevel = string(ann.TrustLevel)
	dt.CallerKeyID = middleware.A2ACallerFromContext(ctx)
	if err := e.store.CreateA2ATask(ctx, dt); err != nil {
		return fmt.Errorf("create a2a task: %w", err)
	}

	// The prompt is screened before any worker sees it (KI-15). A held
	// prompt waits for an admin's review, which publishes it when approved.
	verdict, heldID := e.screen(ctx, ann, taskID, payload)
	state, sdkState, note := a2adomain.TaskStateWorking, sdka2a.TaskStateWorking, ""
	switch verdict {
	case quarantine.VerdictHeld:
		state, sdkState, note = a2adomain.TaskStateSubmitted, sdka2a.TaskStateSubmitted, "held for review"
		if dt.Metadata == nil {
			dt.Metadata = map[string]string{}
		}
		dt.Metadata[a2adomain.MetadataQuarantineMessageID] = heldID
	case quarantine.VerdictRejected:
		state, sdkState, note = a2adomain.TaskStateRejected, sdka2a.TaskStateRejected, "rejected by the quarantine"
	}
	dt.State = state
	if err := e.store.UpdateA2ATask(ctx, dt); err != nil {
		e.failUnrecordedTask(ctx, dt, verdict, heldID, err)
		return fmt.Errorf("record the screened a2a task: %w", err)
	}

	if verdict == quarantine.VerdictPass {
		if err := e.queue.Publish(ctx, messagequeue.SubjectA2ATaskCreated, payload); err != nil {
			slog.Error("a2a: publish task created", "error", err)
		}
	}

	// Emit the task's status via the SDK event queue (reqCtx implements
	// a2a.TaskInfoProvider) and broadcast it to the WS hub.
	var msg *sdka2a.Message
	if note != "" {
		msg = sdka2a.NewMessage(sdka2a.MessageRoleAgent, sdka2a.TextPart{Text: note})
	}
	_ = eq.Write(ctx, sdka2a.NewStatusUpdateEvent(reqCtx, sdkState, msg))
	e.broadcastStatus(ctx, taskID, string(state), "inbound")

	slog.Info("a2a: task created", "task_id", taskID, "prompt_len", len(prompt), "state", state)
	return nil
}

// failUnrecordedTask handles a task whose screening could not be recorded
// (S2-G fix 2, 6). A held prompt whose task does not name it could neither
// be withdrawn by the caller's cancel nor be told apart by an approval: it is
// withdrawn at once (fail closed), and the task is failed; best effort.
func (e *Executor) failUnrecordedTask(ctx context.Context, dt *a2adomain.A2ATask, verdict quarantine.Verdict, heldID string, cause error) {
	if verdict == quarantine.VerdictHeld && heldID != "" && e.screener != nil {
		if err := e.screener.Withdraw(ctx, heldID, "its A2A task could not record it"); err != nil {
			slog.Error("a2a: held prompt of an unrecorded task not withdrawn", "task_id", dt.ID, "quarantine_id", heldID, "error", err)
		}
	}
	dt.State = a2adomain.TaskStateFailed
	dt.ErrorMessage = "the screened task could not be recorded: " + cause.Error()
	delete(dt.Metadata, a2adomain.MetadataQuarantineMessageID)
	if err := e.store.UpdateA2ATask(ctx, dt); err != nil {
		slog.Error("a2a: unrecorded task not failed", "task_id", dt.ID, "error", err)
	}
}

// screen returns the screener's verdict on an inbound task's message, and
// the held message's ID; pass without a screener (quarantine disabled). A
// message that cannot be screened is rejected (fail closed).
func (e *Executor) screen(ctx context.Context, ann *trust.Annotation, taskID string, payload []byte) (verdict quarantine.Verdict, heldID string) {
	if e.screener == nil {
		return quarantine.VerdictPass, ""
	}
	verdict, id, err := e.screener.ScreenMessage(ctx, ann, messagequeue.SubjectA2ATaskCreated, payload, "")
	if err != nil {
		slog.Error("a2a: screening inbound task failed, rejecting it", "task_id", taskID, "error", err)
		return quarantine.VerdictRejected, ""
	}
	return verdict, id
}

// Cancel cancels an inbound A2A task (implements a2asrv.AgentExecutor).
func (e *Executor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, eq eventqueue.Queue) error {
	taskID := fmt.Sprintf("a2a-%s", reqCtx.TaskID)

	// Update task state.
	dt, err := e.store.GetA2ATask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get a2a task for cancel: %w", err)
	}
	// A caller cancels only the inbound tasks its own key created.
	if !ownedByCaller(ctx, dt) {
		return fmt.Errorf("get a2a task for cancel: %w", sdka2a.ErrTaskNotFound)
	}
	// A held task's prompt is withdrawn from the quarantine, so an admin's
	// later approval publishes nothing (S2-G fix, 9; the approval also
	// checks the task's state).
	if id := dt.Metadata[a2adomain.MetadataQuarantineMessageID]; id != "" && dt.State == a2adomain.TaskStateSubmitted && e.screener != nil {
		if err := e.screener.Withdraw(ctx, id, "cancelled by the A2A caller"); err != nil {
			slog.Warn("a2a: held prompt of a cancelled task not withdrawn", "task_id", taskID, "quarantine_id", id, "error", err)
		}
	}
	dt.State = a2adomain.TaskStateCanceled
	if err := e.store.UpdateA2ATask(ctx, dt); err != nil {
		return fmt.Errorf("update a2a task for cancel: %w", err)
	}

	// Publish cancel to NATS.
	cancelPayload, marshalErr := json.Marshal(cancelResult{TaskID: taskID})
	if marshalErr != nil {
		slog.Error("a2a: failed to marshal cancel payload", "task_id", taskID, "error", marshalErr)
		return fmt.Errorf("marshal a2a cancel payload: %w", marshalErr)
	}
	if err := e.queue.Publish(ctx, messagequeue.SubjectA2ATaskCancel, cancelPayload); err != nil {
		slog.Error("a2a: publish task cancel", "error", err)
	}

	// Emit canceled status event.
	_ = eq.Write(ctx, sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateCanceled, nil))

	e.broadcastStatus(ctx, taskID, string(a2adomain.TaskStateCanceled), "inbound")

	slog.Info("a2a: task canceled", "task_id", taskID)
	return nil
}

func (e *Executor) broadcastStatus(ctx context.Context, taskID, state, direction string) {
	e.hub.BroadcastEvent(ctx, event.EventA2ATaskStatus, map[string]string{
		"task_id":   taskID,
		"state":     state,
		"direction": direction,
	})
}
