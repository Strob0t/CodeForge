-- +goose Up

-- Nodes: one row per symbol (function, class, method, module-level)
CREATE TABLE graph_nodes (
    id          TEXT PRIMARY KEY,          -- project_id:filepath:symbol_name
    project_id  TEXT NOT NULL,
    filepath    TEXT NOT NULL,
    symbol_name TEXT NOT NULL,
    kind        TEXT NOT NULL,             -- "function", "class", "method", "module"
    start_line  INT  NOT NULL DEFAULT 0,
    end_line    INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_graph_nodes_project ON graph_nodes(project_id);

-- Edges: directed relationships between nodes
CREATE TABLE graph_edges (
    id          SERIAL PRIMARY KEY,
    project_id  TEXT NOT NULL,
    source_id   TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,             -- "imports", "calls", "inherits"
    weight      REAL NOT NULL DEFAULT 1.0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_graph_edges_project ON graph_edges(project_id);
CREATE INDEX idx_graph_edges_source  ON graph_edges(source_id);
CREATE INDEX idx_graph_edges_target  ON graph_edges(target_id);

-- Per-project graph metadata
CREATE TABLE graph_metadata (
    project_id     TEXT PRIMARY KEY,
    status         TEXT NOT NULL DEFAULT 'pending',  -- pending/building/ready/error
    node_count     INT  NOT NULL DEFAULT 0,
    edge_count     INT  NOT NULL DEFAULT 0,
    languages      TEXT[] NOT NULL DEFAULT '{}',
    error          TEXT,
    built_at       TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS graph_edges;
DROP TABLE IF EXISTS graph_nodes;
DROP TABLE IF EXISTS graph_metadata;
