package tenant

import "testing"

func TestIsToolUID(t *testing.T) {
	tests := []struct {
		uid  int
		want bool
	}{
		{0, false},
		{LegacyToolUID, false},
		{10001, false},
		{SystemToolUID, false},
		{ToolUIDMin, true},
		{ToolUIDMin + 1, true},
		{ToolUIDMax, true},
		{ToolUIDMax + 1, false},
		{-1, false},
	}
	for _, tt := range tests {
		if got := IsToolUID(tt.uid); got != tt.want {
			t.Errorf("IsToolUID(%d) = %v, want %v", tt.uid, got, tt.want)
		}
	}
}
