-- +goose Up
CREATE TABLE IF NOT EXISTS branch_protection_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    branch_pattern TEXT NOT NULL,
    require_reviews BOOLEAN NOT NULL DEFAULT false,
    require_tests   BOOLEAN NOT NULL DEFAULT false,
    require_lint    BOOLEAN NOT NULL DEFAULT false,
    allow_force_push BOOLEAN NOT NULL DEFAULT false,
    allow_delete    BOOLEAN NOT NULL DEFAULT false,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    version         INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_branch_protection_rules_project ON branch_protection_rules(project_id);
CREATE INDEX IF NOT EXISTS idx_branch_protection_rules_tenant ON branch_protection_rules(tenant_id);

-- Reuse existing triggers from earlier migrations.
CREATE TRIGGER set_branch_protection_rules_updated_at
    BEFORE UPDATE ON branch_protection_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER increment_branch_protection_rules_version
    BEFORE UPDATE ON branch_protection_rules
    FOR EACH ROW EXECUTE FUNCTION increment_version();

-- +goose Down
DROP TABLE IF EXISTS branch_protection_rules;
