package project

import (
	"encoding/json"
	"maps"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestConfigPatchApply(t *testing.T) {
	stored := map[string]string{
		"policy_preset":      "trusted-mount-autonomous",
		"execution_mode":     "mount",
		"detected_languages": `["go"]`,
		"expansion_prompt":   "expand",
	}

	tests := []struct {
		name    string
		current map[string]string
		patch   ConfigPatch
		want    map[string]string
	}{
		{
			name:    "new key is added, others kept",
			current: stored,
			patch:   ConfigPatch{"autonomy_level": strPtr("3")},
			want: map[string]string{
				"policy_preset": "trusted-mount-autonomous", "execution_mode": "mount",
				"detected_languages": `["go"]`, "expansion_prompt": "expand", "autonomy_level": "3",
			},
		},
		{
			name:    "existing key is overwritten",
			current: stored,
			patch:   ConfigPatch{"policy_preset": strPtr("plan-readonly")},
			want: map[string]string{
				"policy_preset": "plan-readonly", "execution_mode": "mount",
				"detected_languages": `["go"]`, "expansion_prompt": "expand",
			},
		},
		{
			name:    "null value removes the key",
			current: stored,
			patch:   ConfigPatch{"expansion_prompt": nil},
			want: map[string]string{
				"policy_preset": "trusted-mount-autonomous", "execution_mode": "mount",
				"detected_languages": `["go"]`,
			},
		},
		{
			name:    "removing a missing key is a no-op",
			current: stored,
			patch:   ConfigPatch{"missing": nil},
			want:    stored,
		},
		{
			name:    "empty string is a value, not a removal",
			current: stored,
			patch:   ConfigPatch{"expansion_prompt": strPtr("")},
			want: map[string]string{
				"policy_preset": "trusted-mount-autonomous", "execution_mode": "mount",
				"detected_languages": `["go"]`, "expansion_prompt": "",
			},
		},
		{name: "empty patch keeps everything", current: stored, patch: ConfigPatch{}, want: stored},
		{name: "nil patch keeps everything", current: stored, patch: nil, want: stored},
		{
			name:    "nil config gets the patch",
			current: nil,
			patch:   ConfigPatch{"policy_preset": strPtr("x"), "gone": nil},
			want:    map[string]string{"policy_preset": "x"},
		},
		{name: "nil config and nil patch give an empty config", current: nil, patch: nil, want: map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := maps.Clone(tt.current)
			got := tt.patch.Apply(tt.current)
			if got == nil {
				t.Fatal("Apply returned a nil map")
			}
			if !maps.Equal(got, tt.want) {
				t.Fatalf("Apply = %v, want %v", got, tt.want)
			}
			if !maps.Equal(tt.current, before) {
				t.Fatalf("Apply modified its input: %v, was %v", tt.current, before)
			}
		})
	}
}

// A JSON null in the request body must reach Apply as a removal.
func TestConfigPatchJSON(t *testing.T) {
	var req UpdateRequest
	if err := json.Unmarshal([]byte(`{"config":{"expansion_prompt":null,"policy_preset":"x"}}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := req.Config.Apply(map[string]string{"expansion_prompt": "e", "execution_mode": "mount"})
	want := map[string]string{"policy_preset": "x", "execution_mode": "mount"}
	if !maps.Equal(got, want) {
		t.Fatalf("Apply = %v, want %v", got, want)
	}

	var noConfig UpdateRequest
	if err := json.Unmarshal([]byte(`{"name":"n"}`), &noConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if noConfig.Config != nil {
		t.Fatalf("absent config decoded as %v, want nil", noConfig.Config)
	}
}
