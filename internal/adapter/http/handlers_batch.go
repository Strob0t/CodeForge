package http

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

const maxBatchSize = 50

type batchRequest struct {
	IDs []string `json:"ids"`
}

type batchResultItem struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type batchStatusResultItem struct {
	ID     string             `json:"id"`
	OK     bool               `json:"ok"`
	Error  string             `json:"error,omitempty"`
	Status *project.GitStatus `json:"status,omitempty"`
}

// BatchDeleteProjects handles POST /api/v1/projects/batch/delete.
func (h *Handlers) BatchDeleteProjects(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[batchRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := validateBatchIDs(req.IDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	results := h.runBatch(r, req.IDs, func(ctx context.Context, id string) error {
		return h.Projects.Delete(ctx, id)
	})

	writeJSON(w, http.StatusOK, results)
}

// BatchPullProjects handles POST /api/v1/projects/batch/pull.
func (h *Handlers) BatchPullProjects(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[batchRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := validateBatchIDs(req.IDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	results := h.runBatch(r, req.IDs, func(ctx context.Context, id string) error {
		return h.Projects.Pull(ctx, id)
	})

	writeJSON(w, http.StatusOK, results)
}

// BatchStatusProjects handles POST /api/v1/projects/batch/status.
func (h *Handlers) BatchStatusProjects(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[batchRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}
	if err := validateBatchIDs(req.IDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	results := make([]batchStatusResultItem, len(req.IDs))
	var wg sync.WaitGroup
	for i, id := range req.IDs {
		wg.Add(1)
		go func(idx int, projID string) {
			defer wg.Done()
			status, err := h.Projects.Status(r.Context(), projID)
			if err != nil {
				results[idx] = batchStatusResultItem{ID: projID, OK: false, Error: "status check failed"}
				return
			}
			results[idx] = batchStatusResultItem{ID: projID, OK: true, Status: status}
		}(i, id)
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, results)
}

// validateBatchIDs checks that the ID list is non-empty, within limits, and has no blanks.
func validateBatchIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("ids list is empty")
	}
	if len(ids) > maxBatchSize {
		return fmt.Errorf("too many IDs: maximum is %d", maxBatchSize)
	}
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("empty ID in list")
		}
	}
	return nil
}

// runBatch executes an operation for each ID concurrently and returns results.
func (h *Handlers) runBatch(r *http.Request, ids []string, op func(ctx context.Context, id string) error) []batchResultItem {
	results := make([]batchResultItem, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, projID string) {
			defer wg.Done()
			if err := op(r.Context(), projID); err != nil {
				results[idx] = batchResultItem{ID: projID, OK: false, Error: "operation failed"}
				return
			}
			results[idx] = batchResultItem{ID: projID, OK: true}
		}(i, id)
	}
	wg.Wait()
	return results
}
