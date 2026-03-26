package messagequeue

// --- Review/Refactor payloads (Phase 31) ---

// ReviewTriggerRequestPayload is published to trigger a review run on a project.
type ReviewTriggerRequestPayload struct {
	ProjectID string `json:"project_id"`
	TenantID  string `json:"tenant_id"`
	CommitSHA string `json:"commit_sha"`
	Source    string `json:"source"`
}

// ReviewTriggerCompletePayload is the schema for review.trigger.complete messages.
// Python publishes this after a review trigger has been processed.
type ReviewTriggerCompletePayload struct {
	ProjectID string `json:"project_id"`
	TenantID  string `json:"tenant_id"`
	CommitSHA string `json:"commit_sha"`
	Status    string `json:"status"`
	RunID     string `json:"run_id"`
}

// ReviewBoundaryEntry describes a single detected layer boundary.
type ReviewBoundaryEntry struct {
	Path         string `json:"path"`
	Type         string `json:"type"`
	Counterpart  string `json:"counterpart,omitempty"`
	AutoDetected bool   `json:"auto_detected"`
}

// ReviewBoundaryAnalyzedPayload is published when layer boundaries have been detected.
type ReviewBoundaryAnalyzedPayload struct {
	ProjectID  string                `json:"project_id"`
	TenantID   string                `json:"tenant_id"`
	Boundaries []ReviewBoundaryEntry `json:"boundaries"`
}

// ReviewDiffStats summarizes the diff associated with a review run.
type ReviewDiffStats struct {
	FilesChanged int  `json:"files_changed"`
	LinesAdded   int  `json:"lines_added"`
	LinesRemoved int  `json:"lines_removed"`
	CrossLayer   bool `json:"cross_layer"`
	Structural   bool `json:"structural"`
}

// ReviewApprovalRequiredPayload is published when a review run requires human approval.
type ReviewApprovalRequiredPayload struct {
	RunID       string          `json:"run_id"`
	ProjectID   string          `json:"project_id"`
	TenantID    string          `json:"tenant_id"`
	DiffStats   ReviewDiffStats `json:"diff_stats"`
	ImpactLevel string          `json:"impact_level"`
}

// ReviewApprovalResponsePayload carries the human approval decision back to the worker.
type ReviewApprovalResponsePayload struct {
	RunID    string `json:"run_id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}
