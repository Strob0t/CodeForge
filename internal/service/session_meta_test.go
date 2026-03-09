package service

import (
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

func TestBuildSessionMeta_Fork(t *testing.T) {
	meta, _ := json.Marshal(map[string]string{
		"forked_from": "run-1",
		"from_event":  "evt-1",
		"prompt":      "continue",
	})
	sess := &run.Session{
		ParentSessionID: "sess-parent",
		ParentRunID:     "run-1",
		Metadata:        string(meta),
	}
	sm := buildSessionMeta(sess)
	if sm == nil {
		t.Fatal("expected non-nil SessionMetaPayload")
	}
	if sm.Operation != "fork" {
		t.Errorf("expected operation 'fork', got %q", sm.Operation)
	}
	if sm.ForkEventID != "evt-1" {
		t.Errorf("expected fork_event_id 'evt-1', got %q", sm.ForkEventID)
	}
	if sm.ParentSessionID != "sess-parent" {
		t.Errorf("expected parent_session_id 'sess-parent', got %q", sm.ParentSessionID)
	}
	if sm.ParentRunID != "run-1" {
		t.Errorf("expected parent_run_id 'run-1', got %q", sm.ParentRunID)
	}
}

func TestBuildSessionMeta_ForkConversation(t *testing.T) {
	meta, _ := json.Marshal(map[string]string{
		"forked_from_conversation": "conv-1",
		"from_event":               "evt-3",
	})
	sess := &run.Session{
		ParentSessionID: "sess-p2",
		Metadata:        string(meta),
	}
	sm := buildSessionMeta(sess)
	if sm == nil {
		t.Fatal("expected non-nil SessionMetaPayload")
	}
	if sm.Operation != "fork" {
		t.Errorf("expected operation 'fork', got %q", sm.Operation)
	}
	if sm.ForkEventID != "evt-3" {
		t.Errorf("expected fork_event_id 'evt-3', got %q", sm.ForkEventID)
	}
}

func TestBuildSessionMeta_Rewind(t *testing.T) {
	meta, _ := json.Marshal(map[string]string{
		"rewound_from": "run-2",
		"to_event":     "evt-2",
	})
	sess := &run.Session{
		ParentRunID: "run-2",
		Metadata:    string(meta),
	}
	sm := buildSessionMeta(sess)
	if sm == nil {
		t.Fatal("expected non-nil SessionMetaPayload")
	}
	if sm.Operation != "rewind" {
		t.Errorf("expected operation 'rewind', got %q", sm.Operation)
	}
	if sm.RewindEventID != "evt-2" {
		t.Errorf("expected rewind_event_id 'evt-2', got %q", sm.RewindEventID)
	}
}

func TestBuildSessionMeta_Resume(t *testing.T) {
	meta, _ := json.Marshal(map[string]string{
		"resumed_from": "run-3",
		"prompt":       "keep going",
	})
	sess := &run.Session{
		ParentRunID: "run-3",
		Metadata:    string(meta),
	}
	sm := buildSessionMeta(sess)
	if sm == nil {
		t.Fatal("expected non-nil SessionMetaPayload")
	}
	if sm.Operation != "resume" {
		t.Errorf("expected operation 'resume', got %q", sm.Operation)
	}
}

func TestBuildSessionMeta_EmptyMetadata(t *testing.T) {
	sess := &run.Session{Metadata: ""}
	if sm := buildSessionMeta(sess); sm != nil {
		t.Errorf("expected nil for empty metadata, got %+v", sm)
	}
}

func TestBuildSessionMeta_EmptyJSONObject(t *testing.T) {
	sess := &run.Session{Metadata: "{}"}
	if sm := buildSessionMeta(sess); sm != nil {
		t.Errorf("expected nil for empty JSON object, got %+v", sm)
	}
}

func TestBuildSessionMeta_InvalidJSON(t *testing.T) {
	sess := &run.Session{Metadata: "not json"}
	if sm := buildSessionMeta(sess); sm != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", sm)
	}
}

func TestBuildSessionMeta_UnrecognizedKeys(t *testing.T) {
	meta, _ := json.Marshal(map[string]string{
		"unknown_key": "value",
	})
	sess := &run.Session{Metadata: string(meta)}
	if sm := buildSessionMeta(sess); sm != nil {
		t.Errorf("expected nil for unrecognized metadata keys, got %+v", sm)
	}
}
