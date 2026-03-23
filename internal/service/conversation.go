package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"encoding/json"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	cfmetrics "github.com/Strob0t/CodeForge/internal/port/metrics"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

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
	db              database.Store
	hub             broadcast.Broadcaster
	queue           messagequeue.Queue
	model           string // default model name for LiteLLM
	modelRegistry   *ModelRegistry
	modeSvc         *ModeService
	mcpSvc          *MCPService
	policySvc       *PolicyService
	microagentSvc   *MicroagentService
	goalSvc         *GoalDiscoveryService
	sessionSvc      *SessionService
	agentCfg        *config.Agent
	routingCfg      *config.Routing
	appEnv          string
	metrics         cfmetrics.Recorder
	contextOpt      *ContextOptimizerService
	llmKeySvc       *LLMKeyService
	promptAssembler *PromptAssembler
	scoreCollector  *PromptScoreCollector
	events          eventstore.Store

	// Sub-services for decomposed responsibilities.
	msgSvc    *ConversationMessageService
	promptSvc *PromptAssemblyService

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
func (s *ConversationService) SetMetrics(m cfmetrics.Recorder) { s.metrics = m }

// SetGoalService wires the goal discovery service for system prompt injection.
func (s *ConversationService) SetGoalService(svc *GoalDiscoveryService) { s.goalSvc = svc }

// SetRoutingConfig configures intelligent model routing for conversation runs.
func (s *ConversationService) SetRoutingConfig(cfg *config.Routing) { s.routingCfg = cfg }

// SetAppEnv configures the application environment (e.g. "development") for prompt assembly.
func (s *ConversationService) SetAppEnv(env string) { s.appEnv = env }

// SetContextOptimizer configures the context optimizer for conversation context injection.
func (s *ConversationService) SetContextOptimizer(opt *ContextOptimizerService) { s.contextOpt = opt }

// SetSessionService configures the session service for conversation session tracking.
func (s *ConversationService) SetSessionService(svc *SessionService) { s.sessionSvc = svc }

// SetLLMKeyService configures per-user LLM key resolution for conversation runs.
func (s *ConversationService) SetLLMKeyService(svc *LLMKeyService) { s.llmKeySvc = svc }

// SetPromptAssembler configures the modular prompt assembler for system prompt generation.
func (s *ConversationService) SetPromptAssembler(a *PromptAssembler) { s.promptAssembler = a }

// SetPromptScoreCollector configures automatic score recording on run completion.
func (s *ConversationService) SetPromptScoreCollector(sc *PromptScoreCollector) {
	s.scoreCollector = sc
}

// SetEventStore configures access to trajectory events for budget tracking.
func (s *ConversationService) SetEventStore(es eventstore.Store) { s.events = es }

// SetMessageService configures the message sub-service.
func (s *ConversationService) SetMessageService(svc *ConversationMessageService) { s.msgSvc = svc }

// SetPromptService configures the prompt assembly sub-service.
func (s *ConversationService) SetPromptService(svc *PromptAssemblyService) { s.promptSvc = svc }

// PromptService returns the prompt assembly sub-service for external wiring.
func (s *ConversationService) PromptService() *PromptAssemblyService { return s.promptSvc }

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
	if s.msgSvc != nil {
		return s.msgSvc.ListMessages(ctx, conversationID)
	}
	return s.db.ListMessages(ctx, conversationID)
}

// SearchMessages performs full-text search across conversation messages.
func (s *ConversationService) SearchMessages(ctx context.Context, query string, projectIDs []string, limit int) ([]conversation.Message, error) {
	if s.msgSvc != nil {
		return s.msgSvc.SearchMessages(ctx, query, projectIDs, limit)
	}
	return s.db.SearchConversationMessages(ctx, query, projectIDs, limit)
}

// SendMessage stores the user message and dispatches a simple (non-agentic) chat run
// to the Python worker via NATS. The result arrives asynchronously via WebSocket AG-UI
// events and is stored by HandleConversationRunComplete.
func (s *ConversationService) SendMessage(ctx context.Context, conversationID string, req *conversation.SendMessageRequest) (*conversation.Message, error) {
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

	// Validate images before storing.
	for i := range req.Images {
		if err := req.Images[i].Validate(); err != nil {
			return nil, fmt.Errorf("image %d: %w", i, err)
		}
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
	s.hub.BroadcastEvent(ctx, event.AGUIRunStarted, event.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: "assistant",
	})

	// Publish to NATS for the Python worker.
	if err := s.queue.PublishWithDedup(ctx, messagequeue.SubjectConversationRunStart, data, dedupKey); err != nil {
		s.hub.BroadcastEvent(ctx, event.AGUIRunFinished, event.AGUIRunFinishedEvent{
			RunID:  runID,
			Status: "failed",
			Error:  err.Error(),
		})
		return nil, fmt.Errorf("publish conversation run start: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RecordRunStarted(ctx, "type", "conversation", "project.id", conv.ProjectID)
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
func (s *ConversationService) CompactConversation(ctx context.Context, conversationID string) error {
	if s.msgSvc != nil {
		return s.msgSvc.CompactConversation(ctx, conversationID)
	}
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

// errMissingConversationID is returned when a compact-complete payload lacks a conversation ID.
var errMissingConversationID = errors.New("missing conversation_id")

// HandleCompactComplete processes a conversation.compact.complete message from the Python worker.
func (s *ConversationService) HandleCompactComplete(ctx context.Context, subject string, data []byte) error {
	if s.msgSvc != nil {
		return s.msgSvc.HandleCompactComplete(ctx, subject, data)
	}
	var p messagequeue.ConversationCompactCompletePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("unmarshal compact complete: %w", err)
	}
	if p.ConversationID == "" {
		return errMissingConversationID
	}
	if p.Status != "completed" {
		slog.Warn("compact not completed", "conversation_id", p.ConversationID, "status", p.Status)
		return nil
	}
	slog.Info("compact complete",
		"conversation_id", p.ConversationID,
		"original_count", p.OriginalCount,
		"summary_len", len(p.Summary),
	)
	return nil
}

// StartCompactSubscriber subscribes to conversation.compact.complete and returns a cancel function.
func (s *ConversationService) StartCompactSubscriber(ctx context.Context) (func(), error) {
	if s.msgSvc != nil {
		return s.msgSvc.StartCompactSubscriber(ctx)
	}
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectConversationCompactComplete, s.HandleCompactComplete)
}

// ClearConversation deletes all messages from a conversation.
func (s *ConversationService) ClearConversation(ctx context.Context, conversationID string) error {
	if s.msgSvc != nil {
		return s.msgSvc.ClearConversation(ctx, conversationID)
	}
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
