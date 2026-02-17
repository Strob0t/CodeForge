package messagequeue

import (
	"strings"
	"testing"
)

func TestValidateValidTaskCreated(t *testing.T) {
	data := []byte(`{"task_id":"t1","project_id":"p1","title":"Fix","prompt":"do it"}`)
	if err := Validate(SubjectTaskCreated, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateValidTaskResult(t *testing.T) {
	data := []byte(`{"task_id":"t1","project_id":"p1","status":"completed","output":"ok","files":[],"error":"","tokens_in":10,"tokens_out":5,"cost_usd":0.001}`)
	if err := Validate(SubjectTaskResult, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateValidTaskOutput(t *testing.T) {
	data := []byte(`{"task_id":"t1","project_id":"p1","agent_id":"a1","line":"hello world"}`)
	if err := Validate(SubjectTaskOutput, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateValidTaskCancel(t *testing.T) {
	data := []byte(`{"task_id":"t1"}`)
	if err := Validate(SubjectTaskCancel, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateValidAgentStatus(t *testing.T) {
	data := []byte(`{"agent_id":"a1","project_id":"p1","status":"running"}`)
	if err := Validate(SubjectAgentStatus, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTaskAgentSubject(t *testing.T) {
	// tasks.agent.{backend} accepts any valid JSON.
	data := []byte(`{"id":"t1","title":"test","arbitrary":"field"}`)
	if err := Validate(SubjectTaskAgent+".aider", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateUnknownSubject(t *testing.T) {
	// Unknown subjects should pass (future-proof).
	data := []byte(`{"foo":"bar"}`)
	if err := Validate("unknown.subject", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInvalidJSON(t *testing.T) {
	data := []byte(`{not valid json`)
	err := Validate(SubjectTaskCreated, data)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected 'invalid JSON' in error, got: %v", err)
	}
}

func TestValidateInvalidSchema(t *testing.T) {
	// Valid JSON but cannot unmarshal into TaskCreatedPayload
	// (numbers where strings expected won't cause unmarshal errors in Go,
	// but completely wrong structure will)
	data := []byte(`"just a string"`)
	err := Validate(SubjectTaskCreated, data)
	if err == nil {
		t.Fatal("expected schema validation error")
	}
	if !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("expected 'schema validation failed' in error, got: %v", err)
	}
}

func TestValidateEmptyJSON(t *testing.T) {
	// Empty object is valid JSON and valid for all schemas (all fields are zero-value).
	data := []byte(`{}`)
	if err := Validate(SubjectTaskCreated, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
