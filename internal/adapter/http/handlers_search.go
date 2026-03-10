package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/service"
)

type conversationSearchRequest struct {
	Query      string   `json:"query"`
	ProjectIDs []string `json:"project_ids,omitempty"`
	Limit      int      `json:"limit,omitempty"` // default 20, max 100
}

type conversationSearchResponse struct {
	Query   string                             `json:"query"`
	Total   int                                `json:"total"`
	Results []service.ConversationSearchResult `json:"results"`
}

// SearchConversations handles POST /api/v1/search/conversations.
// Full-text searches conversation messages across projects.
func (h *Handlers) SearchConversations(w http.ResponseWriter, r *http.Request) {
	req, ok := readJSON[conversationSearchRequest](w, r, h.Limits.MaxRequestBodySize)
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

	messages, err := h.Conversations.SearchMessages(r.Context(), req.Query, req.ProjectIDs, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed: "+err.Error())
		return
	}

	results := make([]service.ConversationSearchResult, len(messages))
	for i := range messages {
		results[i] = service.ConversationSearchResult{
			ConversationID: messages[i].ConversationID,
			MessageID:      messages[i].ID,
			Role:           messages[i].Role,
			Content:        messages[i].Content,
			Model:          messages[i].Model,
			CreatedAt:      messages[i].CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, conversationSearchResponse{
		Query:   req.Query,
		Total:   len(results),
		Results: results,
	})
}

type globalSearchRequest struct {
	Query      string   `json:"query"`
	ProjectIDs []string `json:"project_ids,omitempty"` // empty = all tenant projects
	Limit      int      `json:"limit,omitempty"`       // default 20, max 100
}

type globalSearchResponse struct {
	Query   string                       `json:"query"`
	Total   int                          `json:"total"`
	Results []service.GlobalSearchResult `json:"results"`
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

	results, err := h.Retrieval.GlobalSearch(r.Context(), req.Query, req.ProjectIDs, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, globalSearchResponse{
		Query:   req.Query,
		Total:   len(results),
		Results: results,
	})
}
