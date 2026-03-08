package llmkey

import (
	"testing"
)

func TestValidate_EmptyProvider(t *testing.T) {
	req := CreateRequest{Provider: "", Label: "test", APIKey: "sk-xxx"}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestValidate_InvalidProvider(t *testing.T) {
	req := CreateRequest{Provider: "invalid-provider", Label: "test", APIKey: "sk-xxx"}
	err := req.Validate()
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
	if err.Error() != "unsupported provider: invalid-provider" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidate_EmptyLabel(t *testing.T) {
	req := CreateRequest{Provider: "openai", Label: "", APIKey: "sk-xxx"}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestValidate_EmptyAPIKey(t *testing.T) {
	req := CreateRequest{Provider: "openai", Label: "test", APIKey: ""}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for empty api_key")
	}
}

func TestValidate_AllProviders(t *testing.T) {
	for provider := range AllowedProviders {
		req := CreateRequest{Provider: provider, Label: "test", APIKey: "sk-xxx"}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected valid for provider %q: %v", provider, err)
		}
	}
}

func TestValidate_Success(t *testing.T) {
	req := CreateRequest{Provider: "openai", Label: "My Key", APIKey: "sk-abcdef123456"}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMakeKeyPrefix_LongKey(t *testing.T) {
	prefix := MakeKeyPrefix("sk-abcdef1234567890")
	if prefix != "sk-abcde****" {
		t.Fatalf("got %q, want %q", prefix, "sk-abcde****")
	}
}

func TestMakeKeyPrefix_ShortKey(t *testing.T) {
	prefix := MakeKeyPrefix("sk-ab")
	if prefix != "sk****" {
		t.Fatalf("got %q, want %q", prefix, "sk****")
	}
}

func TestMakeKeyPrefix_ExactlyEight(t *testing.T) {
	prefix := MakeKeyPrefix("sk-abcde")
	if prefix != "sk-a****" {
		t.Fatalf("got %q, want %q", prefix, "sk-a****")
	}
}
