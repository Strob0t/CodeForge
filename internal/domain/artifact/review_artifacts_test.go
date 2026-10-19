package artifact_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/artifact"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
)

// The review pipeline modes (boundary_analyzer, contract_reviewer) and the
// debate modes (proponent, moderator) require artifacts the validator did
// not know, so every run in them failed artifact validation.

func TestValidate_ReviewAndDebateArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		typ       string
		output    string
		wantValid bool
	}{
		{"boundaries fenced", "BOUNDARIES.json", "Found:\n```json\n[{\"path\": \"api.proto\", \"type\": \"api\"}]\n```", true},
		{"boundaries bare", "BOUNDARIES.json", `[{"path": "db/001.sql", "type": "data"}]`, true},
		{"boundaries none found", "BOUNDARIES.json", "[]", true},
		{"boundaries prose only", "BOUNDARIES.json", "I could not find any boundaries.", false},
		{"boundaries object", "BOUNDARIES.json", `{"path": "api.proto"}`, false},
		{"boundaries array of strings", "BOUNDARIES.json", `["api.proto"]`, false},
		{"contract review", "CONTRACT_REVIEW.md", "# Contract review\nNo inconsistencies.", true},
		{"contract review empty", "CONTRACT_REVIEW.md", "  \n\t", false},
		{"proposal", "PROPOSAL.md", "Use the existing cache.", true},
		{"proposal empty", "PROPOSAL.md", "", false},
		{"synthesis", "SYNTHESIS.md", "Both proposals agree.", true},
		{"synthesis empty", "SYNTHESIS.md", " ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := artifact.Validate(tt.typ, tt.output)
			if res.Valid != tt.wantValid {
				t.Fatalf("Validate(%s) valid = %v (%v), want %v", tt.typ, res.Valid, res.Errors, tt.wantValid)
			}
		})
	}
}

// Every built-in mode requires an artifact the validator knows: a mode with
// an unknown type fails every run it is used in.
func TestBuiltinModes_RequireKnownArtifacts(t *testing.T) {
	for _, m := range mode.BuiltinModes() {
		if m.RequiredArtifact != "" && !artifact.IsKnownType(m.RequiredArtifact) {
			t.Errorf("mode %s requires unknown artifact type %q", m.ID, m.RequiredArtifact)
		}
	}
}
