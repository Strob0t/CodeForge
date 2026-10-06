package event_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/review"
)

func TestAgentEvent_SequenceNumberJSON(t *testing.T) {
	t.Run("sequence_number serializes to JSON", func(t *testing.T) {
		ev := event.AgentEvent{
			ID:             "evt-1",
			AgentID:        "agent-1",
			TaskID:         "task-1",
			ProjectID:      "proj-1",
			Type:           event.TypeAgentStarted,
			Payload:        json.RawMessage(`{}`),
			Version:        1,
			SequenceNumber: 42,
		}

		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal raw: %v", err)
		}

		seqRaw, ok := raw["sequence_number"]
		if !ok {
			t.Fatal("sequence_number field missing from JSON")
		}

		var seqNum int64
		if err := json.Unmarshal(seqRaw, &seqNum); err != nil {
			t.Fatalf("unmarshal sequence_number: %v", err)
		}
		if seqNum != 42 {
			t.Errorf("sequence_number = %d, want 42", seqNum)
		}
	})

	t.Run("sequence_number deserializes from JSON", func(t *testing.T) {
		jsonStr := `{"id":"evt-2","agent_id":"a","task_id":"t","project_id":"p","type":"agent.started","payload":{},"version":1,"sequence_number":99}`

		var ev event.AgentEvent
		if err := json.Unmarshal([]byte(jsonStr), &ev); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ev.SequenceNumber != 99 {
			t.Errorf("SequenceNumber = %d, want 99", ev.SequenceNumber)
		}
	})

	t.Run("zero sequence_number is default", func(t *testing.T) {
		ev := event.AgentEvent{
			ID:      "evt-3",
			Type:    event.TypeAgentFinished,
			Payload: json.RawMessage(`{}`),
		}
		if ev.SequenceNumber != 0 {
			t.Errorf("default SequenceNumber = %d, want 0", ev.SequenceNumber)
		}
	})
}

// KI-17: the refactoring impact event carries what the frontend's
// ReviewImpactEvent (frontend/src/api/types.ts) and RefactorApproval read.
func TestReviewImpactEvent_JSONFields(t *testing.T) {
	data, err := json.Marshal(event.ReviewImpactEvent{RunID: "r", Reason: "why"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{
		"run_id", "plan_id", "step_id", "project_id", "impact_level",
		"files_changed", "lines_added", "lines_removed", "cross_layer", "structural", "reason",
	} {
		if _, ok := fields[key]; !ok {
			t.Errorf("ReviewImpactEvent JSON has no %q: %s", key, data)
		}
	}
	if len(fields) != 11 {
		t.Errorf("ReviewImpactEvent JSON has %d fields, want 11: %s", len(fields), data)
	}
}

// KI-94: an approval request lists the files users changed while the
// refactoring ran, as the frontend's ReviewUserEdit reads them.
func TestReviewImpactEvent_UserEditsJSON(t *testing.T) {
	data, err := json.Marshal(event.ReviewImpactEvent{RunID: "r", UserEditsTotal: 1, UserEdits: []review.UserEdit{{
		Path: "a.go", Operation: review.UserEditWrite, UserID: "u1", UserName: "Ada",
		EditedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		Total int                          `json:"user_edits_total"`
		Edits []map[string]json.RawMessage `json:"user_edits"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Total != 1 || len(got.Edits) != 1 {
		t.Fatalf("ReviewImpactEvent JSON = %s, want one user edit", data)
	}
	for _, key := range []string{"path", "operation", "user_id", "user_name", "edited_at"} {
		if _, ok := got.Edits[0][key]; !ok {
			t.Errorf("user edit JSON has no %q: %s", key, data)
		}
	}
}
