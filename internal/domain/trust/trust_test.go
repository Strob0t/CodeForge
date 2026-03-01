package trust

import "testing"

func TestInternal(t *testing.T) {
	a := Internal("agent-123")

	if a.Origin != "internal" {
		t.Errorf("Origin = %q, want %q", a.Origin, "internal")
	}
	if a.TrustLevel != LevelFull {
		t.Errorf("TrustLevel = %q, want %q", a.TrustLevel, LevelFull)
	}
	if a.SourceID != "agent-123" {
		t.Errorf("SourceID = %q, want %q", a.SourceID, "agent-123")
	}
	if a.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestIsInternal(t *testing.T) {
	tests := []struct {
		name string
		ann  Annotation
		want bool
	}{
		{
			name: "full trust internal",
			ann:  Annotation{Origin: "internal", TrustLevel: LevelFull},
			want: true,
		},
		{
			name: "a2a origin",
			ann:  Annotation{Origin: "a2a", TrustLevel: LevelFull},
			want: false,
		},
		{
			name: "internal but untrusted",
			ann:  Annotation{Origin: "internal", TrustLevel: LevelUntrusted},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ann.IsInternal(); got != tt.want {
				t.Errorf("IsInternal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRank(t *testing.T) {
	tests := []struct {
		level Level
		want  int
	}{
		{LevelFull, 3},
		{LevelVerified, 2},
		{LevelPartial, 1},
		{LevelUntrusted, 0},
		{Level("unknown"), -1},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := Rank(tt.level); got != tt.want {
				t.Errorf("Rank(%q) = %d, want %d", tt.level, got, tt.want)
			}
		})
	}

	// Verify ordering: full > verified > partial > untrusted
	if Rank(LevelFull) <= Rank(LevelVerified) {
		t.Error("full should rank higher than verified")
	}
	if Rank(LevelVerified) <= Rank(LevelPartial) {
		t.Error("verified should rank higher than partial")
	}
	if Rank(LevelPartial) <= Rank(LevelUntrusted) {
		t.Error("partial should rank higher than untrusted")
	}
}

func TestMeetsMinimum(t *testing.T) {
	tests := []struct {
		name  string
		level Level
		min   Level
		want  bool
	}{
		{"full meets full", LevelFull, LevelFull, true},
		{"full meets verified", LevelFull, LevelVerified, true},
		{"full meets untrusted", LevelFull, LevelUntrusted, true},
		{"verified meets verified", LevelVerified, LevelVerified, true},
		{"verified does not meet full", LevelVerified, LevelFull, false},
		{"untrusted meets untrusted", LevelUntrusted, LevelUntrusted, true},
		{"untrusted does not meet partial", LevelUntrusted, LevelPartial, false},
		{"partial meets partial", LevelPartial, LevelPartial, true},
		{"partial does not meet verified", LevelPartial, LevelVerified, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Annotation{TrustLevel: tt.level}
			if got := a.MeetsMinimum(tt.min); got != tt.want {
				t.Errorf("MeetsMinimum(%q) = %v, want %v", tt.min, got, tt.want)
			}
		})
	}
}
