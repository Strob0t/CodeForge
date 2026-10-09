package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

// FIX-097: Quarantine handlers are intentionally unexported (lowercase).
// They are admin-only and registered via method values in routes.go within
// the same package. No external package needs to reference them directly.

// listQuarantinedMessages handles GET /api/v1/quarantine?project_id=...&status=...&limit=...&offset=...
// Without project_id it lists the messages without project (inbound A2A prompts).
func (h *Handlers) listQuarantinedMessages(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")

	status := quarantine.Status(r.URL.Query().Get("status"))
	limit, offset := parsePagination(r, 50)

	msgs, err := h.Quarantine.List(r.Context(), projectID, status, limit, offset)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSONList(w, http.StatusOK, msgs)
}

// getQuarantinedMessage handles GET /api/v1/quarantine/{id}
func (h *Handlers) getQuarantinedMessage(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	id := chi.URLParam(r, "id")
	msg, err := h.Quarantine.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err, "quarantined message not found")
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

// approveQuarantinedMessage handles POST /api/v1/quarantine/{id}/approve
func (h *Handlers) approveQuarantinedMessage(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	review, ok := h.readQuarantineReview(w, r)
	if !ok {
		return
	}
	if err := h.Quarantine.Approve(r.Context(), chi.URLParam(r, "id"), review); err != nil {
		writeDomainError(w, err, "approve failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// rejectQuarantinedMessage handles POST /api/v1/quarantine/{id}/reject
func (h *Handlers) rejectQuarantinedMessage(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	review, ok := h.readQuarantineReview(w, r)
	if !ok {
		return
	}
	if err := h.Quarantine.Reject(r.Context(), chi.URLParam(r, "id"), review); err != nil {
		writeDomainError(w, err, "reject failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// quarantineStats handles GET /api/v1/quarantine/stats?project_id=...
// Without project_id it counts the messages without project (inbound A2A prompts).
func (h *Handlers) quarantineStats(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")

	// Compute stats by querying each status.
	var stats quarantine.Stats
	for _, s := range []quarantine.Status{quarantine.StatusPending, quarantine.StatusApproved, quarantine.StatusRejected, quarantine.StatusExpired} {
		msgs, err := h.Quarantine.List(r.Context(), projectID, s, 0, 0)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		count := len(msgs)
		switch s {
		case quarantine.StatusPending:
			stats.Pending = count
		case quarantine.StatusApproved:
			stats.Approved = count
		case quarantine.StatusRejected:
			stats.Rejected = count
		case quarantine.StatusExpired:
			stats.Expired = count
		}
	}

	writeJSON(w, http.StatusOK, stats)
}

// readQuarantineReview reads the note of a quarantine review; the reviewer is
// the logged-in user (ID and name at the time), never a name from the body.
func (h *Handlers) readQuarantineReview(w http.ResponseWriter, r *http.Request) (*quarantine.Review, bool) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	req, ok := readJSON[struct {
		Note string `json:"note"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return nil, false
	}
	name := u.Name
	if name == "" {
		name = u.Email
	}
	return &quarantine.Review{ReviewerID: u.ID, ReviewerName: name, Note: req.Note}, true
}
