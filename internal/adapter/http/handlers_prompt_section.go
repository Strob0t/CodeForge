package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/service"
)

// ListPromptSections handles GET /api/v1/prompt-sections?scope=global
func (h *Handlers) ListPromptSections(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}
	rows, err := h.PromptSections.List(r.Context(), scope)
	if err != nil {
		writeDomainError(w, err, "failed to list prompt sections")
		return
	}
	if rows == nil {
		rows = []prompt.SectionRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// UpsertPromptSection handles PUT /api/v1/prompt-sections
func (h *Handlers) UpsertPromptSection(w http.ResponseWriter, r *http.Request) {
	row, ok := readJSON[prompt.SectionRow](w, r)
	if !ok {
		return
	}
	if row.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if row.Scope == "" {
		row.Scope = "global"
	}
	if row.Merge == "" {
		row.Merge = "replace"
	}
	if err := h.PromptSections.Upsert(r.Context(), &row); err != nil {
		writeDomainError(w, err, "failed to upsert prompt section")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// DeletePromptSection handles DELETE /api/v1/prompt-sections/{id}
func (h *Handlers) DeletePromptSection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.PromptSections.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err, "failed to delete prompt section")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// previewRequest is the body for POST /api/v1/prompt-sections/preview.
type previewRequest struct {
	Sections []service.PromptSection `json:"sections"`
	Budget   int                     `json:"budget"`
}

// previewResponse is the response for POST /api/v1/prompt-sections/preview.
type previewResponse struct {
	Text        string                  `json:"text"`
	Sections    []service.PromptSection `json:"sections"`
	TotalTokens int                     `json:"total_tokens"`
}

// PreviewPromptSections handles POST /api/v1/prompt-sections/preview
func (h *Handlers) PreviewPromptSections(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[previewRequest](w, r)
	if !ok {
		return
	}
	if req.Budget <= 0 {
		req.Budget = service.DefaultModePromptBudget
	}
	// Estimate tokens for each section if not already set.
	for i := range req.Sections {
		if req.Sections[i].Tokens == 0 && req.Sections[i].Text != "" {
			req.Sections[i].Tokens = cfcontext.EstimateTokens(req.Sections[i].Text)
		}
	}
	text, sections := h.PromptSections.Preview(req.Sections, req.Budget)
	total := 0
	for i := range sections {
		total += sections[i].Tokens
	}
	writeJSON(w, http.StatusOK, previewResponse{
		Text:        text,
		Sections:    sections,
		TotalTokens: total,
	})
}
