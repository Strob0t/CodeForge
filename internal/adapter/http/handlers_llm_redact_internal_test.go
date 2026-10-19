package http

import (
	"encoding/json"
	"strings"
	"testing"
)

// S6-G review, item 8: redactCredentials drops credential parameters by exact
// name or defined suffix (not by substring), redacts URL userinfo and
// credential query parameters in values, and walks nested maps and lists.
func TestRedactCredentials(t *testing.T) {
	var params map[string]any
	if err := json.Unmarshal([]byte(`{
		"model": "openai/gpt-4",
		"api_key": "sk-SECRET",
		"aws_access_key_id": "AKIA-SECRET",
		"aws_secret_access_key": "aws-SECRET",
		"aws_session_token": "sess-SECRET",
		"vertex_credentials": "vertex-SECRET",
		"max_tokens": 4096,
		"input_cost_per_token": 0.00001,
		"output_cost_per_token": 0.00003,
		"custom_tokenizer": {"identifier": "hf/tok", "revision": "main"},
		"api_base": "https://user:pw-SECRET@proxy.example.com/v1?key=q-SECRET&alt=json",
		"extra_headers": {"Authorization": "Bearer hdr-SECRET", "x-api-key": "xk-SECRET", "Copilot-Integration-Id": "vscode-chat"},
		"fallbacks": [{"api_key": "fb-SECRET", "model": "openai/gpt-4o"}, "https://t0k-SECRET@backup.example.com"],
		"tags": ["a", 1, true, null]
	}`), &params); err != nil {
		t.Fatal(err)
	}

	out, err := json.Marshal(redactCredentials(params))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	body := string(out)

	for _, secret := range []string{"sk-SECRET", "AKIA-SECRET", "aws-SECRET", "sess-SECRET", "vertex-SECRET", "pw-SECRET", "q-SECRET", "hdr-SECRET", "xk-SECRET", "fb-SECRET", "t0k-SECRET"} {
		if strings.Contains(body, secret) {
			t.Errorf("redacted params still contain %q: %s", secret, body)
		}
	}
	for _, kept := range []string{"model", "max_tokens", "input_cost_per_token", "output_cost_per_token", "custom_tokenizer", "api_base", "extra_headers", "fallbacks", "tags"} {
		if _, ok := got[kept]; !ok {
			t.Errorf("redacted params lost %q: %s", kept, body)
		}
	}
	if got["api_base"] != "https://[REDACTED]@proxy.example.com/v1?key=[REDACTED]&alt=json" {
		t.Errorf("api_base = %v", got["api_base"])
	}
	headers, ok := got["extra_headers"].(map[string]any)
	if !ok || headers["Copilot-Integration-Id"] != "vscode-chat" || len(headers) != 1 {
		t.Errorf("extra_headers = %v, want only the non-secret header", got["extra_headers"])
	}
	fallbacks, ok := got["fallbacks"].([]any)
	if !ok || len(fallbacks) != 2 {
		t.Fatalf("fallbacks = %v", got["fallbacks"])
	}
	if first, ok := fallbacks[0].(map[string]any); !ok || first["model"] != "openai/gpt-4o" || len(first) != 1 {
		t.Errorf("fallbacks[0] = %v, want only the model", fallbacks[0])
	}
	if fallbacks[1] != "https://[REDACTED]@backup.example.com" {
		t.Errorf("fallbacks[1] = %v", fallbacks[1])
	}
	if redactCredentials(nil) != nil {
		t.Error("redactCredentials(nil) != nil")
	}
}
