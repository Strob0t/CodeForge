package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

// mockQuarantineStore implements the quarantine-related methods of database.Store.
type mockQuarantineStore struct {
	mockStore // embed the full mock store for interface satisfaction
	messages  map[string]*quarantine.Message
	nextID    int
}

func newMockQuarantineStore() *mockQuarantineStore {
	return &mockQuarantineStore{
		messages: make(map[string]*quarantine.Message),
	}
}

func (m *mockQuarantineStore) QuarantineMessage(_ context.Context, msg *quarantine.Message) error {
	m.nextID++
	msg.ID = "qmsg-" + string(rune('0'+m.nextID))
	m.messages[msg.ID] = msg
	return nil
}

func (m *mockQuarantineStore) GetQuarantinedMessage(_ context.Context, id string) (*quarantine.Message, error) {
	msg, ok := m.messages[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return msg, nil
}

func (m *mockQuarantineStore) ListQuarantinedMessages(_ context.Context, projectID string, status quarantine.Status, _, _ int) ([]*quarantine.Message, error) {
	var result []*quarantine.Message
	for _, msg := range m.messages {
		if msg.ProjectID == projectID && (status == "" || msg.Status == status) {
			result = append(result, msg)
		}
	}
	return result, nil
}

func (m *mockQuarantineStore) UpdateQuarantineStatus(_ context.Context, id string, status quarantine.Status, reviewedBy, note string) error {
	msg, ok := m.messages[id]
	if !ok {
		return domain.ErrNotFound
	}
	msg.Status = status
	msg.ReviewedBy = reviewedBy
	msg.ReviewNote = note
	return nil
}

func TestQuarantineEvaluate_Disabled(t *testing.T) {
	svc := NewQuarantineService(newMockQuarantineStore(), &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled: false,
	})

	ann := &trust.Annotation{Origin: "a2a", TrustLevel: trust.LevelUntrusted, SourceID: "ext"}
	blocked, err := svc.Evaluate(context.Background(), ann, "run.start", []byte(`{}`), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("expected message to pass when quarantine is disabled")
	}
}

func TestQuarantineEvaluate_TrustBypass(t *testing.T) {
	svc := NewQuarantineService(newMockQuarantineStore(), &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	})

	ann := &trust.Annotation{Origin: "internal", TrustLevel: trust.LevelFull, SourceID: "agent-1"}
	blocked, err := svc.Evaluate(context.Background(), ann, "run.start", []byte(`{}`), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("expected full-trust message to bypass quarantine")
	}
}

func TestQuarantineEvaluate_BelowThreshold(t *testing.T) {
	svc := NewQuarantineService(newMockQuarantineStore(), &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	})

	// Partial trust = 0.2, clean payload = no content penalty => 0.2 < 0.7
	ann := &trust.Annotation{Origin: "mcp", TrustLevel: trust.LevelPartial, SourceID: "ext"}
	blocked, err := svc.Evaluate(context.Background(), ann, "run.start", []byte(`{"action":"read"}`), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocked {
		t.Error("expected low-risk message to pass through")
	}
}

func TestQuarantineEvaluate_Quarantined(t *testing.T) {
	store := newMockQuarantineStore()
	svc := NewQuarantineService(store, &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	})

	// Untrusted (0.5) + path traversal (0.2) = 0.7 >= threshold
	ann := &trust.Annotation{Origin: "a2a", TrustLevel: trust.LevelUntrusted, SourceID: "ext"}
	blocked, err := svc.Evaluate(context.Background(), ann, "run.start", []byte(`{"path":"../../etc/passwd"}`), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Error("expected message to be quarantined")
	}

	// Verify message stored
	if len(store.messages) != 1 {
		t.Fatalf("expected 1 quarantined message, got %d", len(store.messages))
	}
	for _, msg := range store.messages {
		if msg.Status != quarantine.StatusPending {
			t.Errorf("expected pending status, got %s", msg.Status)
		}
	}
}

func TestQuarantineEvaluate_AutoBlocked(t *testing.T) {
	store := newMockQuarantineStore()
	svc := NewQuarantineService(store, &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	})

	// Risk: untrusted 0.5 + A2A 0.1 + shell 0.3 + SQL 0.2 = 1.0 >= block threshold 0.95
	ann := &trust.Annotation{Origin: "a2a", TrustLevel: trust.LevelUntrusted, SourceID: "evil"}
	payload := `{"cmd": "; rm -rf /", "query": "DROP TABLE users"}`
	blocked, err := svc.Evaluate(context.Background(), ann, "run.start", []byte(payload), "proj-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !blocked {
		t.Error("expected message to be auto-blocked")
	}

	for _, msg := range store.messages {
		if msg.Status != quarantine.StatusRejected {
			t.Errorf("expected rejected status for auto-block, got %s", msg.Status)
		}
	}
}

func TestQuarantineApprove(t *testing.T) {
	store := newMockQuarantineStore()
	q := &mockQueue{}
	svc := NewQuarantineService(store, q, &mockBroadcaster{}, config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	})

	// Quarantine a message first
	ann := &trust.Annotation{Origin: "a2a", TrustLevel: trust.LevelUntrusted, SourceID: "ext"}
	blocked, _ := svc.Evaluate(context.Background(), ann, "run.start", []byte(`{"path":"../../etc/passwd"}`), "proj-1")
	if !blocked {
		t.Fatal("expected quarantine")
	}

	// Get the message ID
	var msgID string
	for id := range store.messages {
		msgID = id
		break
	}

	// Approve
	err := svc.Approve(context.Background(), msgID, "admin-1", "looks safe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := store.messages[msgID]
	if msg.Status != quarantine.StatusApproved {
		t.Errorf("expected approved status, got %s", msg.Status)
	}
	if msg.ReviewedBy != "admin-1" {
		t.Errorf("expected reviewed_by admin-1, got %s", msg.ReviewedBy)
	}

	// Verify replay published to NATS
	if len(q.published) == 0 {
		t.Error("expected message to be published to NATS after approval")
	}
}

func TestQuarantineReject(t *testing.T) {
	store := newMockQuarantineStore()
	svc := NewQuarantineService(store, &mockQueue{}, &mockBroadcaster{}, config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	})

	// Quarantine a message
	ann := &trust.Annotation{Origin: "a2a", TrustLevel: trust.LevelUntrusted, SourceID: "ext"}
	blocked, _ := svc.Evaluate(context.Background(), ann, "run.start", []byte(`{"path":"../../etc/passwd"}`), "proj-1")
	if !blocked {
		t.Fatal("expected quarantine")
	}

	var msgID string
	for id := range store.messages {
		msgID = id
		break
	}

	err := svc.Reject(context.Background(), msgID, "admin-1", "definitely malicious")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := store.messages[msgID]
	if msg.Status != quarantine.StatusRejected {
		t.Errorf("expected rejected status, got %s", msg.Status)
	}
}
