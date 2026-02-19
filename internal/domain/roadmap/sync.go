package roadmap

// SyncDirection specifies the sync direction.
type SyncDirection string

const (
	SyncDirectionPush SyncDirection = "push" // CodeForge -> PM tool
	SyncDirectionPull SyncDirection = "pull" // PM tool -> CodeForge
	SyncDirectionBidi SyncDirection = "bidi" // Both directions
)

// SyncConfig configures a bidirectional sync operation.
type SyncConfig struct {
	ProjectID   string        `json:"project_id"`
	Provider    string        `json:"provider"`
	ProjectRef  string        `json:"project_ref"` // e.g., "owner/repo" for GitHub
	Direction   SyncDirection `json:"direction"`
	DryRun      bool          `json:"dry_run"`
	CreateNew   bool          `json:"create_new"`   // Create items that don't exist on the other side
	UpdateExist bool          `json:"update_exist"` // Update items that exist on both sides
}

// SyncResult summarizes what happened during a sync operation.
type SyncResult struct {
	Direction string   `json:"direction"`
	Created   int      `json:"created"`
	Updated   int      `json:"updated"`
	Skipped   int      `json:"skipped"`
	Errors    []string `json:"errors,omitempty"`
	DryRun    bool     `json:"dry_run"`
}
