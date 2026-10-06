package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
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

	// Phases 2 and 3 read the workspace through workspacefs (KI-95).
	ws, err := workspacefs.Open(proj.WorkspacePath)
	if err != nil {
		slog.Warn("roadmap detection: cannot open workspace", "project_id", projectID, "error", err)
		result.Platforms = detectPlatforms(proj)
		result.Found = result.Found || len(result.Platforms) > 0
		return result, nil
	}
	defer func() { _ = ws.Close() }()

	// Phase 2: Fallback to hardcoded fileMarkers for formats without a provider.
	seen := map[string]bool{}
	for format, markers := range fileMarkers {
		if coveredFormats[format] || coveredFormats[formatAlias(format)] {
			continue
		}
		for _, marker := range markers {
			fullPath := filepath.Join(proj.WorkspacePath, marker)
			info, err := ws.Stat(marker)
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
	for _, rel := range scanMarkdownKeywords(ws) {
		found := filepath.Join(proj.WorkspacePath, rel)
		if seen[found] {
			continue
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
// containing relevant keywords. Returns the workspace-relative paths of matching files.
func scanMarkdownKeywords(ws *workspacefs.Root) []string {
	var matches []string

	for _, dir := range keywordScanDirs {
		scanDir := dir
		if scanDir == "" {
			scanDir = "."
		}
		entries, err := ws.ReadDir(scanDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			rel := path.Join(dir, entry.Name())
			if containsKeyword(ws, rel) {
				matches = append(matches, rel)
			}
		}
	}

	return matches
}

// containsKeyword reads a workspace file line by line and returns true if any line
// contains one of the keywordScanTerms (case-insensitive). Stops at 200 lines
// to keep the scan shallow; a symlink that leaves the workspace or a FIFO is not read.
func containsKeyword(ws *workspacefs.Root, name string) bool {
	f, _, err := ws.OpenFile(name)
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
				s.importSpecItems(ctx, prov, itemParser, proj.WorkspacePath, spec, ms, result)
				continue
			}

			// Fallback: one feature per spec file.
			s.upsertFeature(ctx, ms.ID, spec.Title, spec.Path, prov.Name(), result)
		}
	}

	return result, nil
}

// importSpecItems imports the checkbox items of a spec file as features
// (KI-203): headings, plain and numbered list items and code are not
// features. The spec_ref of an item's feature is "specPath#Lline".
//
// An item takes the roadmap feature of the same file and title (the n-th
// item with a title the feature whose line comes n-th), wherever the
// feature's milestone is, so inserted lines move references instead of
// duplicating features; a new item becomes a feature of ms. The checkbox
// sets the status as importedStatus says. A feature of the file that no
// item takes keeps the file but loses its line, so "Sync to file" never
// writes to a line that holds something else now. The file's content hash
// is recorded once all of this is stored.
func (s *RoadmapService) importSpecItems(
	ctx context.Context,
	prov specprovider.Provider,
	parser specprovider.ItemParser,
	workspacePath string,
	spec specprovider.Spec,
	ms *roadmap.Milestone,
	result *roadmap.ImportResult,
) {
	fail := func(format string, args ...any) {
		result.Errors = append(result.Errors, fmt.Sprintf(format, args...))
	}
	content, err := prov.ReadSpec(ctx, workspacePath, spec.Path)
	if err != nil {
		fail("read %s: %v", spec.Path, err)
		return
	}
	items, err := parser.ParseItems(content)
	if err != nil {
		fail("parse items from %s: %v", spec.Path, err)
		return
	}
	sum := contentSHA256(content)
	state, err := s.specFileState(ctx, ms.RoadmapID, spec.Path, sum)
	if err != nil {
		fail("spec file record of %s: %v", spec.Path, err)
		return
	}
	features, err := s.store.ListFeaturesByRoadmap(ctx, ms.RoadmapID)
	if err != nil {
		fail("list features: %v", err)
		return
	}
	byTitle := fileFeaturesByTitle(features, spec.Path)

	stored := true
	save := func(f *roadmap.Feature) {
		if err := s.store.UpdateFeature(ctx, f); err != nil {
			fail("update feature %q: %v", f.Title, err)
			stored = false
			return
		}
		result.FeaturesUpdated++
	}
	for _, item := range items {
		if item.Level != specItemCheckbox {
			continue
		}
		ref := fmt.Sprintf("%s#L%d", spec.Path, item.SourceLine)
		checked := item.Status == specItemDone
		if same := byTitle[item.Title]; len(same) > 0 {
			f := same[0]
			byTitle[item.Title] = same[1:]
			status, changed := importedStatus(f.Status, checked, state)
			if f.SpecRef != ref || changed {
				f.SpecRef, f.Status = ref, status
				save(f)
			}
			continue
		}
		if !s.createSpecFeature(ctx, ms.ID, item.Title, ref, prov.Name(), checked, result) {
			stored = false
		}
	}
	for _, rest := range byTitle {
		for _, f := range rest {
			if f.SpecRef != spec.Path {
				f.SpecRef = spec.Path
				save(f)
			}
		}
	}
	if stored {
		if err := s.store.SetSpecFileHash(ctx, ms.RoadmapID, spec.Path, sum); err != nil {
			fail("record spec file %s: %v", spec.Path, err)
		}
	}
}

// createSpecFeature creates the feature of a new checkbox item (done when
// it is checked) and reports whether it was stored.
func (s *RoadmapService) createSpecFeature(ctx context.Context, milestoneID, title, ref, provName string, checked bool, result *roadmap.ImportResult) bool {
	f, err := s.store.CreateFeature(ctx, &roadmap.CreateFeatureRequest{
		MilestoneID: milestoneID,
		Title:       title,
		SpecRef:     ref,
		Labels:      []string{provName},
	})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("create feature %q: %v", title, err))
		return false
	}
	result.FeaturesCreated++
	if !checked {
		return true
	}
	f.Status = roadmap.FeatureDone
	if err := s.store.UpdateFeature(ctx, f); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("mark feature %q done: %v", title, err))
		return false
	}
	return true
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
		return nil, fmt.Errorf("%w: unknown PM provider %q", domain.ErrValidation, providerName)
	}
	// The providers built at startup carry the operator's credentials.
	if credential, ok := operatorPMCredentials[providerName]; ok && !operatorCredentialsServe(ctx) {
		return nil, fmt.Errorf("a %s import uses %s, which serves only the default tenant - sync with your own token instead (POST /projects/{id}/roadmap/sync): %w",
			providerName, credential, domain.ErrValidation)
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

	// One milestone per provider, reused by later imports (KI-203, R4-12).
	msTitle := fmt.Sprintf("Imported from %s", providerName)
	ms, err := s.store.FindMilestoneByTitle(ctx, rm.ID, msTitle)
	if errors.Is(err, domain.ErrNotFound) {
		ms, err = s.store.CreateMilestone(ctx, roadmap.CreateMilestoneRequest{
			RoadmapID:   rm.ID,
			Title:       msTitle,
			Description: fmt.Sprintf("Work items imported from %s (%s)", providerName, projectRef),
		})
		if err == nil {
			result.MilestonesCreated++
		}
	}
	if err != nil {
		return nil, fmt.Errorf("milestone: %w", err)
	}

	// Items imported before are matched by their external ID, wherever
	// their feature is now, and updated instead of created again.
	features, err := s.store.ListFeaturesByRoadmap(ctx, rm.ID)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	imported := make(map[string]*roadmap.Feature)
	for i := range features {
		if id := features[i].ExternalIDs[providerName]; id != "" {
			imported[id] = &features[i]
		}
	}

	for i := range items {
		item := &items[i]
		if f := imported[item.ExternalID]; item.ExternalID != "" && f != nil {
			s.updatePMFeature(ctx, f, item, result)
			continue
		}
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

// updatePMFeature takes a re-imported item's title, description and labels
// into its feature when they changed; status and milestone stay as the
// roadmap has them.
func (s *RoadmapService) updatePMFeature(ctx context.Context, f *roadmap.Feature, item *pmprovider.Item, result *roadmap.ImportResult) {
	if f.Title == item.Title && f.Description == item.Description && slices.Equal(f.Labels, item.Labels) {
		return
	}
	f.Title, f.Description, f.Labels = item.Title, item.Description, item.Labels
	if err := s.store.UpdateFeature(ctx, f); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("update feature %q: %v", item.Title, err))
		return
	}
	result.FeaturesUpdated++
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
