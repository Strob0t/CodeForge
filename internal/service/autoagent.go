package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/autoagent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// AutoAgentService manages the lifecycle of auto-agent runs that iterate
// over pending roadmap features and process them via the conversation loop.
type AutoAgentService struct {
	toolUIDSource
	db            database.Store
	hub           broadcast.Broadcaster
	queue         messagequeue.Queue
	conversations *ConversationService

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // projectID -> cancel func

	// Workspace test runs in the worker (KI-81): waiters by request ID.
	testWaiter     *syncWaiter[messagequeue.WorkspaceTestResultPayload]
	testTimeout    time.Duration // bounds each test or lint command in the worker
	testWaitMargin time.Duration // queueing and delivery on top of the commands' timeouts

	verify AutoAgentVerification // how features are verified (KI-152)
	// commandPolicy decides whether a verification command may run (the
	// feature conversation's policy); nil: every command may.
	commandPolicy commandPolicy
}

// commandPolicy tells whether a conversation's agent may run a command on
// its own (ConversationService.CommandAllowed).
type commandPolicy interface {
	CommandAllowed(ctx context.Context, conversationID, command string) (allowed bool, reason string, err error)
}

// Defaults of the workspace test run (KI-81).
const (
	workspaceTestTimeout    = 5 * time.Minute
	workspaceTestWaitMargin = 2 * time.Minute
)

// NewAutoAgentService creates a new AutoAgentService.
func NewAutoAgentService(
	db database.Store,
	hub broadcast.Broadcaster,
	queue messagequeue.Queue,
	conversations *ConversationService,
) *AutoAgentService {
	s := &AutoAgentService{
		db:             db,
		hub:            hub,
		queue:          queue,
		conversations:  conversations,
		cancels:        make(map[string]context.CancelFunc),
		testWaiter:     newSyncWaiter[messagequeue.WorkspaceTestResultPayload]("workspace-test"),
		testTimeout:    workspaceTestTimeout,
		testWaitMargin: workspaceTestWaitMargin,
		verify:         AutoAgentVerification{FixAttempts: DefaultAutoAgentFixAttempts},
	}
	if conversations != nil {
		s.commandPolicy = conversations
	}
	return s
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

	// Launch background goroutine with cancellable context. It outlives the
	// request but stays in the request's tenant (store queries, conversation
	// runs and WebSocket events).
	loopCtx, cancel := context.WithCancel(detachTenant(ctx)) //nolint:gosec // G118: cancel stored in s.cancels[projectID], called from Stop()
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

		result, err := s.processFeature(ctx, projectID, feat, aa)
		if err != nil {
			slog.Error("auto-agent feature failed",
				"project_id", projectID,
				"feature_id", feat.ID,
				"error", err,
			)
			aa.FeaturesFailed++

			// Mark feature as failed in roadmap, with the reason.
			logBestEffort(ctx, s.finishFeature(ctx, feat.ID, roadmap.FeatureCancelled, "failed: "+err.Error()),
				"finishFeature", slog.String("feature_id", feat.ID))
		} else {
			aa.FeaturesComplete++
			logBestEffort(ctx, s.finishFeature(ctx, feat.ID, roadmap.FeatureDone, result),
				"finishFeature", slog.String("feature_id", feat.ID))
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
	logBestEffort(ctx, s.db.UpdateAutoAgentStatus(ctx, projectID, finalStatus, errMsg),
		"UpdateAutoAgentStatus", slog.String("project_id", projectID))
	s.broadcastStatus(ctx, aa)

	slog.Info("auto-agent completed",
		"project_id", projectID,
		"complete", aa.FeaturesComplete,
		"failed", aa.FeaturesFailed,
		"cost", aa.TotalCostUSD,
	)
}

// processFeature creates a conversation for a feature, runs it and
// verifies the result (KI-152). It returns the feature's result, or an
// error when the feature failed.
func (s *AutoAgentService) processFeature(
	ctx context.Context,
	projectID string,
	feat *roadmap.Feature,
	aa *autoagent.AutoAgent,
) (string, error) {
	proj, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	// The change check compares the workspace after the feature's runs
	// with its state now.
	before := takeWorkspaceSnapshot(ctx, proj.WorkspacePath)

	// Create a conversation for this feature.
	conv, err := s.conversations.Create(ctx, conversation.CreateRequest{
		ProjectID: projectID,
		Title:     fmt.Sprintf("Auto-agent: %s", feat.Title),
	})
	if err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
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

	// Run the prompt via the agentic loop (tool-use enabled) and wait for it.
	if err := s.runAndWait(ctx, conv.ID, prompt, aa); err != nil {
		return "", fmt.Errorf("feature run: %w", err)
	}

	return s.verifyFeature(ctx, &featureVerification{
		projectID:      projectID,
		conversationID: conv.ID,
		title:          feat.Title,
		testFile:       extractTestFile(feat.Description),
		before:         before,
	}, func(fixPrompt string) error {
		slog.Info("auto-agent verification failed, sending fix prompt",
			"project_id", projectID, "feature_id", feat.ID)
		return s.runAndWait(ctx, conv.ID, fixPrompt, aa)
	})
}

// runAndWait dispatches prompt as an agentic run of the conversation and
// waits for the run to end. The waiter is registered before the dispatch, so
// a run that ends at once is not missed (KI-76).
func (s *AutoAgentService) runAndWait(ctx context.Context, conversationID, prompt string, aa *autoagent.AutoAgent) error {
	waiter, err := s.conversations.ExpectCompletion(conversationID)
	if err != nil {
		return fmt.Errorf("expect completion: %w", err)
	}
	defer waiter.Close()

	// A feature turn implements: no planning tools (KI-153).
	req := &conversation.SendMessageRequest{Content: prompt, ImplementationTurn: true}
	if err := s.conversations.SendMessageAgentic(ctx, conversationID, req); err != nil {
		return fmt.Errorf("send agentic message: %w", err)
	}
	if err := s.waitForCompletion(ctx, conversationID, waiter, aa); err != nil {
		return fmt.Errorf("wait for completion: %w", err)
	}
	return nil
}

// autoAgentStopTimeout bounds stopping a run the auto-agent gave up on.
const autoAgentStopTimeout = 10 * time.Second

// waitForCompletion waits for the conversation run to finish via the
// ConversationService's in-process waiter (no duplicate NATS subscription).
// A run the auto-agent stops waiting for (feature timeout, auto-agent
// stopped) is stopped: it would go on changing the workspace next to the
// next feature's run, and its conversation would refuse messages (KI-76).
func (s *AutoAgentService) waitForCompletion(
	ctx context.Context,
	conversationID string,
	waiter *CompletionWaiter,
	aa *autoagent.AutoAgent,
) error {
	timeout := time.Duration(autoagent.FeatureTimeoutMinutes) * time.Minute
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := waiter.Wait(timeoutCtx)
	if err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), autoAgentStopTimeout)
		defer stopCancel()
		logBestEffort(stopCtx, s.conversations.StopConversation(stopCtx, conversationID), "StopConversation",
			slog.String("conversation_id", conversationID))
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("feature timed out after %d minutes", autoagent.FeatureTimeoutMinutes)
		}
		return err
	}

	aa.TotalCostUSD += result.CostUSD

	if result.Status != "completed" {
		return fmt.Errorf("conversation run %s: %s", result.Status, result.Error)
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

// maxFeatureResult bounds the result (in runes) the auto-agent stores on a
// feature.
const maxFeatureResult = 2000

// finishFeature sets a processed feature's status and result.
func (s *AutoAgentService) finishFeature(ctx context.Context, featureID string, status roadmap.FeatureStatus, result string) error {
	feat, err := s.db.GetFeature(ctx, featureID)
	if err != nil {
		return err
	}
	feat.Status = status
	if r := []rune(result); len(r) > maxFeatureResult {
		result = string(r[:maxFeatureResult]) + "..."
	}
	feat.Result = result
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

// maxPromptTestOutput bounds the test output a fix prompt hands the agent
// (S3-F review C7; the worker already sends at most the last 64 KiB).
const maxPromptTestOutput = 16 * 1024

// testOutputForPrompt returns the tail of the test output - the summary and
// the failures pytest prints last - behind a marker when it is cut.
func testOutputForPrompt(output string) string {
	if len(output) <= maxPromptTestOutput {
		return output
	}
	cut := len(output) - maxPromptTestOutput
	for cut < len(output) && !utf8.RuneStart(output[cut]) {
		cut++
	}
	return fmt.Sprintf("[... %d bytes of earlier test output truncated ...]\n%s", cut, output[cut:])
}

// testResult holds parsed pytest output.
type testResult struct {
	Passed    int
	Failed    int
	Total     int
	AllPassed bool
	Output    string
}

// runWorkspaceTest runs pytest for a test file of the project workspace and
// parses the output for pass/fail counts. The test runs in the worker
// (conversation.test.request, KI-81): workspace code - the test, conftest.py,
// pytest plugins and configuration the agent wrote - must never run in the
// Go Core, which holds the platform's secrets. The worker runs it with its
// tool environment, in its own process group and bounded by the timeout.
func (s *AutoAgentService) runWorkspaceTest(ctx context.Context, projectID, conversationID, testFile string) (testResult, error) {
	proj, err := s.db.GetProject(ctx, projectID)
	if err != nil {
		return testResult{}, fmt.Errorf("get project for test: %w", err)
	}
	if proj.WorkspacePath == "" {
		return testResult{}, fmt.Errorf("project has no workspace path")
	}

	// Validate testFile resolves within workspace (defense-in-depth; the
	// worker checks again where it runs the test).
	absTest := filepath.Join(proj.WorkspacePath, testFile)
	cleanTest := filepath.Clean(absTest)
	if !strings.HasPrefix(cleanTest, filepath.Clean(proj.WorkspacePath)+string(filepath.Separator)) {
		return testResult{}, fmt.Errorf("%w: %s escapes the workspace", errTestFileMissing, testFile)
	}
	// Resolved inside the workspace (KI-95): a symlink out of it is refused.
	if info, err := workspacefs.StatAt(proj.WorkspacePath, testFile); err != nil || info.IsDir() {
		return testResult{}, fmt.Errorf("%w: %s not found or a directory", errTestFileMissing, testFile)
	}

	tenantID := tenantctx.FromContext(ctx)
	toolUID, err := s.toolUIDs.PayloadToolUID(ctx, tenantID)
	if err != nil {
		return testResult{}, fmt.Errorf("tool uid: %w", err)
	}
	res, err := s.requestWorkspaceTest(ctx, &messagequeue.WorkspaceTestRequestPayload{
		RequestID:      uuid.New().String(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		ConversationID: conversationID,
		WorkspacePath:  proj.WorkspacePath,
		TestFile:       testFile,
		TimeoutSeconds: int(s.testTimeout.Seconds() + 0.999),
		ToolUID:        toolUID,
	})
	if err != nil {
		return testResult{}, err
	}
	if res.Passed == nil {
		return testResult{Output: res.Output}, fmt.Errorf("workspace test could not run: %s", res.Error)
	}

	output := res.Output
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
	result.AllPassed = result.Failed == 0 && result.Passed > 0 && *res.Passed

	return result, nil
}

// requestWorkspaceTest publishes the request and waits for its result.
func (s *AutoAgentService) requestWorkspaceTest(ctx context.Context, req *messagequeue.WorkspaceTestRequestPayload) (*messagequeue.WorkspaceTestResultPayload, error) {
	ch := s.testWaiter.register(req.RequestID, messagequeue.RelayOf(s.queue))
	defer s.testWaiter.unregister(req.RequestID)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal workspace test request: %w", err)
	}
	if err := s.queue.Publish(ctx, messagequeue.SubjectConversationTestRequest, data); err != nil {
		return nil, fmt.Errorf("publish workspace test request: %w", err)
	}

	// The worker runs the commands one after the other.
	commands := 0
	for _, cmd := range []string{req.TestFile, req.TestCommand, req.LintCommand} {
		if cmd != "" {
			commands++
		}
	}
	wait := time.Duration(max(commands, 1))*s.testTimeout + s.testWaitMargin
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res, nil
	case <-timer.C:
		return nil, fmt.Errorf("workspace checks: no result from the worker within %s", wait)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HandleWorkspaceTestResult hands a worker's test result to the auto-agent
// run waiting for it, on this replica or through the relay on another one
// (KI-86). Results are delivered at least once: a duplicate or a result
// nobody waits for (the run ended) is dropped.
func (s *AutoAgentService) HandleWorkspaceTestResult(ctx context.Context, res *messagequeue.WorkspaceTestResultPayload) error {
	s.testWaiter.deliver(ctx, messagequeue.RelayOf(s.queue), res.RequestID, res)
	return nil
}

// StartTestResultSubscriber subscribes to conversation.test.result. The
// relay sends the worker's message as it came.
func (s *AutoAgentService) StartTestResultSubscriber(ctx context.Context) (func(), error) {
	return s.queue.Subscribe(ctx, messagequeue.SubjectConversationTestResult, func(ctx context.Context, _ string, data []byte) error {
		var res messagequeue.WorkspaceTestResultPayload
		if err := json.Unmarshal(data, &res); err != nil {
			return fmt.Errorf("unmarshal workspace test result: %w", err)
		}
		s.testWaiter.deliverMessage(ctx, messagequeue.RelayOf(s.queue), res.RequestID, &res, data)
		return nil
	})
}

// broadcastStatus sends the current auto-agent state to connected clients.
func (s *AutoAgentService) broadcastStatus(ctx context.Context, aa *autoagent.AutoAgent) {
	s.hub.BroadcastEvent(ctx, "autoagent.status", aa)
}
