package service

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"text/template"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
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

// ConversationService manages conversations and LLM interactions.
type ConversationService struct {
	db            database.Store
	llm           *litellm.Client
	hub           broadcast.Broadcaster
	queue         messagequeue.Queue
	model         string // default model name for LiteLLM
	modelRegistry *ModelRegistry
	modeSvc       *ModeService
	mcpSvc        *MCPService
	policySvc     *PolicyService
	microagentSvc *MicroagentService
	agentCfg      *config.Agent
	metrics       *cfotel.Metrics
}

// NewConversationService creates a new ConversationService.
func NewConversationService(
	db database.Store,
	llm *litellm.Client,
	hub broadcast.Broadcaster,
	defaultModel string,
	modeSvc *ModeService,
) *ConversationService {
	return &ConversationService{db: db, llm: llm, hub: hub, model: defaultModel, modeSvc: modeSvc}
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

// resolveModel picks the best available model using priority:
// AgentConfig.DefaultModel > ModelRegistry.BestModel > static ConversationService.model.
func (s *ConversationService) resolveModel() string {
	if s.agentCfg != nil && s.agentCfg.DefaultModel != "" {
		return s.agentCfg.DefaultModel
	}
	if s.modelRegistry != nil {
		if best := s.modelRegistry.BestModel(); best != "" {
			return best
		}
	}
	return s.model
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

// SendMessage stores the user message, calls LiteLLM, stores the assistant response,
// and broadcasts it via WebSocket AG-UI events. This is the simple (non-agentic) path.
func (s *ConversationService) SendMessage(ctx context.Context, conversationID string, req conversation.SendMessageRequest) (*conversation.Message, error) {
	if req.Content == "" {
		return nil, errors.New("content is required")
	}

	// OTEL: conversation run span
	_, runSpan := cfotel.StartRunSpan(ctx, conversationID, conversationID, "")
	defer runSpan.End()

	// Verify conversation exists
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	// Store user message
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Content,
	}
	if _, err = s.db.CreateMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}

	// Build chat messages from history
	messages, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	// Build dynamic system prompt from template
	systemPrompt := s.buildSystemPrompt(ctx, conv.ProjectID)

	chatMessages := make([]litellm.ChatMessage, 0, len(messages)+1)
	chatMessages = append(chatMessages, litellm.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})
	for i := range messages {
		if messages[i].Role == "system" {
			continue
		}
		chatMessages = append(chatMessages, litellm.ChatMessage{
			Role:    messages[i].Role,
			Content: messages[i].Content,
		})
	}

	// Broadcast run started event
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     conversationID,
		ThreadID:  conversationID,
		AgentName: "assistant",
	})

	// Resolve model for non-agentic chat.
	chatModel := s.resolveModel()
	if chatModel == "" {
		return nil, errors.New("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	// Call LiteLLM with streaming — each chunk is broadcast via AG-UI text_message.
	llmResp, err := s.llm.ChatCompletionStream(ctx, litellm.ChatCompletionRequest{
		Model:    chatModel,
		Messages: chatMessages,
	}, func(chunk litellm.StreamChunk) {
		if chunk.Done {
			return
		}
		if chunk.Content != "" {
			s.hub.BroadcastEvent(ctx, ws.AGUITextMessage, ws.AGUITextMessageEvent{
				RunID:   conversationID,
				Role:    "assistant",
				Content: chunk.Content,
			})
		}
	})
	if err != nil {
		slog.Error("llm chat completion stream failed", "conversation_id", conversationID, "error", err)
		runSpan.SetStatus(codes.Error, err.Error())
		if s.metrics != nil {
			s.metrics.RunsFailed.Add(ctx, 1, metric.WithAttributes(
				attribute.String("type", "conversation"),
			))
		}
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  conversationID,
			Status: "failed",
			Error:  err.Error(),
		})
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	// Store assistant message with the full accumulated response.
	assistantMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        llmResp.Content,
		TokensIn:       llmResp.TokensIn,
		TokensOut:      llmResp.TokensOut,
		Model:          llmResp.Model,
	}
	assistantMsg, err = s.db.CreateMessage(ctx, assistantMsg)
	if err != nil {
		return nil, fmt.Errorf("store assistant message: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RunsCompleted.Add(ctx, 1, metric.WithAttributes(
			attribute.String("type", "conversation"),
		))
	}

	// Broadcast run finished.
	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
		RunID:  conversationID,
		Status: "completed",
	})

	return assistantMsg, nil
}

// IsAgentic determines whether a conversation message should use the agentic loop.
// The request may explicitly set Agentic; otherwise the project must have a workspace
// and the agent config must default to agentic mode.
