package http

import (
	"testing"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		url         string
		contentType string
		want        string
	}{
		{"https://example.com/skill.yaml", "", "codeforge"},
		{"https://example.com/skill.yml", "", "codeforge"},
		{"https://example.com/rules.mdc", "", "cursor"},
		{"https://example.com/.cursorrules", "", "cursor"},
		{"https://example.com/commit.md", "text/markdown", "markdown"},
		{"https://example.com/commit.md", "application/yaml", "claude"},
		{"https://example.com/readme.txt", "", "markdown"},
	}
	for _, tt := range tests {
		got := detectFormat(tt.url, tt.contentType)
		if got != tt.want {
			t.Errorf("detectFormat(%q, %q) = %q, want %q", tt.url, tt.contentType, got, tt.want)
		}
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/tdd-workflow.yaml", "tdd-workflow"},
		{"https://example.com/path/to/skill.md", "skill"},
		{"https://example.com/skill.md?token=abc", "skill"},
		{"https://example.com/", "imported-skill"},
		{"plain-name.yaml", "plain-name"},
	}
	for _, tt := range tests {
		got := extractName(tt.url)
		if got != tt.want {
			t.Errorf("extractName(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
