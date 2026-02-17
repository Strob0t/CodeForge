package agent

import (
	"errors"
	"time"
)

// TeamRole defines the specialization of an agent within a team.
type TeamRole string

const (
	RoleCoder      TeamRole = "coder"
	RoleReviewer   TeamRole = "reviewer"
	RoleTester     TeamRole = "tester"
	RoleDocumenter TeamRole = "documenter"
	RolePlanner    TeamRole = "planner"
)

// ValidTeamRole reports whether r is a known team role.
func ValidTeamRole(r string) bool {
	switch TeamRole(r) {
	case RoleCoder, RoleReviewer, RoleTester, RoleDocumenter, RolePlanner:
		return true
	}
	return false
}

// TeamStatus represents the lifecycle state of an agent team.
type TeamStatus string

const (
	TeamStatusInitializing TeamStatus = "initializing"
	TeamStatusActive       TeamStatus = "active"
	TeamStatusCompleted    TeamStatus = "completed"
	TeamStatusFailed       TeamStatus = "failed"
)

// IsTerminal returns true if the team is in a final state.
func (s TeamStatus) IsTerminal() bool {
	return s == TeamStatusCompleted || s == TeamStatusFailed
}

// Team groups multiple agents for collaborative work on a feature.
type Team struct {
	ID        string       `json:"id"`
	ProjectID string       `json:"project_id"`
	Name      string       `json:"name"`
	Protocol  string       `json:"protocol"` // reuses plan.Protocol values (sequential, parallel, etc.)
	Status    TeamStatus   `json:"status"`
	Members   []TeamMember `json:"members"`
	Version   int          `json:"version"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// TeamMember represents one agent's role in a team.
type TeamMember struct {
	ID      string   `json:"id"`
	TeamID  string   `json:"team_id"`
	AgentID string   `json:"agent_id"`
	Role    TeamRole `json:"role"`
}

// CreateTeamRequest holds the fields needed to create a new team.
type CreateTeamRequest struct {
	ProjectID string                `json:"project_id"`
	Name      string                `json:"name"`
	Protocol  string                `json:"protocol"`
	Members   []CreateMemberRequest `json:"members"`
}

// CreateMemberRequest holds the fields for adding a member to a team.
type CreateMemberRequest struct {
	AgentID string   `json:"agent_id"`
	Role    TeamRole `json:"role"`
}

// Validate checks that a CreateTeamRequest is well-formed.
func (r *CreateTeamRequest) Validate() error {
	if r.ProjectID == "" {
		return errors.New("project_id is required")
	}
	if r.Name == "" {
		return errors.New("team name is required")
	}
	if len(r.Members) == 0 {
		return errors.New("at least one member is required")
	}

	seen := make(map[string]bool, len(r.Members))
	for _, m := range r.Members {
		if m.AgentID == "" {
			return errors.New("agent_id is required for each member")
		}
		if !ValidTeamRole(string(m.Role)) {
			return errors.New("invalid team role: " + string(m.Role))
		}
		if seen[m.AgentID] {
			return errors.New("duplicate agent_id in team members: " + m.AgentID)
		}
		seen[m.AgentID] = true
	}

	return nil
}
