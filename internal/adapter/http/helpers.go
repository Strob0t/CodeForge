package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// ---------------------------------------------------------------------------
// Request helpers
// ---------------------------------------------------------------------------

// readJSON decodes a JSON request body with a size limit.
func readJSON[T any](w http.ResponseWriter, r *http.Request, bodyLimit int64) (T, bool) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		if err.Error() == "http: request body too large" {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return v, false
	}
	return v, true
}

// urlParam is a short alias for chi.URLParam.
func urlParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}

// requireField writes a 400 error and returns false when value is empty.
func requireField(w http.ResponseWriter, value, fieldName string) bool {
	if value == "" {
		writeError(w, http.StatusBadRequest, fieldName+" is required")
		return false
	}
	return true
}

// sanitizeName validates a name is safe for use in file paths.
// It rejects names containing path separators, dots-prefix, or other traversal patterns.
func sanitizeName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 128 {
		return errors.New("name too long (max 128 chars)")
	}
	if strings.ContainsAny(name, `/\`) {
		return errors.New("name must not contain path separators")
	}
	if strings.Contains(name, "..") {
		return errors.New("name must not contain '..'")
	}
	if name[0] == '.' {
		return errors.New("name must not start with '.'")
	}
	cleaned := filepath.Clean(name)
	if cleaned != name {
		return errors.New("name contains invalid path characters")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeDomainError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, fallbackMsg)
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "resource was modified by another request")
	case errors.Is(err, domain.ErrValidation):
		msg := strings.TrimPrefix(err.Error(), domain.ErrValidation.Error()+": ")
		writeError(w, http.StatusBadRequest, msg)
	case strings.Contains(err.Error(), "invalid input syntax"):
		writeError(w, http.StatusBadRequest, "invalid identifier format")
	case strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "SQLSTATE 23505"):
		writeError(w, http.StatusConflict, "resource already exists")
	default:
		slog.Error("unhandled domain error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// writeInternalError logs the actual error server-side and returns a generic message to the client.
func writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
