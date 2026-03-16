package service

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"text/template"

	"encoding/json"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

//go:embed templates/conversation_system.tmpl
var conversationSystemTmpl string

// conversationTmpl is the parsed system prompt template for conversations.
var conversationTmpl = template.Must(template.New("conversation_system").Parse(conversationSystemTmpl))

// conversationPromptData carries project context into the system prompt template.
type conversationPromptData struct {
	ProjectName        string
	ProjectDescription string
	WorkspacePath      string
	Provider           string
	RepoURL            string
	Stack              string
	Agents             []string
	Modes              []string
	RecentTasks        []conversationTaskSummary
	RoadmapSummary     string
	GoalContext        string
	BuiltinTools       []builtinToolSummary
}

// conversationTaskSummary is a minimal task view for the system prompt.
type conversationTaskSummary struct {
	ID     string
	Name   string
	Status string
}

// builtinToolSummary describes a tool for the system prompt template.
type builtinToolSummary struct {
	Name        string
	Description string
}

// CompletionResult carries the outcome of an agentic conversation run.
type CompletionResult struct {
	Status  string
	Error   string
	CostUSD float64
}

// ConversationService manages conversations and LLM interactions.
type ConversationService struct {
	db            database.Store
	hub           broadcast.Broadcaster
	queue         messagequeue.Queue
	model         string // default model name for LiteLLM
	modelRegistry *ModelRegistry
	modeSvc       *ModeService
	mcpSvc        *MCPService
	policySvc     *PolicyService
	microagentSvc *MicroagentService
	goalSvc       *GoalDiscoveryService
	sessionSvc    *SessionService
	agentCfg      *config.Agent
	routingCfg    *config.Routing
	metrics       *cfotel.Metrics
	contextOpt    *ContextOptimizerService
	llmKeySvc     *LLMKeyService

	// completionWaiters allows in-process consumers (e.g. autoagent) to wait for
	// a conversation run to finish without creating a second NATS subscription.
	completionWaiters   map[string]chan CompletionResult
	completionWaitersMu sync.Mutex
}

// NewConversationService creates a new ConversationService.
func NewConversationService(
	db database.Store,
	hub broadcast.Broadcaster,
	defaultModel string,
	modeSvc *ModeService,
) *ConversationService {
	return &ConversationService{
		db:                db,
		hub:               hub,
		model:             defaultModel,
		modeSvc:           modeSvc,
		completionWaiters: make(map[string]chan CompletionResult),
	}
}

// SetQueue configures the NATS queue for agentic message dispatch.
func (s *ConversationService) SetQueue(q messagequeue.Queue) { s.queue = q }

// SetAgentConfig configures agent loop defaults.
func (s *ConversationService) SetAgentConfig(cfg *config.Agent) { s.agentCfg = cfg }

// SetMCPService configures MCP server resolution for agentic runs.
func (s *ConversationService) SetMCPService(mcp *MCPService) { s.mcpSvc = mcp }

// SetPolicyService configures policy evaluation for agentic runs.
func (s *ConversationService) SetPolicyService(p *PolicyService) { s.policySvc = p }

// SetModelRegistry configures dynamic model selection from the registry.
func (s *ConversationService) SetModelRegistry(r *ModelRegistry) { s.modelRegistry = r }

// SetMicroagentService configures microagent matching for agentic runs.
func (s *ConversationService) SetMicroagentService(svc *MicroagentService) { s.microagentSvc = svc }

// SetMetrics sets the OTEL metrics collector.
func (s *ConversationService) SetMetrics(m *cfotel.Metrics) { s.metrics = m }

// SetGoalService wires the goal discovery service for system prompt injection.
func (s *ConversationService) SetGoalService(svc *GoalDiscoveryService) { s.goalSvc = svc }

// SetRoutingConfig configures intelligent model routing for conversation runs.
func (s *ConversationService) SetRoutingConfig(cfg *config.Routing) { s.routingCfg = cfg }

// SetContextOptimizer configures the context optimizer for conversation context injection.
func (s *ConversationService) SetContextOptimizer(opt *ContextOptimizerService) { s.contextOpt = opt }

// SetSessionService configures the session service for conversation session tracking.
func (s *ConversationService) SetSessionService(svc *SessionService) { s.sessionSvc = svc }

// SetLLMKeyService configures per-user LLM key resolution for conversation runs.
func (s *ConversationService) SetLLMKeyService(svc *LLMKeyService) { s.llmKeySvc = svc }

// resolveModel picks the best available model using priority:
// AgentConfig.DefaultModel > ConversationModel (explicit config) > ModelRegistry.BestModel (auto-discovery).
func (s *ConversationService) resolveModel() string {
	if s.agentCfg != nil && s.agentCfg.DefaultModel != "" {
		return s.agentCfg.DefaultModel
	}
	if s.model != "" {
		return s.model
	}
	if s.modelRegistry != nil {
		if best := s.modelRegistry.BestModel(); best != "" {
			return best
		}
	}
	return ""
}

// Create creates a new conversation for a project.
func (s *ConversationService) Create(ctx context.Context, req conversation.CreateRequest) (*conversation.Conversation, error) {
	if req.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}
	c := &conversation.Conversation{
		ProjectID: req.ProjectID,
		Title:     req.Title,
	}
	if c.Title == "" {
		c.Title = "New Conversation"
	}
	return s.db.CreateConversation(ctx, c)
}

// Get returns a conversation by ID.
func (s *ConversationService) Get(ctx context.Context, id string) (*conversation.Conversation, error) {
	return s.db.GetConversation(ctx, id)
}

// ListByProject returns all conversations for a project.
func (s *ConversationService) ListByProject(ctx context.Context, projectID string) ([]conversation.Conversation, error) {
	return s.db.ListConversationsByProject(ctx, projectID)
}

// Delete removes a conversation.
func (s *ConversationService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteConversation(ctx, id)
}

// ListMessages returns all messages in a conversation.
func (s *ConversationService) ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error) {
	return s.db.ListMessages(ctx, conversationID)
}

// SearchMessages performs full-text search across conversation messages.
func (s *ConversationService) SearchMessages(ctx context.Context, query string, projectIDs []string, limit int) ([]conversation.Message, error) {
	return s.db.SearchConversationMessages(ctx, query, projectIDs, limit)
}

// SendMessage stores the user message and dispatches a simple (non-agentic) chat run
// to the Python worker via NATS. The result arrives asynchronously via WebSocket AG-UI
// events and is stored by HandleConversationRunComplete.
func (s *ConversationService) SendMessage(ctx context.Context, conversationID string, req conversation.SendMessageRequest) (*conversation.Message, error) {
	if req.Content == "" {
		return nil, errors.New("content is required")
	}
	if s.queue == nil {
		return nil, errors.New("chat requires NATS queue")
	}

	// Verify conversation exists.
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	// Store user message.
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Content,
	}
	if _, err = s.db.CreateMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}

	// Load full conversation history.
	history, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	// Build system prompt and convert history.
	systemPrompt := s.buildSystemPrompt(ctx, conv.ProjectID)
	protoMessages := s.historyToPayload(history)

	// Resolve model.
	model := s.resolveModel()
	if model == "" {
		return nil, errors.New("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	// RunID matches conversationID for tool-call policy lookups (the policy system
	// uses RunID to find the conversation). A separate unique dedup key prevents
	// NATS JetStream from silently dropping follow-up messages.
	runID := conversationID
	dedupKey := "conv-start-" + uuid.New().String()

	payload := messagequeue.ConversationRunStartPayload{
		RunID:          runID,
		ConversationID: conversationID,
		ProjectID:      conv.ProjectID,
		Messages:       protoMessages,
		SystemPrompt:   systemPrompt,
		Model:          model,
		Agentic:        false,
		Termination: messagequeue.TerminationPayload{
			MaxSteps:       1,
			TimeoutSeconds: 120,
		},
		RoutingEnabled: s.routingCfg != nil && s.routingCfg.Enabled,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal conversation run start: %w", err)
	}

	// Broadcast run started via WebSocket.
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: "assistant",
	})

	// Publish to NATS for the Python worker.
	if err := s.queue.PublishWithDedup(ctx, messagequeue.SubjectConversationRunStart, data, dedupKey); err != nil {
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  runID,
			Status: "failed",
			Error:  err.Error(),
		})
		return nil, fmt.Errorf("publish conversation run start: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RunsStarted.Add(ctx, 1, metric.WithAttributes(
			attribute.String("type", "conversation"),
			attribute.String("project.id", conv.ProjectID),
		))
	}

	slog.Info("conversation simple run dispatched",
		"run_id", runID,
		"conversation_id", conversationID,
		"project_id", conv.ProjectID,
		"model", model,
	)

	return nil, nil
}

// CompactConversation publishes a compaction request to the Python worker via NATS.
// The worker will summarise the conversation history to reduce token usage.
func (s *ConversationService) CompactConversation(ctx context.Context, conversationID string) error {
	_, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if s.queue == nil {
		return errors.New("message queue not configured")
	}
	payload := map[string]string{
		"conversation_id": conversationID,
		"tenant_id":       tenantctx.FromContext(ctx),
	}
	data, _ := json.Marshal(payload)
	return s.queue.Publish(ctx, messagequeue.SubjectConversationCompactRequest, data)
}

// ClearConversation deletes all messages from a conversation.
func (s *ConversationService) ClearConversation(ctx context.Context, conversationID string) error {
	_, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	return s.db.DeleteConversationMessages(ctx, conversationID)
}

// SetMode validates and stores the mode for a conversation.
func (s *ConversationService) SetMode(ctx context.Context, conversationID, mode string) error {
	_, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if s.modeSvc != nil {
		if _, modeErr := s.modeSvc.Get(mode); modeErr != nil {
			return fmt.Errorf("unknown mode: %s", mode)
		}
	}
	return s.db.UpdateConversationMode(ctx, conversationID, mode)
}

// SetModel stores a model override for a conversation.
func (s *ConversationService) SetModel(ctx context.Context, conversationID, model string) error {
	_, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	return s.db.UpdateConversationModel(ctx, conversationID, model)
}

// IsAgentic determines whether a conversation message should use the agentic loop.
// The request may explicitly set Agentic; otherwise the project must have a workspace
// and the agent config must default to agentic mode.
