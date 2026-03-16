package service

// ImpactLevel classifies a diff's impact for routing to auto-apply, notify, or HITL approval.
type ImpactLevel string

const (
	ImpactLow    ImpactLevel = "low"
	ImpactMedium ImpactLevel = "medium"
	ImpactHigh   ImpactLevel = "high"
)

// DiffStats holds the measurable properties of a refactoring diff.
type DiffStats struct {
	FilesChanged int  `json:"files_changed"`
	LinesAdded   int  `json:"lines_added"`
	LinesRemoved int  `json:"lines_removed"`
	CrossLayer   bool `json:"cross_layer"`
	Structural   bool `json:"structural"`
}

// TotalLines returns the total number of lines touched by the diff.
func (d DiffStats) TotalLines() int {
	return d.LinesAdded + d.LinesRemoved
}

// DiffImpactConfig holds the thresholds and flags that govern impact scoring.
type DiffImpactConfig struct {
	AutoApplyThreshold      int  `json:"auto_apply_threshold"      yaml:"auto_apply_threshold"`
	ApprovalThreshold       int  `json:"approval_threshold"        yaml:"approval_threshold"`
	AlwaysApproveBoundary   bool `json:"always_approve_boundary"   yaml:"always_approve_boundary"`
	AlwaysApproveStructural bool `json:"always_approve_structural" yaml:"always_approve_structural"`
}

// DiffImpactScorer scores a DiffStats value as low, medium, or high impact.
// Cross-layer and structural changes are unconditionally high when the
// corresponding flag is enabled in the config.
type DiffImpactScorer struct {
	cfg DiffImpactConfig
}

// NewDiffImpactScorer returns a scorer configured with cfg.
func NewDiffImpactScorer(cfg DiffImpactConfig) *DiffImpactScorer {
	return &DiffImpactScorer{cfg: cfg}
}

// Score evaluates stats and returns the appropriate ImpactLevel.
func (s *DiffImpactScorer) Score(stats DiffStats) ImpactLevel {
	if s.cfg.AlwaysApproveBoundary && stats.CrossLayer {
		return ImpactHigh
	}
	if s.cfg.AlwaysApproveStructural && stats.Structural {
		return ImpactHigh
	}
	total := stats.TotalLines()
	if total >= s.cfg.ApprovalThreshold {
		return ImpactHigh
	}
	if total >= s.cfg.AutoApplyThreshold {
		return ImpactMedium
	}
	return ImpactLow
}
