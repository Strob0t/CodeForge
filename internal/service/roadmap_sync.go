package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

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
