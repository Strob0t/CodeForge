package context

import (
	"errors"
	"regexp"
	"time"
)

// uuidRE matches a standard UUID format (lowercase hex).
var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// SharedContext is a versioned, team-level collection of key-value items
// that accumulates as team members complete steps.
type SharedContext struct {
	ID        string              `json:"id"`
	TeamID    string              `json:"team_id"`
	ProjectID string              `json:"project_id"`
	Version   int                 `json:"version"`
	Items     []SharedContextItem `json:"items"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// SharedContextItem is a single key-value entry within a SharedContext.
type SharedContextItem struct {
	ID        string    `json:"id"`
	SharedID  string    `json:"shared_id"`
	Key       string    `json:"key"`    // unique within shared context
	Value     string    `json:"value"`  // content
	Author    string    `json:"author"` // agent ID that added this
	Tokens    int       `json:"tokens"` // estimated token count
	CreatedAt time.Time `json:"created_at"`
}

// AddSharedItemRequest holds the input for adding an item to shared context.
type AddSharedItemRequest struct {
	TeamID string `json:"team_id"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Author string `json:"author"`
}

// Validate checks that a SharedContext is well-formed.
func (sc *SharedContext) Validate() error {
	if sc.TeamID == "" {
		return errors.New("team_id is required")
	}
	if sc.ProjectID == "" {
		return errors.New("project_id is required")
	}
	return nil
}

// Validate checks that an AddSharedItemRequest is well-formed.
func (r *AddSharedItemRequest) Validate() error {
	if r.TeamID == "" {
		return errors.New("team_id is required")
	}
	if r.Key == "" {
		return errors.New("key is required")
	}
	if r.Value == "" {
		return errors.New("value is required")
	}
	if r.Author != "" && !uuidRE.MatchString(r.Author) {
		return errors.New("author must be a valid UUID")
	}
	return nil
}
