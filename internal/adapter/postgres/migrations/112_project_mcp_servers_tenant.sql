-- +goose Up
-- KI-71 review: project_mcp_servers rows were written without their tenant
-- (every row got the default tenant), and an assignment did not check the
-- project's tenant, so a server could be linked to a project of another
-- tenant. A link belongs to the tenant of its project and its server; links
-- between a project and a server of different tenants are removed.
DELETE FROM project_mcp_servers ps
USING projects p, mcp_servers s
WHERE ps.project_id = p.id AND ps.mcp_server_id = s.id AND p.tenant_id <> s.tenant_id;

UPDATE project_mcp_servers ps SET tenant_id = p.tenant_id
FROM projects p
WHERE p.id = ps.project_id AND ps.tenant_id <> p.tenant_id;

-- +goose Down
-- The removed cross-tenant links are not restored; the tenant column keeps
-- the corrected values.
SELECT 1;
