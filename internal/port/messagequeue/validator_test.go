package messagequeue

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateValidTaskResult(t *testing.T) {
	data := []byte(`{"task_id":"t1","project_id":"p1","status":"completed","output":"ok","files":[],"error":"","tokens_in":10,"tokens_out":5,"cost_usd":0.001}`)
	if err := Validate(SubjectTaskResult, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateValidTaskCancel(t *testing.T) {
	data := []byte(`{"task_id":"t1"}`)
	if err := Validate(SubjectTaskCancel, data); err != nil {
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
	err := Validate(SubjectTaskCancel, data)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected 'invalid JSON' in error, got: %v", err)
	}
}

func TestValidateInvalidSchema(t *testing.T) {
	// Valid JSON but cannot unmarshal into TaskCancelPayload
	// (numbers where strings expected won't cause unmarshal errors in Go,
	// but completely wrong structure will)
	data := []byte(`"just a string"`)
	err := Validate(SubjectTaskCancel, data)
	if err == nil {
		t.Fatal("expected schema validation error")
	}
	if !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("expected 'schema validation failed' in error, got: %v", err)
	}
}

func TestValidateEmptyJSON(t *testing.T) {
	// Empty JSON objects are rejected — a valid payload must carry at least one field.
	data := []byte(`{}`)
	if err := Validate(SubjectTaskCancel, data); err == nil {
		t.Fatal("expected error for empty JSON object, got nil")
	}
}

func TestValidateNullPayload(t *testing.T) {
	// JSON null unmarshals into any struct without error, so it must be rejected
	// explicitly like the empty object.
	for _, data := range []string{`null`, ` null `} {
		if err := Validate(SubjectRunComplete, []byte(data)); err == nil {
			t.Fatalf("expected error for %q, got nil", data)
		}
	}
}

// schemaCase is one payload checked against the struct of its subject.
type schemaCase struct {
	name    string
	subject string
	data    string
	wantErr string // empty: payload must be accepted
}

func runSchemaCases(t *testing.T, cases []schemaCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.subject, []byte(tc.data))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate(%s) unexpected error: %v", tc.subject, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate(%s, %s) = nil, want error containing %q", tc.subject, tc.data, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate(%s) error = %v, want it to contain %q", tc.subject, err, tc.wantErr)
			}
		})
	}
}

const schemaErr = "schema validation failed"

// TestValidateRunSubjects checks that runs.* payloads are validated against
// their structs (KI-20) instead of being accepted as any JSON.
func TestValidateRunSubjects(t *testing.T) {
	runSchemaCases(t, []schemaCase{
		// runs.start (Go -> Python)
		{"start valid", SubjectRunStart, `{"run_id":"r1","task_id":"t1","project_id":"p1","agent_id":"a1","prompt":"fix it","policy_profile":"standard","exec_mode":"mount","termination":{"max_steps":50,"timeout_seconds":600,"max_cost":1.5},"config":{"k":"v"},"mode":{"id":"coder","prompt_prefix":"","tools":["bash"]}}`, ""},
		{"start run_id not a string", SubjectRunStart, `{"run_id":42,"task_id":"t1"}`, schemaErr},
		{"start termination not an object", SubjectRunStart, `{"run_id":"r1","termination":"forever"}`, schemaErr},
		{"start config values not strings", SubjectRunStart, `{"run_id":"r1","config":{"k":1}}`, schemaErr},
		{"start array payload", SubjectRunStart, `[{"run_id":"r1"}]`, schemaErr},

		// runs.toolcall.request (Python -> Go)
		{"toolcall request valid", SubjectRunToolCallRequest, `{"run_id":"r1","call_id":"c1","tool":"bash","command":"ls","path":""}`, ""},
		{"toolcall request tool not a string", SubjectRunToolCallRequest, `{"run_id":"r1","call_id":"c1","tool":["bash"]}`, schemaErr},

		// runs.toolcall.response (Go -> Python)
		{"toolcall response valid", SubjectRunToolCallResponse, `{"run_id":"r1","call_id":"c1","decision":"allow","reason":""}`, ""},
		{"toolcall response decision not a string", SubjectRunToolCallResponse, `{"run_id":"r1","call_id":"c1","decision":true}`, schemaErr},

		// runs.toolcall.result (Python -> Go), as RuntimeClient.report_tool_result sends it
		{"toolcall result valid", SubjectRunToolCallResult, `{"run_id":"r1","call_id":"c1","tool":"edit_file","success":true,"output":"ok","error":"","cost_usd":0.002,"tokens_in":120,"tokens_out":30,"model":"openai/gpt-4o","diff":{"path":"a.go","hunks":[]}}`, ""},
		{"toolcall result success not a bool", SubjectRunToolCallResult, `{"run_id":"r1","call_id":"c1","success":"yes"}`, schemaErr},
		{"toolcall result tokens not an integer", SubjectRunToolCallResult, `{"run_id":"r1","call_id":"c1","tokens_in":1.5}`, schemaErr},

		// runs.complete (Python -> Go), as RunCompleteMessage serializes
		{"complete valid", SubjectRunComplete, `{"run_id":"r1","task_id":"t1","tenant_id":"","project_id":"p1","status":"completed","output":"done","error":"","cost_usd":0.1,"step_count":3,"tokens_in":10,"tokens_out":5,"model":"m"}`, ""},
		{"complete step_count not an integer", SubjectRunComplete, `{"run_id":"r1","step_count":"3"}`, schemaErr},
		{"complete cost not a number", SubjectRunComplete, `{"run_id":"r1","cost_usd":"0.1"}`, schemaErr},

		// runs.output (Python -> Go)
		{"output valid", SubjectRunOutput, `{"run_id":"r1","task_id":"t1","line":"hello","stream":"stdout"}`, ""},
		{"output line not a string", SubjectRunOutput, `{"run_id":"r1","line":["hello"]}`, schemaErr},

		// runs.heartbeat (Python -> Go), timestamp is an ISO string
		{"heartbeat valid", SubjectRunHeartbeat, `{"run_id":"r1","timestamp":"2026-09-30T10:00:00+00:00"}`, ""},
		{"heartbeat timestamp not a string", SubjectRunHeartbeat, `{"run_id":"r1","timestamp":1727690400}`, schemaErr},
		{"conversation heartbeat valid", SubjectRunHeartbeat, `{"run_id":"c1","tenant_id":"t1","turn_id":"turn-1","timestamp":"2026-09-30T10:00:00+00:00"}`, ""},
		{"task heartbeat valid", SubjectTaskHeartbeat, `{"task_id":"t1","tenant_id":"t1","timestamp":"2026-09-30T10:00:00+00:00"}`, ""},
		{"task heartbeat task_id not a string", SubjectTaskHeartbeat, `{"task_id":7,"timestamp":"2026-09-30T10:00:00+00:00"}`, schemaErr},

		// runs.qualitygate.request (Go -> Python)
		{"qualitygate request valid", SubjectQualityGateRequest, `{"run_id":"r1","project_id":"p1","workspace_path":"/ws","run_tests":true,"run_lint":false,"test_command":"go test ./..."}`, ""},
		{"qualitygate request run_tests not a bool", SubjectQualityGateRequest, `{"run_id":"r1","run_tests":"true"}`, schemaErr},
		{"qualitygate request with timeout", SubjectQualityGateRequest, `{"run_id":"r1","project_id":"p1","workspace_path":"/ws","run_tests":true,"test_command":"pytest","timeout_seconds":60}`, ""},
		{"qualitygate request timeout not a number", SubjectQualityGateRequest, `{"run_id":"r1","timeout_seconds":"60s"}`, schemaErr},

		// runs.qualitygate.result (Python -> Go), unset gates are null
		{"qualitygate result valid", SubjectQualityGateResult, `{"run_id":"r1","tests_passed":true,"lint_passed":null,"test_output":"ok","lint_output":"","error":""}`, ""},
		{"qualitygate result tests_passed not a bool", SubjectQualityGateResult, `{"run_id":"r1","tests_passed":"yes"}`, schemaErr},

		// Subjects without a port struct stay JSON-only.
		{"cancel any object", SubjectRunCancel, `{"run_id":"r1"}`, ""},
		{"trajectory event any object", SubjectTrajectoryEvent, `{"run_id":"r1","event_type":"tool_call","step":1}`, ""},
		{"cancel invalid JSON", SubjectRunCancel, `{"run_id":`, "invalid JSON"},
	})
}

// TestValidateContextSubjects checks that context.* payloads are validated
// against their structs (KI-20).
func TestValidateContextSubjects(t *testing.T) {
	runSchemaCases(t, []schemaCase{
		{"shared updated valid", SubjectSharedUpdated, `{"team_id":"team1","project_id":"p1","key":"step-1","author":"agent-1","version":2}`, ""},
		{"shared updated version not an integer", SubjectSharedUpdated, `{"team_id":"team1","version":"2"}`, schemaErr},
		{"shared updated array payload", SubjectSharedUpdated, `["team1"]`, schemaErr},

		{"rerank request valid", SubjectContextRerankRequest, `{"request_id":"q1","project_id":"p1","query":"auth","entries":[{"path":"a.go","kind":"file","content":"x","priority":50,"tokens":3}],"model":"m"}`, ""},
		{"rerank request entries not a list", SubjectContextRerankRequest, `{"request_id":"q1","entries":{"path":"a.go"}}`, schemaErr},
		{"rerank request entry priority not an integer", SubjectContextRerankRequest, `{"request_id":"q1","entries":[{"path":"a.go","priority":"high"}]}`, schemaErr},

		{"rerank result valid", SubjectContextRerankResult, `{"request_id":"q1","entries":[],"fallback_used":false,"tokens_in":10,"tokens_out":2,"cost_usd":0.001,"error":""}`, ""},
		{"rerank result fallback_used not a bool", SubjectContextRerankResult, `{"request_id":"q1","fallback_used":"no"}`, schemaErr},
		{"rerank result invalid JSON", SubjectContextRerankResult, `{"request_id":`, "invalid JSON"},
	})
}

// TestValidateRepoMapSubjects checks that repomap.* payloads are validated
// against their structs (KI-20).
func TestValidateRepoMapSubjects(t *testing.T) {
	runSchemaCases(t, []schemaCase{
		{"request valid", SubjectRepoMapRequest, `{"project_id":"p1","workspace_path":"/ws","token_budget":1024,"active_files":["main.go"]}`, ""},
		{"request token_budget not an integer", SubjectRepoMapRequest, `{"project_id":"p1","token_budget":"1024"}`, schemaErr},
		{"request active_files not a list", SubjectRepoMapRequest, `{"project_id":"p1","active_files":"main.go"}`, schemaErr},

		{"result valid", SubjectRepoMapResult, `{"project_id":"p1","map_text":"main.go: main","token_count":12,"file_count":1,"symbol_count":1,"languages":["go"],"error":""}`, ""},
		{"result languages not a list", SubjectRepoMapResult, `{"project_id":"p1","languages":"go"}`, schemaErr},
		{"result file_count not an integer", SubjectRepoMapResult, `{"project_id":"p1","file_count":1.5}`, schemaErr},
		{"result empty object", SubjectRepoMapResult, `{}`, "empty JSON object"},
	})
}

// TestValidateHandoffRequestMetadata (S2-G fix, 4): handoff.request metadata
// is a map of strings. The worker JSON-encodes the LLM's non-string values
// (workers/codeforge/tools/handoff.py, test_handoff_metadata_values_are_strings);
// a raw nested or numeric value does not decode, so such a request would be
// dead-lettered.
func TestValidateHandoffRequestMetadata(t *testing.T) {
	fromWorker := []byte(`{"tenant_id":"t","project_id":"p","source_run_id":"r","target_agent_id":"a","context":"c",` +
		`"metadata":{"priority":"high","attempts":"3","urgent":"true","none":"null","files":"[\"a.go\", \"b.go\"]",` +
		`"nested":"{\"depth\": 2, \"tags\": [\"x\"]}","handoff_chain_id":"chain-1","handoff_hop":"2"}}`)
	if err := Validate(SubjectHandoffRequest, fromWorker); err != nil {
		t.Fatalf("the worker's handoff request does not validate: %v", err)
	}
	var p HandoffRequestPayload
	if err := json.Unmarshal(fromWorker, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Metadata["attempts"] != "3" || p.Metadata["nested"] != `{"depth": 2, "tags": ["x"]}` {
		t.Fatalf("metadata = %v", p.Metadata)
	}

	for _, raw := range []string{`{"attempts":3}`, `{"nested":{"depth":2}}`} {
		data := []byte(`{"tenant_id":"t","source_run_id":"r","target_agent_id":"a","context":"c","metadata":` + raw + `}`)
		if err := Validate(SubjectHandoffRequest, data); err == nil {
			t.Errorf("metadata %s validated; the worker must send strings", raw)
		}
	}
}
