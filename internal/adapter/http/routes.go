package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

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
func MountRoutes(r chi.Router, h *Handlers) {
	r.Route("/api/v1", func(r chi.Router) {
		// Version
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"0.1.0"}`))
		})

		// Projects
		r.Get("/projects", h.ListProjects)
		r.Post("/projects", h.CreateProject)
		r.Get("/projects/{id}", h.GetProject)
		r.Delete("/projects/{id}", h.DeleteProject)

		// Git operations (nested under projects)
		r.Post("/projects/{id}/clone", h.CloneProject)
		r.Get("/projects/{id}/git/status", h.ProjectGitStatus)
		r.Post("/projects/{id}/git/pull", h.PullProject)
		r.Get("/projects/{id}/git/branches", h.ListProjectBranches)
		r.Post("/projects/{id}/git/checkout", h.CheckoutBranch)

		// Agents (nested under projects)
		r.Post("/projects/{id}/agents", h.CreateAgent)
		r.Get("/projects/{id}/agents", h.ListAgents)

		// Agents (direct access)
		r.Get("/agents/{id}", h.GetAgent)
		r.Delete("/agents/{id}", h.DeleteAgent)
		r.Post("/agents/{id}/dispatch", h.DispatchTask)
		r.Post("/agents/{id}/stop", h.StopAgentTask)

		// Tasks (nested under projects)
		r.Post("/projects/{id}/tasks", h.CreateTask)
		r.Get("/projects/{id}/tasks", h.ListTasks)

		// Tasks (direct access)
		r.Get("/tasks/{id}", h.GetTask)
		r.Get("/tasks/{id}/events", h.ListTaskEvents)
		r.Get("/tasks/{id}/runs", h.ListTaskRuns)
		r.Get("/tasks/{id}/context", h.GetContextPack)
		r.Post("/tasks/{id}/context", h.BuildContextPack)

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

		// Provider registries
		r.Get("/providers/git", h.ListGitProviders)
		r.Get("/providers/agent", h.ListAgentBackends)
		r.Get("/providers/spec", h.ListSpecProviders)
		r.Get("/providers/pm", h.ListPMProviders)

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

		// Agent Teams (nested under projects)
		r.Post("/projects/{id}/teams", h.CreateTeam)
		r.Get("/projects/{id}/teams", h.ListTeams)

		// Agent Teams (direct access)
		r.Get("/teams/{id}", h.GetTeam)
		r.Delete("/teams/{id}", h.DeleteTeam)

		// Shared Context (nested under teams)
		r.Get("/teams/{id}/shared-context", h.GetSharedContext)
		r.Post("/teams/{id}/shared-context", h.AddSharedContextItem)

		// Modes
		r.Get("/modes", h.ListModes)
		r.Get("/modes/{id}", h.GetMode)
		r.Post("/modes", h.CreateMode)

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

		// Cost aggregation
		r.Get("/costs", h.GlobalCostSummary)
		r.Get("/projects/{id}/costs", h.ProjectCostSummary)
		r.Get("/projects/{id}/costs/by-model", h.ProjectCostByModel)
		r.Get("/projects/{id}/costs/daily", h.ProjectCostTimeSeries)
		r.Get("/projects/{id}/costs/runs", h.ProjectRecentRuns)

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

		// Tenants
		r.Get("/tenants", h.ListTenants)
		r.Post("/tenants", h.CreateTenant)
		r.Get("/tenants/{id}", h.GetTenant)
		r.Put("/tenants/{id}", h.UpdateTenant)

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

		// VCS Webhooks
		r.Post("/webhooks/vcs/github", h.HandleGitHubWebhook)
		r.Post("/webhooks/vcs/gitlab", h.HandleGitLabWebhook)

		// PM Webhooks
		r.Post("/webhooks/pm/github", h.HandleGitHubIssueWebhook)
		r.Post("/webhooks/pm/gitlab", h.HandleGitLabIssueWebhook)
		r.Post("/webhooks/pm/plane", h.HandlePlaneWebhook)

		// Bidirectional Sync (nested under projects)
		r.Post("/projects/{id}/roadmap/sync", h.SyncRoadmap)

		// Auth (public routes handled by middleware exemption)
		r.Post("/auth/login", h.Login)
		r.Post("/auth/refresh", h.Refresh)

		// Auth (authenticated)
		r.Post("/auth/logout", h.Logout)
		r.Get("/auth/me", h.GetCurrentUser)
		r.Post("/auth/change-password", h.ChangePassword)
		r.Post("/auth/api-keys", h.CreateAPIKeyHandler)
		r.Get("/auth/api-keys", h.ListAPIKeysHandler)
		r.Delete("/auth/api-keys/{id}", h.DeleteAPIKeyHandler)

		// Users (admin only)
		r.Route("/users", func(r chi.Router) {
			r.Use(middleware.RequireRole(user.RoleAdmin))
			r.Get("/", h.ListUsersHandler)
			r.Post("/", h.CreateUserHandler)
			r.Put("/{id}", h.UpdateUserHandler)
			r.Delete("/{id}", h.DeleteUserHandler)
		})
	})
}
