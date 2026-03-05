package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// Executor implements a2asrv.AgentExecutor for inbound A2A tasks.
type Executor struct {
	store database.Store
	queue messagequeue.Queue
	hub   broadcast.Broadcaster
	modes []string
}

// NewExecutor creates an Executor.
func NewExecutor(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, modes []string) *Executor {
	return &Executor{store: store, queue: queue, hub: hub, modes: modes}
}

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

	// Create domain task.
	taskID := fmt.Sprintf("a2a-%s", reqCtx.TaskID)
	dt := a2adomain.NewA2ATask(taskID)
	dt.State = a2adomain.TaskStateWorking
	dt.Direction = a2adomain.DirectionInbound
	dt.TrustOrigin = "a2a"
	dt.TrustLevel = "untrusted"

	if err := e.store.CreateA2ATask(ctx, dt); err != nil {
		return fmt.Errorf("create a2a task: %w", err)
	}

	// Publish to NATS for worker pickup.
	payload, _ := json.Marshal(messagequeue.A2ATaskCreatedPayload{
		TaskID:  taskID,
		SkillID: "",
		Prompt:  prompt,
	})
	if err := e.queue.Publish(ctx, messagequeue.SubjectA2ATaskCreated, payload); err != nil {
		slog.Error("a2a: publish task created", "error", err)
	}

	// Emit working status event via SDK event queue.
	// reqCtx implements a2a.TaskInfoProvider.
	_ = eq.Write(ctx, sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateWorking, nil))

	// Broadcast to WS hub.
	e.broadcastStatus(ctx, taskID, string(a2adomain.TaskStateWorking), "inbound")

	slog.Info("a2a: task created", "task_id", taskID, "prompt_len", len(prompt))
	return nil
}

// Cancel cancels an inbound A2A task (implements a2asrv.AgentExecutor).
func (e *Executor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, eq eventqueue.Queue) error {
	taskID := fmt.Sprintf("a2a-%s", reqCtx.TaskID)

	// Update task state.
	dt, err := e.store.GetA2ATask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get a2a task for cancel: %w", err)
	}
	dt.State = a2adomain.TaskStateCanceled
	if err := e.store.UpdateA2ATask(ctx, dt); err != nil {
		return fmt.Errorf("update a2a task for cancel: %w", err)
	}

	// Publish cancel to NATS.
	cancelPayload, _ := json.Marshal(map[string]string{"task_id": taskID})
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
	e.hub.BroadcastEvent(ctx, ws.EventA2ATaskStatus, map[string]string{
		"task_id":   taskID,
		"state":     state,
		"direction": direction,
	})
}
