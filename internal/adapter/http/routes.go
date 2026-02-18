package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
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

		// RepoMap (nested under projects)
		r.Get("/projects/{id}/repomap", h.GetRepoMap)
		r.Post("/projects/{id}/repomap", h.GenerateRepoMap)

		// Retrieval (nested under projects)
		r.Post("/projects/{id}/search", h.SearchProject)
		r.Post("/projects/{id}/search/agent", h.AgentSearchProject)
		r.Post("/projects/{id}/index", h.IndexProject)
		r.Get("/projects/{id}/index", h.GetIndexStatus)

		// Cost aggregation
		r.Get("/costs", h.GlobalCostSummary)
		r.Get("/projects/{id}/costs", h.ProjectCostSummary)
		r.Get("/projects/{id}/costs/by-model", h.ProjectCostByModel)
		r.Get("/projects/{id}/costs/daily", h.ProjectCostTimeSeries)
		r.Get("/projects/{id}/costs/runs", h.ProjectRecentRuns)
	})
}
