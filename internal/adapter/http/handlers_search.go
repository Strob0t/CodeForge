package http

import (
	"net/http"
	"sort"
	"sync"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

type globalSearchRequest struct {
	Query      string   `json:"query"`
	ProjectIDs []string `json:"project_ids,omitempty"` // empty = all tenant projects
	Limit      int      `json:"limit,omitempty"`       // default 20, max 100
}

type globalSearchResponse struct {
	Query   string                                   `json:"query"`
	Total   int                                      `json:"total"`
	Results []messagequeue.RetrievalSearchHitPayload `json:"results"`
}

// GlobalSearch handles POST /api/v1/search.
// Searches across one or more projects, merges and ranks results by score.
func (h *Handlers) GlobalSearch(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[globalSearchRequest](w, r, h.Limits.MaxRequestBodySize)
	if !ok {
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Determine which projects to search.
	projectIDs := req.ProjectIDs
	if len(projectIDs) == 0 {
		projects, err := h.Projects.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list projects")
			return
		}
		for i := range projects {
			projectIDs = append(projectIDs, projects[i].ID)
		}
	}

	if len(projectIDs) == 0 {
		writeJSON(w, http.StatusOK, globalSearchResponse{Query: req.Query, Results: []messagequeue.RetrievalSearchHitPayload{}})
		return
	}

	// Search each project concurrently.
	type projectResult struct {
		hits []messagequeue.RetrievalSearchHitPayload
	}

	results := make([]projectResult, len(projectIDs))
	var wg sync.WaitGroup

	for i, pid := range projectIDs {
		wg.Add(1)
		go func(idx int, projectID string) {
			defer wg.Done()
			resp, err := h.Retrieval.SearchSync(r.Context(), projectID, req.Query, limit, 0.5, 0.5)
			if err != nil || resp == nil {
				return // skip failed projects silently
			}
			// Tag each hit with its project ID.
			for j := range resp.Results {
				resp.Results[j].ProjectID = projectID
			}
			results[idx] = projectResult{hits: resp.Results}
		}(i, pid)
	}
	wg.Wait()

	// Merge all results.
	var allHits []messagequeue.RetrievalSearchHitPayload
	for _, res := range results {
		allHits = append(allHits, res.hits...)
	}

	// Sort by score descending.
	sort.Slice(allHits, func(i, j int) bool {
		return allHits[i].Score > allHits[j].Score
	})

	// Apply limit.
	if len(allHits) > limit {
		allHits = allHits[:limit]
	}

	if allHits == nil {
		allHits = []messagequeue.RetrievalSearchHitPayload{}
	}

	writeJSON(w, http.StatusOK, globalSearchResponse{
		Query:   req.Query,
		Total:   len(allHits),
		Results: allHits,
	})
}
