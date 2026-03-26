package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// handleToolCallRequest unmarshals a tool call permission request from a worker
// and delegates to HandleToolCallRequest.
func (s *RuntimeService) handleToolCallRequest(ctx context.Context, data []byte) error {
	var req messagequeue.ToolCallRequestPayload
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("unmarshal tool call request: %w", err)
	}
	return s.HandleToolCallRequest(ctx, &req)
}

// handleToolCallResult unmarshals a tool call result from a worker
// and delegates to HandleToolCallResult.
func (s *RuntimeService) handleToolCallResult(ctx context.Context, data []byte) error {
	var result messagequeue.ToolCallResultPayload
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("unmarshal tool call result: %w", err)
	}
	return s.HandleToolCallResult(ctx, &result)
}

// handleRunComplete unmarshals a run completion payload from a worker
// and delegates to HandleRunComplete.
func (s *RuntimeService) handleRunComplete(ctx context.Context, data []byte) error {
	var payload messagequeue.RunCompletePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal run complete: %w", err)
	}
	return s.HandleRunComplete(ctx, &payload)
}

// handleQualityGateResult unmarshals a quality gate result from a worker
// and delegates to HandleQualityGateResult.
func (s *RuntimeService) handleQualityGateResult(ctx context.Context, data []byte) error {
	var result messagequeue.QualityGateResultPayload
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("unmarshal quality gate result: %w", err)
	}
	return s.HandleQualityGateResult(ctx, &result)
}

// handleHeartbeat records the latest heartbeat timestamp for a run.
func (s *RuntimeService) handleHeartbeat(_ context.Context, data []byte) error {
	var hb messagequeue.RunHeartbeatPayload
	if err := json.Unmarshal(data, &hb); err != nil {
		return fmt.Errorf("unmarshal heartbeat: %w", err)
	}
	s.heartbeats.Store(hb.RunID, time.Now())
	return nil
}

// handleRunOutput broadcasts streaming output from a worker to the frontend
// via both TaskOutput and AG-UI text_message events.
func (s *RuntimeService) handleRunOutput(ctx context.Context, data []byte) error {
	var output messagequeue.RunOutputPayload
	if err := json.Unmarshal(data, &output); err != nil {
		return fmt.Errorf("unmarshal run output: %w", err)
	}
	s.hub.BroadcastEvent(ctx, event.EventTaskOutput, event.TaskOutputEvent{
		TaskID: output.TaskID,
		Line:   output.Line,
		Stream: output.Stream,
	})
	// Also emit AG-UI text_message for agentic conversation streaming.
	if output.Line != "" && output.Stream != "stderr" {
		s.hub.BroadcastEvent(ctx, event.AGUITextMessage, event.AGUITextMessageEvent{
			RunID:   output.TaskID,
			Role:    "assistant",
			Content: output.Line,
		})
	}
	return nil
}

// trajectoryPayload is the common envelope for all trajectory events from Python workers.
type trajectoryPayload struct {
	EventType string  `json:"event_type"`
	RunID     string  `json:"run_id"`
	ProjectID string  `json:"project_id"`
	ToolName  string  `json:"tool_name,omitempty"`
	Model     string  `json:"model,omitempty"`
	Input     string  `json:"input,omitempty"`
	Output    string  `json:"output,omitempty"`
	Success   *bool   `json:"success,omitempty"`
	Step      int     `json:"step,omitempty"`
	TokensIn  int64   `json:"tokens_in,omitempty"`
	TokensOut int64   `json:"tokens_out,omitempty"`
	CostUSD   float64 `json:"cost_usd,omitempty"`
}

// handleTrajectoryEvent persists a trajectory event from a Python worker and
// broadcasts it to the frontend. Special event types (action_suggestion,
// goal_proposed) are handled by dedicated sub-methods.
func (s *RuntimeService) handleTrajectoryEvent(ctx context.Context, data []byte) error {
	var payload trajectoryPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal trajectory event: %w", err)
	}

	// Use RunID as fallback for AgentID/TaskID when not available
	// (conversation runs don't have separate agent/task IDs).
	agentID := payload.RunID
	taskID := payload.RunID

	ev := &event.AgentEvent{
		AgentID:   agentID,
		TaskID:    taskID,
		RunID:     payload.RunID,
		ProjectID: payload.ProjectID,
		Type:      event.Type(payload.EventType),
		Payload:   data,
		ToolName:  payload.ToolName,
		Model:     payload.Model,
		TokensIn:  payload.TokensIn,
		TokensOut: payload.TokensOut,
		CostUSD:   payload.CostUSD,
	}

	if err := s.events.Append(ctx, ev); err != nil {
		slog.Error("failed to persist trajectory event", "run_id", payload.RunID, "type", payload.EventType, "error", err)
		return nil // Log and continue, don't fail the subscription
	}

	s.hub.BroadcastEvent(ctx, event.EventTrajectoryEvent, event.TrajectoryEventPayload{
		RunID:          payload.RunID,
		ProjectID:      payload.ProjectID,
		EventType:      payload.EventType,
		SequenceNumber: ev.SequenceNumber,
		ToolName:       payload.ToolName,
		Model:          payload.Model,
		Input:          payload.Input,
		Output:         payload.Output,
		Success:        payload.Success,
		Step:           payload.Step,
		CostUSD:        payload.CostUSD,
		TokensIn:       payload.TokensIn,
		TokensOut:      payload.TokensOut,
	})

	switch payload.EventType {
	case "agent.action_suggestion":
		s.handleTrajectoryActionSuggestion(ctx, payload.RunID, data)
	case "agent.goal_proposed":
		s.handleTrajectoryGoalProposed(ctx, payload.RunID, payload.ProjectID, data)
	}

	return nil
}

// handleTrajectoryActionSuggestion broadcasts an AG-UI action suggestion event.
func (s *RuntimeService) handleTrajectoryActionSuggestion(ctx context.Context, runID string, data []byte) {
	var suggestion struct {
		Label  string `json:"label"`
		Action string `json:"action"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(data, &suggestion); err == nil {
		s.hub.BroadcastEvent(ctx, event.AGUIActionSuggestion, event.AGUIActionSuggestionEvent{
			RunID:  runID,
			Label:  suggestion.Label,
			Action: suggestion.Action,
			Value:  suggestion.Value,
		})
	}
}

// handleTrajectoryGoalProposed broadcasts an AG-UI goal proposal event and
// auto-persists "create" proposals to the database.
func (s *RuntimeService) handleTrajectoryGoalProposed(ctx context.Context, runID, projectID string, data []byte) {
	var proposal struct {
		Data struct {
			ProposalID string `json:"proposal_id"`
			Action     string `json:"action"`
			Kind       string `json:"kind"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			Priority   int    `json:"priority"`
			GoalID     string `json:"goal_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &proposal); err != nil {
		return
	}

	s.hub.BroadcastEvent(ctx, event.AGUIGoalProposal, event.AGUIGoalProposalEvent{
		RunID:      runID,
		ProposalID: proposal.Data.ProposalID,
		Action:     proposal.Data.Action,
		Kind:       proposal.Data.Kind,
		Title:      proposal.Data.Title,
		Content:    proposal.Data.Content,
		Priority:   proposal.Data.Priority,
		GoalID:     proposal.Data.GoalID,
	})

	// Auto-persist goal proposals with action "create" to the database.
	if s.goalSvc != nil && proposal.Data.Action == "create" {
		if createErr := s.PersistGoalProposal(ctx, projectID, proposal.Data.Kind, proposal.Data.Title, proposal.Data.Content, proposal.Data.Priority); createErr != nil {
			slog.Warn("auto-persist goal failed", "project_id", projectID, "title", proposal.Data.Title, "error", createErr)
		} else {
			slog.Info("goal auto-persisted", "project_id", projectID, "title", proposal.Data.Title, "kind", proposal.Data.Kind)
		}
	}
}
