package service

import "testing"

func TestDiffImpactScorer_LowImpact(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold:      50,
		ApprovalThreshold:       200,
		AlwaysApproveBoundary:   true,
		AlwaysApproveStructural: true,
	})
	stats := DiffStats{FilesChanged: 2, LinesAdded: 20, LinesRemoved: 5, CrossLayer: false, Structural: false}
	level := scorer.Score(stats)
	if level != ImpactLow {
		t.Errorf("Score() = %q, want %q", level, ImpactLow)
	}
}

func TestDiffImpactScorer_MediumImpact(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold: 50, ApprovalThreshold: 200,
		AlwaysApproveBoundary: true, AlwaysApproveStructural: true,
	})
	stats := DiffStats{FilesChanged: 5, LinesAdded: 60, LinesRemoved: 20}
	level := scorer.Score(stats)
	if level != ImpactMedium {
		t.Errorf("Score() = %q, want %q", level, ImpactMedium)
	}
}

func TestDiffImpactScorer_HighImpact(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold: 50, ApprovalThreshold: 200,
		AlwaysApproveBoundary: true, AlwaysApproveStructural: true,
	})
	stats := DiffStats{FilesChanged: 10, LinesAdded: 150, LinesRemoved: 80}
	level := scorer.Score(stats)
	if level != ImpactHigh {
		t.Errorf("Score() = %q, want %q", level, ImpactHigh)
	}
}

func TestDiffImpactScorer_CrossLayerAlwaysHigh(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold: 50, ApprovalThreshold: 200,
		AlwaysApproveBoundary: true,
	})
	stats := DiffStats{FilesChanged: 1, LinesAdded: 5, LinesRemoved: 2, CrossLayer: true}
	level := scorer.Score(stats)
	if level != ImpactHigh {
		t.Errorf("Score() = %q, want %q for cross-layer change", level, ImpactHigh)
	}
}

func TestDiffImpactScorer_StructuralAlwaysHigh(t *testing.T) {
	scorer := NewDiffImpactScorer(DiffImpactConfig{
		AutoApplyThreshold: 50, ApprovalThreshold: 200,
		AlwaysApproveStructural: true,
	})
	stats := DiffStats{FilesChanged: 1, LinesAdded: 3, Structural: true}
	level := scorer.Score(stats)
	if level != ImpactHigh {
		t.Errorf("Score() = %q, want %q for structural change", level, ImpactHigh)
	}
}
