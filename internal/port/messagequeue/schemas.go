package messagequeue

// TaskCreatedPayload is the schema for tasks.created messages.
type TaskCreatedPayload struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
}

// TaskResultPayload is the schema for tasks.result messages.
type TaskResultPayload struct {
	TaskID    string   `json:"task_id"`
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"`
	Output    string   `json:"output"`
	Files     []string `json:"files"`
	Error     string   `json:"error"`
	TokensIn  int      `json:"tokens_in"`
	TokensOut int      `json:"tokens_out"`
	CostUSD   float64  `json:"cost_usd"`
}

// TaskOutputPayload is the schema for tasks.output messages.
type TaskOutputPayload struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	AgentID   string `json:"agent_id"`
	Line      string `json:"line"`
}

// TaskCancelPayload is the schema for tasks.cancel messages.
type TaskCancelPayload struct {
	TaskID string `json:"task_id"`
}

// AgentStatusPayload is the schema for agents.status messages.
type AgentStatusPayload struct {
	AgentID   string `json:"agent_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
}

// --- Run protocol payloads (Phase 4B) ---

// ModePayload carries agent mode metadata to the Python worker.
type ModePayload struct {
	ID               string   `json:"id"`
	PromptPrefix     string   `json:"prompt_prefix"`
	Tools            []string `json:"tools"`
	DeniedTools      []string `json:"denied_tools,omitempty"`
	DeniedActions    []string `json:"denied_actions,omitempty"`
	RequiredArtifact string   `json:"required_artifact,omitempty"`
	LLMScenario      string   `json:"llm_scenario,omitempty"`
}

// RunStartPayload is the schema for runs.start messages.
type RunStartPayload struct {
	RunID         string                `json:"run_id"`
	TaskID        string                `json:"task_id"`
	ProjectID     string                `json:"project_id"`
	AgentID       string                `json:"agent_id"`
	Prompt        string                `json:"prompt"`
	PolicyProfile string                `json:"policy_profile"`
	ExecMode      string                `json:"exec_mode"`
	DeliverMode   string                `json:"deliver_mode,omitempty"`
	Mode          *ModePayload          `json:"mode,omitempty"`
	Config        map[string]string     `json:"config,omitempty"`
	Termination   TerminationPayload    `json:"termination"`
	Context       []ContextEntryPayload `json:"context,omitempty"` // Pre-packed context entries (Phase 5D)
}

// TerminationPayload carries the termination limits for a run.
type TerminationPayload struct {
	MaxSteps       int     `json:"max_steps"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	MaxCost        float64 `json:"max_cost"`
}

// ToolCallRequestPayload is the schema for runs.toolcall.request messages.
type ToolCallRequestPayload struct {
	RunID   string `json:"run_id"`
	CallID  string `json:"call_id"`
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

// ToolCallResponsePayload is the schema for runs.toolcall.response messages.
type ToolCallResponsePayload struct {
	RunID       string `json:"run_id"`
	CallID      string `json:"call_id"`
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
	ExecMode    string `json:"exec_mode,omitempty"`
	ContainerID string `json:"container_id,omitempty"`
}

// ToolCallResultPayload is the schema for runs.toolcall.result messages.
type ToolCallResultPayload struct {
	RunID     string  `json:"run_id"`
	CallID    string  `json:"call_id"`
	Tool      string  `json:"tool"`
	Success   bool    `json:"success"`
	Output    string  `json:"output"`
	Error     string  `json:"error"`
	CostUSD   float64 `json:"cost_usd"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	Model     string  `json:"model,omitempty"`
}

// RunCompletePayload is the schema for runs.complete messages.
type RunCompletePayload struct {
	RunID     string  `json:"run_id"`
	TaskID    string  `json:"task_id"`
	ProjectID string  `json:"project_id"`
	Status    string  `json:"status"`
	Output    string  `json:"output"`
	Error     string  `json:"error"`
	CostUSD   float64 `json:"cost_usd"`
	StepCount int     `json:"step_count"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	Model     string  `json:"model,omitempty"`
}

// RunOutputPayload is the schema for runs.output messages.
type RunOutputPayload struct {
	RunID  string `json:"run_id"`
	TaskID string `json:"task_id"`
	Line   string `json:"line"`
	Stream string `json:"stream"`
}

// --- Heartbeat payload (Phase 3C) ---

// RunHeartbeatPayload is the schema for runs.heartbeat messages.
type RunHeartbeatPayload struct {
	RunID     string `json:"run_id"`
	Timestamp string `json:"timestamp"`
}

// --- Quality Gate payloads (Phase 4C) ---

// QualityGateRequestPayload is published to request test/lint execution.
type QualityGateRequestPayload struct {
	RunID         string `json:"run_id"`
	ProjectID     string `json:"project_id"`
	WorkspacePath string `json:"workspace_path"`
	RunTests      bool   `json:"run_tests"`
	RunLint       bool   `json:"run_lint"`
	TestCommand   string `json:"test_command,omitempty"`
	LintCommand   string `json:"lint_command,omitempty"`
}

// QualityGateResultPayload is published with the outcome of a quality gate execution.
type QualityGateResultPayload struct {
	RunID       string `json:"run_id"`
	TestsPassed *bool  `json:"tests_passed,omitempty"`
	LintPassed  *bool  `json:"lint_passed,omitempty"`
	TestOutput  string `json:"test_output,omitempty"`
	LintOutput  string `json:"lint_output,omitempty"`
	Error       string `json:"error,omitempty"`
}

// --- Context payloads (Phase 5D) ---

// ContextEntryPayload represents a single context entry in a NATS message.
type ContextEntryPayload struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Tokens   int    `json:"tokens"`
	Priority int    `json:"priority"`
}

// ContextPackedPayload notifies the worker that a context pack is available for a run.
type ContextPackedPayload struct {
	RunID     string                `json:"run_id"`
	TaskID    string                `json:"task_id"`
	ProjectID string                `json:"project_id"`
	Entries   []ContextEntryPayload `json:"entries"`
}

// SharedContextUpdatedPayload notifies that a team's shared context has changed.
type SharedContextUpdatedPayload struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id,omitempty"`
	Key       string `json:"key"`
	Author    string `json:"author"`
	Version   int    `json:"version"`
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
