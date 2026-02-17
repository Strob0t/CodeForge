package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MountRoutes registers all API routes on the given chi router.
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

		// LLM management (proxied to LiteLLM)
		r.Get("/llm/models", h.ListLLMModels)
		r.Post("/llm/models", h.AddLLMModel)
		r.Post("/llm/models/delete", h.DeleteLLMModel)
		r.Get("/llm/health", h.LLMHealth)

		// Provider registries
		r.Get("/providers/git", h.ListGitProviders)
		r.Get("/providers/agent", h.ListAgentBackends)
	})
}
