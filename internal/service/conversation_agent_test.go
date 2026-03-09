package service

import (
	"fmt"
	"testing"
)

func TestPolicyForAutonomy(t *testing.T) {
	tests := []struct {
		autonomy int
		expected string
	}{
		{1, "supervised-ask-all"},
		{2, "headless-safe-sandbox"},
		{3, "headless-safe-sandbox"},
		{4, "trusted-mount-autonomous"},
		{5, "trusted-mount-autonomous"},
		{0, "headless-safe-sandbox"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("autonomy_%d", tc.autonomy), func(t *testing.T) {
			result := policyForAutonomy(tc.autonomy)
			if result != tc.expected {
				t.Errorf("policyForAutonomy(%d) = %q, want %q", tc.autonomy, result, tc.expected)
			}
		})
	}
}
