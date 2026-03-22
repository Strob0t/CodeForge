package postgres

import (
	"strconv"
	"strings"
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

func TestQueryBuilder_EmptyTenantID(t *testing.T) {
	// An empty tenant ID should not cause a panic; the builder still works
	// and produces a valid WHERE clause with the empty string as $1.
	qb := newQueryBuilder("")

	wantWhere := "tenant_id = $1"
	if got := qb.where(); got != wantWhere {
		t.Errorf("where() = %q, want %q", got, wantWhere)
	}
	if len(qb.args) != 1 {
		t.Fatalf("args length = %d, want 1", len(qb.args))
	}
	if qb.args[0] != "" {
		t.Errorf("args[0] = %v, want empty string", qb.args[0])
	}

	// Adding conditions after empty tenant still works.
	qb.addCondition("status = $%d", "active")
	wantWhere = "tenant_id = $1 AND status = $2"
	if got := qb.where(); got != wantWhere {
		t.Errorf("where() after addCondition = %q, want %q", got, wantWhere)
	}
	if qb.argIdx != 3 {
		t.Errorf("argIdx = %d, want 3", qb.argIdx)
	}
}

func TestQueryBuilder_ManyConditions(t *testing.T) {
	// Verify correct placeholder numbering with 10+ conditions.
	qb := newQueryBuilder("tenant-1")

	conditions := []struct {
		tmpl string
		val  string
	}{
		{"col_a = $%d", "a"},
		{"col_b = $%d", "b"},
		{"col_c = $%d", "c"},
		{"col_d = $%d", "d"},
		{"col_e = $%d", "e"},
		{"col_f = $%d", "f"},
		{"col_g = $%d", "g"},
		{"col_h = $%d", "h"},
		{"col_i = $%d", "i"},
		{"col_j = $%d", "j"},
		{"col_k = $%d", "k"},
		{"col_l = $%d", "l"},
	}
	for _, c := range conditions {
		qb.addCondition(c.tmpl, c.val)
	}

	// 1 (tenant) + 12 conditions = 13 args total.
	if len(qb.args) != 13 {
		t.Fatalf("args length = %d, want 13", len(qb.args))
	}
	if qb.argIdx != 14 {
		t.Errorf("argIdx = %d, want 14", qb.argIdx)
	}

	where := qb.where()
	// Verify double-digit placeholders are correct.
	for i := 1; i <= 13; i++ {
		placeholder := "$" + strconv.Itoa(i)
		if !strings.Contains(where, placeholder) {
			t.Errorf("where clause missing placeholder %s", placeholder)
		}
	}

	// Verify the last condition uses $13.
	if !strings.Contains(where, "col_l = $13") {
		t.Errorf("expected 'col_l = $13' in where clause, got: %s", where)
	}
}

func TestQueryBuilder_ZeroLimit(t *testing.T) {
	// addLimit(0) should work without panic and store 0 as the limit arg.
	qb := newQueryBuilder("tenant-1")
	qb.addCondition("status = $%d", "active")

	limitIdx := qb.addLimit(0)

	// tenant($1) + status($2) -> limit gets $3
	if limitIdx != 3 {
		t.Errorf("addLimit(0) returned idx = %d, want 3", limitIdx)
	}
	if len(qb.args) != 3 {
		t.Fatalf("args length = %d, want 3", len(qb.args))
	}
	if qb.args[2] != 0 {
		t.Errorf("args[2] = %v, want 0", qb.args[2])
	}
	if qb.argIdx != 4 {
		t.Errorf("argIdx = %d, want 4", qb.argIdx)
	}
}
