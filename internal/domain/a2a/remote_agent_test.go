package a2a

import "testing"

func TestRemoteAgent_Valid(t *testing.T) {
	agent := NewRemoteAgent("test-agent", "https://example.com/agent")
	if err := agent.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoteAgent_MissingName(t *testing.T) {
	agent := NewRemoteAgent("", "https://example.com")
	if err := agent.Validate(); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRemoteAgent_MissingURL(t *testing.T) {
	agent := NewRemoteAgent("test", "")
	if err := agent.Validate(); err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestRemoteAgent_InvalidScheme(t *testing.T) {
	agent := NewRemoteAgent("test", "ftp://example.com")
	if err := agent.Validate(); err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestRemoteAgent_TrustDefault(t *testing.T) {
	agent := NewRemoteAgent("test", "https://example.com")
	if agent.TrustLevel != "partial" {
		t.Errorf("expected partial, got %s", agent.TrustLevel)
	}
}

func TestRemoteAgent_SkillsNilSafety(t *testing.T) {
	agent := NewRemoteAgent("test", "https://example.com")
	if agent.Skills == nil {
		t.Fatal("skills should not be nil")
	}
}
