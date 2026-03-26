package http

import (
	"fmt"
	"net/http"

	"github.com/Strob0t/CodeForge/internal/middleware"
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

// ExportMyData handles GET /api/v1/me/export
// Self-service GDPR data export — returns all personal data for the
// authenticated user without requiring admin role (Art. 15, 20).
func (h *Handlers) ExportMyData(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	export, err := h.GDPR.ExportUserData(r.Context(), u.ID)
	if err != nil {
		writeDomainError(w, err, "export failed")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="codeforge-export-%s.json"`, u.ID))
	writeJSON(w, http.StatusOK, export)
}

// DeleteMyData handles DELETE /api/v1/me/data
// Self-service GDPR data deletion — cascade-deletes all personal data for the
// authenticated user without requiring admin role (Art. 17).
func (h *Handlers) DeleteMyData(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.GDPR.DeleteUserData(r.Context(), u.ID); err != nil {
		writeDomainError(w, err, "deletion failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
