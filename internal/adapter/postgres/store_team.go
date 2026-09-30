package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// --- Agent Teams ---

func (s *Store) CreateTeam(ctx context.Context, req agent.CreateTeamRequest) (*agent.Team, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	tid := tenantFromCtx(ctx)

	var t agent.Team
	err = tx.QueryRow(ctx,
		`INSERT INTO agent_teams (tenant_id, project_id, name, protocol)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, name, protocol, status, version, created_at, updated_at`,
		tid, req.ProjectID, req.Name, req.Protocol,
	).Scan(&t.ID, &t.ProjectID, &t.Name, &t.Protocol, &t.Status, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert team: %w", err)
	}

	for _, m := range req.Members {
		var member agent.TeamMember
		err = tx.QueryRow(ctx,
			`INSERT INTO team_members (tenant_id, team_id, agent_id, role)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, team_id, agent_id, role`,
			tid, t.ID, m.AgentID, string(m.Role),
		).Scan(&member.ID, &member.TeamID, &member.AgentID, &member.Role)
		if err != nil {
			return nil, fmt.Errorf("insert team member: %w", err)
		}
		t.Members = append(t.Members, member)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit team: %w", err)
	}
	return &t, nil
}

func (s *Store) GetTeam(ctx context.Context, id string) (*agent.Team, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, protocol, status, version, created_at, updated_at
		 FROM agent_teams WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))

	t, err := scanTeam(row)
	if err != nil {
		return nil, notFoundWrap(err, "get team %s", id)
	}

	members, err := s.listTeamMembers(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Members = members
	return &t, nil
}

func (s *Store) ListTeamsByProject(ctx context.Context, projectID string) ([]agent.Team, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, protocol, status, version, created_at, updated_at
		 FROM agent_teams WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`, projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	teams, err := scanRows(rows, func(r pgx.Rows) (agent.Team, error) {
		return scanTeam(r)
	})
	if err != nil {
		return nil, err
	}

	// Load members for each team
	for i := range teams {
		members, err := s.listTeamMembers(ctx, teams[i].ID)
		if err != nil {
			return nil, err
		}
		teams[i].Members = members
	}
	return teams, nil
}

// teamTerminalStatuses parameterizes the status predicate of team updates.
var teamTerminalStatuses = statusStrings(agent.TerminalTeamStatuses())

const teamExistsSQL = `SELECT EXISTS (SELECT 1 FROM agent_teams WHERE id = $1 AND tenant_id = $2)`

// UpdateTeamStatus sets the status of a team that has not ended. It returns
// domain.ErrConflict when the team already ended.
func (s *Store) UpdateTeamStatus(ctx context.Context, id string, status agent.TeamStatus) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE agent_teams SET status = $2 WHERE id = $1 AND tenant_id = $3 AND status <> ALL($4)`,
		id, string(status), tenantFromCtx(ctx), teamTerminalStatuses)
	return s.guardedUpdateResult(ctx, tag, err, teamExistsSQL, "update team status", id)
}

// ListEndedTeams returns up to limit teams that have not ended although
// every execution plan of theirs has - a cancelled plan, or one that ended
// while Go Core was down, ends no team otherwise. A team without plans is
// left alone: a plan may still be created for it.
//
// INTENTIONALLY CROSS-TENANT: the stuck-work watchdog ends such teams in
// every tenant. Each row carries its tenant_id, and the caller ends each team
// in its tenant's context.
func (s *Store) ListEndedTeams(ctx context.Context, limit int) ([]database.EndedTeam, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.tenant_id, bool_or(p.status <> $3)
		 FROM agent_teams t
		 JOIN execution_plans p ON p.team_id = t.id AND p.tenant_id = t.tenant_id
		 WHERE t.status <> ALL($1)
		 GROUP BY t.id, t.tenant_id
		 HAVING bool_and(p.status = ANY($2))
		 LIMIT $4`,
		teamTerminalStatuses, planTerminalStatuses, string(plan.StatusCompleted), limit)
	if err != nil {
		return nil, fmt.Errorf("list ended teams: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (database.EndedTeam, error) {
		var t database.EndedTeam
		err := r.Scan(&t.ID, &t.TenantID, &t.Failed)
		return t, err
	})
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM agent_teams WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete team %s", id)
}

func (s *Store) listTeamMembers(ctx context.Context, teamID string) ([]agent.TeamMember, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, agent_id, role FROM team_members WHERE team_id = $1 AND tenant_id = $2`, teamID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (agent.TeamMember, error) {
		var m agent.TeamMember
		err := r.Scan(&m.ID, &m.TeamID, &m.AgentID, &m.Role)
		return m, err
	})
}

func scanTeam(row scannable) (agent.Team, error) {
	var t agent.Team
	err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Protocol, &t.Status, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}
