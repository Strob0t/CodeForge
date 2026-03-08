package llmkey

import (
	"testing"
)

func TestCreateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRequest
		wantErr string
	}{
		{name: "valid", req: CreateRequest{Provider: "openai", Label: "My Key", APIKey: "sk-abc123"}, wantErr: ""},
		{name: "empty provider", req: CreateRequest{Provider: "", Label: "x", APIKey: "sk-abc"}, wantErr: "provider is required"},
		{name: "invalid provider", req: CreateRequest{Provider: "unknown", Label: "x", APIKey: "sk-abc"}, wantErr: "unsupported provider"},
		{name: "empty label", req: CreateRequest{Provider: "openai", Label: "", APIKey: "sk-abc"}, wantErr: "label is required"},
		{name: "empty api_key", req: CreateRequest{Provider: "openai", Label: "x", APIKey: ""}, wantErr: "api_key is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMakeKeyPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-abcdef1234567890", "sk-abcde****"},
		{"short", "sh****"},
		{"ab", "a****"},
		{"sk-proj-abc123def456", "sk-proj-****"},
	}
	for _, tt := range tests {
		got := MakeKeyPrefix(tt.input)
		if got != tt.want {
			t.Errorf("MakeKeyPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAllowedProviders(t *testing.T) {
	for _, p := range []string{"openai", "anthropic", "gemini", "groq", "mistral", "openrouter"} {
		if !AllowedProviders[p] {
			t.Errorf("expected %q to be allowed", p)
		}
	}
	if AllowedProviders["invalid"] {
		t.Error("expected 'invalid' to not be allowed")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
