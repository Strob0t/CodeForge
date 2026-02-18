// Package database defines the database store port (interface).
package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/resource"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Store is the port interface for database operations.
type Store interface {
	// Projects
	ListProjects(ctx context.Context) ([]project.Project, error)
	GetProject(ctx context.Context, id string) (*project.Project, error)
	CreateProject(ctx context.Context, req project.CreateRequest) (*project.Project, error)
	UpdateProject(ctx context.Context, p *project.Project) error
	DeleteProject(ctx context.Context, id string) error

	// Agents
	ListAgents(ctx context.Context, projectID string) ([]agent.Agent, error)
	GetAgent(ctx context.Context, id string) (*agent.Agent, error)
	CreateAgent(ctx context.Context, projectID, name, backend string, config map[string]string, limits *resource.Limits) (*agent.Agent, error)
	UpdateAgentStatus(ctx context.Context, id string, status agent.Status) error
	DeleteAgent(ctx context.Context, id string) error

	// Tasks
	ListTasks(ctx context.Context, projectID string) ([]task.Task, error)
	GetTask(ctx context.Context, id string) (*task.Task, error)
	CreateTask(ctx context.Context, req task.CreateRequest) (*task.Task, error)
	UpdateTaskStatus(ctx context.Context, id string, status task.Status) error
	UpdateTaskResult(ctx context.Context, id string, result task.Result, costUSD float64) error

	// Runs
	CreateRun(ctx context.Context, r *run.Run) error
	GetRun(ctx context.Context, id string) (*run.Run, error)
	UpdateRunStatus(ctx context.Context, id string, status run.Status, stepCount int, costUSD float64, tokensIn, tokensOut int64) error
	CompleteRun(ctx context.Context, id string, status run.Status, output, errMsg string, costUSD float64, stepCount int, tokensIn, tokensOut int64, model string) error
	ListRunsByTask(ctx context.Context, taskID string) ([]run.Run, error)

	// Agent Teams
	CreateTeam(ctx context.Context, req agent.CreateTeamRequest) (*agent.Team, error)
	GetTeam(ctx context.Context, id string) (*agent.Team, error)
	ListTeamsByProject(ctx context.Context, projectID string) ([]agent.Team, error)
	UpdateTeamStatus(ctx context.Context, id string, status agent.TeamStatus) error
	DeleteTeam(ctx context.Context, id string) error

	// Execution Plans
	CreatePlan(ctx context.Context, p *plan.ExecutionPlan) error
	GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error)
	ListPlansByProject(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error)
	UpdatePlanStatus(ctx context.Context, id string, status plan.Status) error
	CreatePlanStep(ctx context.Context, step *plan.Step) error
	ListPlanSteps(ctx context.Context, planID string) ([]plan.Step, error)
	UpdatePlanStepStatus(ctx context.Context, stepID string, status plan.StepStatus, runID string, errMsg string) error
	GetPlanStepByRunID(ctx context.Context, runID string) (*plan.Step, error)
	UpdatePlanStepRound(ctx context.Context, stepID string, round int) error

	// Context Packs
	CreateContextPack(ctx context.Context, pack *cfcontext.ContextPack) error
	GetContextPack(ctx context.Context, id string) (*cfcontext.ContextPack, error)
	GetContextPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error)
	DeleteContextPack(ctx context.Context, id string) error

	// Shared Context
	CreateSharedContext(ctx context.Context, sc *cfcontext.SharedContext) error
	GetSharedContext(ctx context.Context, id string) (*cfcontext.SharedContext, error)
	GetSharedContextByTeam(ctx context.Context, teamID string) (*cfcontext.SharedContext, error)
	AddSharedContextItem(ctx context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error)
	DeleteSharedContext(ctx context.Context, id string) error

	// Repo Maps
	UpsertRepoMap(ctx context.Context, m *cfcontext.RepoMap) error
	GetRepoMap(ctx context.Context, projectID string) (*cfcontext.RepoMap, error)
	DeleteRepoMap(ctx context.Context, projectID string) error

	// Cost Aggregation
	CostSummaryGlobal(ctx context.Context) ([]cost.ProjectSummary, error)
	CostSummaryByProject(ctx context.Context, projectID string) (*cost.Summary, error)
	CostByModel(ctx context.Context, projectID string) ([]cost.ModelSummary, error)
	CostTimeSeries(ctx context.Context, projectID string, days int) ([]cost.DailyCost, error)
	RecentRunsWithCost(ctx context.Context, projectID string, limit int) ([]run.Run, error)
}
