package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ListFiles handles GET /api/v1/projects/{id}/files?path=.
func (h *Handlers) ListFiles(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	entries, err := h.Files.ListDirectory(r.Context(), projectID, path)
	if err != nil {
		writeDomainError(w, err, "list directory failed")
		return
	}
	writeJSONList(w, http.StatusOK, entries)
}

// ListTree handles GET /api/v1/projects/{id}/files/tree?max_entries=10000
func (h *Handlers) ListTree(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	maxEntries := queryParamInt(r, "max_entries", 10000)
	if maxEntries > 50000 {
		maxEntries = 50000
	}

	entries, err := h.Files.ListTree(r.Context(), projectID, maxEntries)
	if err != nil {
		writeDomainError(w, err, "list tree failed")
		return
	}
	writeJSONList(w, http.StatusOK, entries)
}

// ReadFile handles GET /api/v1/projects/{id}/files/content?path=src/main.go
func (h *Handlers) ReadFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	content, err := h.Files.ReadFile(r.Context(), projectID, path)
	if err != nil {
		writeDomainError(w, err, "read file failed")
		return
	}
	writeJSON(w, http.StatusOK, content)
}

// WriteFile handles PUT /api/v1/projects/{id}/files/content
func (h *Handlers) WriteFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	body, ok := readJSON[struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	req = body

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := h.Files.WriteFile(r.Context(), projectID, req.Path, req.Content); err != nil {
		writeDomainError(w, err, "write file failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteFile handles DELETE /api/v1/projects/{id}/files?path=src/old.go
func (h *Handlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	if err := h.Files.DeleteFile(r.Context(), projectID, path); err != nil {
		writeDomainError(w, err, "delete file failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RenameFile handles PATCH /api/v1/projects/{id}/files/rename
func (h *Handlers) RenameFile(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	body, ok := readJSON[struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if body.OldPath == "" || body.NewPath == "" {
		writeError(w, http.StatusBadRequest, "old_path and new_path are required")
		return
	}

	if err := h.Files.RenameFile(r.Context(), projectID, body.OldPath, body.NewPath); err != nil {
		writeDomainError(w, err, "rename file failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
