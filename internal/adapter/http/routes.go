package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// MountRoutes registers all API routes on the given chi router.
//
// When /api/v2 is introduced, apply the Deprecation middleware to the v1 group:
//
//	r.Route("/api/v1", func(r chi.Router) {
//	    r.Use(middleware.Deprecation(sunsetDate))
//	    // ... existing v1 routes ...
//	})
func MountRoutes(r chi.Router, h *Handlers, webhookCfg config.Webhook) {
	// VCS Webhooks (outside auth, use HMAC/token verification)
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

	r.Route("/api/v1", func(r chi.Router) {
		// Version
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"0.1.0"}`))
		})

		// Projects
		r.Get("/projects", h.ListProjects)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects", h.CreateProject)
		r.Get("/projects/remote-branches", h.ListRemoteBranches)

		// Batch project operations
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/batch/delete", h.BatchDeleteProjects)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/batch/pull", h.BatchPullProjects)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/batch/status", h.BatchStatusProjects)

		// Global search
		r.Post("/search", h.GlobalSearch)

		r.Get("/projects/{id}", h.GetProject)
		r.With(middleware.RequireRole(user.RoleAdmin)).Delete("/projects/{id}", h.DeleteProject)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/projects/{id}", h.UpdateProject)

		// Workspace operations (nested under projects)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/clone", h.CloneProject)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/adopt", h.AdoptProject)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/setup", h.SetupProject)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/projects/{id}/init-workspace", h.InitWorkspace)
		r.Get("/projects/{id}/workspace", h.GetWorkspaceInfo)

		// File operations (nested under projects)
		r.Get("/projects/{id}/files", h.ListFiles)
		r.Get("/projects/{id}/files/tree", h.ListTree)
		r.Get("/projects/{id}/files/content", h.ReadFile)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/projects/{id}/files/content", h.WriteFile)

		// Stack Detection
		r.Get("/projects/{id}/detect-stack", h.DetectProjectStack)
		r.Post("/detect-stack", h.DetectStackByPath)

		// Git operations (nested under projects)
		r.Get("/projects/{id}/git/status", h.ProjectGitStatus)
		r.Post("/projects/{id}/git/pull", h.PullProject)
		r.Get("/projects/{id}/git/branches", h.ListProjectBranches)
		r.Post("/projects/{id}/git/checkout", h.CheckoutBranch)

		// Agents (nested under projects)
		r.Post("/projects/{id}/agents", h.CreateAgent)
		r.Get("/projects/{id}/agents", h.ListAgents)

		// Agents (direct access)
		r.Get("/agents/{id}", h.GetAgent)
		r.With(middleware.RequireRole(user.RoleAdmin)).Delete("/agents/{id}", h.DeleteAgent)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/agents/{id}/dispatch", h.DispatchTask)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/agents/{id}/stop", h.StopAgentTask)

		// Agent Identity (Phase 23C)
		r.Get("/agents/{id}/inbox", h.ListAgentInbox)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/agents/{id}/inbox", h.SendAgentMessage)
		r.Post("/agents/{id}/inbox/{msgId}/read", h.MarkInboxRead)
		r.Get("/agents/{id}/state", h.GetAgentState)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Put("/agents/{id}/state", h.UpdateAgentState)

		// Tasks (nested under projects)
		r.Post("/projects/{id}/tasks", h.CreateTask)
		r.Get("/projects/{id}/tasks", h.ListTasks)

		// Active agents (Phase 23D War Room)
		r.Get("/projects/{id}/agents/active", h.ListActiveAgents)

		// Active Work (Phase 24)
		r.Get("/projects/{id}/active-work", h.ListActiveWork)

		// Tasks (direct access)
		r.Get("/tasks/{id}", h.GetTask)
		r.Get("/tasks/{id}/events", h.ListTaskEvents)
		r.Get("/tasks/{id}/runs", h.ListTaskRuns)
		r.Get("/tasks/{id}/context", h.GetContextPack)
		r.Post("/tasks/{id}/context", h.BuildContextPack)
		r.Post("/tasks/{id}/claim", h.ClaimTask)

		// Runs
		r.Post("/runs", h.StartRun)
		r.Get("/runs/{id}", h.GetRun)
		r.Post("/runs/{id}/cancel", h.CancelRun)
		r.Get("/runs/{id}/events", h.ListRunEvents)

		// LLM management (proxied to LiteLLM)
		r.Get("/llm/models", h.ListLLMModels)
		r.Post("/llm/models", h.AddLLMModel)
		r.Post("/llm/models/delete", h.DeleteLLMModel)
		r.Get("/llm/health", h.LLMHealth)
		r.Get("/llm/discover", h.DiscoverLLMModels)

		// Provider registries
		r.Get("/providers/git", h.ListGitProviders)
		r.Get("/providers/agent", h.ListAgentBackends)
		r.Get("/providers/spec", h.ListSpecProviders)
		r.Get("/providers/pm", h.ListPMProviders)

		// Backend health
		r.Get("/backends/health", h.CheckBackendHealth)

		// Parse repo URL
		r.Post("/parse-repo-url", h.ParseRepoURL)

		// Repo info (fetch metadata from remote hosting API)
		r.Get("/repos/info", h.FetchRepoInfo)

		// Policy profiles
		r.Get("/policies", h.ListPolicyProfiles)
		r.Post("/policies", h.CreatePolicyProfile)
		r.Get("/policies/{name}", h.GetPolicyProfile)
		r.Delete("/policies/{name}", h.DeletePolicyProfile)
		r.Post("/policies/{name}/evaluate", h.EvaluatePolicy)

		// Feature Decomposition (Meta-Agent)
		r.Post("/projects/{id}/decompose", h.DecomposeFeature)

		// Context-Optimized Feature Planning
		r.Post("/projects/{id}/plan-feature", h.PlanFeature)

		// Execution Plans (nested under projects)
		r.Post("/projects/{id}/plans", h.CreatePlan)
		r.Get("/projects/{id}/plans", h.ListPlans)

		// Execution Plans (direct access)
		r.Get("/plans/{id}", h.GetPlan)
		r.Post("/plans/{id}/start", h.StartPlan)
		r.Post("/plans/{id}/cancel", h.CancelPlan)
		r.Get("/plans/{id}/graph", h.GetPlanGraph)
		r.Post("/plans/{id}/steps/{stepId}/evaluate", h.EvaluateStep)

		// Modes
		r.Get("/modes", h.ListModes)
		r.Get("/modes/scenarios", h.ListScenarios)
		r.Get("/modes/tools", h.ListModeTools)
		r.Get("/modes/artifact-types", h.ListArtifactTypes)
		r.Get("/modes/{id}", h.GetMode)
		r.Post("/modes", h.CreateMode)
		r.Put("/modes/{id}", h.UpdateMode)
		r.Delete("/modes/{id}", h.DeleteMode)

		// Pipeline Templates
		r.Get("/pipelines", h.ListPipelines)
		r.Post("/pipelines", h.RegisterPipeline)
		r.Get("/pipelines/{id}", h.GetPipeline)
		r.Post("/pipelines/{id}/instantiate", h.InstantiatePipeline)

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
		r.Post("/scopes", h.CreateScope)
		r.Get("/scopes", h.ListScopes)
		r.Get("/scopes/{id}", h.GetScope)
		r.Put("/scopes/{id}", h.UpdateScope)
		r.Delete("/scopes/{id}", h.DeleteScope)
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

		// Cost aggregation
		r.Get("/costs", h.GlobalCostSummary)
		r.Get("/projects/{id}/costs", h.ProjectCostSummary)
		r.Get("/projects/{id}/costs/by-model", h.ProjectCostByModel)
		r.Get("/projects/{id}/costs/by-tool", h.ProjectCostByTool)
		r.Get("/projects/{id}/costs/daily", h.ProjectCostTimeSeries)
		r.Get("/projects/{id}/costs/runs", h.ProjectRecentRuns)
		r.Get("/runs/{id}/costs/by-tool", h.RunCostByTool)

		// Dashboard aggregation
		r.Get("/dashboard/stats", h.DashboardStats)
		r.Get("/dashboard/charts/cost-trend", h.DashboardCostTrend)
		r.Get("/dashboard/charts/run-outcomes", h.DashboardRunOutcomes)
		r.Get("/dashboard/charts/agent-performance", h.DashboardAgentPerformance)
		r.Get("/dashboard/charts/model-usage", h.DashboardModelUsage)
		r.Get("/dashboard/charts/cost-by-project", h.DashboardCostByProject)
		r.Get("/projects/{id}/health", h.ProjectHealth)

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

		// Trajectory (nested under runs)
		r.Get("/runs/{id}/trajectory", h.GetTrajectory)
		r.Get("/runs/{id}/trajectory/export", h.ExportTrajectory)

		// Tenants (admin only)
		r.Route("/tenants", func(r chi.Router) {
			r.Use(middleware.RequireRole(user.RoleAdmin))
			r.Get("/", h.ListTenants)
			r.Post("/", h.CreateTenant)
			r.Get("/{id}", h.GetTenant)
			r.Put("/{id}", h.UpdateTenant)
		})

		// Branch Protection Rules (nested under projects + direct access)
		r.Post("/projects/{id}/branch-rules", h.CreateBranchProtectionRule)
		r.Get("/projects/{id}/branch-rules", h.ListBranchProtectionRules)
		r.Get("/branch-rules/{id}", h.GetBranchProtectionRule)
		r.Put("/branch-rules/{id}", h.UpdateBranchProtectionRule)
		r.Delete("/branch-rules/{id}", h.DeleteBranchProtectionRule)

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

		// Review Policies & Reviews (Phase 12I)
		r.Get("/projects/{id}/review-policies", h.ListReviewPolicies)
		r.Post("/projects/{id}/review-policies", h.CreateReviewPolicy)
		r.Get("/review-policies/{id}", h.GetReviewPolicy)
		r.Put("/review-policies/{id}", h.UpdateReviewPolicy)
		r.Delete("/review-policies/{id}", h.DeleteReviewPolicy)
		r.Post("/review-policies/{id}/trigger", h.TriggerReview)
		r.Get("/projects/{id}/reviews", h.ListReviews)
		r.Get("/reviews/{id}", h.GetReviewHandler)

		// Bidirectional Sync (nested under projects)
		r.Post("/projects/{id}/roadmap/sync", h.SyncRoadmap)
		r.Post("/projects/{id}/roadmap/sync-to-file", h.SyncToSpecFile)

		// Settings
		r.Get("/settings", h.GetSettings)
		r.With(middleware.RequireRole(user.RoleAdmin)).Put("/settings", h.UpdateSettings)

		// Auth (public routes handled by middleware exemption)
		r.Post("/auth/login", h.Login)
		r.Post("/auth/refresh", h.Refresh)
		r.Get("/auth/setup-status", h.SetupStatus)
		r.Post("/auth/setup", h.InitialSetup)
		r.Post("/auth/forgot-password", h.RequestPasswordReset)
		r.Post("/auth/reset-password", h.ConfirmPasswordReset)
		r.Get("/auth/github", h.StartGitHubOAuth)
		r.Get("/auth/github/callback", h.GitHubOAuthCallback)

		// Auth (authenticated)
		r.Post("/auth/logout", h.Logout)
		r.Get("/auth/me", h.GetCurrentUser)
		r.Post("/auth/change-password", h.ChangePassword)
		r.Post("/auth/api-keys", h.CreateAPIKeyHandler)
		r.Get("/auth/api-keys", h.ListAPIKeysHandler)
		r.Delete("/auth/api-keys/{id}", h.DeleteAPIKeyHandler)

		// VCS Accounts
		r.Get("/vcs-accounts", h.ListVCSAccounts)
		r.Post("/vcs-accounts", h.CreateVCSAccount)
		r.Delete("/vcs-accounts/{id}", h.DeleteVCSAccount)
		r.Post("/vcs-accounts/{id}/test", h.TestVCSAccount)

		// LLM Keys
		r.Get("/llm-keys", h.ListLLMKeys)
		r.Post("/llm-keys", h.CreateLLMKey)
		r.Delete("/llm-keys/{id}", h.DeleteLLMKey)

		// Conversations
		r.Post("/projects/{id}/conversations", h.CreateConversation)
		r.Get("/projects/{id}/conversations", h.ListConversations)
		r.Get("/conversations/{id}", h.GetConversation)
		r.Delete("/conversations/{id}", h.DeleteConversation)
		r.Get("/conversations/{id}/messages", h.ListConversationMessages)
		r.Post("/conversations/{id}/messages", h.SendConversationMessage)
		r.Post("/conversations/{id}/stop", h.StopConversation)
		r.Get("/conversations/{id}/session", h.GetConversationSession)
		r.Post("/conversations/{id}/fork", h.ForkConversation)
		r.Post("/conversations/{id}/rewind", h.RewindConversation)

		// HITL (Human-in-the-Loop) Approval
		r.Post("/runs/{id}/approve/{callId}", h.ApproveToolCall)
		r.Post("/runs/{id}/revert/{callId}", h.RevertToolCall)

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

		// Users (admin only)
		r.Route("/users", func(r chi.Router) {
			r.Use(middleware.RequireRole(user.RoleAdmin))
			r.Get("/", h.ListUsersHandler)
			r.Post("/", h.CreateUserHandler)
			r.Put("/{id}", h.UpdateUserHandler)
			r.Delete("/{id}", h.DeleteUserHandler)
			r.Post("/{id}/force-password-change", h.AdminForcePasswordChange)
		})

		// Prompt Sections
		r.Get("/prompt-sections", h.ListPromptSections)
		r.Put("/prompt-sections", h.UpsertPromptSection)
		r.Delete("/prompt-sections/{id}", h.DeletePromptSection)
		r.Post("/prompt-sections/preview", h.PreviewPromptSections)

		// Dev tools (behind DEV_MODE env var)
		r.With(middleware.DevModeOnly).Post("/dev/benchmark", h.BenchmarkPrompt)

		// Benchmark Mode (Phase 20D — dev-mode only, requires APP_ENV=development)
		r.Route("/benchmarks", func(r chi.Router) {
			r.Use(middleware.DevModeOnly)
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
			r.Get("/leaderboard", h.BenchmarkLeaderboard)
			r.Get("/datasets", h.ListBenchmarkDatasets)
			r.Put("/suites/{id}", h.UpdateBenchmarkSuite)
			r.Post("/runs/{id}/analyze", h.AnalyzeBenchmarkRun)
		})

		// Model Registry (Phase 22)
		r.Get("/llm/available", h.AvailableLLMModels)
		r.Post("/llm/refresh", h.RefreshLLMModels)

		// Copilot Token Exchange (Phase 22A)
		r.Post("/copilot/exchange", h.HandleCopilotExchange)

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

		// Human Feedback (Phase 22D)
		r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/feedback/{run_id}/{call_id}", h.HandleFeedbackCallback)
		r.Get("/runs/{id}/feedback", h.ListFeedbackAudit)

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

		// A2A Management (Phase 27L + 27O)
		if h.A2A != nil {
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

		// Project Goals (Phase 28 — Goal Discovery)
		if h.GoalDiscovery != nil {
			r.Get("/projects/{id}/goals", h.ListProjectGoals)
			r.Post("/projects/{id}/goals", h.CreateProjectGoal)
			r.Post("/projects/{id}/goals/detect", h.DetectProjectGoals)
			r.Post("/projects/{id}/goals/ai-discover", h.AIDiscoverProjectGoals)
			r.Get("/goals/{id}", h.GetProjectGoal)
			r.Put("/goals/{id}", h.UpdateProjectGoal)
			r.Delete("/goals/{id}", h.DeleteProjectGoal)
		}

		// Quarantine (Phase 23B — admin only)
		r.Route("/quarantine", func(r chi.Router) {
			r.Use(middleware.RequireRole(user.RoleAdmin))
			r.Get("/", h.listQuarantinedMessages)
			r.Get("/stats", h.quarantineStats)
			r.Get("/{id}", h.getQuarantinedMessage)
			r.Post("/{id}/approve", h.approveQuarantinedMessage)
			r.Post("/{id}/reject", h.rejectQuarantinedMessage)
		})
	})
}
