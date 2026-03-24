package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// AutoAgentService manages the lifecycle of auto-agent runs that iterate
// over pending roadmap features and process them via the conversation loop.
type AutoAgentService struct {
	db            database.Store
	hub           broadcast.Broadcaster
	queue         messagequeue.Queue
	conversations *ConversationService

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // projectID -> cancel func
}

// NewAutoAgentService creates a new AutoAgentService.
func NewAutoAgentService(
	db database.Store,
	hub broadcast.Broadcaster,
	queue messagequeue.Queue,
	conversations *ConversationService,
) *AutoAgentService {
	return &AutoAgentService{
		db:            db,
		hub:           hub,
		queue:         queue,
		conversations: conversations,
		cancels:       make(map[string]context.CancelFunc),
	}
}

// Start launches the auto-agent loop for a project in a background goroutine.
func (s *AutoAgentService) Start(ctx context.Context, projectID string) (*autoagent.AutoAgent, error) {
	// Check that the project exists.
	proj, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if proj.WorkspacePath == "" {
		return nil, fmt.Errorf("project has no workspace: %w", domain.ErrValidation)
	}

	s.mu.Lock()
	if _, running := s.cancels[projectID]; running {
		s.mu.Unlock()
		return nil, fmt.Errorf("auto-agent already running for project: %w", domain.ErrConflict)
	}
	// Reserve the slot while holding the lock to prevent TOCTOU races.
	// A nil cancel func signals "starting" — Stop() will treat it as not-yet-running.
	s.cancels[projectID] = nil
	s.mu.Unlock()

	// If setup fails below, clean up the reservation.
	setupOK := false
	defer func() {
		if !setupOK {
			s.mu.Lock()
			if s.cancels[projectID] == nil {
				delete(s.cancels, projectID)
			}
			s.mu.Unlock()
		}
	}()

	// Fetch pending features from the roadmap.
	features, err := s.pendingFeatures(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: create a roadmap with features before starting auto-agent", domain.ErrValidation)
		}
		return nil, fmt.Errorf("fetch pending features: %w", err)
	}
	if len(features) == 0 {
		return nil, fmt.Errorf("%w: all roadmap features are already completed — add new features to continue", domain.ErrValidation)
	}

	aa := &autoagent.AutoAgent{
		ProjectID:     projectID,
		Status:        autoagent.StatusRunning,
		FeaturesTotal: len(features),
		StartedAt:     time.Now(),
	}
	if err := s.db.UpsertAutoAgent(ctx, aa); err != nil {
		return nil, fmt.Errorf("upsert auto-agent: %w", err)
	}

	s.broadcastStatus(ctx, aa)

	// Launch background goroutine with cancellable context.
	loopCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel stored in s.cancels[projectID], called from Stop()
	s.mu.Lock()
	s.cancels[projectID] = cancel
	s.mu.Unlock()
	setupOK = true

	go s.runLoop(loopCtx, projectID, features)

	return aa, nil
}

// Stop cancels the running auto-agent for a project.
func (s *AutoAgentService) Stop(ctx context.Context, projectID string) error {
	s.mu.Lock()
	cancel, ok := s.cancels[projectID]
	s.mu.Unlock()

	if !ok {
		// Try to update DB status anyway (might be stale from a restart).
		_ = s.db.UpdateAutoAgentStatus(ctx, projectID, autoagent.StatusIdle, "stopped by user")
		return nil
	}

	// Mark as stopping, then cancel.
	_ = s.db.UpdateAutoAgentStatus(ctx, projectID, autoagent.StatusStopping, "")
	cancel()

	return nil
}

// Status returns the current auto-agent state for a project.
func (s *AutoAgentService) Status(ctx context.Context, projectID string) (*autoagent.AutoAgent, error) {
	aa, err := s.db.GetAutoAgent(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// No auto-agent run yet — return idle state.
			return &autoagent.AutoAgent{
				ProjectID: projectID,
				Status:    autoagent.StatusIdle,
			}, nil
		}
		return nil, err
	}
	return aa, nil
}

// runLoop is the background goroutine that processes features one by one.
func (s *AutoAgentService) runLoop(ctx context.Context, projectID string, features []roadmap.Feature) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, projectID)
		s.mu.Unlock()
	}()

	aa := &autoagent.AutoAgent{
		ProjectID:     projectID,
		Status:        autoagent.StatusRunning,
		FeaturesTotal: len(features),
		StartedAt:     time.Now(),
	}

	for i := range features {
		feat := &features[i]
		if ctx.Err() != nil {
			slog.Info("auto-agent stopped", "project_id", projectID)
			_ = s.db.UpdateAutoAgentStatus(ctx, projectID, autoagent.StatusIdle, "stopped")
			aa.Status = autoagent.StatusIdle
			s.broadcastStatus(ctx, aa)
			return
		}

		aa.CurrentFeatureID = feat.ID
		_ = s.db.UpdateAutoAgentProgress(ctx, aa)
		s.broadcastStatus(ctx, aa)

		err := s.processFeature(ctx, projectID, feat, aa)
		if err != nil {
			slog.Error("auto-agent feature failed",
				"project_id", projectID,
				"feature_id", feat.ID,
				"error", err,
			)
			aa.FeaturesFailed++

			// Mark feature as failed in roadmap.
			_ = s.updateFeatureStatus(ctx, feat.ID, roadmap.FeatureCancelled)
		} else {
			aa.FeaturesComplete++
			_ = s.updateFeatureStatus(ctx, feat.ID, roadmap.FeatureDone)
		}

		_ = s.db.UpdateAutoAgentProgress(ctx, aa)
		s.broadcastStatus(ctx, aa)
	}

	// All features processed.
	finalStatus := autoagent.StatusIdle
	errMsg := ""
	if aa.FeaturesFailed > 0 && aa.FeaturesComplete == 0 {
		finalStatus = autoagent.StatusFailed
		errMsg = fmt.Sprintf("all %d features failed", aa.FeaturesFailed)
	}

	aa.Status = finalStatus
	aa.Error = errMsg
	aa.CurrentFeatureID = ""
	aa.ConversationID = ""
	_ = s.db.UpdateAutoAgentStatus(ctx, projectID, finalStatus, errMsg)
	s.broadcastStatus(ctx, aa)

	slog.Info("auto-agent completed",
		"project_id", projectID,
		"complete", aa.FeaturesComplete,
		"failed", aa.FeaturesFailed,
		"cost", aa.TotalCostUSD,
	)
}

// processFeature creates a conversation for a feature and waits for completion.
func (s *AutoAgentService) processFeature(
	ctx context.Context,
	projectID string,
	feat *roadmap.Feature,
	aa *autoagent.AutoAgent,
) error {
	// Create a conversation for this feature.
	conv, err := s.conversations.Create(ctx, conversation.CreateRequest{
		ProjectID: projectID,
		Title:     fmt.Sprintf("Auto-agent: %s", feat.Title),
	})
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}

	aa.ConversationID = conv.ID
	_ = s.db.UpdateAutoAgentProgress(ctx, aa)

	// Mark feature as in-progress.
	_ = s.updateFeatureStatus(ctx, feat.ID, roadmap.FeatureInProgress)

	// Build the prompt for the feature.
	prompt := fmt.Sprintf(
		"Implement the following feature:\n\nTitle: %s\nDescription: %s\n\n"+
			"Please implement this feature in the codebase. Read relevant files first, "+
			"then make the necessary changes. Run tests if available.",
		feat.Title,
		feat.Description,
	)

	// Send the message via the agentic loop (tool-use enabled).
	err = s.conversations.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{
		Content: prompt,
	})
	if err != nil {
		return fmt.Errorf("send agentic message: %w", err)
	}

	// Wait for the conversation run to complete via NATS.
	err = s.waitForCompletion(ctx, conv.ID, aa)
	if err != nil {
		return fmt.Errorf("wait for completion: %w", err)
	}

	// Post-completion verification: run associated tests and send a fix prompt if they fail.
	testFile := extractTestFile(feat.Description)
	if testFile != "" {
		result, testErr := s.runWorkspaceTest(ctx, projectID, testFile)
		if testErr != nil || !result.AllPassed {
			passed := result.Passed
			total := result.Total
			output := result.Output
			if testErr != nil && total == 0 {
				output = testErr.Error()
			}

			slog.Info("auto-agent post-verification failed, sending fix prompt",
				"project_id", projectID,
				"feature_id", feat.ID,
				"test_file", testFile,
				"passed", passed,
				"total", total,
			)

			fixPrompt := fmt.Sprintf(
				"The tests are failing. %d/%d tests passed.\n\nTest output:\n```\n%s\n```\n\nPlease fix the implementation to make all tests pass.",
				passed, total, strings.TrimSpace(output),
			)
			err = s.conversations.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{
				Content: fixPrompt,
			})
			if err != nil {
				return fmt.Errorf("send fix prompt: %w", err)
			}

			err = s.waitForCompletion(ctx, conv.ID, aa)
			if err != nil {
				return fmt.Errorf("wait for fix completion: %w", err)
			}
		}
	}

	return nil
}

// waitForCompletion waits for the conversation run to finish via the
// ConversationService's in-process waiter (no duplicate NATS subscription).
func (s *AutoAgentService) waitForCompletion(
	ctx context.Context,
	conversationID string,
	aa *autoagent.AutoAgent,
) error {
	timeout := time.Duration(autoagent.FeatureTimeoutMinutes) * time.Minute
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := s.conversations.WaitForCompletion(timeoutCtx, conversationID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("feature timed out after %d minutes", autoagent.FeatureTimeoutMinutes)
		}
		return err
	}

	aa.TotalCostUSD += result.CostUSD

	if result.Status == "failed" {
		return fmt.Errorf("conversation run failed: %s", result.Error)
	}
	return nil
}

// pendingFeatures returns all features with backlog, planned, or in_progress status.
// In-progress features are included so that interrupted runs are recovered on restart.
func (s *AutoAgentService) pendingFeatures(ctx context.Context, projectID string) ([]roadmap.Feature, error) {
	rm, err := s.db.GetRoadmapByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	allFeatures, err := s.db.ListFeaturesByRoadmap(ctx, rm.ID)
	if err != nil {
		return nil, err
	}

	var pending []roadmap.Feature
	for i := range allFeatures {
		switch allFeatures[i].Status {
		case roadmap.FeatureBacklog, roadmap.FeaturePlanned, roadmap.FeatureInProgress:
			pending = append(pending, allFeatures[i])
		}
	}

	var recovered int
	for i := range pending {
		if pending[i].Status == roadmap.FeatureInProgress {
			recovered++
		}
	}
	if recovered > 0 {
		slog.Info("auto-agent recovering interrupted features",
			"project_id", rm.ProjectID,
			"recovered_count", recovered,
		)
	}

	return pending, nil
}

// updateFeatureStatus updates a feature's status in the database.
func (s *AutoAgentService) updateFeatureStatus(ctx context.Context, featureID string, status roadmap.FeatureStatus) error {
	feat, err := s.db.GetFeature(ctx, featureID)
	if err != nil {
		return err
	}
	feat.Status = status
	return s.db.UpdateFeature(ctx, feat)
}

// extractTestFile extracts the test filename from a feature description.
// Looks for patterns like "Tests: test_lru_cache.py" or "Tests: test_lru_cache.py (25 tests)".
func extractTestFile(description string) string {
	re := regexp.MustCompile(`Tests:\s+(test_\w+\.py)`)
	matches := re.FindStringSubmatch(description)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// testResult holds parsed pytest output.
type testResult struct {
	Passed    int
	Failed    int
	Total     int
	AllPassed bool
	Output    string
}

// runWorkspaceTest runs pytest for a test file inside the project workspace
// and parses the output for pass/fail counts.
func (s *AutoAgentService) runWorkspaceTest(ctx context.Context, projectID, testFile string) (testResult, error) {
	proj, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		return testResult{}, fmt.Errorf("get project for test: %w", err)
	}
	if proj.WorkspacePath == "" {
		return testResult{}, fmt.Errorf("project has no workspace path")
	}

	//nolint:gosec // testFile is extracted via regex from trusted feature descriptions.
	cmd := exec.CommandContext(ctx, "python", "-m", "pytest", testFile, "-v", "--tb=short")
	cmd.Dir = proj.WorkspacePath

	out, runErr := cmd.CombinedOutput()
	output := string(out)

	result := testResult{Output: output}

	// Parse "X passed" from pytest summary line.
	if m := regexp.MustCompile(`(\d+)\s+passed`).FindStringSubmatch(output); len(m) >= 2 {
		result.Passed, _ = strconv.Atoi(m[1])
	}
	// Parse "X failed" from pytest summary line.
	if m := regexp.MustCompile(`(\d+)\s+failed`).FindStringSubmatch(output); len(m) >= 2 {
		result.Failed, _ = strconv.Atoi(m[1])
	}

	result.Total = result.Passed + result.Failed
	result.AllPassed = result.Failed == 0 && result.Passed > 0 && runErr == nil

	return result, nil
}

// broadcastStatus sends the current auto-agent state to connected clients.
func (s *AutoAgentService) broadcastStatus(ctx context.Context, aa *autoagent.AutoAgent) {
	s.hub.BroadcastEvent(ctx, "autoagent.status", aa)
}
