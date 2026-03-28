package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// setConsentRequest is the JSON body for PUT /me/consent/{purposeID}.
type setConsentRequest struct {
	Granted bool `json:"granted"`
}

// ListConsentPurposes handles GET /api/v1/me/consent/purposes
// Returns all consent purposes defined for the current tenant.
func (h *Handlers) ListConsentPurposes(w http.ResponseWriter, r *http.Request) {
	purposes, err := h.Consent.ListPurposes(r.Context())
	if err != nil {
		writeDomainError(w, err, "failed to list consent purposes")
		return
	}
	writeJSON(w, http.StatusOK, purposes)
}

// GetMyConsentStatus handles GET /api/v1/me/consent
// Returns the current consent state for the authenticated user.
func (h *Handlers) GetMyConsentStatus(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	statuses, err := h.Consent.GetStatus(r.Context(), u.ID)
	if err != nil {
		writeDomainError(w, err, "failed to get consent status")
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

// SetMyConsent handles PUT /api/v1/me/consent/{purposeID}
// Records the authenticated user granting or withdrawing consent for a purpose.
func (h *Handlers) SetMyConsent(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	purposeID := urlParam(r, "purposeID")
	if !requireField(w, purposeID, "purpose id") {
		return
	}

	body, ok := readJSON[setConsentRequest](w, r, 1024)
	if !ok {
		return
	}

	record := &database.ConsentRecord{
		UserID:    u.ID,
		PurposeID: purposeID,
		Granted:   body.Granted,
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}

	if err := h.Consent.SetConsent(r.Context(), record); err != nil {
		writeDomainError(w, err, "failed to set consent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
