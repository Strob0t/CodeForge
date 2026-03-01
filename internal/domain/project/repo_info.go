package project

// RepoInfo holds metadata fetched from a remote repository hosting API.
type RepoInfo struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Language      string `json:"language,omitempty"`
	Stars         int    `json:"stars"`
	Private       bool   `json:"private"`
}
