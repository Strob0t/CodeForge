package http

import (
	"net/http"
)

// ListCommands handles GET /api/v1/commands.
// It returns all available chat slash commands.
func (h *Handlers) ListCommands(w http.ResponseWriter, r *http.Request) {
	cmds := h.Commands.ListCommands(r.Context())
	writeJSONList(w, http.StatusOK, cmds)
}
