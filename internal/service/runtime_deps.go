package service

// Consumer-defined interfaces for RuntimeService dependencies.
// Each interface captures only the methods that RuntimeService actually calls,
// following the Go idiom "accept interfaces, return structs".

import (
	"context"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

// runtimePolicyEvaluator is the subset of PolicyService used by RuntimeService.
type runtimePolicyEvaluator interface {
	DefaultProfile() string
	GetProfile(name string) (policy.PolicyProfile, bool)
	EvaluateWithReason(ctx context.Context, profileName string, call policy.ToolCall) (*policy.EvaluationResult, error)
}

// runtimeModeProvider is the subset of ModeService used by RuntimeService.
type runtimeModeProvider interface {
	Get(id string) (*mode.Mode, error)
}

// runtimeDeliverer is the subset of DeliverService used by RuntimeService.
type runtimeDeliverer interface {
	Deliver(ctx context.Context, r *run.Run, taskTitle string) (*DeliveryResult, error)
}

// runtimeContextOptimizer is the subset of ContextOptimizerService used by RuntimeService.
type runtimeContextOptimizer interface {
	BuildContextPack(ctx context.Context, taskID, projectID, teamID string) (*cfcontext.ContextPack, error)
}

// runtimeCheckpointer is the subset of CheckpointService used by RuntimeService.
type runtimeCheckpointer interface {
	CreateCheckpoint(ctx context.Context, runID, workspacePath, tool, callID string) error
	CleanupCheckpoints(ctx context.Context, runID, workspacePath string) error
	RewindToFirst(ctx context.Context, runID, workspacePath string) error
}

// runtimeSandboxManager is the subset of SandboxService used by RuntimeService.
type runtimeSandboxManager interface {
	Create(ctx context.Context, runID, workspacePath string, overrides ...resource.Limits) (*Sandbox, error)
	CreateHybrid(ctx context.Context, runID, workspacePath string, overrides ...resource.Limits) (*Sandbox, error)
	Start(ctx context.Context, runID string) error
	Stop(ctx context.Context, runID string) error
	Remove(ctx context.Context, runID string) error
	Get(runID string) (*Sandbox, bool)
}

// runtimeMCPResolver is the subset of MCPService used by RuntimeService.
type runtimeMCPResolver interface {
	ResolveForRun(projectID, modeID string) []mcp.ServerDef
}

// runtimeMicroagentMatcher is the subset of MicroagentService used by RuntimeService.
type runtimeMicroagentMatcher interface {
	Match(ctx context.Context, projectID, text string) ([]microagent.Microagent, error)
}

// runtimeQuarantineEvaluator is the subset of QuarantineService used by RuntimeService.
type runtimeQuarantineEvaluator interface {
	Evaluate(ctx context.Context, ann *trust.Annotation, subject string, payload []byte, projectID string) (bool, error)
}

// runtimeGoalCreator is the subset of GoalDiscoveryService used by RuntimeService.
type runtimeGoalCreator interface {
	Create(ctx context.Context, projectID string, req *goal.CreateRequest) (*goal.ProjectGoal, error)
}
