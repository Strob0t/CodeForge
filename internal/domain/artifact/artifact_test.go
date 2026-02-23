package artifact

import (
	"strings"
	"testing"
)

func TestValidate_EmptyType(t *testing.T) {
	r := Validate("", "anything")
	if !r.Valid {
		t.Fatal("empty artifact type should always pass")
	}
}

func TestValidate_UnknownType(t *testing.T) {
	r := Validate("UNKNOWN", "anything")
	if r.Valid {
		t.Fatal("unknown artifact type should fail")
	}
	if len(r.Errors) == 0 || !strings.Contains(r.Errors[0], "unknown artifact type") {
		t.Fatalf("expected unknown type error, got: %v", r.Errors)
	}
}

func TestIsKnownType(t *testing.T) {
	known := []string{"PLAN.md", "DIFF", "REVIEW.md", "TEST_REPORT", "AUDIT_REPORT", "DECISION.md"}
	for _, k := range known {
		if !IsKnownType(k) {
			t.Errorf("expected %q to be known", k)
		}
	}
	if IsKnownType("UNKNOWN") {
		t.Error("expected UNKNOWN to be unknown")
	}
}

func TestValidate_PlanMD(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"valid", "## Architecture Overview\n\nThis plan describes the architecture for the new system. " +
			"We will implement a layered design with clear separation of concerns between the frontend and backend layers.", false},
		{"too short", "## Short\nNot enough", true},
		{"no header", strings.Repeat("x", 120), true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate("PLAN.md", tt.output)
			if r.Valid == tt.wantErr {
				t.Errorf("Valid=%v, wantErr=%v, errors=%v", r.Valid, tt.wantErr, r.Errors)
			}
		})
	}
}

func TestValidate_Diff(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"valid diff", "--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n+import \"fmt\"\n func main() {", false},
		{"minus line", "- removed line\n+ added line", false},
		{"no changes", "No changes needed for this task.", false},
		{"no changes mixed case", "There were NO CHANGES required.", false},
		{"empty", "", true},
		{"no markers", "just some text without diff markers", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate("DIFF", tt.output)
			if r.Valid == tt.wantErr {
				t.Errorf("Valid=%v, wantErr=%v, errors=%v", r.Valid, tt.wantErr, r.Errors)
			}
		})
	}
}

func TestValidate_ReviewMD(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"valid", "# Code Review\n\n## Findings\n\nFound an issue with error handling in the main function.", false},
		{"with suggestion", "# Review\n\nSuggestion: use a constant instead of magic number.", false},
		{"too short", "# R\nshort", true},
		{"no header", "Found an issue but no markdown header present in this output text at all.", true},
		{"no keyword", "# Review\n\nThe code looks good, nothing to report here at all in detail.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate("REVIEW.md", tt.output)
			if r.Valid == tt.wantErr {
				t.Errorf("Valid=%v, wantErr=%v, errors=%v", r.Valid, tt.wantErr, r.Errors)
			}
		})
	}
}

func TestValidate_TestReport(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"with status", `{"status": "passed", "tests": 10, "failures": 0}`, false},
		{"with passed", `{"passed": true, "total": 5}`, false},
		{"not json", "this is not json", true},
		{"json array", `[1, 2, 3]`, true},
		{"no status key", `{"total": 5, "coverage": 80}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate("TEST_REPORT", tt.output)
			if r.Valid == tt.wantErr {
				t.Errorf("Valid=%v, wantErr=%v, errors=%v", r.Valid, tt.wantErr, r.Errors)
			}
		})
	}
}

func TestValidate_AuditReport(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"valid", "# Security Audit\n\n## Findings\n\nFound a vulnerability in the authentication module.", false},
		{"with risk", "# Audit\n\nIdentified a risk in the input validation layer of the application.", false},
		{"too short", "# A\nshort", true},
		{"no keyword", "# Audit\n\nThe code has no issues found in this particular review pass overall.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate("AUDIT_REPORT", tt.output)
			if r.Valid == tt.wantErr {
				t.Errorf("Valid=%v, wantErr=%v, errors=%v", r.Valid, tt.wantErr, r.Errors)
			}
		})
	}
}

func TestValidate_DecisionMD(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"with rationale", "## Decision\n\nWe chose approach A. Rationale: it has lower complexity.", false},
		{"with reason", "The decision was to use PostgreSQL. The reason is that it supports JSONB natively.", false},
		{"no decision", "The rationale for this choice is cost efficiency across the board.", true},
		{"no rationale", "The decision was to proceed with the current architecture plan as designed.", true},
		{"too short", "decision reason", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Validate("DECISION.md", tt.output)
			if r.Valid == tt.wantErr {
				t.Errorf("Valid=%v, wantErr=%v, errors=%v", r.Valid, tt.wantErr, r.Errors)
			}
		})
	}
}

func TestValidationResult_ArtifactType(t *testing.T) {
	r := Validate("PLAN.md", "## Valid Plan\n\n"+strings.Repeat("This is a valid plan. ", 10))
	if r.ArtifactType != TypePlanMD {
		t.Errorf("expected ArtifactType=%q, got %q", TypePlanMD, r.ArtifactType)
	}
}
