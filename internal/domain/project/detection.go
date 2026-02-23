package project

// Language represents a detected programming language.
type Language struct {
	Name       string   `json:"name"`       // e.g. "go", "typescript", "python", "rust"
	Confidence float64  `json:"confidence"` // 0.0–1.0
	Manifests  []string `json:"manifests"`  // files that triggered detection
	Frameworks []string `json:"frameworks"` // e.g. ["chi", "solidjs", "django"]
}

// ToolRecommendation holds a recommended tool/mode/pipeline for the detected stack.
type ToolRecommendation struct {
	Category string `json:"category"` // "mode", "pipeline", "linter", "formatter"
	ID       string `json:"id"`       // e.g. "coder", "standard-dev", "golangci-lint"
	Name     string `json:"name"`     // human-readable name
	Reason   string `json:"reason"`   // e.g. "Go project detected"
}

// StackDetectionResult is the output of scanning a workspace.
type StackDetectionResult struct {
	Languages       []Language           `json:"languages"`
	Recommendations []ToolRecommendation `json:"recommendations"`
	ScannedPath     string               `json:"scanned_path"`
}
