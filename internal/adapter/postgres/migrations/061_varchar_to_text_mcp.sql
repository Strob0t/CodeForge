-- +goose Up

-- Normalize VARCHAR columns to TEXT for consistency (pgx treats them identically).
ALTER TABLE mcp_servers ALTER COLUMN name TYPE TEXT;
ALTER TABLE mcp_servers ALTER COLUMN transport TYPE TEXT;
ALTER TABLE mcp_servers ALTER COLUMN command TYPE TEXT;
ALTER TABLE mcp_servers ALTER COLUMN url TYPE TEXT;
ALTER TABLE mcp_servers ALTER COLUMN status TYPE TEXT;
ALTER TABLE mcp_server_tools ALTER COLUMN name TYPE TEXT;

-- +goose Down

ALTER TABLE mcp_server_tools ALTER COLUMN name TYPE VARCHAR(256);
ALTER TABLE mcp_servers ALTER COLUMN status TYPE VARCHAR(32);
ALTER TABLE mcp_servers ALTER COLUMN url TYPE VARCHAR(2048);
ALTER TABLE mcp_servers ALTER COLUMN command TYPE VARCHAR(512);
ALTER TABLE mcp_servers ALTER COLUMN transport TYPE VARCHAR(16);
ALTER TABLE mcp_servers ALTER COLUMN name TYPE VARCHAR(128);
