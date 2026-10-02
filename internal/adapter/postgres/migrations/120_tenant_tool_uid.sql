-- +goose Up
-- KI-96 (S7-H, ADR-018): every tenant that runs agent tools gets its own tool
-- UID from 20000-29999; the worker starts the tenant's tool processes as that
-- UID and the tenant directories grant it access through POSIX ACLs. A UID is
-- allocated lazily (the first time the Go Core creates the tenant's workspace
-- directory or dispatches tool work, with workspace.tool_acls: required), is
-- immutable once set and is never reused in this round (KI-112).
CREATE SEQUENCE tenant_tool_uid_seq AS integer MINVALUE 20000 MAXVALUE 29999 START 20000 NO CYCLE;

ALTER TABLE tenants ADD COLUMN tool_uid integer
    CONSTRAINT tenants_tool_uid_range CHECK (tool_uid BETWEEN 20000 AND 29999)
    CONSTRAINT tenants_tool_uid_key UNIQUE;

-- Tenants with projects may already have tenant directories the worker
-- migrates: number them in creation order.
WITH numbered AS (
    SELECT t.id, 19999 + row_number() OVER (ORDER BY t.created_at, t.id) AS uid
    FROM tenants t
    WHERE EXISTS (SELECT 1 FROM projects p WHERE p.tenant_id = t.id))
UPDATE tenants t SET tool_uid = n.uid FROM numbered n WHERE t.id = n.id;

SELECT setval('tenant_tool_uid_seq', COALESCE((SELECT max(tool_uid) FROM tenants), 20000),
              EXISTS (SELECT 1 FROM tenants WHERE tool_uid IS NOT NULL));

ALTER SEQUENCE tenant_tool_uid_seq OWNED BY tenants.tool_uid;

-- +goose StatementBegin
CREATE FUNCTION tenants_tool_uid_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.tool_uid IS NOT NULL AND NEW.tool_uid IS DISTINCT FROM OLD.tool_uid THEN
        RAISE EXCEPTION 'tenants.tool_uid is immutable once set';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER tenants_tool_uid_immutable BEFORE UPDATE OF tool_uid ON tenants
    FOR EACH ROW EXECUTE FUNCTION tenants_tool_uid_immutable();

-- +goose Down
DROP TRIGGER IF EXISTS tenants_tool_uid_immutable ON tenants;
DROP FUNCTION IF EXISTS tenants_tool_uid_immutable();
-- Dropping the column drops the sequence it owns.
ALTER TABLE tenants DROP COLUMN IF EXISTS tool_uid;
DROP SEQUENCE IF EXISTS tenant_tool_uid_seq;
