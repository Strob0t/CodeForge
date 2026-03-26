package messagequeue

// --- Context payloads (Phase 5D) ---

// ContextEntryPayload represents a single context entry in a NATS message.
type ContextEntryPayload struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Tokens   int    `json:"tokens"`
	Priority int    `json:"priority"`
}

// SharedContextUpdatedPayload notifies that a team's shared context has changed.
type SharedContextUpdatedPayload struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
	Author    string `json:"author"`
	Version   int    `json:"version"`
}

// --- Context re-ranking payloads (Phase 3 — Context Intelligence) ---

// ContextRerankRequestPayload is sent from Go to Python for LLM-based re-ranking.
type ContextRerankRequestPayload struct {
	RequestID string                      `json:"request_id"`
	ProjectID string                      `json:"project_id"`
	Query     string                      `json:"query"`
	Entries   []ContextRerankEntryPayload `json:"entries"`
	Model     string                      `json:"model,omitempty"`
}

// ContextRerankEntryPayload represents a single context entry for re-ranking.
type ContextRerankEntryPayload struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Content  string `json:"content"`
	Priority int    `json:"priority"`
	Tokens   int    `json:"tokens"`
}

// ContextRerankResultPayload is the response from Python after re-ranking.
type ContextRerankResultPayload struct {
	RequestID    string                      `json:"request_id"`
	Entries      []ContextRerankEntryPayload `json:"entries"`
	FallbackUsed bool                        `json:"fallback_used"`
	TokensIn     int64                       `json:"tokens_in"`
	TokensOut    int64                       `json:"tokens_out"`
	CostUSD      float64                     `json:"cost_usd"`
	Error        string                      `json:"error,omitempty"`
}

// --- RepoMap payloads (Phase 6A) ---

// RepoMapRequestPayload is the schema for repomap.generate.request messages.
type RepoMapRequestPayload struct {
	ProjectID     string   `json:"project_id"`
	WorkspacePath string   `json:"workspace_path"`
	TokenBudget   int      `json:"token_budget"`
	ActiveFiles   []string `json:"active_files"`
}

// RepoMapResultPayload is the schema for repomap.generate.result messages.
type RepoMapResultPayload struct {
	ProjectID   string   `json:"project_id"`
	MapText     string   `json:"map_text"`
	TokenCount  int      `json:"token_count"`
	FileCount   int      `json:"file_count"`
	SymbolCount int      `json:"symbol_count"`
	Languages   []string `json:"languages"`
	Error       string   `json:"error"`
}

// --- Retrieval payloads (Phase 6B) ---

// RetrievalIndexRequestPayload is the schema for retrieval.index.request messages.
type RetrievalIndexRequestPayload struct {
	ProjectID      string   `json:"project_id"`
	WorkspacePath  string   `json:"workspace_path"`
	EmbeddingModel string   `json:"embedding_model"`
	FileExtensions []string `json:"file_extensions,omitempty"`
}

// RetrievalIndexResultPayload is the schema for retrieval.index.result messages.
type RetrievalIndexResultPayload struct {
	ProjectID      string `json:"project_id"`
	Status         string `json:"status"` // "ready" or "error"
	FileCount      int    `json:"file_count"`
	ChunkCount     int    `json:"chunk_count"`
	EmbeddingModel string `json:"embedding_model"`
	Error          string `json:"error,omitempty"`
	Incremental    bool   `json:"incremental,omitempty"`
	FilesChanged   int    `json:"files_changed,omitempty"`
	FilesUnchanged int    `json:"files_unchanged,omitempty"`
}

// RetrievalSearchRequestPayload is the schema for retrieval.search.request messages.
type RetrievalSearchRequestPayload struct {
	ProjectID      string  `json:"project_id"`
	Query          string  `json:"query"`
	RequestID      string  `json:"request_id"`
	TopK           int     `json:"top_k"`
	BM25Weight     float64 `json:"bm25_weight"`
	SemanticWeight float64 `json:"semantic_weight"`
	ScopeID        string  `json:"scope_id,omitempty"`
}

// RetrievalSearchResultPayload is the schema for retrieval.search.result messages.
type RetrievalSearchResultPayload struct {
	ProjectID string                      `json:"project_id"`
	Query     string                      `json:"query"`
	RequestID string                      `json:"request_id"`
	Results   []RetrievalSearchHitPayload `json:"results"`
	Error     string                      `json:"error,omitempty"`
}

// RetrievalSearchHitPayload represents a single search result hit.
type RetrievalSearchHitPayload struct {
	Filepath     string  `json:"filepath"`
	StartLine    int     `json:"start_line"`
	EndLine      int     `json:"end_line"`
	Content      string  `json:"content"`
	Language     string  `json:"language"`
	SymbolName   string  `json:"symbol_name,omitempty"`
	Score        float64 `json:"score"`
	BM25Rank     int     `json:"bm25_rank"`
	SemanticRank int     `json:"semantic_rank"`
	ProjectID    string  `json:"project_id,omitempty"`
}

// --- Retrieval Sub-Agent payloads (Phase 6C) ---

// SubAgentSearchRequestPayload is the schema for retrieval.subagent.request messages.
type SubAgentSearchRequestPayload struct {
	ProjectID       string `json:"project_id"`
	Query           string `json:"query"`
	RequestID       string `json:"request_id"`
	TopK            int    `json:"top_k"`
	MaxQueries      int    `json:"max_queries"`
	Model           string `json:"model"`
	Rerank          bool   `json:"rerank"`
	ScopeID         string `json:"scope_id,omitempty"`
	ExpansionPrompt string `json:"expansion_prompt,omitempty"`
}

// SubAgentSearchResultPayload is the schema for retrieval.subagent.result messages.
type SubAgentSearchResultPayload struct {
	ProjectID       string                      `json:"project_id"`
	Query           string                      `json:"query"`
	RequestID       string                      `json:"request_id"`
	Results         []RetrievalSearchHitPayload `json:"results"`
	ExpandedQueries []string                    `json:"expanded_queries"`
	TotalCandidates int                         `json:"total_candidates"`
	Error           string                      `json:"error,omitempty"`
	Model           string                      `json:"model,omitempty"`
	TokensIn        int64                       `json:"tokens_in,omitempty"`
	TokensOut       int64                       `json:"tokens_out,omitempty"`
	CostUSD         float64                     `json:"cost_usd,omitempty"`
}

// --- GraphRAG payloads (Phase 6D) ---

// GraphBuildRequestPayload is the schema for graph.build.request messages.
type GraphBuildRequestPayload struct {
	ProjectID     string `json:"project_id"`
	WorkspacePath string `json:"workspace_path"`
	ScopeID       string `json:"scope_id,omitempty"`
}

// GraphBuildResultPayload is the schema for graph.build.result messages.
type GraphBuildResultPayload struct {
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"` // "ready" or "error"
	NodeCount int      `json:"node_count"`
	EdgeCount int      `json:"edge_count"`
	Languages []string `json:"languages"`
	Error     string   `json:"error,omitempty"`
}

// GraphSearchRequestPayload is the schema for graph.search.request messages.
type GraphSearchRequestPayload struct {
	ProjectID   string   `json:"project_id"`
	RequestID   string   `json:"request_id"`
	SeedSymbols []string `json:"seed_symbols"`
	MaxHops     int      `json:"max_hops"`
	TopK        int      `json:"top_k"`
	ScopeID     string   `json:"scope_id,omitempty"`
}

// GraphSearchHitPayload represents a single graph search result hit.
type GraphSearchHitPayload struct {
	Filepath   string   `json:"filepath"`
	SymbolName string   `json:"symbol_name"`
	Kind       string   `json:"kind"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
	Distance   int      `json:"distance"`
	Score      float64  `json:"score"`
	EdgePath   []string `json:"edge_path"`
	ProjectID  string   `json:"project_id,omitempty"`
}

// GraphSearchResultPayload is the schema for graph.search.result messages.
type GraphSearchResultPayload struct {
	ProjectID string                  `json:"project_id"`
	RequestID string                  `json:"request_id"`
	Results   []GraphSearchHitPayload `json:"results"`
	Error     string                  `json:"error,omitempty"`
}
