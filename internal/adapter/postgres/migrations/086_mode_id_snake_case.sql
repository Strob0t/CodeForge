-- +goose Up
-- Convert kebab-case mode IDs to snake_case across all tables.
UPDATE conversations SET mode = REPLACE(mode, '-', '_') WHERE mode LIKE '%-%';
UPDATE agents SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE runs SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE plan_steps SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE prompt_scores SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';
UPDATE prompt_sections SET mode_id = REPLACE(mode_id, '-', '_') WHERE mode_id LIKE '%-%';

-- +goose Down
UPDATE conversations SET mode = REPLACE(mode, '_', '-')
  WHERE mode IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE agents SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE runs SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE plan_steps SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE prompt_scores SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
UPDATE prompt_sections SET mode_id = REPLACE(mode_id, '_', '-')
  WHERE mode_id IN ('api_tester','backend_architect','lsp_engineer','workflow_optimizer','infra_maintainer','goal_researcher','boundary_analyzer','contract_reviewer');
