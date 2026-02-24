package service

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
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
}

// conversationTaskSummary is a minimal task view for the system prompt.
type conversationTaskSummary struct {
	ID     string
	Name   string
	Status string
}

// ConversationService manages conversations and LLM interactions.
type ConversationService struct {
	db      database.Store
	llm     *litellm.Client
	hub     broadcast.Broadcaster
	model   string // default model name for LiteLLM
	modeSvc *ModeService
}

// NewConversationService creates a new ConversationService.
func NewConversationService(db database.Store, llm *litellm.Client, hub broadcast.Broadcaster, defaultModel string, modeSvc *ModeService) *ConversationService {
	if defaultModel == "" {
		defaultModel = "default"
	}
	return &ConversationService{db: db, llm: llm, hub: hub, model: defaultModel, modeSvc: modeSvc}
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
// and broadcasts it via WebSocket AG-UI events.
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
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		chatMessages = append(chatMessages, litellm.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	// Broadcast run started event
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     conversationID,
		ThreadID:  conversationID,
		AgentName: "assistant",
	})

	// Call LiteLLM (non-streaming for now; streaming will be added in Phase 8)
	llmResp, err := s.llm.ChatCompletion(ctx, litellm.ChatCompletionRequest{
		Model:    s.model,
		Messages: chatMessages,
	})
	if err != nil {
		slog.Error("llm chat completion failed", "conversation_id", conversationID, "error", err)
		// Broadcast run finished with error
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  conversationID,
			Status: "failed",
		})
		return nil, fmt.Errorf("llm completion: %w", err)
	}

	// Store assistant message
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

	// Broadcast the response via AG-UI text message event
	s.hub.BroadcastEvent(ctx, ws.AGUITextMessage, ws.AGUITextMessageEvent{
		RunID:   conversationID,
		Role:    "assistant",
		Content: llmResp.Content,
	})

	// Broadcast run finished
	s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
		RunID:  conversationID,
		Status: "completed",
	})

	return assistantMsg, nil
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
		limit := 10
		if len(tasks) < limit {
			limit = len(tasks)
		}
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

	// Fetch roadmap summary (optional).
	rm, err := s.db.GetRoadmapByProject(ctx, projectID)
	if err == nil && rm != nil {
		var parts []string
		parts = append(parts, rm.Title)
		if rm.Description != "" {
			parts = append(parts, rm.Description)
		}
		data.RoadmapSummary = strings.Join(parts, " - ")
	}

	// Detect tech stack summary if workspace path is available.
	if data.WorkspacePath != "" {
		stack, stackErr := detectStackSummary(data.WorkspacePath)
		if stackErr == nil && stack != "" {
			data.Stack = stack
		}
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
