-- +goose Up
ALTER TABLE graph_edges ALTER COLUMN weight TYPE NUMERIC(12,6) USING weight::numeric(12,6);
ALTER TABLE agent_memories ALTER COLUMN importance TYPE NUMERIC(12,6) USING importance::numeric(12,6);
ALTER TABLE experience_entries ALTER COLUMN result_cost TYPE NUMERIC(12,6) USING result_cost::numeric(12,6);
ALTER TABLE experience_entries ALTER COLUMN confidence TYPE NUMERIC(12,6) USING confidence::numeric(12,6);

-- +goose Down
ALTER TABLE experience_entries ALTER COLUMN confidence TYPE REAL USING confidence::real;
ALTER TABLE experience_entries ALTER COLUMN result_cost TYPE REAL USING result_cost::real;
ALTER TABLE agent_memories ALTER COLUMN importance TYPE REAL USING importance::real;
ALTER TABLE graph_edges ALTER COLUMN weight TYPE REAL USING weight::real;
