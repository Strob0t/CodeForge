package service

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

func TestConversationPolicyProfile(t *testing.T) {
	tests := []struct {
		name     string
		proj     project.Project
		autonomy int
		want     string
	}{
		{"project field wins", project.Project{PolicyProfile: "plan-readonly", Config: map[string]string{"policy_preset": "trusted-mount-autonomous"}}, 4, "plan-readonly"},
		{"config preset", project.Project{Config: map[string]string{"policy_preset": "plan-readonly"}}, 4, "plan-readonly"},
		{"mode autonomy 4", project.Project{}, 4, "trusted-mount-autonomous"},
		{"mode autonomy 3", project.Project{}, 3, "headless-safe-sandbox"},
		{"mode autonomy 1", project.Project{}, 1, "supervised-ask-all"},
		{"no mode", project.Project{}, 0, "the-default"},
		{"empty config preset ignored", project.Project{Config: map[string]string{"policy_preset": ""}}, 0, "the-default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := conversationPolicyProfile(&tt.proj, tt.autonomy, "the-default"); got != tt.want {
				t.Errorf("conversationPolicyProfile = %q, want %q", got, tt.want)
			}
		})
	}
}
