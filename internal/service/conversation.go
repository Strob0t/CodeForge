package service

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
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
	db        database.Store
	llm       *litellm.Client
	hub       broadcast.Broadcaster
	queue     messagequeue.Queue
	model     string // default model name for LiteLLM
	modeSvc   *ModeService
	mcpSvc    *MCPService
	policySvc *PolicyService
	agentCfg  *config.Agent
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
	chatModel := s.model
	if s.agentCfg != nil && s.agentCfg.DefaultModel != "" {
		chatModel = s.agentCfg.DefaultModel
	}
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
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  conversationID,
			Status: "failed",
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
func (s *ConversationService) IsAgentic(ctx context.Context, conversationID string, req conversation.SendMessageRequest) bool {
	if req.Agentic != nil {
		return *req.Agentic
	}
	// No queue means no worker dispatch capability.
	if s.queue == nil {
		return false
	}
	// Default from agent config.
	if s.agentCfg == nil || !s.agentCfg.AgenticByDefault {
		return false
	}
	// Agentic mode requires a workspace path on the project.
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return false
	}
	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return false
	}
	return proj.WorkspacePath != ""
}

// SendMessageAgentic stores the user message and dispatches an agentic run to the
// Python worker via NATS. Streaming results arrive asynchronously via WebSocket.
// The method returns immediately after dispatch.
func (s *ConversationService) SendMessageAgentic(ctx context.Context, conversationID string, req conversation.SendMessageRequest) error {
	if req.Content == "" {
		return errors.New("content is required")
	}
	if s.queue == nil {
		return errors.New("agentic mode requires NATS queue")
	}

	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}

	// Store user message.
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Content,
	}
	if _, err = s.db.CreateMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("store user message: %w", err)
	}

	// Load full conversation history (including tool_calls and tool results).
	history, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}

	// Fetch project for workspace path, policy profile, etc.
	proj, err := s.db.GetProject(ctx, conv.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// Build system prompt.
	systemPrompt := s.buildSystemPrompt(ctx, conv.ProjectID)

	// Convert history to protocol messages.
	protoMessages := s.historyToPayload(history)

	// Resolve model.
	model := s.model
	if s.agentCfg != nil && s.agentCfg.DefaultModel != "" {
		model = s.agentCfg.DefaultModel
	}
	if model == "" {
		return fmt.Errorf("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	// Resolve policy profile.
	policyProfile := ""
	if s.policySvc != nil {
		policyProfile = s.policySvc.ResolveProfile("", proj.PolicyProfile)
	}

	// Termination config.
	termination := messagequeue.TerminationPayload{
		MaxSteps:       50,
		TimeoutSeconds: 600,
	}
	if s.agentCfg != nil && s.agentCfg.MaxLoopIterations > 0 {
		termination.MaxSteps = s.agentCfg.MaxLoopIterations
	}

	// MCP servers.
	var mcpDefs []messagequeue.MCPServerDefPayload
	if s.mcpSvc != nil {
		servers := s.mcpSvc.ResolveForRun(proj.ID, "")
		for i := range servers {
			mcpDefs = append(mcpDefs, messagequeue.MCPServerDefPayload{
				ID:        servers[i].ID,
				Name:      servers[i].Name,
				Transport: string(servers[i].Transport),
				Command:   servers[i].Command,
				Args:      servers[i].Args,
				URL:       servers[i].URL,
				Env:       servers[i].Env,
			})
		}
	}

	// Use conversation ID as run ID for simplicity (one active run per conversation).
	runID := conversationID

	payload := messagequeue.ConversationRunStartPayload{
		RunID:          runID,
		ConversationID: conversationID,
		ProjectID:      proj.ID,
		Messages:       protoMessages,
		SystemPrompt:   systemPrompt,
		Model:          model,
		PolicyProfile:  policyProfile,
		WorkspacePath:  proj.WorkspacePath,
		Termination:    termination,
		MCPServers:     mcpDefs,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal conversation run start: %w", err)
	}

	// Broadcast run started via WebSocket.
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: "agent",
	})

	// Publish to NATS for the Python worker.
	if err := s.queue.Publish(ctx, messagequeue.SubjectConversationRunStart, data); err != nil {
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  runID,
			Status: "failed",
		})
		return fmt.Errorf("publish conversation run start: %w", err)
	}

	slog.Info("conversation agentic run dispatched",
		"run_id", runID,
		"conversation_id", conversationID,
		"project_id", proj.ID,
		"model", model,
	)

	return nil
}

// HandleConversationRunComplete processes the completion message from the Python worker.
// It stores the assistant message and intermediate tool messages, then broadcasts the
// run finished event.
func (s *ConversationService) HandleConversationRunComplete(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.ConversationRunCompletePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal conversation run complete: %w", err)
	}

	slog.Info("conversation run complete received",
		"run_id", payload.RunID,
		"conversation_id", payload.ConversationID,
		"status", payload.Status,
		"steps", payload.StepCount,
		"cost", payload.CostUSD,
	)

	// Store intermediate tool messages (assistant messages with tool_calls + tool results).
	if len(payload.ToolMessages) > 0 {
		toolMsgs := make([]conversation.Message, 0, len(payload.ToolMessages))
		for _, tm := range payload.ToolMessages {
			msg := conversation.Message{
				ConversationID: payload.ConversationID,
				Role:           tm.Role,
				Content:        tm.Content,
				ToolCallID:     tm.ToolCallID,
				ToolName:       tm.Name,
			}
			// Serialize tool_calls for assistant messages.
			if len(tm.ToolCalls) > 0 {
				tcJSON, err := json.Marshal(tm.ToolCalls)
				if err == nil {
					msg.ToolCalls = tcJSON
				}
			}
			toolMsgs = append(toolMsgs, msg)
		}
		if err := s.db.CreateToolMessages(ctx, payload.ConversationID, toolMsgs); err != nil {
			slog.Error("failed to store tool messages", "conversation_id", payload.ConversationID, "error", err)
		}
	}

	// Store final assistant message.
	if payload.AssistantContent != "" || payload.Status == "completed" {
		assistantMsg := &conversation.Message{
			ConversationID: payload.ConversationID,
			Role:           "assistant",
			Content:        payload.AssistantContent,
			TokensIn:       payload.TokensIn,
			TokensOut:      payload.TokensOut,
			Model:          payload.Model,
		}
		if _, err := s.db.CreateMessage(ctx, assistantMsg); err != nil {
			slog.Error("failed to store assistant message", "conversation_id", payload.ConversationID, "error", err)
		}
	}

	// Determine WS status.
	wsStatus := "completed"
	if payload.Status != "completed" {
		wsStatus = "failed"
	}

	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
		RunID:  payload.RunID,
		Status: wsStatus,
	})

	return nil
}

// StartCompletionSubscriber subscribes to conversation.run.complete on NATS.
// Returns a cancel function to stop the subscription.
func (s *ConversationService) StartCompletionSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectConversationRunComplete, s.HandleConversationRunComplete)
}

// historyToPayload converts domain messages to protocol payload messages.
func (s *ConversationService) historyToPayload(messages []conversation.Message) []messagequeue.ConversationMessagePayload {
	result := make([]messagequeue.ConversationMessagePayload, 0, len(messages))
	for i := range messages {
		if messages[i].Role == "system" {
			continue
		}
		pm := messagequeue.ConversationMessagePayload{
			Role:       messages[i].Role,
			Content:    messages[i].Content,
			ToolCallID: messages[i].ToolCallID,
			Name:       messages[i].ToolName,
		}
		// Parse tool_calls from stored JSON.
		if len(messages[i].ToolCalls) > 0 {
			var tcs []messagequeue.ConversationToolCall
			if err := json.Unmarshal(messages[i].ToolCalls, &tcs); err == nil {
				pm.ToolCalls = tcs
			}
		}
		result = append(result, pm)
	}
	return result
}

// buildSystemPrompt assembles the system prompt for a conversation using the
// embedded template and project context. Failures in fetching optional context
// (agents, tasks, roadmap) are logged and skipped gracefully.
func (s *ConversationService) buildSystemPrompt(ctx context.Context, projectID string) string {
	data := conversationPromptData{}

	// Fetch project info (required for a meaningful prompt).
	proj, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		slog.Warn("conversation: failed to fetch project for system prompt", "project_id", projectID, "error", err)
		data.ProjectName = projectID
	} else {
		data.ProjectName = proj.Name
		data.ProjectDescription = proj.Description
		data.WorkspacePath = proj.WorkspacePath
		data.Provider = proj.Provider
		data.RepoURL = proj.RepoURL
	}

	// Fetch available agents (optional).
	agents, err := s.db.ListAgents(ctx, projectID)
	if err != nil {
		slog.Debug("conversation: failed to list agents for system prompt", "project_id", projectID, "error", err)
	} else {
		for i := range agents {
			label := agents[i].Name
			if agents[i].Backend != "" {
				label += " (" + agents[i].Backend + ")"
			}
			data.Agents = append(data.Agents, label)
		}
	}

	// Fetch available modes (optional).
	if s.modeSvc != nil {
		modes := s.modeSvc.List()
		for i := range modes {
			data.Modes = append(data.Modes, modes[i].Name)
		}
	}

	// Fetch recent tasks (optional, limit to last 10).
	tasks, err := s.db.ListTasks(ctx, projectID)
	if err != nil {
		slog.Debug("conversation: failed to list tasks for system prompt", "project_id", projectID, "error", err)
	} else {
		limit := min(10, len(tasks))
		// Take the last N tasks (most recent).
		start := len(tasks) - limit
		for i := range tasks[start:] {
			data.RecentTasks = append(data.RecentTasks, conversationTaskSummary{
				ID:     tasks[start+i].ID,
				Name:   tasks[start+i].Title,
				Status: string(tasks[start+i].Status),
			})
		}
	}

	// Fetch roadmap summary with milestones and features (optional).
	rm, err := s.db.GetRoadmapByProject(ctx, projectID)
	if err == nil && rm != nil {
		var sb strings.Builder
		sb.WriteString(rm.Title)
		if rm.Description != "" {
			sb.WriteString(" - ")
			sb.WriteString(rm.Description)
		}
		for i := range rm.Milestones {
			ms := &rm.Milestones[i]
			sb.WriteString("\n  ")
			sb.WriteString(ms.Title)
			sb.WriteString(" [")
			sb.WriteString(string(ms.Status))
			sb.WriteString("]")
			for j := range ms.Features {
				f := &ms.Features[j]
				sb.WriteString("\n    - ")
				sb.WriteString(f.Title)
				sb.WriteString(" (")
				sb.WriteString(string(f.Status))
				sb.WriteString(")")
			}
		}
		data.RoadmapSummary = sb.String()
	}

	// Detect tech stack summary if workspace path is available.
	if data.WorkspacePath != "" {
		stack, stackErr := detectStackSummary(data.WorkspacePath)
		if stackErr == nil && stack != "" {
			data.Stack = stack
		}
	}

	// Add built-in tool descriptions for agentic mode.
	data.BuiltinTools = []builtinToolSummary{
		{Name: "Read", Description: "Read file contents with optional line range"},
		{Name: "Write", Description: "Create or overwrite a file"},
		{Name: "Edit", Description: "Search-and-replace edit within a file"},
		{Name: "Bash", Description: "Execute a shell command"},
		{Name: "Search", Description: "Regex search across files"},
		{Name: "Glob", Description: "Find files by glob pattern"},
		{Name: "ListDir", Description: "List directory contents"},
	}

	var buf bytes.Buffer
	if err := conversationTmpl.Execute(&buf, data); err != nil {
		slog.Error("conversation: failed to render system prompt template", "error", err)
		return fmt.Sprintf("You are CodeForge, an AI coding orchestrator. Project: %s", data.ProjectName)
	}

	return buf.String()
}

// detectStackSummary runs a lightweight stack detection and returns a comma-separated
// summary of detected languages. Returns empty string on any failure.
func detectStackSummary(workspacePath string) (string, error) {
	result, err := project.ScanWorkspace(workspacePath)
	if err != nil {
		return "", err
	}
	if len(result.Languages) == 0 {
		return "", nil
	}
	names := make([]string, len(result.Languages))
	for i, lang := range result.Languages {
		names[i] = lang.Name
	}
	return strings.Join(names, ", "), nil
}
