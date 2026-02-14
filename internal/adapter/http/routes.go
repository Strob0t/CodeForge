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

		// Tasks (nested under projects)
		r.Post("/projects/{id}/tasks", h.CreateTask)
		r.Get("/projects/{id}/tasks", h.ListTasks)

		// Tasks (direct access)
		r.Get("/tasks/{id}", h.GetTask)

		// Provider registries
		r.Get("/providers/git", h.ListGitProviders)
		r.Get("/providers/agent", h.ListAgentBackends)
	})
}
