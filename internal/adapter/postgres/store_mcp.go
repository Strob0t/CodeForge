package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/mcp"
)

// CreateMCPServer inserts a new MCP server definition.
func (s *Store) CreateMCPServer(ctx context.Context, srv *mcp.ServerDef) error {
	argsJSON, err := json.Marshal(srv.Args)
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}
	envJSON, err := json.Marshal(srv.Env)
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}
	headersJSON, err := json.Marshal(srv.Headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	const q = `INSERT INTO mcp_servers
		(id, tenant_id, name, description, transport, command, args, url, env, headers, enabled, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err = s.pool.Exec(ctx, q,
		srv.ID, tenantFromCtx(ctx), srv.Name, srv.Description,
		string(srv.Transport), srv.Command, argsJSON, srv.URL,
		envJSON, headersJSON, srv.Enabled, string(srv.Status),
	)
	if err != nil {
		return fmt.Errorf("create mcp server: %w", err)
	}
	return nil
}

// GetMCPServer retrieves an MCP server by ID.
func (s *Store) GetMCPServer(ctx context.Context, id string) (*mcp.ServerDef, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, name, description, transport, command, args, url, env, headers, enabled, status
		FROM mcp_servers WHERE id = $1 AND tenant_id = $2`
	srv, err := scanMCPServer(s.pool.QueryRow(ctx, q, id, tid))
	if err != nil {
		return nil, notFoundWrap(err, "get mcp server %s", id)
	}
	return &srv, nil
}

// ListMCPServers returns all MCP servers for the current tenant.
func (s *Store) ListMCPServers(ctx context.Context) ([]mcp.ServerDef, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT id, name, description, transport, command, args, url, env, headers, enabled, status
		FROM mcp_servers WHERE tenant_id = $1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, tid)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer rows.Close()

	var result []mcp.ServerDef
	for rows.Next() {
		srv, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, srv)
	}
	return result, rows.Err()
}

// UpdateMCPServer updates an existing MCP server definition.
func (s *Store) UpdateMCPServer(ctx context.Context, srv *mcp.ServerDef) error {
	tid := tenantFromCtx(ctx)
	argsJSON, err := json.Marshal(srv.Args)
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}
	envJSON, err := json.Marshal(srv.Env)
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}
	headersJSON, err := json.Marshal(srv.Headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	const q = `UPDATE mcp_servers SET
		name=$2, description=$3, transport=$4, command=$5, args=$6,
		url=$7, env=$8, headers=$9, enabled=$10, updated_at=now()
		WHERE id=$1 AND tenant_id=$11`
	tag, err := s.pool.Exec(ctx, q,
		srv.ID, srv.Name, srv.Description, string(srv.Transport),
		srv.Command, argsJSON, srv.URL, envJSON, headersJSON,
		srv.Enabled, tid,
	)
	return execExpectOne(tag, err, "update mcp server %s", srv.ID)
}

// DeleteMCPServer deletes an MCP server by ID.
func (s *Store) DeleteMCPServer(ctx context.Context, id string) error {
	tid := tenantFromCtx(ctx)
	tag, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1 AND tenant_id = $2`, id, tid)
	return execExpectOne(tag, err, "delete mcp server %s", id)
}

// UpdateMCPServerStatus updates the status and last health check timestamp.
func (s *Store) UpdateMCPServerStatus(ctx context.Context, id string, status mcp.ServerStatus) error {
	tid := tenantFromCtx(ctx)
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx,
		`UPDATE mcp_servers SET status=$2, last_health_check=$3, updated_at=now() WHERE id=$1 AND tenant_id=$4`,
		id, string(status), now, tid,
	)
	return execExpectOne(tag, err, "update mcp server status %s", id)
}

// AssignMCPServerToProject links an MCP server to a project.
func (s *Store) AssignMCPServerToProject(ctx context.Context, projectID, serverID string) error {
	const q = `INSERT INTO project_mcp_servers (project_id, mcp_server_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := s.pool.Exec(ctx, q, projectID, serverID)
	if err != nil {
		return fmt.Errorf("assign mcp server %s to project %s: %w", serverID, projectID, err)
	}
	return nil
}

// UnassignMCPServerFromProject removes the link between an MCP server and a project.
func (s *Store) UnassignMCPServerFromProject(ctx context.Context, projectID, serverID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM project_mcp_servers WHERE project_id = $1 AND mcp_server_id = $2`,
		projectID, serverID,
	)
	return execExpectOne(tag, err, "unassign mcp server %s from project %s", serverID, projectID)
}

// ListMCPServersByProject returns all MCP servers assigned to a project.
func (s *Store) ListMCPServersByProject(ctx context.Context, projectID string) ([]mcp.ServerDef, error) {
	tid := tenantFromCtx(ctx)
	const q = `SELECT s.id, s.name, s.description, s.transport, s.command, s.args, s.url, s.env, s.headers, s.enabled, s.status
		FROM mcp_servers s
		JOIN project_mcp_servers ps ON ps.mcp_server_id = s.id
		WHERE ps.project_id = $1 AND s.tenant_id = $2
		ORDER BY s.name`
	rows, err := s.pool.Query(ctx, q, projectID, tid)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers for project %s: %w", projectID, err)
	}
	defer rows.Close()

	var result []mcp.ServerDef
	for rows.Next() {
		srv, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, srv)
	}
	return result, rows.Err()
}

// UpsertMCPServerTools replaces all cached tools for an MCP server.
func (s *Store) UpsertMCPServerTools(ctx context.Context, serverID string, tools []mcp.ServerTool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM mcp_server_tools WHERE server_id = $1`, serverID); err != nil {
		return fmt.Errorf("delete old tools: %w", err)
	}

	for _, t := range tools {
		schemaJSON, err := json.Marshal(t.InputSchema)
		if err != nil {
			return fmt.Errorf("marshal input_schema: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO mcp_server_tools (server_id, name, description, input_schema) VALUES ($1, $2, $3, $4)`,
			serverID, t.Name, t.Description, schemaJSON,
		)
		if err != nil {
			return fmt.Errorf("insert tool %s: %w", t.Name, err)
		}
	}

	return tx.Commit(ctx)
}

// ListMCPServerTools returns all cached tools for an MCP server.
func (s *Store) ListMCPServerTools(ctx context.Context, serverID string) ([]mcp.ServerTool, error) {
	const q = `SELECT server_id, name, description, input_schema FROM mcp_server_tools WHERE server_id = $1 ORDER BY name`
	rows, err := s.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, fmt.Errorf("list mcp server tools %s: %w", serverID, err)
	}
	defer rows.Close()

	var result []mcp.ServerTool
	for rows.Next() {
		var t mcp.ServerTool
		var schemaJSON []byte
		if err := rows.Scan(&t.ServerID, &t.Name, &t.Description, &schemaJSON); err != nil {
			return nil, err
		}
		if schemaJSON != nil {
			t.InputSchema = schemaJSON
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// scanMCPServer scans a single MCP server row.
func scanMCPServer(row scannable) (mcp.ServerDef, error) {
	var srv mcp.ServerDef
	var argsJSON, envJSON, headersJSON []byte
	err := row.Scan(
		&srv.ID, &srv.Name, &srv.Description, &srv.Transport,
		&srv.Command, &argsJSON, &srv.URL, &envJSON, &headersJSON,
		&srv.Enabled, &srv.Status,
	)
	if err != nil {
		return srv, err
	}
	if argsJSON != nil {
		if err := json.Unmarshal(argsJSON, &srv.Args); err != nil {
			return srv, fmt.Errorf("unmarshal args: %w", err)
		}
	}
	if envJSON != nil {
		if err := json.Unmarshal(envJSON, &srv.Env); err != nil {
			return srv, fmt.Errorf("unmarshal env: %w", err)
		}
	}
	if headersJSON != nil {
		if err := json.Unmarshal(headersJSON, &srv.Headers); err != nil {
			return srv, fmt.Errorf("unmarshal headers: %w", err)
		}
	}
	return srv, nil
}
