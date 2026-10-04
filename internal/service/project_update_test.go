package service

import (
	"context"
	"maps"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

func configPtr(s string) *string { return &s }

// KI-41: the settings popover sent {"config": {"autonomy_level": ...}} and
// Update replaced the whole config, dropping every other key.
func TestProjectServiceUpdate_ConfigIsMerged(t *testing.T) {
	stored := map[string]string{
		"policy_preset":      "trusted-mount-autonomous",
		"execution_mode":     "mount",
		"detected_languages": `["go","typescript"]`,
		"expansion_prompt":   "expand queries",
	}

	tests := []struct {
		name string
		req  project.UpdateRequest
		want map[string]string
	}{
		{
			name: "setting one key keeps the others",
			req:  project.UpdateRequest{Config: project.ConfigPatch{"policy_preset": configPtr("plan-readonly")}},
			want: map[string]string{
				"policy_preset": "plan-readonly", "execution_mode": "mount",
				"detected_languages": `["go","typescript"]`, "expansion_prompt": "expand queries",
			},
		},
		{
			name: "null removes only that key",
			req:  project.UpdateRequest{Config: project.ConfigPatch{"expansion_prompt": nil}},
			want: map[string]string{
				"policy_preset": "trusted-mount-autonomous", "execution_mode": "mount",
				"detected_languages": `["go","typescript"]`,
			},
		},
		{
			name: "an update without config keeps it",
			req:  project.UpdateRequest{Name: configPtr("renamed")},
			want: stored,
		},
		{
			name: "an empty config patch keeps it",
			req:  project.UpdateRequest{Config: project.ConfigPatch{}},
			want: stored,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha", Config: maps.Clone(stored)}}}
			svc := NewProjectService(store, t.TempDir())

			got, err := svc.Update(context.Background(), "p1", tt.req)
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if !maps.Equal(got.Config, tt.want) {
				t.Fatalf("returned config = %v, want %v", got.Config, tt.want)
			}
			if !maps.Equal(store.projects[0].Config, tt.want) {
				t.Fatalf("stored config = %v, want %v", store.projects[0].Config, tt.want)
			}
		})
	}
}
