package postgres

import (
	"testing"
	"time"
)

func TestQueryBuilder_EmptyFilter(t *testing.T) {
	qb := newQueryBuilder("tenant-1")

	want := "tenant_id = $1"
	if got := qb.where(); got != want {
		t.Errorf("where() = %q, want %q", got, want)
	}
	if len(qb.args) != 1 {
		t.Fatalf("args length = %d, want 1", len(qb.args))
	}
	if qb.args[0] != "tenant-1" {
		t.Errorf("args[0] = %v, want %q", qb.args[0], "tenant-1")
	}
	if qb.argIdx != 2 {
		t.Errorf("argIdx = %d, want 2", qb.argIdx)
	}
}

func TestQueryBuilder_WithInitialColumn(t *testing.T) {
	qb := newQueryBuilderWith("run_id", "run-abc", "tenant-1")

	want := "run_id = $1 AND tenant_id = $2"
	if got := qb.where(); got != want {
		t.Errorf("where() = %q, want %q", got, want)
	}
	if len(qb.args) != 2 {
		t.Fatalf("args length = %d, want 2", len(qb.args))
	}
	if qb.args[0] != "run-abc" {
		t.Errorf("args[0] = %v, want %q", qb.args[0], "run-abc")
	}
	if qb.args[1] != "tenant-1" {
		t.Errorf("args[1] = %v, want %q", qb.args[1], "tenant-1")
	}
	if qb.argIdx != 3 {
		t.Errorf("argIdx = %d, want 3", qb.argIdx)
	}
}

func TestQueryBuilder_AllFilters(t *testing.T) {
	qb := newQueryBuilderWith("run_id", "run-abc", "tenant-1")

	qb.addCondition("agent_id = $%d", "agent-1")

	types := []string{"tool_call", "tool_result"}
	qb.addCondition("event_type = ANY($%d)", types)

	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	qb.addCondition("created_at > $%d", after)

	before := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	qb.addCondition("created_at < $%d", before)

	qb.addCondition("sequence_number > $%d", int64(42))

	wantWhere := "run_id = $1 AND tenant_id = $2 AND agent_id = $3 AND event_type = ANY($4) AND created_at > $5 AND created_at < $6 AND sequence_number > $7"
	if got := qb.where(); got != wantWhere {
		t.Errorf("where() = %q, want %q", got, wantWhere)
	}

	if len(qb.args) != 7 {
		t.Fatalf("args length = %d, want 7", len(qb.args))
	}
	if qb.argIdx != 8 {
		t.Errorf("argIdx = %d, want 8", qb.argIdx)
	}
}

func TestQueryBuilder_ArgIndexing(t *testing.T) {
	qb := newQueryBuilder("tenant-1")
	qb.addCondition("project_id = $%d", "proj-1")
	qb.addCondition("run_id = $%d", "run-1")
	qb.addCondition("agent_id = $%d", "agent-1")
	qb.addCondition("action = $%d", "create")

	wantArgs := []any{"tenant-1", "proj-1", "run-1", "agent-1", "create"}
	if len(qb.args) != len(wantArgs) {
		t.Fatalf("args length = %d, want %d", len(qb.args), len(wantArgs))
	}
	for i, want := range wantArgs {
		if qb.args[i] != want {
			t.Errorf("args[%d] = %v, want %v", i, qb.args[i], want)
		}
	}

	wantWhere := "tenant_id = $1 AND project_id = $2 AND run_id = $3 AND agent_id = $4 AND action = $5"
	if got := qb.where(); got != wantWhere {
		t.Errorf("where() = %q, want %q", got, wantWhere)
	}

	if qb.argIdx != 6 {
		t.Errorf("argIdx = %d, want 6", qb.argIdx)
	}
}

func TestQueryBuilder_AddLimit(t *testing.T) {
	qb := newQueryBuilderWith("run_id", "run-1", "tenant-1")
	qb.addCondition("id > $%d", "cursor-abc")

	limitIdx := qb.addLimit(51)

	if limitIdx != 4 {
		t.Errorf("addLimit returned idx = %d, want 4", limitIdx)
	}
	if len(qb.args) != 4 {
		t.Fatalf("args length = %d, want 4", len(qb.args))
	}
	if qb.args[3] != 51 {
		t.Errorf("args[3] = %v, want 51", qb.args[3])
	}
	if qb.argIdx != 5 {
		t.Errorf("argIdx = %d, want 5", qb.argIdx)
	}
}

func TestQueryBuilder_AddRawCondition(t *testing.T) {
	qb := newQueryBuilder("tenant-1")
	qb.addRawCondition("deleted_at IS NULL")
	qb.addCondition("status = $%d", "active")

	wantWhere := "tenant_id = $1 AND deleted_at IS NULL AND status = $2"
	if got := qb.where(); got != wantWhere {
		t.Errorf("where() = %q, want %q", got, wantWhere)
	}

	if len(qb.args) != 2 {
		t.Fatalf("args length = %d, want 2", len(qb.args))
	}
	if qb.args[0] != "tenant-1" {
		t.Errorf("args[0] = %v, want %q", qb.args[0], "tenant-1")
	}
	if qb.args[1] != "active" {
		t.Errorf("args[1] = %v, want %q", qb.args[1], "active")
	}
}
