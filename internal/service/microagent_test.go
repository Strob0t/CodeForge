package service

import (
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/microagent"
)

func TestMatchesTrigger_ReDoSProtection(t *testing.T) {
	t.Parallel()

	// Classic ReDoS pattern: catastrophic backtracking on non-matching input.
	// Go's regexp uses a linear-time engine so it won't hang, but we still
	// verify the function completes quickly with our safety bounds in place.
	pattern := "(a+)+b"
	input := strings.Repeat("a", 10_000) // long non-matching input

	done := make(chan bool, 1)
	go func() {
		matchesTrigger(pattern, input)
		done <- true
	}()

	select {
	case <-done:
		// completed in time
	case <-time.After(5 * time.Second):
		t.Fatal("matchesTrigger did not complete within 5s -- possible ReDoS")
	}
}

func TestMatchesTrigger_InvalidRegex(t *testing.T) {
	t.Parallel()

	// Invalid regex pattern (unclosed bracket) must return false, not panic.
	got := matchesTrigger("[invalid", "hello world")
	if got {
		t.Error("matchesTrigger([invalid, ...) = true, want false")
	}
}

func TestMatchesTrigger_ValidSubstring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"docker", "How do I use Docker?", true}, // case-insensitive
		{"python", "Working with Python files", true},
		{"rust", "Go programming guide", false},
		{"", "anything", true}, // empty pattern always matches via Contains; blocked by Validate()
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			t.Parallel()
			got := matchesTrigger(tt.pattern, tt.text)
			if got != tt.want {
				t.Errorf("matchesTrigger(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestMatchesTrigger_ValidRegex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		{
			name:    "caret prefix matches",
			pattern: "^hello",
			text:    "hello world",
			want:    true,
		},
		{
			name:    "caret prefix no match",
			pattern: "^hello",
			text:    "say hello",
			want:    false,
		},
		{
			name:    "paren prefix matches",
			pattern: "(error|warning)",
			text:    "found an error in code",
			want:    true,
		},
		{
			name:    "paren prefix no match",
			pattern: "(error|warning)",
			text:    "all good",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchesTrigger(tt.pattern, tt.text)
			if got != tt.want {
				t.Errorf("matchesTrigger(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestMatchesTrigger_PatternTooLong(t *testing.T) {
	t.Parallel()

	// Pattern exceeding MaxTriggerPatternLength must be rejected.
	longPattern := "^" + strings.Repeat("a", microagent.MaxTriggerPatternLength+1)
	got := matchesTrigger(longPattern, "aaa")
	if got {
		t.Error("matchesTrigger with oversized pattern = true, want false")
	}
}

func TestMatchesTrigger_InputTruncation(t *testing.T) {
	t.Parallel()

	// Pattern that matches text only beyond the 10K truncation boundary.
	// The match target "MARKER" is placed past the limit.
	input := strings.Repeat("x", maxTriggerInputLength) + "MARKER"
	got := matchesTrigger("^.*MARKER", input)
	if got {
		t.Error("matchesTrigger should not find MARKER beyond truncation limit")
	}

	// Same pattern with MARKER within the limit.
	shortInput := strings.Repeat("x", 100) + "MARKER"
	got2 := matchesTrigger("(MARKER)", shortInput)
	if !got2 {
		t.Error("matchesTrigger should find MARKER within truncation limit")
	}
}

func TestCreateRequest_Validate_RegexPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		wantErr string
	}{
		{
			name:    "valid substring pattern",
			pattern: "docker",
			wantErr: "",
		},
		{
			name:    "valid caret regex",
			pattern: "^test_.*\\.py$",
			wantErr: "",
		},
		{
			name:    "valid paren regex",
			pattern: "(error|warning|critical)",
			wantErr: "",
		},
		{
			name:    "invalid regex - unclosed bracket",
			pattern: "^[invalid",
			wantErr: "invalid trigger_pattern regex:",
		},
		{
			name:    "invalid regex - bad repetition",
			pattern: "(abc",
			wantErr: "invalid trigger_pattern regex:",
		},
		{
			name:    "pattern too long",
			pattern: strings.Repeat("a", microagent.MaxTriggerPatternLength+1),
			wantErr: "trigger_pattern exceeds maximum length of 512",
		},
		{
			name:    "pattern at max length (valid)",
			pattern: strings.Repeat("a", microagent.MaxTriggerPatternLength),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := microagent.CreateRequest{
				Name:           "test-agent",
				Type:           microagent.TypeKnowledge,
				TriggerPattern: tt.pattern,
				Prompt:         "test prompt",
			}
			err := req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() = %q, want error containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}
