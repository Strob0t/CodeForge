package project

// GitStatus holds the current state of a git repository.
type GitStatus struct {
	Branch        string   `json:"branch"`
	CommitHash    string   `json:"commit_hash"`
	CommitMessage string   `json:"commit_message"`
	Dirty         bool     `json:"dirty"`
	Modified      []string `json:"modified,omitempty"`
	Untracked     []string `json:"untracked,omitempty"`
	Ahead         int      `json:"ahead"`
	Behind        int      `json:"behind"`
}

// Branch represents a git branch.
type Branch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}
