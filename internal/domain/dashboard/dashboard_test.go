package dashboard_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

func TestHealthScore_Clamp(t *testing.T) {
	tests := []struct {
		name  string
		input dashboard.HealthFactors
		want  int
	}{
		{
			name: "all perfect",
			input: dashboard.HealthFactors{
				SuccessRate:       100,
				ErrorRateInv:      100,
				ActivityFreshness: 100,
				TaskVelocity:      100,
				CostStability:     100,
			},
			want: 100,
		},
		{
			name:  "all zero",
			input: dashboard.HealthFactors{},
			want:  0,
		},
		{
			name: "typical healthy",
			input: dashboard.HealthFactors{
				SuccessRate:       92,
				ErrorRateInv:      95,
				ActivityFreshness: 80,
				TaskVelocity:      72,
				CostStability:     85,
			},
			want: 86, // 92*0.30 + 95*0.25 + 80*0.20 + 72*0.15 + 85*0.10 = 86.65 -> 86
		},
		{
			name: "values above 100 clamped",
			input: dashboard.HealthFactors{
				SuccessRate:       150,
				ErrorRateInv:      150,
				ActivityFreshness: 150,
				TaskVelocity:      150,
				CostStability:     150,
			},
			want: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Score()
			if got != tt.want {
				t.Errorf("Score() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHealthLevel(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{87, "healthy"},
		{75, "healthy"},
		{74, "warning"},
		{40, "warning"},
		{39, "critical"},
		{0, "critical"},
		{100, "healthy"},
	}
	for _, tt := range tests {
		got := dashboard.HealthLevel(tt.score)
		if got != tt.want {
			t.Errorf("HealthLevel(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}
