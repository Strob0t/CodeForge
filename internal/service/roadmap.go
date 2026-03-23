package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// RoadmapService manages roadmaps, milestones, and features.
type RoadmapService struct {
	store     database.Store
	hub       broadcast.Broadcaster
	specProvs []specprovider.Provider
	pmProvs   []pmprovider.Provider
}

// NewRoadmapService creates a new RoadmapService.
func NewRoadmapService(store database.Store, hub broadcast.Broadcaster, specProvs []specprovider.Provider, pmProvs []pmprovider.Provider) *RoadmapService {
	return &RoadmapService{store: store, hub: hub, specProvs: specProvs, pmProvs: pmProvs}
}

// --- Roadmaps ---

// Create creates a new roadmap for a project.
func (s *RoadmapService) Create(ctx context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	if err := roadmap.ValidateCreateRoadmap(&req); err != nil {
		return nil, err
	}

	r, err := s.store.CreateRoadmap(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create roadmap: %w", err)
	}

	s.broadcastStatus(ctx, r)
	return r, nil
}

// GetByProject returns the roadmap for a project with all milestones and features.
// Uses batch loading to avoid N+1 queries: milestones and features are each
// loaded in a single query, then features are grouped by milestone in Go.
func (s *RoadmapService) GetByProject(ctx context.Context, projectID string) (*roadmap.Roadmap, error) {
	r, err := s.store.GetRoadmapByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	milestones, err := s.store.ListMilestones(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("load milestones: %w", err)
	}

	// Batch-load all features for the roadmap in a single query.
	allFeatures, err := s.store.ListFeaturesByRoadmap(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("load features: %w", err)
	}

	// Group features by milestone ID.
	featuresByMilestone := make(map[string][]roadmap.Feature, len(milestones))
	for _, f := range allFeatures {
		featuresByMilestone[f.MilestoneID] = append(featuresByMilestone[f.MilestoneID], f)
	}

	for i := range milestones {
		milestones[i].Features = featuresByMilestone[milestones[i].ID]
	}

	r.Milestones = milestones
	return r, nil
}

// Update updates a roadmap's title, description, and status.
func (s *RoadmapService) Update(ctx context.Context, r *roadmap.Roadmap) error {
	if err := roadmap.ValidateRoadmapStatus(r.Status); err != nil {
		return err
	}

	if err := s.store.UpdateRoadmap(ctx, r); err != nil {
		return err
	}

	s.broadcastStatus(ctx, r)
	return nil
}

// Delete removes a roadmap and all its milestones/features.
func (s *RoadmapService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteRoadmap(ctx, id)
}

// --- Milestones ---

// CreateMilestone creates a new milestone within a roadmap.
func (s *RoadmapService) CreateMilestone(ctx context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	if err := roadmap.ValidateCreateMilestone(&req); err != nil {
		return nil, err
	}
	return s.store.CreateMilestone(ctx, req)
}

// UpdateMilestone updates a milestone.
func (s *RoadmapService) UpdateMilestone(ctx context.Context, m *roadmap.Milestone) error {
	if err := roadmap.ValidateRoadmapStatus(m.Status); err != nil {
		return err
	}
	return s.store.UpdateMilestone(ctx, m)
}

// DeleteMilestone removes a milestone and its features.
func (s *RoadmapService) DeleteMilestone(ctx context.Context, id string) error {
	return s.store.DeleteMilestone(ctx, id)
}

// --- Features ---

// CreateFeature creates a new feature within a milestone.
func (s *RoadmapService) CreateFeature(ctx context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	if err := roadmap.ValidateCreateFeature(req); err != nil {
		return nil, err
	}
	return s.store.CreateFeature(ctx, req)
}

// UpdateFeature updates a feature.
func (s *RoadmapService) UpdateFeature(ctx context.Context, f *roadmap.Feature) error {
	if err := roadmap.ValidateFeatureStatus(f.Status); err != nil {
		return err
	}
	return s.store.UpdateFeature(ctx, f)
}

// DeleteFeature removes a feature.
func (s *RoadmapService) DeleteFeature(ctx context.Context, id string) error {
	return s.store.DeleteFeature(ctx, id)
}

// GetMilestone returns a single milestone by ID.
func (s *RoadmapService) GetMilestone(ctx context.Context, id string) (*roadmap.Milestone, error) {
	return s.store.GetMilestone(ctx, id)
}

// GetFeature returns a single feature by ID.
func (s *RoadmapService) GetFeature(ctx context.Context, id string) (*roadmap.Feature, error) {
	return s.store.GetFeature(ctx, id)
}

// --- Auto-Detect ---

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

// --- AI View ---

// AIView returns an LLM-optimized representation of a project's roadmap.
func (s *RoadmapService) AIView(ctx context.Context, projectID, format string) (*roadmap.AIRoadmapView, error) {
	r, err := s.GetByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	view := &roadmap.AIRoadmapView{
		ProjectID:   projectID,
		Format:      format,
		GeneratedAt: time.Now(),
	}

	switch format {
	case "json":
		data, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("marshal roadmap: %w", err)
		}
		view.Content = string(data)
		view.RawData = data
	case "yaml":
		view.Content = renderYAML(r)
	default:
		view.Content = renderMarkdown(r)
		view.Format = "markdown"
	}

	return view, nil
}

func renderMarkdown(r *roadmap.Roadmap) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Title)
	if r.Description != "" {
		b.WriteString(r.Description + "\n\n")
	}
	fmt.Fprintf(&b, "Status: %s\n\n", r.Status)

	for i := range r.Milestones {
		m := &r.Milestones[i]
		fmt.Fprintf(&b, "## %s [%s]\n\n", m.Title, m.Status)
		if m.Description != "" {
			b.WriteString(m.Description + "\n\n")
		}
		for j := range m.Features {
			f := &m.Features[j]
			checkbox := "[ ]"
			if f.Status == roadmap.FeatureDone {
				checkbox = "[x]"
			}
			fmt.Fprintf(&b, "- %s %s [%s]", checkbox, f.Title, f.Status)
			if len(f.Labels) > 0 {
				fmt.Fprintf(&b, " (%s)", strings.Join(f.Labels, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func renderYAML(r *roadmap.Roadmap) string {
	var b strings.Builder
	fmt.Fprintf(&b, "title: %q\n", r.Title)
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	b.WriteString("milestones:\n")

	for i := range r.Milestones {
		m := &r.Milestones[i]
		fmt.Fprintf(&b, "  - title: %q\n", m.Title)
		fmt.Fprintf(&b, "    status: %s\n", m.Status)
		b.WriteString("    features:\n")
		for j := range m.Features {
			f := &m.Features[j]
			fmt.Fprintf(&b, "      - title: %q\n", f.Title)
			fmt.Fprintf(&b, "        status: %s\n", f.Status)
			if len(f.Labels) > 0 {
				b.WriteString("        labels:\n")
				for _, l := range f.Labels {
					fmt.Fprintf(&b, "          - %q\n", l)
				}
			}
		}
	}

	return b.String()
}

// SyncToSpecFile writes the current roadmap state back to a markdown spec file
// in the project's workspace. If an ItemWriter provider is available and features
// have line-level spec_refs, it performs item-level write-back preserving the
// original file structure. Otherwise it falls back to a full markdown render.
func (s *RoadmapService) SyncToSpecFile(ctx context.Context, projectID string) error {
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if proj.WorkspacePath == "" {
		return fmt.Errorf("project has no workspace path")
	}

	// Load roadmap with milestones and features.
	rm, err := s.GetByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get roadmap: %w", err)
	}

	// Try item-level write-back via spec providers that support ItemWriter.
	if s.syncViaItemWriter(ctx, proj.WorkspacePath, rm) {
		slog.Info("synced roadmap via item writer", "project", projectID)
		return nil
	}

	// Fallback: full markdown render.
	targetPath := s.findSpecFile(proj.WorkspacePath)
	content := renderMarkdown(rm)

	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil { //nolint:gosec // workspace path is trusted
		return fmt.Errorf("write spec file: %w", err)
	}

	slog.Info("synced roadmap to spec file", "project", projectID, "path", targetPath)
	return nil
}

// syncViaItemWriter attempts to write features back to spec files using
// ItemWriter providers. It groups features by their spec file (from spec_ref)
// and writes each file separately. Returns true if at least one file was synced.
func (s *RoadmapService) syncViaItemWriter(ctx context.Context, workspacePath string, rm *roadmap.Roadmap) bool {
	// Find an ItemWriter provider.
	var writer specprovider.ItemWriter
	for _, prov := range s.specProvs {
		if w, ok := prov.(specprovider.ItemWriter); ok {
			writer = w
			break
		}
	}
	if writer == nil {
		return false
	}

	// Group features by spec file path (extracted from spec_ref "path#Lline").
	type featureRef struct {
		feature *roadmap.Feature
		line    int
	}
	fileFeatures := map[string][]featureRef{}

	for i := range rm.Milestones {
		for j := range rm.Milestones[i].Features {
			f := &rm.Milestones[i].Features[j]
			filePath, line := parseSpecRef(f.SpecRef)
			if filePath == "" || line == 0 {
				continue
			}
			fileFeatures[filePath] = append(fileFeatures[filePath], featureRef{feature: f, line: line})
		}
	}

	if len(fileFeatures) == 0 {
		return false
	}

	synced := false
	for filePath, refs := range fileFeatures {
		// Read current file to get existing items, then update statuses.
		parser, ok := writer.(specprovider.ItemParser)
		if !ok {
			continue
		}

		items, err := parser.ParseItems(ctx, workspacePath, filePath)
		if err != nil {
			slog.Warn("sync write-back: failed to parse spec file", "path", filePath, "error", err)
			continue
		}

		// Build a lookup from line number to feature status.
		lineStatus := make(map[int]string, len(refs))
		for _, ref := range refs {
			lineStatus[ref.line] = featureStatusToItemStatus(ref.feature.Status)
		}

		// Update item statuses from features.
		for i := range items {
			if status, ok := lineStatus[items[i].SourceLine]; ok {
				items[i].Status = status
			}
		}

		if err := writer.WriteItems(ctx, workspacePath, filePath, items); err != nil {
			slog.Warn("sync write-back: failed to write spec file", "path", filePath, "error", err)
			continue
		}
		synced = true
	}

	return synced
}

// parseSpecRef extracts the file path and line number from a spec_ref
// in the format "path/to/file.md#L42". Returns ("", 0) if the format
// is not recognized.
func parseSpecRef(specRef string) (filePath string, line int) {
	idx := strings.LastIndex(specRef, "#L")
	if idx < 0 {
		return "", 0
	}
	filePath = specRef[:idx]
	if _, err := fmt.Sscanf(specRef[idx:], "#L%d", &line); err != nil {
		return "", 0
	}
	return filePath, line
}

// featureStatusToItemStatus maps roadmap feature status to spec item status.
func featureStatusToItemStatus(status roadmap.FeatureStatus) string {
	switch status {
	case roadmap.FeatureDone:
		return "done"
	case roadmap.FeatureInProgress:
		return "in_progress"
	default:
		return "todo"
	}
}

// findSpecFile returns the path to the first existing spec file candidate,
// or defaults to ROADMAP.md in the workspace root.
func (s *RoadmapService) findSpecFile(workspacePath string) string {
	for _, name := range []string{
		"ROADMAP.md", "roadmap.md", "TODO.md", "todo.md",
		"docs/ROADMAP.md", "docs/roadmap.md", "docs/TODO.md", "docs/todo.md",
	} {
		fp := filepath.Join(workspacePath, name)
		info, statErr := os.Stat(fp)
		if statErr == nil && !info.IsDir() {
			return fp
		}
	}
	return filepath.Join(workspacePath, "ROADMAP.md")
}

func (s *RoadmapService) broadcastStatus(ctx context.Context, r *roadmap.Roadmap) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastEvent(ctx, event.EventRoadmapStatus, event.RoadmapStatusEvent{
		RoadmapID: r.ID,
		ProjectID: r.ProjectID,
		Status:    string(r.Status),
		Title:     r.Title,
	})
}
