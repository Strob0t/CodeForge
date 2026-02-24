// Package service implements business logic on top of ports.
package service

import "context"

// RoadmapSpecDetector adapts *RoadmapService to the SpecDetector interface
// used by ProjectService.SetupProject.
type RoadmapSpecDetector struct {
	roadmap *RoadmapService
}

// NewRoadmapSpecDetector creates a SpecDetector backed by the given RoadmapService.
func NewRoadmapSpecDetector(r *RoadmapService) *RoadmapSpecDetector {
	return &RoadmapSpecDetector{roadmap: r}
}

// DetectAndImport runs auto-detection and, if specs are found, imports them.
func (a *RoadmapSpecDetector) DetectAndImport(ctx context.Context, projectID string) (bool, error) {
	det, err := a.roadmap.AutoDetect(ctx, projectID)
	if err != nil {
		return false, err
	}
	if !det.Found {
		return false, nil
	}

	_, importErr := a.roadmap.ImportSpecs(ctx, projectID)
	if importErr != nil {
		return true, importErr
	}

	return true, nil
}
