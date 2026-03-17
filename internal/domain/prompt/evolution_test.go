package prompt

import (
	"testing"
)

func TestValidSignalType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input SignalType
		want  bool
	}{
		{"benchmark valid", SignalBenchmark, true},
		{"success valid", SignalSuccess, true},
		{"cost valid", SignalCost, true},
		{"user valid", SignalUser, true},
		{"efficiency valid", SignalEfficiency, true},
		{"empty invalid", SignalType(""), false},
		{"unknown invalid", SignalType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidSignalType(tt.input); got != tt.want {
				t.Errorf("ValidSignalType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompositeScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scores map[SignalType]float64
		want   float64
		delta  float64
	}{
		{
			name:   "empty scores returns zero",
			scores: map[SignalType]float64{},
			want:   0,
			delta:  0,
		},
		{
			name:   "nil scores returns zero",
			scores: nil,
			want:   0,
			delta:  0,
		},
		{
			name: "all signals at 1.0 returns 1.0",
			scores: map[SignalType]float64{
				SignalBenchmark:  1.0,
				SignalSuccess:    1.0,
				SignalCost:       1.0,
				SignalUser:       1.0,
				SignalEfficiency: 1.0,
			},
			want:  1.0,
			delta: 0.001,
		},
		{
			name: "all signals at 0.0 returns 0.0",
			scores: map[SignalType]float64{
				SignalBenchmark:  0.0,
				SignalSuccess:    0.0,
				SignalCost:       0.0,
				SignalUser:       0.0,
				SignalEfficiency: 0.0,
			},
			want:  0.0,
			delta: 0.001,
		},
		{
			name: "single signal uses its weight",
			scores: map[SignalType]float64{
				SignalBenchmark: 0.8,
			},
			// 0.8 * 0.35 / 0.35 = 0.8
			want:  0.8,
			delta: 0.001,
		},
		{
			name: "mixed signals weighted correctly",
			scores: map[SignalType]float64{
				SignalBenchmark: 0.9,
				SignalSuccess:   0.7,
			},
			// (0.9*0.35 + 0.7*0.25) / (0.35+0.25) = (0.315+0.175)/0.60 = 0.8167
			want:  0.8167,
			delta: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CompositeScore(tt.scores)
			if got < tt.want-tt.delta || got > tt.want+tt.delta {
				t.Errorf("CompositeScore() = %f, want %f (±%f)", got, tt.want, tt.delta)
			}
		})
	}
}

func TestFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("empty entries returns consistent hash", func(t *testing.T) {
		t.Parallel()
		fp := Fingerprint(nil)
		if fp == "" {
			t.Error("fingerprint of nil should not be empty string")
		}
		fp2 := Fingerprint([]PromptEntry{})
		if fp != fp2 {
			t.Error("nil and empty should produce same fingerprint")
		}
	})

	t.Run("same entries produce same fingerprint", func(t *testing.T) {
		t.Parallel()
		entries := []PromptEntry{
			{ID: "a", Content: "Hello"},
			{ID: "b", Content: "World"},
		}
		fp1 := Fingerprint(entries)
		fp2 := Fingerprint(entries)
		if fp1 != fp2 {
			t.Error("same entries should produce same fingerprint")
		}
	})

	t.Run("order does not matter (sorted internally)", func(t *testing.T) {
		t.Parallel()
		entries1 := []PromptEntry{
			{ID: "a", Content: "Hello"},
			{ID: "b", Content: "World"},
		}
		entries2 := []PromptEntry{
			{ID: "b", Content: "World"},
			{ID: "a", Content: "Hello"},
		}
		if Fingerprint(entries1) != Fingerprint(entries2) {
			t.Error("different order should produce same fingerprint")
		}
	})

	t.Run("different content produces different fingerprint", func(t *testing.T) {
		t.Parallel()
		entries1 := []PromptEntry{{ID: "a", Content: "Hello"}}
		entries2 := []PromptEntry{{ID: "a", Content: "Goodbye"}}
		if Fingerprint(entries1) == Fingerprint(entries2) {
			t.Error("different content should produce different fingerprint")
		}
	})

	t.Run("fingerprint is hex string of expected length", func(t *testing.T) {
		t.Parallel()
		fp := Fingerprint([]PromptEntry{{ID: "test", Content: "data"}})
		if len(fp) != 64 { // SHA256 hex = 64 chars
			t.Errorf("fingerprint length = %d, want 64", len(fp))
		}
	})
}

func TestDefaultEvolutionConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultEvolutionConfig()
	if !cfg.Enabled {
		t.Error("default config should be enabled")
	}
	if cfg.Trigger != TriggerBenchmark {
		t.Errorf("default trigger = %q, want %q", cfg.Trigger, TriggerBenchmark)
	}
	if cfg.MinFailures != 5 {
		t.Errorf("default min_failures = %d, want 5", cfg.MinFailures)
	}
	if cfg.MinTrials != 30 {
		t.Errorf("default min_trials = %d, want 30", cfg.MinTrials)
	}
	if cfg.ImprovementThreshold != 0.05 {
		t.Errorf("default improvement_threshold = %f, want 0.05", cfg.ImprovementThreshold)
	}
	if cfg.MaxVariantsPerMode != 3 {
		t.Errorf("default max_variants_per_mode = %d, want 3", cfg.MaxVariantsPerMode)
	}
	if cfg.PromotionStrategy != StrategyAuto {
		t.Errorf("default strategy = %q, want %q", cfg.PromotionStrategy, StrategyAuto)
	}
}

func TestEvolutionConfig_StrategyForMode(t *testing.T) {
	t.Parallel()

	t.Run("returns global strategy when no override", func(t *testing.T) {
		t.Parallel()
		cfg := EvolutionConfig{
			PromotionStrategy: StrategyAuto,
		}
		if got := cfg.StrategyForMode("coder"); got != StrategyAuto {
			t.Errorf("got %q, want %q", got, StrategyAuto)
		}
	})

	t.Run("returns mode override when present", func(t *testing.T) {
		t.Parallel()
		cfg := EvolutionConfig{
			PromotionStrategy: StrategyAuto,
			ModeOverrides: map[string]ModeEvolutionOverride{
				"reviewer": {PromotionStrategy: StrategyManual},
			},
		}
		if got := cfg.StrategyForMode("reviewer"); got != StrategyManual {
			t.Errorf("got %q, want %q", got, StrategyManual)
		}
	})

	t.Run("falls back to global when override has empty strategy", func(t *testing.T) {
		t.Parallel()
		cfg := EvolutionConfig{
			PromotionStrategy: StrategyShadow,
			ModeOverrides: map[string]ModeEvolutionOverride{
				"coder": {PromotionStrategy: ""},
			},
		}
		if got := cfg.StrategyForMode("coder"); got != StrategyShadow {
			t.Errorf("got %q, want %q", got, StrategyShadow)
		}
	})

	t.Run("nil overrides map returns global", func(t *testing.T) {
		t.Parallel()
		cfg := EvolutionConfig{
			PromotionStrategy: StrategyManual,
		}
		if got := cfg.StrategyForMode("anything"); got != StrategyManual {
			t.Errorf("got %q, want %q", got, StrategyManual)
		}
	})
}
