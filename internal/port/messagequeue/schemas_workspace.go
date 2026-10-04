package messagequeue

// WorkspaceDeleteRequestPayload is the schema for workspace.delete.request
// (KI-96 D11): the worker removes the deleted project's workspace as the
// tenant's tool UID. Delivered at least once; a workspace that is already
// gone counts as removed.
type WorkspaceDeleteRequestPayload struct {
	DeletionID    string `json:"deletion_id"`
	TenantID      string `json:"tenant_id"`
	ToolUID       int    `json:"tool_uid"`
	ProjectID     string `json:"project_id"`
	WorkspacePath string `json:"workspace_path"`
}

// WorkspaceDeleteResultPayload is the schema for workspace.delete.result.
type WorkspaceDeleteResultPayload struct {
	DeletionID string `json:"deletion_id"`
	TenantID   string `json:"tenant_id"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}
