package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
)

// listQuarantinedMessages handles GET /api/v1/quarantine?project_id=...&status=...&limit=...&offset=...
func (h *Handlers) listQuarantinedMessages(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	status := quarantine.Status(r.URL.Query().Get("status"))
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	msgs, err := h.Quarantine.List(r.Context(), projectID, status, limit, offset)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if msgs == nil {
		msgs = []*quarantine.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
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

	id := chi.URLParam(r, "id")
	req, ok := readJSON[struct {
		ReviewedBy string `json:"reviewed_by"`
		Note       string `json:"note"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if err := h.Quarantine.Approve(r.Context(), id, req.ReviewedBy, req.Note); err != nil {
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

	id := chi.URLParam(r, "id")
	req, ok := readJSON[struct {
		ReviewedBy string `json:"reviewed_by"`
		Note       string `json:"note"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if err := h.Quarantine.Reject(r.Context(), id, req.ReviewedBy, req.Note); err != nil {
		writeDomainError(w, err, "reject failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// quarantineStats handles GET /api/v1/quarantine/stats?project_id=...
func (h *Handlers) quarantineStats(w http.ResponseWriter, r *http.Request) {
	if h.Quarantine == nil {
		writeError(w, http.StatusServiceUnavailable, "quarantine not enabled")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

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

// queryInt reads an integer query parameter with a default value.
func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
