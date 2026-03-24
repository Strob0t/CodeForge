package http

import (
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/llm"
	"github.com/Strob0t/CodeForge/internal/port/tokenexchange"
	"github.com/Strob0t/CodeForge/internal/service"
)

// llmFull combines all LLM port interfaces used by handlers.
type llmFull interface {
	llm.Provider
	llm.ModelDiscoverer
	llm.ModelAdmin
}

// Handlers holds the HTTP handler dependencies.
// Domain-specific handler groups (Project, Agent, Task, Run, Policy, Utility)
// own their respective methods and service dependencies.
// The remaining fields serve existing handler files (conversation, benchmark, etc.).
type Handlers struct {
	// Domain-specific handler groups
	Project *ProjectHandlers
	Agent   *AgentHandlers
	Task    *TaskHandlers
	Run     *RunHandlers
	Policy  *PolicyHandlers
	Utility *UtilityHandlers

	Projects         *service.ProjectService
	Tasks            *service.TaskService
	Agents           *service.AgentService
	LLM              llmFull
	Policies         *service.PolicyService
	PolicyDir        string // Custom policy YAML directory (empty = no persistence)
	Runtime          *service.RuntimeService
	Orchestrator     *service.OrchestratorService
	MetaAgent        *service.MetaAgentService
	PoolManager      *service.PoolManagerService
	TaskPlanner      *service.TaskPlannerService
	ContextOptimizer *service.ContextOptimizerService
	SharedContext    *service.SharedContextService
	Modes            *service.ModeService
	RepoMap          *service.RepoMapService
	Retrieval        *service.RetrievalService
	Graph            *service.GraphService
	Events           eventstore.Store
	Cost             *service.CostService
	Roadmap          *service.RoadmapService
	Tenants          *service.TenantService
	BranchProtection *service.BranchProtectionService
	Replay           *service.ReplayService
	Sessions         *service.SessionService
	VCSWebhook       *service.VCSWebhookService
	Sync             *service.SyncService
	PMWebhook        *service.PMWebhookService
	Notification     *service.NotificationService
	Auth             *service.AuthService
	Scope            *service.ScopeService
	Pipelines        *service.PipelineService
	Review           *service.ReviewService
	KnowledgeBases   *service.KnowledgeBaseService
	Settings         *service.SettingsService
	VCSAccounts      *service.VCSAccountService
	LLMKeys          *service.LLMKeyService
	Conversations    *service.ConversationService
	LSP              *service.LSPService
	MCP              *service.MCPService
	PromptSections   *service.PromptSectionService
	Benchmarks       *service.BenchmarkService
	ReviewRouter     *service.ReviewRouterService
	ModelRegistry    *service.ModelRegistry
	TokenExchanger   tokenexchange.Exchanger
	Memory           *service.MemoryService
	ExperiencePool   *service.ExperiencePoolService
	Microagents      *service.MicroagentService
	Skills           *service.SkillService
	Files            *service.FileService
	AutoAgent        *service.AutoAgentService
	Quarantine       *service.QuarantineService
	ActiveWork       *service.ActiveWorkService
	Routing          *service.RoutingService
	A2A              *service.A2AService
	GoalDiscovery    *service.GoalDiscoveryService
	Dashboard        *service.DashboardService
	GitHubOAuth      *service.GitHubOAuthService
	BackendHealth    *service.BackendHealthService
	Checkpoint       *service.CheckpointService
	Commands         *service.CommandService
	Channels         *service.ChannelService
	Subscription     *service.SubscriptionService
	Limits           *config.Limits
	AgentConfig      *config.Agent
	AppEnv           string // From cfg.AppEnv (APP_ENV env var)
	OllamaBaseURL    string // From cfg.Ollama.BaseURL (OLLAMA_BASE_URL env var)
	Boundaries       *service.BoundaryService
	ReviewTrigger    *service.ReviewTriggerService
	PromptEvolution  *service.PromptEvolutionService
	GDPR             *service.GDPRService
}
