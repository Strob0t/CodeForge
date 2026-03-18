-- +goose Up
CREATE INDEX IF NOT EXISTS idx_agent_events_payload_gin
    ON agent_events USING GIN(payload);

CREATE INDEX IF NOT EXISTS idx_conversation_messages_tool_calls_gin
    ON conversation_messages USING GIN(tool_calls)
    WHERE tool_calls IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_conversation_messages_images_gin
    ON conversation_messages USING GIN(images)
    WHERE images IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_benchmark_results_evaluator_scores_gin
    ON benchmark_results USING GIN(evaluator_scores)
    WHERE evaluator_scores IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_a2a_tasks_metadata_gin
    ON a2a_tasks USING GIN(metadata);

-- +goose Down
DROP INDEX IF EXISTS idx_a2a_tasks_metadata_gin;
DROP INDEX IF EXISTS idx_benchmark_results_evaluator_scores_gin;
DROP INDEX IF EXISTS idx_conversation_messages_images_gin;
DROP INDEX IF EXISTS idx_conversation_messages_tool_calls_gin;
DROP INDEX IF EXISTS idx_agent_events_payload_gin;
