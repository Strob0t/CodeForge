package service

// Consumer-defined interfaces for ConversationService dependencies.
// Each interface captures only the methods that ConversationService actually calls,
// following the Go idiom "accept interfaces, return structs".
//
// Note: promptAssembler, msgSvc, and promptSvc are NOT interfaced here because
// ConversationService accesses their internal struct fields directly.

import (
	"context"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// convModelResolver is the subset of ModelRegistry used by ConversationService.
type convModelResolver interface {
	BestModel() string
}

// convModeProvider is the subset of ModeService used by ConversationService.
type convModeProvider interface {
	Get(id string) (*mode.Mode, error)
	List() []mode.Mode
}

// convPolicyEvaluator is the subset of PolicyService used by ConversationService.
type convPolicyEvaluator interface {
	GetProfile(name string) (policy.PolicyProfile, bool)
	ResolveProfile(runProfile, projectProfile string) string
}

// convMCPResolver is the subset of MCPService used by ConversationService.
type convMCPResolver interface {
	ResolveForRun(projectID, modeID string) []mcp.ServerDef
}

// convMicroagentMatcher is the subset of MicroagentService used by ConversationService.
type convMicroagentMatcher interface {
	Match(ctx context.Context, projectID, text string) ([]microagent.Microagent, error)
}

// convGoalProvider is the subset of GoalDiscoveryService used by ConversationService.
type convGoalProvider interface {
	ListEnabled(ctx context.Context, projectID string) ([]goal.ProjectGoal, error)
}

// convSessionProvider is the subset of SessionService used by ConversationService.
type convSessionProvider interface {
	EnsureConversationSession(ctx context.Context, projectID, conversationID string) (*run.Session, error)
}

// convContextOptimizer is the subset of ContextOptimizerService used by ConversationService.
type convContextOptimizer interface {
	BuildConversationContext(ctx context.Context, projectID, userMessage, teamID string, opts ConversationContextOpts) ([]cfcontext.ContextEntry, error)
}

// convLLMKeyResolver is the subset of LLMKeyService used by ConversationService.
type convLLMKeyResolver interface {
	ResolveKeyForProvider(ctx context.Context, userID, provider string) (string, error)
}

// convScoreRecorder is the subset of PromptScoreCollector used by ConversationService.
type convScoreRecorder interface {
	RecordSuccessScore(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, succeeded bool) error
	RecordCostScore(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, qualityPerDollar float64) error
}
