package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// A2AService manages outbound A2A federation (Phase 27K).
type A2AService struct {
	store    database.Store
	queue    messagequeue.Queue
	hub      broadcast.Broadcaster
	resolver *agentcard.Resolver

	mu      sync.RWMutex
	clients map[string]*a2aclient.Client
}

// NewA2AService creates an A2AService for outbound A2A task delegation.
func NewA2AService(store database.Store, queue messagequeue.Queue, hub ...broadcast.Broadcaster) *A2AService {
	svc := &A2AService{
		store:    store,
		queue:    queue,
		resolver: agentcard.DefaultResolver,
		clients:  make(map[string]*a2aclient.Client),
	}
	if len(hub) > 0 {
		svc.hub = hub[0]
	}
	return svc
}

// DiscoverAgent fetches an AgentCard from a remote URL.
func (s *A2AService) DiscoverAgent(ctx context.Context, baseURL string) (*sdka2a.AgentCard, error) {
	card, err := s.resolver.Resolve(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("discover agent: %w", err)
	}
	return card, nil
}

// RegisterRemoteAgent discovers a remote agent and persists it.
func (s *A2AService) RegisterRemoteAgent(ctx context.Context, name, agentURL, trustLevel string) (*a2adomain.RemoteAgent, error) {
	if name == "" {
		return nil, fmt.Errorf("remote agent: name is required")
	}
	if agentURL == "" {
		return nil, fmt.Errorf("remote agent: url is required")
	}

	card, err := s.DiscoverAgent(ctx, agentURL)
	if err != nil {
		return nil, fmt.Errorf("register remote agent: %w", err)
	}

	ra := a2adomain.NewRemoteAgent(name, agentURL)
	if trustLevel != "" {
		ra.TrustLevel = trustLevel
	}

	// Extract skill IDs from card.
	for i := range card.Skills {
		ra.Skills = append(ra.Skills, card.Skills[i].ID)
	}

	cardJSON, _ := json.Marshal(card)
	ra.CardJSON = cardJSON
	now := time.Now().UTC()
	ra.LastSeen = &now

	if err := s.store.CreateRemoteAgent(ctx, ra); err != nil {
		return nil, fmt.Errorf("register remote agent: %w", err)
	}
	return ra, nil
}

// RefreshAgent re-fetches the AgentCard for an existing remote agent.
func (s *A2AService) RefreshAgent(ctx context.Context, agentID string) (*a2adomain.RemoteAgent, error) {
	ra, err := s.store.GetRemoteAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	card, err := s.DiscoverAgent(ctx, ra.URL)
	if err != nil {
		return nil, fmt.Errorf("refresh agent: %w", err)
	}

	ra.Skills = ra.Skills[:0]
	for i := range card.Skills {
		ra.Skills = append(ra.Skills, card.Skills[i].ID)
	}
	cardJSON, _ := json.Marshal(card)
	ra.CardJSON = cardJSON
	now := time.Now().UTC()
	ra.LastSeen = &now
	ra.UpdatedAt = now

	if err := s.store.UpdateRemoteAgent(ctx, ra); err != nil {
		return nil, fmt.Errorf("refresh agent: %w", err)
	}

	// Invalidate cached client.
	s.mu.Lock()
	if c, ok := s.clients[agentID]; ok {
		_ = c.Destroy()
		delete(s.clients, agentID)
	}
	s.mu.Unlock()

	return ra, nil
}

// SendTask sends a task to a remote A2A agent.
func (s *A2AService) SendTask(ctx context.Context, remoteAgentID, skillID, prompt string) (*a2adomain.A2ATask, error) {
	client, ra, err := s.getOrCreateClient(ctx, remoteAgentID)
	if err != nil {
		return nil, err
	}

	msg := &sdka2a.Message{
		Role: sdka2a.MessageRoleUser,
		Parts: []sdka2a.Part{
			sdka2a.TextPart{Text: prompt},
		},
	}
	params := &sdka2a.MessageSendParams{
		Message: msg,
	}

	result, err := client.SendMessage(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("send task to %s: %w", ra.Name, err)
	}

	// Extract the task from the result.
	sdkTask, ok := result.(*sdka2a.Task)
	if !ok {
		return nil, fmt.Errorf("send task: unexpected result type %T", result)
	}

	// Persist as outbound A2A task.
	dt := a2adomain.NewA2ATask(string(sdkTask.ID))
	dt.Direction = a2adomain.DirectionOutbound
	dt.State = a2adomain.TaskState(string(sdkTask.Status.State))
	dt.SkillID = skillID
	dt.RemoteAgentID = remoteAgentID

	historyJSON, _ := json.Marshal(sdkTask.History)
	dt.History = historyJSON
	artifactsJSON, _ := json.Marshal(sdkTask.Artifacts)
	dt.Artifacts = artifactsJSON

	if err := s.store.CreateA2ATask(ctx, dt); err != nil {
		slog.Warn("failed to persist outbound A2A task", "error", err)
	}

	// Broadcast to WS.
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, ws.EventA2ATaskCreated, ws.A2ATaskStatusEvent{
			TaskID:        dt.ID,
			State:         string(dt.State),
			SkillID:       dt.SkillID,
			Direction:     string(dt.Direction),
			RemoteAgentID: dt.RemoteAgentID,
		})
	}

	return dt, nil
}

// GetRemoteTask fetches a task from a remote agent and updates local state.
func (s *A2AService) GetRemoteTask(ctx context.Context, remoteAgentID, taskID string) (*sdka2a.Task, error) {
	client, _, err := s.getOrCreateClient(ctx, remoteAgentID)
	if err != nil {
		return nil, err
	}

	sdkTask, err := client.GetTask(ctx, &sdka2a.TaskQueryParams{
		ID: sdka2a.TaskID(taskID),
	})
	if err != nil {
		return nil, fmt.Errorf("get remote task: %w", err)
	}

	// Update local state.
	dt, getErr := s.store.GetA2ATask(ctx, taskID)
	if getErr == nil {
		dt.State = a2adomain.TaskState(string(sdkTask.Status.State))
		dt.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateA2ATask(ctx, dt)
	}

	return sdkTask, nil
}

// CancelRemoteTask cancels a task on a remote agent.
func (s *A2AService) CancelRemoteTask(ctx context.Context, remoteAgentID, taskID string) error {
	client, _, err := s.getOrCreateClient(ctx, remoteAgentID)
	if err != nil {
		return err
	}

	_, err = client.CancelTask(ctx, &sdka2a.TaskIDParams{
		ID: sdka2a.TaskID(taskID),
	})
	if err != nil {
		return fmt.Errorf("cancel remote task: %w", err)
	}

	// Update local state.
	dt, getErr := s.store.GetA2ATask(ctx, taskID)
	if getErr == nil {
		dt.State = a2adomain.TaskStateCanceled
		dt.UpdatedAt = time.Now().UTC()
		_ = s.store.UpdateA2ATask(ctx, dt)
	}

	return nil
}

// ListRemoteAgents returns all registered remote agents.
func (s *A2AService) ListRemoteAgents(ctx context.Context, tenantID string) ([]a2adomain.RemoteAgent, error) {
	return s.store.ListRemoteAgents(ctx, tenantID, false)
}

// DeleteRemoteAgent removes a remote agent and its cached client.
func (s *A2AService) DeleteRemoteAgent(ctx context.Context, id string) error {
	if err := s.store.DeleteRemoteAgent(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	if c, ok := s.clients[id]; ok {
		_ = c.Destroy()
		delete(s.clients, id)
	}
	s.mu.Unlock()
	return nil
}

// ListTasks returns A2A tasks matching the filter.
func (s *A2AService) ListTasks(ctx context.Context, filter *database.A2ATaskFilter) ([]a2adomain.A2ATask, int, error) {
	return s.store.ListA2ATasks(ctx, filter)
}

// GetTask returns a single A2A task by ID.
func (s *A2AService) GetTask(ctx context.Context, id string) (*a2adomain.A2ATask, error) {
	return s.store.GetA2ATask(ctx, id)
}

// CancelTask cancels an A2A task (outbound: remote cancel; inbound: NATS).
func (s *A2AService) CancelTask(ctx context.Context, id string) error {
	dt, err := s.store.GetA2ATask(ctx, id)
	if err != nil {
		return err
	}

	if dt.Direction == a2adomain.DirectionOutbound && dt.RemoteAgentID != "" {
		return s.CancelRemoteTask(ctx, dt.RemoteAgentID, id)
	}

	// Inbound: publish cancel to NATS for Python worker.
	data, _ := json.Marshal(map[string]string{"task_id": id})
	if err := s.queue.Publish(ctx, messagequeue.SubjectA2ATaskCancel, data); err != nil {
		return fmt.Errorf("cancel inbound a2a task: %w", err)
	}

	dt.State = a2adomain.TaskStateCanceled
	dt.UpdatedAt = time.Now().UTC()
	return s.store.UpdateA2ATask(ctx, dt)
}

// getOrCreateClient lazily creates or retrieves an a2aclient.Client for a remote agent.
func (s *A2AService) getOrCreateClient(ctx context.Context, remoteAgentID string) (*a2aclient.Client, *a2adomain.RemoteAgent, error) {
	s.mu.RLock()
	if c, ok := s.clients[remoteAgentID]; ok {
		s.mu.RUnlock()
		ra, _ := s.store.GetRemoteAgent(ctx, remoteAgentID)
		return c, ra, nil
	}
	s.mu.RUnlock()

	ra, err := s.store.GetRemoteAgent(ctx, remoteAgentID)
	if err != nil {
		return nil, nil, fmt.Errorf("get remote agent: %w", err)
	}

	// Try to resolve from cached card JSON first.
	var card *sdka2a.AgentCard
	if len(ra.CardJSON) > 0 {
		card = &sdka2a.AgentCard{}
		if jsonErr := json.Unmarshal(ra.CardJSON, card); jsonErr != nil {
			card = nil
		}
	}
	if card == nil {
		card, err = s.resolver.Resolve(ctx, ra.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve agent card: %w", err)
		}
	}

	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, nil, fmt.Errorf("create a2a client: %w", err)
	}

	s.mu.Lock()
	s.clients[remoteAgentID] = client
	s.mu.Unlock()

	return client, ra, nil
}
