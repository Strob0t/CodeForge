package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
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
func urlParam(r *http.Request, name string) string { //nolint:unparam // name varies by endpoint
	return chi.URLParam(r, name)
}

// queryParamInt parses an integer query parameter with a positive-value constraint.
func queryParamInt(r *http.Request, name string, defaultVal int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
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

// validationErrorResponse represents a 422 Unprocessable Entity response with field-level detail.
type validationErrorResponse struct { //nolint:unused // available for handlers to adopt incrementally
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

// writeValidationError returns a 422 Unprocessable Entity with field-level error detail.
// Use for semantic validation failures (e.g., invalid field values that parsed correctly as JSON).
func writeValidationError(w http.ResponseWriter, field, message string) { //nolint:unused // available for handlers to adopt incrementally
	writeJSON(w, http.StatusUnprocessableEntity, validationErrorResponse{
		Error: message,
		Field: field,
	})
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

// parsePagination extracts limit and offset query parameters with safe defaults.
// limit is capped at maxLimit (100). offset defaults to 0.
func parsePagination(r *http.Request, maxLimit int) (limit, offset int) {
	limit = queryParamInt(r, "limit", maxLimit)
	if limit > maxLimit {
		limit = maxLimit
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr != "" {
		n, err := strconv.Atoi(offsetStr)
		if err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// applyPagination slices a result set by offset and limit for in-memory pagination.
func applyPagination[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}
	items = items[offset:]
	if limit < len(items) {
		items = items[:limit]
	}
	return items
}

// writeJSONList writes a JSON array response, converting nil slices to empty arrays.
func writeJSONList[T any](w http.ResponseWriter, status int, items []T) {
	if items == nil {
		items = []T{}
	}
	writeJSON(w, status, items)
}

// writeInternalError logs the actual error server-side and returns a generic message to the client.
func writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
