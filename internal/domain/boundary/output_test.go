package boundary_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// S6-F 10: the BOUNDARIES.json array is found in prose that has brackets of
// its own before and after it.

func TestFromOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string // paths; nil for an error
	}{
		{"bare array", `[{"path": "api.proto", "type": "api"}]`, []string{"api.proto"}},
		{"empty array", "[]", []string{}},
		{
			"fenced block wins over an earlier bare array",
			"Draft: [{\"path\": \"old.proto\", \"type\": \"api\"}]\nFinal:\n```json\n[{\"path\": \"api.proto\", \"type\": \"api\"}]\n```",
			[]string{"api.proto"},
		},
		{
			"prose brackets before and after",
			"I checked the files [1] and [the migrations].\n[{\"path\": \"db/001.sql\", \"type\": \"data\"}]\nSee [2] for details.",
			[]string{"db/001.sql"},
		},
		{
			"array of strings in prose is skipped",
			"Candidates: [\"a.go\", \"b.go\"]. Result:\n[{\"path\": \"schema.graphql\", \"type\": \"api\"}]",
			[]string{"schema.graphql"},
		},
		{
			"an empty array before a found one",
			"Previously [] boundaries were known; now:\n[{\"path\": \"api.proto\", \"type\": \"api\"}] (see [docs])",
			[]string{"api.proto"},
		},
		{
			"invalid fenced block falls back to the text",
			"```json\nnot json\n```\n[{\"path\": \"api.proto\", \"type\": \"api\"}]",
			[]string{"api.proto"},
		},
		{"prose only", "I could not find any boundaries.", nil},
		{"brackets but no array of boundaries", "Files [a.go] and [b.go] look fine; [1, 2]", nil},
		{"an object, not an array", `{"path": "api.proto"}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := boundary.FromOutput(tt.output)
			if tt.want == nil {
				if !errors.Is(err, boundary.ErrNoBoundariesJSON) {
					t.Fatalf("FromOutput = %+v, %v, want ErrNoBoundariesJSON", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FromOutput: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("FromOutput = %+v, want paths %v", got, tt.want)
			}
			for i := range got {
				if got[i].Path != tt.want[i] {
					t.Fatalf("entry %d = %q, want %q", i, got[i].Path, tt.want[i])
				}
			}
		})
	}
}
