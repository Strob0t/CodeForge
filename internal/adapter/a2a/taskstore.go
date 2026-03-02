package a2a

import (
	"context"
	"encoding/json"

	sdka2a "github.com/a2aproject/a2a-go/a2a"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// TaskStoreAdapter wraps database.Store to implement a2asrv.TaskStore.
type TaskStoreAdapter struct {
	store database.Store
}

// NewTaskStoreAdapter creates a TaskStoreAdapter.
func NewTaskStoreAdapter(store database.Store) *TaskStoreAdapter {
	return &TaskStoreAdapter{store: store}
}

// Save persists a task via database.Store (implements a2asrv.TaskStore).
func (a *TaskStoreAdapter) Save(ctx context.Context, task *sdka2a.Task, _ sdka2a.Event, prev sdka2a.TaskVersion) (sdka2a.TaskVersion, error) {
	dt := sdkToDomainTask(task, "inbound")
	if prev == sdka2a.TaskVersionMissing {
		dt.Version = 1
		if err := a.store.CreateA2ATask(ctx, dt); err != nil {
			return 0, err
		}
		return sdka2a.TaskVersion(dt.Version), nil
	}
	dt.Version = int(prev) + 1
	if err := a.store.UpdateA2ATask(ctx, dt); err != nil {
		return 0, err
	}
	return sdka2a.TaskVersion(dt.Version), nil
}

// Get retrieves a task by ID (implements a2asrv.TaskStore).
func (a *TaskStoreAdapter) Get(ctx context.Context, id sdka2a.TaskID) (*sdka2a.Task, sdka2a.TaskVersion, error) {
	dt, err := a.store.GetA2ATask(ctx, string(id))
	if err != nil {
		return nil, 0, err
	}
	t := domainToSDKTask(dt)
	return t, sdka2a.TaskVersion(dt.Version), nil
}

// List returns tasks matching the filter (implements a2asrv.TaskStore).
func (a *TaskStoreAdapter) List(ctx context.Context, _ *sdka2a.ListTasksRequest) (*sdka2a.ListTasksResponse, error) {
	filter := &database.A2ATaskFilter{}
	tasks, _, err := a.store.ListA2ATasks(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]*sdka2a.Task, 0, len(tasks))
	for i := range tasks {
		out = append(out, domainToSDKTask(&tasks[i]))
	}
	return &sdka2a.ListTasksResponse{Tasks: out}, nil
}

// domainToSDKTask converts a domain A2ATask to an SDK Task.
func domainToSDKTask(dt *a2adomain.A2ATask) *sdka2a.Task {
	t := &sdka2a.Task{
		ID:        sdka2a.TaskID(dt.ID),
		ContextID: dt.ContextID,
		Status: sdka2a.TaskStatus{
			State: sdka2a.TaskState(dt.State),
		},
	}
	if dt.ErrorMessage != "" {
		t.Status.Message = &sdka2a.Message{
			Role: sdka2a.MessageRoleAgent,
			Parts: []sdka2a.Part{
				sdka2a.TextPart{Text: dt.ErrorMessage},
			},
		}
	}
	if len(dt.History) > 2 {
		var msgs []*sdka2a.Message
		if err := json.Unmarshal(dt.History, &msgs); err == nil {
			t.History = msgs
		}
	}
	if len(dt.Artifacts) > 2 {
		var arts []*sdka2a.Artifact
		if err := json.Unmarshal(dt.Artifacts, &arts); err == nil {
			t.Artifacts = arts
		}
	}
	return t
}

// sdkToDomainTask converts an SDK Task to a domain A2ATask.
func sdkToDomainTask(t *sdka2a.Task, direction string) *a2adomain.A2ATask {
	dt := &a2adomain.A2ATask{
		ID:        string(t.ID),
		ContextID: t.ContextID,
		State:     a2adomain.TaskState(t.Status.State),
		Direction: a2adomain.Direction(direction),
	}
	if t.Status.Message != nil {
		for _, p := range t.Status.Message.Parts {
			if tp, ok := p.(sdka2a.TextPart); ok {
				dt.ErrorMessage = tp.Text
				break
			}
		}
	}
	if len(t.History) > 0 {
		dt.History, _ = json.Marshal(t.History)
	}
	if len(t.Artifacts) > 0 {
		dt.Artifacts, _ = json.Marshal(t.Artifacts)
	}
	return dt
}
