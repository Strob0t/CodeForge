// Package webhook defines domain types for VCS and PM webhook events.
package webhook

import "time"

// VCSEventType classifies the VCS webhook event.
type VCSEventType string

const (
	VCSEventPush         VCSEventType = "push"
	VCSEventPullRequest  VCSEventType = "pull_request"
	VCSEventTag          VCSEventType = "tag"
	VCSEventBranchCreate VCSEventType = "branch_create"
	VCSEventBranchDelete VCSEventType = "branch_delete"
)

// VCSEvent is a normalized VCS webhook event.
type VCSEvent struct {
	Type       VCSEventType `json:"type"`
	Provider   string       `json:"provider"` // "github", "gitlab"
	Repository string       `json:"repository"`
	Branch     string       `json:"branch"`
	Sender     string       `json:"sender"`
	CommitHash string       `json:"commit_hash,omitempty"`
	Message    string       `json:"message,omitempty"`
	URL        string       `json:"url,omitempty"`
	ReceivedAt time.Time    `json:"received_at"`
}

// VCSPushEvent contains details specific to push events.
type VCSPushEvent struct {
	VCSEvent
	Commits   []VCSCommit `json:"commits"`
	Before    string      `json:"before"`
	After     string      `json:"after"`
	Forced    bool        `json:"forced"`
	FileCount int         `json:"file_count"`
}

// VCSCommit represents a single commit in a push event.
type VCSCommit struct {
	Hash     string   `json:"hash"`
	Message  string   `json:"message"`
	Author   string   `json:"author"`
	Added    []string `json:"added"`
	Modified []string `json:"modified"`
	Removed  []string `json:"removed"`
}

// VCSPullRequestEvent contains details specific to pull request events.
type VCSPullRequestEvent struct {
	VCSEvent
	Action     string `json:"action"` // "opened", "closed", "merged", "updated"
	PRNumber   int    `json:"pr_number"`
	Title      string `json:"title"`
	BaseBranch string `json:"base_branch"`
	HeadBranch string `json:"head_branch"`
	Draft      bool   `json:"draft"`
}
