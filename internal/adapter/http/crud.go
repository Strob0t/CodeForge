package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// Generic CRUD handler factories
// ---------------------------------------------------------------------------

// handleList creates a handler that lists resources and returns JSON.
func handleList[T any](listFn func(ctx context.Context) ([]T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := listFn(r.Context())
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if items == nil {
			items = []T{}
		}
		writeJSON(w, http.StatusOK, items)
	}
}

// handleListByParam creates a handler that lists resources scoped by a URL parameter.
func handleListByParam[T any](param string, listFn func(ctx context.Context, paramVal string) ([]T, error), notFoundMsg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		val := chi.URLParam(r, param)
		items, err := listFn(r.Context(), val)
		if err != nil {
			writeDomainError(w, err, notFoundMsg)
			return
		}
		if items == nil {
			items = []T{}
		}
		writeJSON(w, http.StatusOK, items)
	}
}

// handleGet creates a handler that retrieves a single resource by URL param "id".
func handleGet[T any](getFn func(ctx context.Context, id string) (*T, error), notFoundMsg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		item, err := getFn(r.Context(), id)
		if err != nil {
			writeDomainError(w, err, notFoundMsg)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

// handleCreate creates a handler that decodes a JSON body and creates a resource.
func handleCreate[Req any, Res any](bodyLimit int64, createFn func(ctx context.Context, req *Req) (*Res, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := readJSON[Req](w, r, bodyLimit)
		if !ok {
			return
		}
		res, err := createFn(r.Context(), &req)
		if err != nil {
			writeDomainError(w, err, "creation failed")
			return
		}
		writeJSON(w, http.StatusCreated, res)
	}
}

// handleUpdate creates a handler that decodes a JSON body and updates a resource by URL param "id".
func handleUpdate[Req any, Res any](bodyLimit int64, updateFn func(ctx context.Context, id string, req Req) (*Res, error), notFoundMsg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		req, ok := readJSON[Req](w, r, bodyLimit)
		if !ok {
			return
		}
		res, err := updateFn(r.Context(), id, req)
		if err != nil {
			writeDomainError(w, err, notFoundMsg)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// handleDelete creates a handler that deletes a resource by URL param "id".
func handleDelete(deleteFn func(ctx context.Context, id string) error, notFoundMsg string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deleteFn(r.Context(), id); err != nil {
			writeDomainError(w, err, notFoundMsg)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
