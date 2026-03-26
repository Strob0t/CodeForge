package http

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/version"
)

// auditDB groups the store methods needed by audit logging (write + read).
type auditDB interface {
	middleware.AuditStore
	auditLogReader
}

// routeOptions holds optional configuration for MountRoutes.
type routeOptions struct {
	authRateLimiter *middleware.RateLimiter
	auditStore      auditDB
}

// RouteOption configures optional behavior in MountRoutes.
type RouteOption func(*routeOptions)

// WithAuthRateLimiter sets a stricter rate limiter for authentication endpoints
// (login, forgot-password, reset-password, setup). This limits brute-force
// attempts independently of the global rate limiter.
func WithAuthRateLimiter(rl *middleware.RateLimiter) RouteOption {
	return func(o *routeOptions) { o.authRateLimiter = rl }
}

// WithAuditStore enables audit logging middleware and the GET /audit-logs endpoint.
func WithAuditStore(s auditDB) RouteOption {
	return func(o *routeOptions) { o.auditStore = s }
}

// auditFunc is the type for the audit middleware factory used across mount functions.
type auditFunc func(action, resource string) func(http.Handler) http.Handler

// MountRoutes registers all API routes on the given chi router.
//
// TODO: FIX-061: Several endpoints use verbs in URLs (e.g., /detect-stack,
// /parse-repo-url, /discover, /decompose). Migrate to noun-based resources
// in v2 (breaking change — do not change v1 URLs now).
//
// TODO: FIX-063: Not all list endpoints return a consistent pagination envelope
// (items + total + limit + offset). Audit remaining endpoints for v2.
//
// TODO: FIX-095: No CSRF token beyond SameSite cookie. SameSite=Lax is sufficient
// for JSON API-only endpoints (no form posts). If HTML form support is added in
// the future, add a CSRF token middleware.
//
// TODO: FIX-098: Some DELETE operations use POST (e.g., /llm/models/delete,
// /projects/batch/delete). Migrate to proper HTTP DELETE in v2 (breaking change).
//
// TODO: FIX-100: Partial updates should use PATCH, not PUT. Audit endpoints
// that accept partial payloads and migrate to PATCH in v2 (breaking change).
//
// When /api/v2 is introduced, apply the Deprecation middleware to the v1 group:
//
//	r.Route("/api/v1", func(r chi.Router) {
//	    r.Use(middleware.Deprecation(sunsetDate))
//	    // ... existing v1 routes ...
//	})
func MountRoutes(r chi.Router, h *Handlers, webhookCfg config.Webhook, opts ...RouteOption) {
	var ro routeOptions
	for _, o := range opts {
		o(&ro)
	}

	// audit returns AuditLog middleware when an audit store is configured,
	// or a pass-through no-op otherwise.
	audit := auditFunc(func(action, resource string) func(http.Handler) http.Handler {
		if ro.auditStore == nil {
			return func(next http.Handler) http.Handler { return next }
		}
		return middleware.AuditLog(ro.auditStore, action, resource)
	})

	mountWebhookRoutes(r, h, webhookCfg)

	r.Route("/api/v1", func(r chi.Router) {
		// Version
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"version":"%s","git_sha":"%s"}`, version.Version, version.GitSHA)
		})

		// AGPL-3.0 source notice (Section 13 — Corresponding Source for network use)
		r.Get("/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"repository":"https://github.com/Strob0t/CodeForge","version":"%s","git_sha":"%s","license":"AGPL-3.0-or-later"}`, version.Version, version.GitSHA)
		})

		mountProjectRoutes(r, h)
		mountConversationRoutes(r, h)
		mountRunRoutes(r, h)
		mountOrchestrationRoutes(r, h, audit)
		mountLLMRoutes(r, h)
		mountRoadmapRoutes(r, h)
		mountReviewRoutes(r, h)
		mountIntelligenceRoutes(r, h)
		mountBenchmarkRoutes(r, h)
		mountSecurityRoutes(r, h, &ro, audit)
		mountDevToolRoutes(r, h)
		mountChannelRoutes(r, h)
		mountA2ARoutes(r, h)
		mountGoalRoutes(r, h)
		mountQuarantineRoutes(r, h, audit)
		mountMiscRoutes(r, h, &ro, audit)
	})
}

// mountWebhookRoutes registers VCS/PM webhook endpoints (outside auth, use HMAC/token verification).
func mountWebhookRoutes(r chi.Router, h *Handlers, webhookCfg config.Webhook) {
	r.Route("/api/v1/webhooks", func(r chi.Router) {
		r.With(middleware.WebhookHMAC(webhookCfg.GitHubSecret, "X-Hub-Signature-256")).
			Post("/vcs/github", h.HandleGitHubWebhook)
		r.With(middleware.WebhookToken(webhookCfg.GitLabToken, "X-Gitlab-Token")).
			Post("/vcs/gitlab", h.HandleGitLabWebhook)
		r.With(middleware.WebhookHMAC(webhookCfg.GitHubSecret, "X-Hub-Signature-256")).
			Post("/pm/github", h.HandleGitHubIssueWebhook)
		r.With(middleware.WebhookToken(webhookCfg.GitLabToken, "X-Gitlab-Token")).
			Post("/pm/gitlab", h.HandleGitLabIssueWebhook)
		r.With(middleware.WebhookHMAC(webhookCfg.PlaneSecret, "X-Plane-Signature")).
			Post("/pm/plane", h.HandlePlaneWebhook)
	})
}

// mountProjectRoutes registers project CRUD, workspace, git, file, agent, and task endpoints.
func mountProjectRoutes(r chi.Router, h *Handlers) {
	// Projects
	r.Get("/projects", h.Project.ListProjects)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects", h.Project.CreateProject)
	r.Get("/projects/remote-branches", h.Project.ListRemoteBranches)

	// Batch project operations
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/batch/delete", h.BatchDeleteProjects)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/batch/pull", h.BatchPullProjects)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/batch/status", h.BatchStatusProjects)

	// Global search
	r.Post("/search", h.GlobalSearch)
	r.Post("/search/conversations", h.SearchConversations)

	r.Get("/projects/{id}", h.Project.GetProject)
	r.With(middleware.RequireRole(user.RoleAdmin)).Delete("/projects/{id}", h.Project.DeleteProject)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/projects/{id}", h.Project.UpdateProject)

	// Workspace operations (nested under projects)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/clone", h.Project.CloneProject)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/adopt", h.Project.AdoptProject)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/setup", h.Project.SetupProject)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/init-workspace", h.Project.InitWorkspace)
	r.Get("/projects/{id}/workspace", h.Project.GetWorkspaceInfo)

	// File operations (nested under projects)
	r.Get("/projects/{id}/files", h.ListFiles)
	r.Get("/projects/{id}/files/tree", h.ListTree)
	r.Get("/projects/{id}/files/content", h.ReadFile)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/projects/{id}/files/content", h.WriteFile)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Delete("/projects/{id}/files", h.DeleteFile)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Patch("/projects/{id}/files/rename", h.RenameFile)

	// Stack Detection
	r.Get("/projects/{id}/detect-stack", h.Project.DetectProjectStack)
	r.Post("/detect-stack", h.Project.DetectStackByPath)

	// Git operations (nested under projects)
	r.Get("/projects/{id}/git/status", h.Project.ProjectGitStatus)
	r.Post("/projects/{id}/git/pull", h.Project.PullProject)
	r.Get("/projects/{id}/git/branches", h.Project.ListProjectBranches)
	r.Post("/projects/{id}/git/checkout", h.Project.CheckoutBranch)

	// Agents (nested under projects)
	r.Post("/projects/{id}/agents", h.Agent.CreateAgent)
	r.Get("/projects/{id}/agents", h.Agent.ListAgents)

	// Agents (direct access)
	r.Get("/agents/{id}", h.Agent.GetAgent)
	r.With(middleware.RequireRole(user.RoleAdmin)).Delete("/agents/{id}", h.Agent.DeleteAgent)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/agents/{id}/dispatch", h.Agent.DispatchTask)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/agents/{id}/stop", h.Agent.StopAgentTask)

	// Agent Identity (Phase 23C)
	r.Get("/agents/{id}/inbox", h.Agent.ListAgentInbox)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/agents/{id}/inbox", h.Agent.SendAgentMessage)
	r.Post("/agents/{id}/inbox/{msgId}/read", h.Agent.MarkInboxRead)
	r.Get("/agents/{id}/state", h.Agent.GetAgentState)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/agents/{id}/state", h.Agent.UpdateAgentState)

	// Tasks (nested under projects)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/tasks", h.Task.CreateTask)
	r.Get("/projects/{id}/tasks", h.Task.ListTasks)

	// Active agents (Phase 23D War Room)
	r.Get("/projects/{id}/agents/active", h.Agent.ListActiveAgents)

	// Active Work (Phase 24)
	r.Get("/projects/{id}/active-work", h.Task.ListActiveWork)

	// Tasks (direct access)
	r.Get("/tasks/{id}", h.Task.GetTask)
	r.Get("/tasks/{id}/events", h.Agent.ListTaskEvents)
	r.Get("/tasks/{id}/runs", h.Run.ListTaskRuns)
	r.Get("/tasks/{id}/context", h.GetContextPack)
	r.Post("/tasks/{id}/context", h.BuildContextPack)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/tasks/{id}/claim", h.Task.ClaimTask)

	// Branch Protection Rules (nested under projects + direct access)
	r.Post("/projects/{id}/branch-rules", h.CreateBranchProtectionRule)
	r.Get("/projects/{id}/branch-rules", h.ListBranchProtectionRules)
	r.Get("/branch-rules/{id}", h.GetBranchProtectionRule)
	r.Put("/branch-rules/{id}", h.UpdateBranchProtectionRule)
	r.Delete("/branch-rules/{id}", h.DeleteBranchProtectionRule)

	// Parse repo URL
	r.Post("/parse-repo-url", h.Project.ParseRepoURL)

	// Repo info (fetch metadata from remote hosting API)
	r.Get("/repos/info", h.Project.FetchRepoInfo)

	// Provider registries
	r.Get("/providers/git", h.Utility.ListGitProviders)
	r.Get("/providers/agent", h.Utility.ListAgentBackends)
	r.Get("/providers/spec", h.ListSpecProviders)
	r.Get("/providers/pm", h.ListPMProviders)

	// Backend health
	r.Get("/backends/health", h.CheckBackendHealth)
}

// mountConversationRoutes registers conversation and channel-related endpoints.
func mountConversationRoutes(r chi.Router, h *Handlers) {
	r.Post("/projects/{id}/conversations", h.CreateConversation)
	r.Get("/projects/{id}/conversations", h.ListConversations)
	r.Get("/conversations/{id}", h.GetConversation)
	r.Delete("/conversations/{id}", h.DeleteConversation)
	r.Get("/conversations/{id}/messages", h.ListConversationMessages)
	r.Post("/conversations/{id}/messages", h.SendConversationMessage)
	r.Post("/conversations/{id}/stop", h.StopConversation)
	r.Post("/conversations/{id}/bypass-approvals", h.BypassConversationApprovals)
	r.Get("/conversations/{id}/session", h.GetConversationSession)
	r.Post("/conversations/{id}/fork", h.ForkConversation)
	r.Post("/conversations/{id}/rewind", h.RewindConversation)
	r.Post("/conversations/{id}/compact", h.CompactConversation)
	r.Post("/conversations/{id}/clear", h.ClearConversation)
	r.Post("/conversations/{id}/mode", h.SetConversationMode)
	r.Post("/conversations/{id}/model", h.SetConversationModel)

	// Commands (slash commands for chat)
	r.Get("/commands", h.ListCommands)

	// Prompt Sections
	r.Get("/prompt-sections", h.ListPromptSections)
	r.Put("/prompt-sections", h.UpsertPromptSection)
	r.Delete("/prompt-sections/{id}", h.DeletePromptSection)
	r.Post("/prompt-sections/preview", h.PreviewPromptSections)
}

// mountRunRoutes registers run CRUD, HITL approval, trajectory, replay, and session endpoints.
func mountRunRoutes(r chi.Router, h *Handlers) {
	// Runs
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/runs", h.Run.StartRun)
	r.Get("/runs/{id}", h.Run.GetRun)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/runs/{id}/cancel", h.Run.CancelRun)
	r.Get("/runs/{id}/events", h.Run.ListRunEvents)

	// Run Approval (Phase 31)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
		Post("/runs/{id}/approve", h.ApproveRun)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
		Post("/runs/{id}/reject", h.RejectRun)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
		Post("/runs/{id}/approve-partial", h.ApproveRunPartial)

	// HITL (Human-in-the-Loop) Approval
	r.Post("/runs/{id}/approve/{callId}", h.ApproveToolCall)
	r.Post("/runs/{id}/revert/{callId}", h.RevertToolCall)

	// Trajectory (nested under runs)
	r.Get("/runs/{id}/trajectory", h.GetTrajectory)
	r.Get("/runs/{id}/trajectory/export", h.ExportTrajectory)

	// Cost per run
	r.Get("/runs/{id}/costs/by-tool", h.RunCostByTool)

	// Human Feedback (Phase 22D)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/feedback/{run_id}/{call_id}", h.HandleFeedbackCallback)
	r.Get("/runs/{id}/feedback", h.ListFeedbackAudit)

	// Replay / Audit Trail (nested under runs + global)
	r.Get("/runs/{id}/checkpoints", h.ListRunCheckpoints)
	r.Post("/runs/{id}/replay", h.ReplayRun)
	r.Get("/audit", h.GlobalAuditTrail)
	r.Get("/projects/{id}/audit", h.ProjectAuditTrail)

	// Sessions (nested under runs + projects + direct access)
	r.Post("/runs/{id}/resume", h.ResumeRun)
	r.Post("/runs/{id}/fork", h.ForkRun)
	r.Post("/runs/{id}/rewind", h.RewindRun)
	r.Get("/projects/{id}/sessions", h.ListProjectSessions)
	r.Get("/sessions/{id}", h.GetSession)
}

// mountOrchestrationRoutes registers orchestrator, mode, pipeline, meta-agent, and auto-agent endpoints.
func mountOrchestrationRoutes(r chi.Router, h *Handlers, audit auditFunc) {
	// Agent configuration (frontend-visible config values)
	r.Get("/agent-config", h.GetAgentConfig)

	// Policy profiles
	r.Get("/policies", h.Policy.ListPolicyProfiles)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor), audit("create", "policy")).Post("/policies", h.Policy.CreatePolicyProfile)
	r.Post("/policies/allow-always", h.Policy.AllowAlwaysPolicy)
	r.Get("/policies/{name}", h.Policy.GetPolicyProfile)
	r.With(middleware.RequireRole(user.RoleAdmin), audit("delete", "policy")).Delete("/policies/{name}", h.Policy.DeletePolicyProfile)
	r.Post("/policies/{name}/evaluate", h.Policy.EvaluatePolicy)

	// Feature Decomposition (Meta-Agent)
	r.Post("/projects/{id}/decompose", h.DecomposeFeature)

	// Context-Optimized Feature Planning
	r.Post("/projects/{id}/plan-feature", h.PlanFeature)

	// Execution Plans (nested under projects)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/plans", h.CreatePlan)
	r.Get("/projects/{id}/plans", h.ListPlans)

	// Execution Plans (direct access)
	r.Get("/plans/{id}", h.GetPlan)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/plans/{id}/start", h.StartPlan)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/plans/{id}/cancel", h.CancelPlan)
	r.Get("/plans/{id}/graph", h.GetPlanGraph)
	r.Post("/plans/{id}/steps/{stepId}/evaluate", h.EvaluateStep)

	// Modes
	r.Get("/modes", h.ListModes)
	r.Get("/modes/scenarios", h.ListScenarios)
	r.Get("/modes/tools", h.ListModeTools)
	r.Get("/modes/artifact-types", h.ListArtifactTypes)
	r.Get("/modes/{id}", h.GetMode)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor), audit("create", "mode")).Post("/modes", h.CreateMode)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/modes/{id}", h.UpdateMode)
	r.With(middleware.RequireRole(user.RoleAdmin), audit("delete", "mode")).Delete("/modes/{id}", h.DeleteMode)

	// Pipeline Templates
	r.Get("/pipelines", h.ListPipelines)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/pipelines", h.RegisterPipeline)
	r.Get("/pipelines/{id}", h.GetPipeline)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/pipelines/{id}/instantiate", h.InstantiatePipeline)

	// Prompt Evolution
	if h.PromptEvolution != nil {
		r.Get("/prompt-evolution/status", h.GetPromptEvolutionStatus)
		r.Get("/prompt-evolution/variants", h.ListPromptEvolutionVariants)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
			Post("/prompt-evolution/reflect", h.TriggerPromptEvolutionReflect)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
			Post("/prompt-evolution/revert/{modeId}", h.RevertPromptEvolutionMode)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
			Post("/prompt-evolution/promote/{variantId}", h.PromotePromptEvolutionVariant)
	}

	// Shared Context
	if h.SharedContext != nil {
		r.Post("/teams/{teamId}/shared-context", h.InitSharedContext)
		r.Get("/teams/{teamId}/shared-context", h.GetSharedContext)
		r.Post("/teams/{teamId}/shared-context/items", h.AddSharedContextItem)
	}

	// Auto-Agent (PR3.3)
	r.Post("/projects/{id}/auto-agent/start", h.StartAutoAgent)
	r.Post("/projects/{id}/auto-agent/stop", h.StopAutoAgent)
	r.Get("/projects/{id}/auto-agent/status", h.GetAutoAgentStatus)

	// Routing (Phase 29)
	r.Route("/routing", func(r chi.Router) {
		r.Get("/stats", h.HandleListRoutingStats)
		r.Post("/stats/refresh", h.HandleRefreshRoutingStats)
		r.Get("/outcomes", h.HandleListRoutingOutcomes)
		r.Post("/outcomes", h.HandleCreateRoutingOutcome)
		r.Post("/seed-from-benchmarks", h.HandleSeedFromBenchmarks)
	})
}

// mountLLMRoutes registers LLM model management, model registry, copilot, and LLM key endpoints.
func mountLLMRoutes(r chi.Router, h *Handlers) {
	// LLM management (proxied to LiteLLM)
	r.Get("/llm/models", h.ListLLMModels)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/llm/models", h.AddLLMModel)
	r.With(middleware.RequireRole(user.RoleAdmin)).Post("/llm/models/delete", h.DeleteLLMModel)
	r.Get("/llm/health", h.LLMHealth)
	r.Get("/llm/discover", h.DiscoverLLMModels)

	// Model Registry (Phase 22)
	r.Get("/llm/available", h.AvailableLLMModels)
	r.Post("/llm/refresh", h.RefreshLLMModels)

	// Copilot Token Exchange (Phase 22A)
	r.Post("/copilot/exchange", h.HandleCopilotExchange)

	// LLM Keys
	r.Get("/llm-keys", h.ListLLMKeys)
	r.Post("/llm-keys", h.CreateLLMKey)
	r.Delete("/llm-keys/{id}", h.DeleteLLMKey)
}

// mountRoadmapRoutes registers roadmap, milestone, feature, and bidirectional sync endpoints.
func mountRoadmapRoutes(r chi.Router, h *Handlers) {
	// Roadmap (nested under projects)
	r.Get("/projects/{id}/roadmap", h.GetProjectRoadmap)
	r.Post("/projects/{id}/roadmap", h.CreateProjectRoadmap)
	r.Put("/projects/{id}/roadmap", h.UpdateProjectRoadmap)
	r.Delete("/projects/{id}/roadmap", h.DeleteProjectRoadmap)
	r.Get("/projects/{id}/roadmap/ai", h.GetRoadmapAI)
	r.Post("/projects/{id}/roadmap/detect", h.DetectRoadmap)
	r.Post("/projects/{id}/roadmap/import", h.ImportSpecs)
	r.Post("/projects/{id}/roadmap/import/pm", h.ImportPMItems)
	r.Post("/projects/{id}/roadmap/milestones", h.CreateMilestone)

	// Milestones (direct access)
	r.Get("/milestones/{id}", h.GetMilestone)
	r.Put("/milestones/{id}", h.UpdateMilestone)
	r.Delete("/milestones/{id}", h.DeleteMilestone)

	// Features (nested under milestones + direct access)
	r.Post("/milestones/{id}/features", h.CreateFeature)
	r.Get("/features/{id}", h.GetFeature)
	r.Put("/features/{id}", h.UpdateFeature)
	r.Delete("/features/{id}", h.DeleteFeature)

	// Bidirectional Sync (nested under projects)
	r.Post("/projects/{id}/roadmap/sync", h.SyncRoadmap)
	r.Post("/projects/{id}/roadmap/sync-to-file", h.SyncToSpecFile)
}

// mountReviewRoutes registers review policies, boundaries, and review/refactor endpoints.
func mountReviewRoutes(r chi.Router, h *Handlers) {
	// Boundaries (Phase 31)
	r.Get("/projects/{id}/boundaries", h.GetProjectBoundaries)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
		Put("/projects/{id}/boundaries", h.UpdateProjectBoundaries)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
		Post("/projects/{id}/boundaries/analyze", h.TriggerBoundaryAnalysis)

	// Review/Refactor (Phase 31)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).
		Post("/projects/{id}/review-refactor", h.TriggerReviewRefactor)

	// Review Policies & Reviews (Phase 12I)
	r.Get("/projects/{id}/review-policies", h.ListReviewPolicies)
	r.Post("/projects/{id}/review-policies", h.CreateReviewPolicy)
	r.Get("/review-policies/{id}", h.GetReviewPolicy)
	r.Put("/review-policies/{id}", h.UpdateReviewPolicy)
	r.Delete("/review-policies/{id}", h.DeleteReviewPolicy)
	r.Post("/review-policies/{id}/trigger", h.TriggerReview)
	r.Get("/projects/{id}/reviews", h.ListReviews)
	r.Get("/reviews/{id}", h.GetReviewHandler)
}

// mountIntelligenceRoutes registers context, repomap, retrieval, graph, scope, knowledge base,
// memory, experience, microagent, and skill endpoints.
func mountIntelligenceRoutes(r chi.Router, h *Handlers) {
	// RepoMap (nested under projects)
	r.Get("/projects/{id}/repomap", h.GetRepoMap)
	r.Post("/projects/{id}/repomap", h.GenerateRepoMap)

	// Retrieval (nested under projects)
	r.Post("/projects/{id}/search", h.SearchProject)
	r.Post("/projects/{id}/search/agent", h.AgentSearchProject)
	r.Post("/projects/{id}/index", h.IndexProject)
	r.Get("/projects/{id}/index", h.GetIndexStatus)

	// GraphRAG (nested under projects)
	r.Post("/projects/{id}/graph/build", h.BuildGraph)
	r.Get("/projects/{id}/graph/status", h.GetGraphStatus)
	r.Post("/projects/{id}/graph/search", h.SearchGraph)

	// Retrieval Scopes (cross-project search)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/scopes", h.CreateScope)
	r.Get("/scopes", h.ListScopes)
	r.Get("/scopes/{id}", h.GetScope)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/scopes/{id}", h.UpdateScope)
	r.With(middleware.RequireRole(user.RoleAdmin)).Delete("/scopes/{id}", h.DeleteScope)
	r.Post("/scopes/{id}/projects", h.AddProjectToScope)
	r.Delete("/scopes/{id}/projects/{pid}", h.RemoveProjectFromScope)
	r.Post("/scopes/{id}/search", h.SearchScope)
	r.Post("/scopes/{id}/graph/search", h.SearchScopeGraph)

	// Knowledge Bases on Scopes
	r.Post("/scopes/{id}/knowledge-bases", h.AttachKnowledgeBaseToScope)
	r.Delete("/scopes/{id}/knowledge-bases/{kbid}", h.DetachKnowledgeBaseFromScope)
	r.Get("/scopes/{id}/knowledge-bases", h.ListScopeKnowledgeBases)

	// Knowledge Bases
	r.Get("/knowledge-bases", h.ListKnowledgeBases)
	r.Post("/knowledge-bases", h.CreateKnowledgeBase)
	r.Get("/knowledge-bases/{id}", h.GetKnowledgeBase)
	r.Put("/knowledge-bases/{id}", h.UpdateKnowledgeBase)
	r.Delete("/knowledge-bases/{id}", h.DeleteKnowledgeBase)
	r.Post("/knowledge-bases/{id}/index", h.IndexKnowledgeBase)

	// Memories (Phase 22B)
	r.Get("/projects/{id}/memories", h.ListMemories)
	r.Post("/projects/{id}/memories", h.StoreMemory)
	r.Post("/projects/{id}/memories/recall", h.RecallMemories)

	// Experience Pool (Phase 22B)
	r.Get("/projects/{id}/experience", h.ListExperienceEntries)
	r.Delete("/experience/{id}", h.DeleteExperienceEntry)

	// Microagents (Phase 22C)
	r.Get("/projects/{id}/microagents", h.ListMicroagents)
	r.Post("/projects/{id}/microagents", h.CreateMicroagent)
	r.Get("/microagents/{id}", h.GetMicroagent)
	r.Put("/microagents/{id}", h.UpdateMicroagent)
	r.Delete("/microagents/{id}", h.DeleteMicroagent)

	// Skills (Phase 22D)
	r.Get("/projects/{id}/skills", h.ListSkills)
	r.Post("/projects/{id}/skills", h.CreateSkill)
	r.Get("/skills/{id}", h.GetSkill)
	r.Put("/skills/{id}", h.UpdateSkill)
	r.Delete("/skills/{id}", h.DeleteSkill)
	r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/skills/import", h.ImportSkill)
}

// mountBenchmarkRoutes registers benchmark suite, run, and comparison endpoints (dev-mode only).
func mountBenchmarkRoutes(r chi.Router, h *Handlers) {
	// Dev tools (behind APP_ENV=development)
	r.With(middleware.DevModeOnly(h.AppEnv)).Post("/dev/benchmark", h.BenchmarkPrompt)

	// Benchmark Mode (Phase 20D — dev-mode only, requires APP_ENV=development)
	r.Route("/benchmarks", func(r chi.Router) {
		r.Use(middleware.DevModeOnly(h.AppEnv))
		// Suite CRUD (Phase 26)
		r.Get("/suites", h.ListBenchmarkSuites)
		r.Post("/suites", h.CreateBenchmarkSuite)
		r.Get("/suites/{id}", h.GetBenchmarkSuite)
		r.Delete("/suites/{id}", h.DeleteBenchmarkSuite)
		// Run CRUD
		r.Get("/runs", h.ListBenchmarkRuns)
		r.Post("/runs", h.CreateBenchmarkRun)
		r.Get("/runs/{id}", h.GetBenchmarkRun)
		r.Delete("/runs/{id}", h.DeleteBenchmarkRun)
		r.Patch("/runs/{id}", h.CancelBenchmarkRun)
		r.Get("/runs/{id}/results", h.ListBenchmarkResults)
		r.Get("/runs/{id}/export/results", h.ExportBenchmarkResults)
		r.Post("/compare", h.CompareBenchmarkRuns)
		r.Post("/compare-multi", h.MultiCompareBenchmarkRuns)
		r.Get("/runs/{id}/cost-analysis", h.BenchmarkCostAnalysis)
		r.Get("/runs/{id}/export/training", h.ExportTrainingData)
		r.Get("/runs/{id}/export/rlvr", h.ExportRLVRData)
		r.Get("/leaderboard", h.BenchmarkLeaderboard)
		r.Get("/datasets", h.ListBenchmarkDatasets)
		r.Put("/suites/{id}", h.UpdateBenchmarkSuite)
		r.Post("/runs/{id}/analyze", h.AnalyzeBenchmarkRun)
	})
}

// mountSecurityRoutes registers auth, user management, GDPR, tenant, and VCS account endpoints.
func mountSecurityRoutes(r chi.Router, h *Handlers, ro *routeOptions, audit auditFunc) {
	// Auth (public routes handled by middleware exemption)
	// FIX-084: Apply stricter per-route rate limiting for auth endpoints
	// to mitigate brute-force attacks independently of the global rate limiter.
	if ro.authRateLimiter != nil {
		r.With(ro.authRateLimiter.Handler).Post("/auth/login", h.Login)
		r.Post("/auth/refresh", h.Refresh)
		r.Get("/auth/setup-status", h.SetupStatus)
		r.With(ro.authRateLimiter.Handler).Post("/auth/setup", h.InitialSetup)
		r.With(ro.authRateLimiter.Handler).Post("/auth/forgot-password", h.RequestPasswordReset)
		r.With(ro.authRateLimiter.Handler).Post("/auth/reset-password", h.ConfirmPasswordReset)
		r.Get("/auth/github", h.StartGitHubOAuth)
		r.Get("/auth/github/callback", h.GitHubOAuthCallback)
	} else {
		r.Post("/auth/login", h.Login)
		r.Post("/auth/refresh", h.Refresh)
		r.Get("/auth/setup-status", h.SetupStatus)
		r.Post("/auth/setup", h.InitialSetup)
		r.Post("/auth/forgot-password", h.RequestPasswordReset)
		r.Post("/auth/reset-password", h.ConfirmPasswordReset)
		r.Get("/auth/github", h.StartGitHubOAuth)
		r.Get("/auth/github/callback", h.GitHubOAuthCallback)
	}

	// Auth (authenticated)
	r.Post("/auth/logout", h.Logout)
	r.Get("/auth/me", h.GetCurrentUser)
	r.Post("/auth/change-password", h.ChangePassword)
	r.Post("/auth/api-keys", h.CreateAPIKeyHandler)
	r.Get("/auth/api-keys", h.ListAPIKeysHandler)
	r.Delete("/auth/api-keys/{id}", h.DeleteAPIKeyHandler)

	// Subscription Providers (OAuth device flow connect)
	r.Get("/auth/providers", h.ListSubscriptionProviders)
	r.Post("/auth/providers/{provider}/connect", h.StartProviderConnect)
	r.Get("/auth/providers/{provider}/status", h.GetProviderStatus)
	r.Delete("/auth/providers/{provider}/disconnect", h.DisconnectProvider)

	// VCS Accounts
	r.Get("/vcs-accounts", h.ListVCSAccounts)
	r.Post("/vcs-accounts", h.CreateVCSAccount)
	r.Delete("/vcs-accounts/{id}", h.DeleteVCSAccount)
	r.Post("/vcs-accounts/{id}/test", h.TestVCSAccount)

	// Users (admin only)
	r.Route("/users", func(r chi.Router) {
		r.Use(middleware.RequireRole(user.RoleAdmin))
		r.Get("/", h.ListUsersHandler)
		r.With(audit("create", "user")).Post("/", h.CreateUserHandler)
		r.With(audit("update", "user")).Put("/{id}", h.UpdateUserHandler)
		r.With(audit("delete", "user")).Delete("/{id}", h.DeleteUserHandler)
		r.With(audit("force_password_change", "user")).Post("/{id}/force-password-change", h.AdminForcePasswordChange)
		// GDPR endpoints (Article 17: erasure, Article 20: portability)
		r.Post("/{id}/export", h.ExportUserData)
		r.With(audit("delete", "user_data")).Delete("/{id}/data", h.DeleteUserData)
	})

	// Tenants (admin only)
	r.Route("/tenants", func(r chi.Router) {
		r.Use(middleware.RequireRole(user.RoleAdmin))
		r.Get("/", h.ListTenants)
		r.Post("/", h.CreateTenant)
		r.Get("/{id}", h.GetTenant)
		r.Put("/{id}", h.UpdateTenant)
	})
}

// mountDevToolRoutes registers LSP, MCP, and project MCP server endpoints.
func mountDevToolRoutes(r chi.Router, h *Handlers) {
	// LSP (Language Server Protocol)
	r.Post("/projects/{id}/lsp/start", h.StartLSP)
	r.Post("/projects/{id}/lsp/stop", h.StopLSP)
	r.Get("/projects/{id}/lsp/status", h.LSPStatus)
	r.Get("/projects/{id}/lsp/diagnostics", h.LSPDiagnostics)
	r.Post("/projects/{id}/lsp/definition", h.LSPDefinition)
	r.Post("/projects/{id}/lsp/references", h.LSPReferences)
	r.Post("/projects/{id}/lsp/symbols", h.LSPDocumentSymbols)
	r.Post("/projects/{id}/lsp/hover", h.LSPHover)

	// MCP Servers (Phase 15C + 19H)
	r.Get("/mcp/servers", h.ListMCPServers)
	r.Post("/mcp/servers", h.CreateMCPServer)
	r.Post("/mcp/servers/test", h.TestMCPServerConnection) // pre-save test (no ID)
	r.Get("/mcp/servers/{id}", h.GetMCPServer)
	r.Put("/mcp/servers/{id}", h.UpdateMCPServer)
	r.Delete("/mcp/servers/{id}", h.DeleteMCPServer)
	r.Post("/mcp/servers/{id}/test", h.TestMCPServer)
	r.Get("/mcp/servers/{id}/tools", h.ListMCPServerTools)
	r.Get("/projects/{id}/mcp-servers", h.ListProjectMCPServers)
	r.Post("/projects/{id}/mcp-servers", h.AssignMCPServerToProject)
	r.Delete("/projects/{id}/mcp-servers/{serverId}", h.UnassignMCPServerFromProject)
}

// mountChannelRoutes registers real-time channel endpoints.
func mountChannelRoutes(r chi.Router, h *Handlers) {
	r.Route("/channels", func(r chi.Router) {
		r.Get("/", h.ListChannels)
		r.Post("/", h.CreateChannel)
		r.Get("/{id}", h.GetChannel)
		r.Delete("/{id}", h.DeleteChannel)
		r.Get("/{id}/messages", h.ListChannelMessages)
		r.Post("/{id}/messages", h.SendChannelMessage)
		r.Post("/{id}/messages/{mid}/thread", h.SendThreadReply)
		r.Put("/{id}/members/{uid}", h.UpdateMemberNotify)
		r.Post("/{id}/webhook", h.WebhookMessage)
	})
}

// mountA2ARoutes registers Agent-to-Agent protocol management endpoints.
func mountA2ARoutes(r chi.Router, h *Handlers) {
	if h.A2A == nil {
		return
	}
	r.Route("/a2a", func(r chi.Router) {
		r.Post("/agents", h.RegisterRemoteAgent)
		r.Get("/agents", h.ListRemoteAgents)
		r.Delete("/agents/{id}", h.DeleteRemoteAgent)
		r.Post("/agents/{id}/discover", h.DiscoverRemoteAgent)
		r.Post("/agents/{id}/send", h.SendA2ATask)
		r.Get("/tasks", h.ListA2ATasks)
		r.Get("/tasks/{id}", h.GetA2ATask)
		r.Post("/tasks/{id}/cancel", h.CancelA2ATask)
		// Push notification configs (Phase 27O)
		r.Post("/tasks/{id}/push-config", h.CreateA2APushConfig)
		r.Get("/tasks/{id}/push-config", h.ListA2APushConfigs)
		// SSE streaming (Phase 27O)
		r.Get("/tasks/{id}/subscribe", h.SubscribeA2ATask)
		// Push config delete (by config ID)
		r.Delete("/push-config/{id}", h.DeleteA2APushConfig)
	})
}

// mountGoalRoutes registers project goal discovery endpoints.
func mountGoalRoutes(r chi.Router, h *Handlers) {
	if h.GoalDiscovery == nil {
		return
	}
	r.Get("/projects/{id}/goals", h.ListProjectGoals)
	r.Post("/projects/{id}/goals", h.CreateProjectGoal)
	r.Post("/projects/{id}/goals/detect", h.DetectProjectGoals)
	r.Post("/projects/{id}/goals/ai-discover", h.AIDiscoverProjectGoals)
	r.Get("/goals/{id}", h.GetProjectGoal)
	r.Put("/goals/{id}", h.UpdateProjectGoal)
	r.Delete("/goals/{id}", h.DeleteProjectGoal)
}

// mountQuarantineRoutes registers quarantine management endpoints (admin only).
func mountQuarantineRoutes(r chi.Router, h *Handlers, audit auditFunc) {
	r.Route("/quarantine", func(r chi.Router) {
		r.Use(middleware.RequireRole(user.RoleAdmin))
		r.Get("/", h.listQuarantinedMessages)
		r.Get("/stats", h.quarantineStats)
		r.Get("/{id}", h.getQuarantinedMessage)
		r.With(audit("approve", "quarantine")).Post("/{id}/approve", h.approveQuarantinedMessage)
		r.With(audit("reject", "quarantine")).Post("/{id}/reject", h.rejectQuarantinedMessage)
	})
}

// mountMiscRoutes registers cost, dashboard, settings, and admin audit log endpoints.
func mountMiscRoutes(r chi.Router, h *Handlers, ro *routeOptions, _ auditFunc) {
	// Cost aggregation
	r.Get("/costs", h.GlobalCostSummary)
	r.Get("/projects/{id}/costs", h.ProjectCostSummary)
	r.Get("/projects/{id}/costs/by-model", h.ProjectCostByModel)
	r.Get("/projects/{id}/costs/by-tool", h.ProjectCostByTool)
	r.Get("/projects/{id}/costs/daily", h.ProjectCostTimeSeries)
	r.Get("/projects/{id}/costs/runs", h.ProjectRecentRuns)

	// Dashboard aggregation
	r.Get("/dashboard/stats", h.DashboardStats)
	r.Get("/dashboard/charts/cost-trend", h.DashboardCostTrend)
	r.Get("/dashboard/charts/run-outcomes", h.DashboardRunOutcomes)
	r.Get("/dashboard/charts/agent-performance", h.DashboardAgentPerformance)
	r.Get("/dashboard/charts/model-usage", h.DashboardModelUsage)
	r.Get("/dashboard/charts/cost-by-project", h.DashboardCostByProject)
	r.Get("/projects/{id}/health", h.ProjectHealth)

	// Settings
	r.Get("/settings", h.GetSettings)
	r.With(middleware.RequireRole(user.RoleAdmin)).Put("/settings", h.UpdateSettings)

	// Admin Audit Logs (SOC 2 CC6.1)
	if ro.auditStore != nil {
		r.With(middleware.RequireRole(user.RoleAdmin)).Get("/audit-logs", ListAuditLogs(ro.auditStore))
	}
}
