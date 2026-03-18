package event_test

import (
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
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
