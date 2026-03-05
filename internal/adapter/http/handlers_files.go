package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/service"
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
	if entries == nil {
		entries = []service.FileEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// ListTree handles GET /api/v1/projects/{id}/files/tree?max_entries=10000
func (h *Handlers) ListTree(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	maxEntries := 10000
	if v := r.URL.Query().Get("max_entries"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50000 {
			maxEntries = n
		}
	}

	entries, err := h.Files.ListTree(r.Context(), projectID, maxEntries)
	if err != nil {
		writeDomainError(w, err, "list tree failed")
		return
	}
	if entries == nil {
		entries = []service.FileEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
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
