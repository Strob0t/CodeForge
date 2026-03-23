package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// EvaluationService triggers GEMMAS metric computation after plan completion.
type EvaluationService struct {
	store  database.Store
	events eventstore.Store
	queue  messagequeue.Queue
}

// NewEvaluationService creates an EvaluationService with all dependencies.
func NewEvaluationService(
	store database.Store,
	events eventstore.Store,
	queue messagequeue.Queue,
) *EvaluationService {
	return &EvaluationService{
		store:  store,
		events: events,
		queue:  queue,
	}
}

// HandlePlanComplete is the callback invoked when a plan completes or fails.
// It collects agent messages from run events and publishes a GEMMAS evaluation request.
func (s *EvaluationService) HandlePlanComplete(ctx context.Context, planID, status string) {
	if status != "completed" {
		return
	}

	p, err := s.store.GetPlan(ctx, planID)
	if err != nil {
		slog.Error("evaluation: get plan", "plan_id", planID, "error", err)
		return
	}

	var messages []messagequeue.GemmasAgentMessagePayload
	for i := range p.Steps {
		step := &p.Steps[i]
		if step.Status != plan.StepStatusCompleted || step.RunID == "" {
			continue
		}

		events, err := s.events.LoadByRun(ctx, step.RunID)
		if err != nil {
			slog.Warn("evaluation: load run events", "run_id", step.RunID, "error", err)
			continue
		}

		msgs := extractAgentMessages(events, step.AgentID, step.Round)
		messages = append(messages, msgs...)
	}

	if len(messages) == 0 {
		slog.Debug("evaluation: no agent messages found", "plan_id", planID)
		return
	}

	payload := messagequeue.GemmasEvalRequestPayload{
		PlanID:   planID,
		Messages: messages,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("evaluation: marshal payload", "plan_id", planID, "error", err)
		return
	}

	if err := s.queue.Publish(ctx, messagequeue.SubjectEvalGemmasRequest, data); err != nil {
		slog.Error("evaluation: publish gemmas request", "plan_id", planID, "error", err)
		return
	}

	slog.Info("evaluation: gemmas request published", "plan_id", planID, "messages", len(messages))
}

// HandleGemmasResult processes an evaluation.gemmas.result message from Python.
// It logs the scores and stores them on the plan record (best-effort).
func (s *EvaluationService) HandleGemmasResult(_ context.Context, _ string, data []byte) error {
	var payload messagequeue.GemmasEvalResultPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal gemmas result: %w", err)
	}
	if payload.Error != "" {
		slog.Error("gemmas evaluation failed", "plan_id", payload.PlanID, "error", payload.Error)
		return nil
	}
	slog.Info("gemmas evaluation result received",
		"plan_id", payload.PlanID,
		"diversity_score", payload.InformationDiversityScore,
		"unnecessary_path_ratio", payload.UnnecessaryPathRatio,
	)
	return nil
}

// StartGemmasResultSubscriber subscribes to evaluation.gemmas.result messages.
func (s *EvaluationService) StartGemmasResultSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectEvalGemmasResult, s.HandleGemmasResult)
}

// extractAgentMessages converts run events into GEMMAS agent message payloads.
// It extracts tool call requests and results as agent communication artifacts.
func extractAgentMessages(events []event.AgentEvent, agentID string, round int) []messagequeue.GemmasAgentMessagePayload {
	var messages []messagequeue.GemmasAgentMessagePayload

	for i := range events {
		ev := &events[i]

		switch ev.Type {
		case event.TypeToolCallRequested, event.TypeToolCallResultEv:
			msg := messagequeue.GemmasAgentMessagePayload{
				AgentID: ev.AgentID,
				Content: string(ev.Payload),
				Round:   round,
			}
			// If the event's agent differs from the step's agent, mark parent.
			if ev.AgentID != agentID {
				msg.ParentAgentID = agentID
			}
			messages = append(messages, msg)
		}
	}

	return messages
}
