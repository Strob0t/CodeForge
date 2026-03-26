package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/netutil"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// MaxA2APromptLength limits the size of prompts submitted via A2A to prevent abuse.
const MaxA2APromptLength = 100_000

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

	cardJSON, marshalErr := json.Marshal(card)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal agent card: %w", marshalErr)
	}
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
	cardJSON, marshalErr := json.Marshal(card)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal agent card: %w", marshalErr)
	}
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
	if len(prompt) > MaxA2APromptLength {
		return nil, fmt.Errorf("send task: prompt exceeds maximum length of %d bytes", MaxA2APromptLength)
	}

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

	historyJSON, err := json.Marshal(sdkTask.History)
	if err != nil {
		return nil, fmt.Errorf("marshal task history: %w", err)
	}
	dt.History = historyJSON
	artifactsJSON, err := json.Marshal(sdkTask.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("marshal task artifacts: %w", err)
	}
	dt.Artifacts = artifactsJSON

	if err := s.store.CreateA2ATask(ctx, dt); err != nil {
		slog.Warn("failed to persist outbound A2A task", "error", err)
	}

	// Broadcast to WS.
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, event.EventA2ATaskCreated, event.A2ATaskStatusEvent{
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
		logBestEffort(ctx, s.store.UpdateA2ATask(ctx, dt), "UpdateA2ATask", slog.String("task_id", taskID))
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
		logBestEffort(ctx, s.store.UpdateA2ATask(ctx, dt), "UpdateA2ATask", slog.String("task_id", taskID))
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
	data, err := json.Marshal(map[string]string{"task_id": id})
	if err != nil {
		return fmt.Errorf("marshal cancel payload: %w", err)
	}
	if err := s.queue.Publish(ctx, messagequeue.SubjectA2ATaskCancel, data); err != nil {
		return fmt.Errorf("cancel inbound a2a task: %w", err)
	}

	dt.State = a2adomain.TaskStateCanceled
	dt.UpdatedAt = time.Now().UTC()
	return s.store.UpdateA2ATask(ctx, dt)
}

// --- Push Notification Config CRUD (Phase 27O) ---

// CreatePushConfig creates a push notification config for a task.
func (s *A2AService) CreatePushConfig(ctx context.Context, taskID, webhookURL, token string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("push config: task_id is required")
	}
	if webhookURL == "" {
		return "", fmt.Errorf("push config: url is required")
	}
	if err := validateWebhookURL(webhookURL); err != nil {
		return "", fmt.Errorf("push config: %w", err)
	}
	// Verify task exists.
	if _, err := s.store.GetA2ATask(ctx, taskID); err != nil {
		return "", fmt.Errorf("push config: %w", err)
	}
	return s.store.CreateA2APushConfig(ctx, taskID, webhookURL, token)
}

// validateWebhookURL validates that a webhook URL is safe for server-side requests.
// It requires https (or http://localhost for dev), and blocks private/reserved IP ranges.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	hostname := u.Hostname()
	isLoopback := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"

	// Require https, except localhost/loopback for development.
	if u.Scheme != "https" {
		if u.Scheme == "http" && isLoopback {
			return nil // Allow http for localhost development; skip SSRF check.
		}
		return fmt.Errorf("webhook url must use https (got %q)", u.Scheme)
	}

	// For https URLs: resolve hostname to check for private IPs (SSRF prevention).
	if ip := net.ParseIP(hostname); ip != nil {
		if netutil.IsPrivateIP(ip) {
			return fmt.Errorf("webhook url must not target private/reserved IP addresses")
		}
	} else {
		resolver := &net.Resolver{}
		addrs, resolveErr := resolver.LookupHost(context.Background(), hostname)
		if resolveErr == nil {
			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil && netutil.IsPrivateIP(ip) {
					return fmt.Errorf("webhook url hostname %q resolves to private IP", hostname)
				}
			}
		}
	}

	return nil
}

// ListPushConfigs returns all push configs for a task.
func (s *A2AService) ListPushConfigs(ctx context.Context, taskID string) ([]database.A2APushConfig, error) {
	return s.store.ListA2APushConfigs(ctx, taskID)
}

// DeletePushConfig removes a push notification config.
func (s *A2AService) DeletePushConfig(ctx context.Context, id string) error {
	return s.store.DeleteA2APushConfig(ctx, id)
}

// a2aPushPayload is the JSON structure sent to push notification webhooks.
type a2aPushPayload struct {
	TaskID    string          `json:"task_id"`
	State     string          `json:"state"`
	Artifacts json.RawMessage `json:"artifacts"`
}

// DispatchPushNotifications sends webhook POST to all push configs for a task.
func (s *A2AService) DispatchPushNotifications(ctx context.Context, taskID string) {
	configs, err := s.store.ListA2APushConfigs(ctx, taskID)
	if err != nil || len(configs) == 0 {
		return
	}

	task, err := s.store.GetA2ATask(ctx, taskID)
	if err != nil {
		slog.Warn("push dispatch: task not found", "task_id", taskID, "error", err)
		return
	}

	payload, err := json.Marshal(a2aPushPayload{
		TaskID:    task.ID,
		State:     string(task.State),
		Artifacts: task.Artifacts,
	})
	if err != nil {
		slog.Warn("push dispatch: marshal payload", "task_id", taskID, "error", err)
		return
	}

	for _, cfg := range configs {
		go s.sendWebhook(cfg.URL, cfg.Token, payload) //nolint:gosec // G118: webhook must outlive the HTTP request
	}
}

// sendWebhook POSTs a payload to a webhook URL with optional Bearer token, HMAC signature, and retry.
func (s *A2AService) sendWebhook(webhookURL, token string, payload []byte) {
	const maxRetries = 3
	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()

	for attempt := range maxRetries {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			slog.Warn("push webhook: bad request", "url", webhookURL, "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			// HMAC-SHA256 signature for payload integrity verification.
			req.Header.Set("X-CodeForge-Signature", computeHMAC(payload, token))
		}

		resp, err := client.Do(req) //nolint:gosec // URL is validated at push config creation time
		if err != nil {
			slog.Warn("push webhook: request failed", "url", webhookURL, "attempt", attempt+1, "error", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode < 300 {
			slog.Debug("push webhook: delivered", "url", webhookURL, "status", resp.StatusCode)
			return
		}
		slog.Warn("push webhook: non-2xx response", "url", webhookURL, "status", resp.StatusCode, "attempt", attempt+1)
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	slog.Error("push webhook: all retries exhausted", "url", webhookURL)
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of payload using key as the secret.
func computeHMAC(payload []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// HandleTaskComplete processes a completed A2A task: updates state, broadcasts WS, dispatches push.
func (s *A2AService) HandleTaskComplete(ctx context.Context, taskID, state, errMsg string) error {
	dt, err := s.store.GetA2ATask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("handle task complete: %w", err)
	}

	dt.State = a2adomain.TaskState(state)
	dt.ErrorMessage = errMsg
	dt.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateA2ATask(ctx, dt); err != nil {
		return fmt.Errorf("handle task complete: %w", err)
	}

	// Broadcast WS event.
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, event.EventA2ATaskComplete, event.A2ATaskStatusEvent{
			TaskID:        dt.ID,
			State:         string(dt.State),
			Direction:     string(dt.Direction),
			RemoteAgentID: dt.RemoteAgentID,
		})
	}

	// Dispatch push notifications asynchronously.
	s.DispatchPushNotifications(ctx, taskID)
	return nil
}

// StartCompletionSubscriber subscribes to A2A task completion events from NATS.
// It updates task state, broadcasts WS events, and dispatches push notifications.
func (s *A2AService) StartCompletionSubscriber(ctx context.Context) (cancel func(), err error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectA2ATaskComplete, func(ctx context.Context, _ string, data []byte) error {
		var payload messagequeue.A2ATaskCompletePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			slog.Warn("a2a: invalid completion payload", "error", err)
			return nil // don't retry malformed messages
		}
		if payload.TenantID != "" {
			ctx = tenantctx.WithTenant(ctx, payload.TenantID)
		}
		if err := s.HandleTaskComplete(ctx, payload.TaskID, payload.State, payload.Error); err != nil {
			slog.Warn("a2a: handle task complete", "task_id", payload.TaskID, "error", err)
		}
		return nil
	})
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
