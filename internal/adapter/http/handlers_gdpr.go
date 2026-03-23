package http

import (
	"net/http"
)

// ExportUserData handles POST /api/v1/users/{id}/export
// Returns all personal data for the specified user (GDPR Article 20).
func (h *Handlers) ExportUserData(w http.ResponseWriter, r *http.Request) {
	userID := urlParam(r, "id")
	if !requireField(w, userID, "user id") {
		return
	}

	export, err := h.GDPR.ExportUserData(r.Context(), userID)
	if err != nil {
		writeDomainError(w, err, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, export)
}

// DeleteUserData handles DELETE /api/v1/users/{id}/data
// Cascade-deletes all personal data for the specified user (GDPR Article 17).
func (h *Handlers) DeleteUserData(w http.ResponseWriter, r *http.Request) {
	userID := urlParam(r, "id")
	if !requireField(w, userID, "user id") {
		return
	}

	if err := h.GDPR.DeleteUserData(r.Context(), userID); err != nil {
		writeDomainError(w, err, "user not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
