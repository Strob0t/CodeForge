package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// fileMarkers maps spec format names to their file/directory indicators.
var fileMarkers = map[string][]string{
	"roadmap_md":   {"ROADMAP.md", "roadmap.md", "docs/roadmap.md", "docs/ROADMAP.md"},
	"todo_md":      {"TODO.md", "todo.md", "docs/TODO.md", "docs/todo.md"},
	"changelog_md": {"CHANGELOG.md", "changelog.md"},
	"openspec":     {"openspec/"},
	"speckit":      {".specify/"},
	"autospec":     {"specs/spec.yaml", "specs/spec.yml"},
}

// AutoDetect scans a workspace for known spec file markers.
// It first consults registered spec providers (openspec, speckit, autospec),
// then falls back to hardcoded fileMarkers for formats without a registered provider.
func (s *RoadmapService) AutoDetect(ctx context.Context, projectID string) (*roadmap.DetectionResult, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if proj.WorkspacePath == "" {
		return &roadmap.DetectionResult{Found: false}, nil
	}

	result := &roadmap.DetectionResult{}
	coveredFormats := map[string]bool{}

	// Phase 1: Ask registered spec providers.
	for _, prov := range s.specProvs {
		detected, err := prov.Detect(ctx, proj.WorkspacePath)
		if err != nil {
			slog.Warn("spec provider detect error", "provider", prov.Name(), "error", err)
			continue
		}
		if detected {
			result.Found = true
			result.FileMarkers = append(result.FileMarkers, prov.Name())
			result.Format = prov.Name()
		}
		coveredFormats[prov.Name()] = true
	}

	// Phase 2: Fallback to hardcoded fileMarkers for formats without a provider.
	seen := map[string]bool{}
	for format, markers := range fileMarkers {
		if coveredFormats[format] || coveredFormats[formatAlias(format)] {
			continue
		}
		for _, marker := range markers {
			fullPath := filepath.Join(proj.WorkspacePath, marker)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			if strings.HasSuffix(marker, "/") && info.IsDir() {
				result.Found = true
				result.FileMarkers = append(result.FileMarkers, marker)
				result.Format = format
				result.Path = fullPath
				seen[fullPath] = true
			} else if !info.IsDir() {
				result.Found = true
				result.FileMarkers = append(result.FileMarkers, marker)
				result.Format = format
				result.Path = fullPath
				seen[fullPath] = true
			}
		}
	}

	// Phase 3: Shallow scan of root and docs/ for .md files with relevant keywords.
	for _, found := range scanMarkdownKeywords(proj.WorkspacePath) {
		if seen[found] {
			continue
		}
		rel, err := filepath.Rel(proj.WorkspacePath, found)
		if err != nil {
			rel = found
		}
		result.Found = true
		result.FileMarkers = append(result.FileMarkers, rel)
		if result.Format == "" {
			result.Format = "keyword_scan"
		}
		seen[found] = true
	}

	// Phase 4: Detect PM platforms from git remote URL and project config.
	result.Platforms = detectPlatforms(proj)
	if len(result.Platforms) > 0 {
		result.Found = true
	}

	return result, nil
}

// formatAlias maps fileMarkers keys to provider names for deduplication.
func formatAlias(format string) string {
	aliases := map[string]string{
		"roadmap_md":   "markdown",
		"todo_md":      "markdown",
		"changelog_md": "markdown",
		"openspec":     "openspec",
	}
	if alias, ok := aliases[format]; ok {
		return alias
	}
	return format
}

// keywordScanDirs lists directories to scan relative to the workspace root.
// An empty string represents the root itself.
var keywordScanDirs = []string{"", "docs"}

// keywordScanTerms are matched case-insensitively inside .md files.
var keywordScanTerms = []string{"roadmap", "todo", "spec", "feature", "milestone"}

// scanMarkdownKeywords performs a shallow scan of root and docs/ for .md files
// containing relevant keywords. Returns absolute paths of matching files.
func scanMarkdownKeywords(workspacePath string) []string {
	var matches []string

	for _, dir := range keywordScanDirs {
		scanDir := filepath.Join(workspacePath, dir)
		entries, err := os.ReadDir(scanDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			fullPath := filepath.Join(scanDir, entry.Name())
			if containsKeyword(fullPath) {
				matches = append(matches, fullPath)
			}
		}
	}

	return matches
}

// containsKeyword reads a file line by line and returns true if any line
// contains one of the keywordScanTerms (case-insensitive). Stops at 200 lines
// to keep the scan shallow.
func containsKeyword(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path is constructed from workspace root + known subdirs
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 200 {
			break
		}
		lower := strings.ToLower(scanner.Text())
		for _, kw := range keywordScanTerms {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

// ImportSpecs discovers specs in the workspace via providers and imports them
// into the roadmap as milestones and features. It uses an upsert pattern:
// milestones are matched by title, features by spec_ref, to prevent duplicates
// on re-import.
func (s *RoadmapService) ImportSpecs(ctx context.Context, projectID string) (*roadmap.ImportResult, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if proj.WorkspacePath == "" {
		return nil, fmt.Errorf("project has no workspace path")
	}

	result := &roadmap.ImportResult{Source: "spec-providers"}

	// Ensure a roadmap exists.
	rm, err := s.getOrCreateRoadmap(ctx, projectID, proj.Name)
	if err != nil {
		return nil, fmt.Errorf("get/create roadmap: %w", err)
	}

	for _, prov := range s.specProvs {
		detected, err := prov.Detect(ctx, proj.WorkspacePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s detect: %v", prov.Name(), err))
			continue
		}
		if !detected {
			continue
		}

		specs, err := prov.ListSpecs(ctx, proj.WorkspacePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s list: %v", prov.Name(), err))
			continue
		}
		if len(specs) == 0 {
			continue
		}

		// Upsert milestone: find existing by title or create new.
		msTitle := fmt.Sprintf("Imported from %s", prov.Name())
		ms, err := s.store.FindMilestoneByTitle(ctx, rm.ID, msTitle)
		if err != nil {
			if !errors.Is(err, domain.ErrNotFound) {
				result.Errors = append(result.Errors, fmt.Sprintf("find milestone for %s: %v", prov.Name(), err))
				continue
			}
			ms, err = s.store.CreateMilestone(ctx, roadmap.CreateMilestoneRequest{
				RoadmapID:   rm.ID,
				Title:       msTitle,
				Description: fmt.Sprintf("Specs discovered by the %s provider", prov.Name()),
			})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create milestone for %s: %v", prov.Name(), err))
				continue
			}
			result.MilestonesCreated++
		}

		// If the provider supports item-level parsing, import individual items
		// as separate features instead of one feature per file.
		itemParser, hasItemParser := prov.(specprovider.ItemParser)

		for _, spec := range specs {
			if hasItemParser {
				s.importSpecItems(ctx, itemParser, proj.WorkspacePath, spec, ms, prov.Name(), result)
				continue
			}

			// Fallback: one feature per spec file.
			s.upsertFeature(ctx, ms.ID, spec.Title, spec.Path, prov.Name(), result)
		}
	}

	return result, nil
}

// importSpecItems parses individual items from a spec file and creates/updates
// one feature per item. The spec_ref format is "specPath#Lline".
func (s *RoadmapService) importSpecItems(
	ctx context.Context,
	parser specprovider.ItemParser,
	workspacePath string,
	spec specprovider.Spec,
	ms *roadmap.Milestone,
	provName string,
	result *roadmap.ImportResult,
) {
	items, err := parser.ParseItems(ctx, workspacePath, spec.Path)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse items from %s: %v", spec.Path, err))
		return
	}

	for _, item := range items {
		specRef := fmt.Sprintf("%s#L%d", spec.Path, item.SourceLine)
		s.upsertFeature(ctx, ms.ID, item.Title, specRef, provName, result)
	}
}

// upsertFeature finds an existing feature by spec_ref and updates it, or creates
// a new one. Updates the result counters accordingly.
func (s *RoadmapService) upsertFeature(
	ctx context.Context,
	milestoneID, title, specRef, provName string,
	result *roadmap.ImportResult,
) {
	existing, findErr := s.store.FindFeatureBySpecRef(ctx, milestoneID, specRef)
	if findErr != nil && !errors.Is(findErr, domain.ErrNotFound) {
		result.Errors = append(result.Errors, fmt.Sprintf("find feature %q: %v", title, findErr))
		return
	}

	if existing != nil {
		if existing.Title != title {
			existing.Title = title
			if err := s.store.UpdateFeature(ctx, existing); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update feature %q: %v", title, err))
				return
			}
			result.FeaturesUpdated++
		}
		return
	}

	_, err := s.store.CreateFeature(ctx, &roadmap.CreateFeatureRequest{
		MilestoneID: milestoneID,
		Title:       title,
		SpecRef:     specRef,
		Labels:      []string{provName},
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("create feature %q: %v", title, err))
		return
	}
	result.FeaturesCreated++
}

// ImportPMItems imports work items from a PM provider into the roadmap.
func (s *RoadmapService) ImportPMItems(ctx context.Context, projectID, providerName, projectRef string) (*roadmap.ImportResult, error) {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Find the requested PM provider.
	var prov pmprovider.Provider
	for _, p := range s.pmProvs {
		if p.Name() == providerName {
			prov = p
			break
		}
	}
	if prov == nil {
		return nil, fmt.Errorf("unknown PM provider: %s", providerName)
	}

	result := &roadmap.ImportResult{Source: providerName}

	items, err := prov.ListItems(ctx, projectRef)
	if err != nil {
		return nil, fmt.Errorf("list items from %s: %w", providerName, err)
	}

	// Ensure a roadmap exists.
	rm, err := s.getOrCreateRoadmap(ctx, projectID, proj.Name)
	if err != nil {
		return nil, fmt.Errorf("get/create roadmap: %w", err)
	}

	// Create a milestone for this import.
	ms, err := s.store.CreateMilestone(ctx, roadmap.CreateMilestoneRequest{
		RoadmapID:   rm.ID,
		Title:       fmt.Sprintf("Imported from %s", providerName),
		Description: fmt.Sprintf("Work items imported from %s (%s)", providerName, projectRef),
	})
	if err != nil {
		return nil, fmt.Errorf("create milestone: %w", err)
	}
	result.MilestonesCreated++

	for i := range items {
		item := &items[i]
		_, err := s.store.CreateFeature(ctx, &roadmap.CreateFeatureRequest{
			MilestoneID: ms.ID,
			Title:       item.Title,
			Description: item.Description,
			Labels:      item.Labels,
			ExternalIDs: map[string]string{providerName: item.ExternalID},
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("create feature %q: %v", item.Title, err))
			continue
		}
		result.FeaturesCreated++
	}

	return result, nil
}

// getOrCreateRoadmap returns the existing roadmap for a project or creates one.
func (s *RoadmapService) getOrCreateRoadmap(ctx context.Context, projectID, projectName string) (*roadmap.Roadmap, error) {
	rm, err := s.store.GetRoadmapByProject(ctx, projectID)
	if err == nil {
		return rm, nil
	}

	// Create a new roadmap.
	return s.store.CreateRoadmap(ctx, roadmap.CreateRoadmapRequest{
		ProjectID:   projectID,
		Title:       fmt.Sprintf("%s Roadmap", projectName),
		Description: "Auto-created during import",
	})
}
