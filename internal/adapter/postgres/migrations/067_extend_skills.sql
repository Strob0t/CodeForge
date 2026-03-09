-- 067_extend_skills.sql
-- +goose Up
ALTER TABLE skills ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'pattern';
ALTER TABLE skills ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'user';
ALTER TABLE skills ADD COLUMN IF NOT EXISTS source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE skills ADD COLUMN IF NOT EXISTS format_origin TEXT NOT NULL DEFAULT 'codeforge';
ALTER TABLE skills ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE skills ADD COLUMN IF NOT EXISTS usage_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE skills ADD COLUMN IF NOT EXISTS content TEXT NOT NULL DEFAULT '';

-- Migrate existing data: copy code -> content, keep code for backwards compat
UPDATE skills SET content = code WHERE content = '' AND code != '';

-- Add check constraints
ALTER TABLE skills ADD CONSTRAINT chk_skill_type CHECK (type IN ('workflow', 'pattern'));
ALTER TABLE skills ADD CONSTRAINT chk_skill_source CHECK (source IN ('builtin', 'import', 'user', 'agent'));
ALTER TABLE skills ADD CONSTRAINT chk_skill_status CHECK (status IN ('draft', 'active', 'disabled'));
ALTER TABLE skills ADD CONSTRAINT chk_skill_format CHECK (format_origin IN ('claude', 'cursor', 'markdown', 'codeforge'));

-- Index for status filtering (replaces enabled-only index)
CREATE INDEX IF NOT EXISTS idx_skills_status ON skills(tenant_id, status) WHERE status = 'active';

-- +goose Down
ALTER TABLE skills DROP CONSTRAINT IF EXISTS chk_skill_format;
ALTER TABLE skills DROP CONSTRAINT IF EXISTS chk_skill_status;
ALTER TABLE skills DROP CONSTRAINT IF EXISTS chk_skill_source;
ALTER TABLE skills DROP CONSTRAINT IF EXISTS chk_skill_type;
DROP INDEX IF EXISTS idx_skills_status;
ALTER TABLE skills DROP COLUMN IF EXISTS content;
ALTER TABLE skills DROP COLUMN IF EXISTS usage_count;
ALTER TABLE skills DROP COLUMN IF EXISTS status;
ALTER TABLE skills DROP COLUMN IF EXISTS format_origin;
ALTER TABLE skills DROP COLUMN IF EXISTS source_url;
ALTER TABLE skills DROP COLUMN IF EXISTS source;
ALTER TABLE skills DROP COLUMN IF EXISTS type;
