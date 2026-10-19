package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
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

// SyncToSpecFile writes the roadmap's feature statuses back to the
// workspace's markdown spec files (KI-203). Features imported from a spec
// file ("path#Lline" spec_refs) get their status written into the checkbox
// marker of their line, and nothing else in the file changes. Without such
// features, the roadmap is rendered into a new ROADMAP.md, never over an
// existing spec file.
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

	refs := specLineRefs(rm)
	if len(refs) == 0 {
		return renderIntoNewSpecFile(proj.WorkspacePath, rm)
	}
	if err := s.patchSpecFiles(ctx, proj.WorkspacePath, rm.ID, refs); err != nil {
		return err
	}
	slog.Info("synced roadmap statuses to spec files", "project", projectID, "files", len(refs))
	return nil
}

// specLineRef is a feature whose spec_ref names a line of a spec file.
type specLineRef struct {
	feature *roadmap.Feature
	line    int
}

// specLineRefs groups the roadmap's features with a "path#Lline" spec_ref
// by file path.
func specLineRefs(rm *roadmap.Roadmap) map[string][]specLineRef {
	refs := make(map[string][]specLineRef)
	for i := range rm.Milestones {
		for j := range rm.Milestones[i].Features {
			f := &rm.Milestones[i].Features[j]
			if path, line := parseSpecRef(f.SpecRef); line > 0 {
				refs[path] = append(refs[path], specLineRef{feature: f, line: line})
			}
		}
	}
	return refs
}

// specFilePatch is a spec file's content before and after its markers were
// set, and the record of the boxes after.
type specFilePatch struct {
	path          string
	before, after []byte
	checked       map[string]bool
}

// patchSpecFiles writes the statuses of the features in refs into the
// markers of their lines. Every file is checked before any is written:
//   - its path is one of the workspace's markdown spec files (a spec_ref
//     can be set by hand; no other file is ever written);
//   - it did not change since the roadmap last imported or synced it, and
//     it was imported since the roadmap records that;
//   - each feature's line still holds a checkbox, and no two features name
//     one line (the checkbox's title may differ: a feature renamed in the
//     UI keeps its line while the file does not change).
//
// A failed check writes nothing (ErrValidation for a path, ErrConflict
// otherwise: import the file again).
func (s *RoadmapService) patchSpecFiles(ctx context.Context, workspacePath, roadmapID string, refs map[string][]specLineRef) error {
	prov, writer := s.specItemWriter()
	if writer == nil {
		return fmt.Errorf("%w: no spec provider writes spec files", domain.ErrValidation)
	}
	specs, err := prov.ListSpecs(ctx, workspacePath)
	if err != nil {
		return fmt.Errorf("list spec files: %w", err)
	}
	known := make(map[string]bool, len(specs))
	for _, spec := range specs {
		known[spec.Path] = true
	}

	paths := slices.Sorted(maps.Keys(refs))
	patches := make([]specFilePatch, 0, len(paths))
	for _, path := range paths {
		if !known[path] {
			f := refs[path][0].feature
			return fmt.Errorf("%w: feature %q has spec_ref %q, which is not one of the workspace's markdown spec files (ROADMAP.md, TODO.md, docs/ROADMAP.md, docs/TODO.md); nothing was written",
				domain.ErrValidation, f.Title, f.SpecRef)
		}
		patch, err := s.patchSpecFile(ctx, prov, writer, workspacePath, roadmapID, path, refs[path])
		if err != nil {
			return err
		}
		patches = append(patches, patch)
	}

	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		return fmt.Errorf("open workspace: %w", err)
	}
	defer func() { _ = ws.Close() }()
	for _, p := range patches {
		if bytes.Equal(p.before, p.after) {
			continue
		}
		if err := writeUnchangedSpecFile(ws, p); err != nil {
			return err
		}
		record := roadmap.SpecFile{RoadmapID: roadmapID, Path: p.path, ContentSHA256: contentSHA256(p.after), Checked: p.checked}
		if err := s.store.SetSpecFile(ctx, &record); err != nil {
			return fmt.Errorf("record spec file %s: %w", p.path, err)
		}
	}
	return nil
}

// patchSpecFile checks the spec file path and sets the markers of its
// features' lines in its content (nothing is written yet).
func (s *RoadmapService) patchSpecFile(
	ctx context.Context,
	prov specprovider.Provider,
	writer specprovider.ItemWriter,
	workspacePath, roadmapID, path string,
	refs []specLineRef,
) (specFilePatch, error) {
	content, err := prov.ReadSpec(ctx, workspacePath, path)
	if err != nil {
		return specFilePatch{}, fmt.Errorf("read %s: %w", path, err)
	}
	state, checked, err := s.specFileState(ctx, roadmapID, path, contentSHA256(content))
	if err != nil {
		return specFilePatch{}, fmt.Errorf("spec file record of %s: %w", path, err)
	}
	switch state {
	case specFileUnrecorded:
		return specFilePatch{}, fmt.Errorf("%w: %s has no import record; import the specs again before syncing (nothing was written)", domain.ErrConflict, path)
	case specFileChanged:
		return specFilePatch{}, fmt.Errorf("%w: %s changed since it was last imported or synced; import the specs again before syncing (nothing was written)", domain.ErrConflict, path)
	}

	items := make([]specprovider.SpecItemDetail, 0, len(refs))
	onLine := make(map[int]string, len(refs))
	checked = maps.Clone(checked)
	if checked == nil {
		checked = make(map[string]bool, len(refs))
	}
	for _, ref := range refs {
		if other, taken := onLine[ref.line]; taken {
			return specFilePatch{}, fmt.Errorf("%w: features %q and %q both refer to %s line %d (nothing was written)", domain.ErrConflict, other, ref.feature.Title, path, ref.line)
		}
		onLine[ref.line] = ref.feature.Title
		checked[ref.feature.ID] = ref.feature.Status == roadmap.FeatureDone
		items = append(items, specprovider.SpecItemDetail{
			Title:      ref.feature.Title,
			Status:     featureStatusToItemStatus(ref.feature.Status),
			SourceLine: ref.line,
			Level:      specItemCheckbox,
		})
	}
	after, err := writer.PatchItems(content, items)
	if errors.Is(err, specprovider.ErrItemMoved) {
		return specFilePatch{}, fmt.Errorf("%w: %s: %w; import the specs again before syncing (nothing was written)", domain.ErrConflict, path, err)
	}
	if err != nil {
		return specFilePatch{}, fmt.Errorf("patch %s: %w", path, err)
	}
	return specFilePatch{path: path, before: content, after: after, checked: checked}, nil
}

// writeUnchangedSpecFile writes the changed marker bytes of a spec file in
// place if the file still holds the content the patch was made from. The
// file is compared and written through one descriptor and never truncated
// (a patch keeps the file's length), so a concurrent reader or writer never
// sees it empty or half rewritten.
func writeUnchangedSpecFile(ws *workspacefs.Root, p specFilePatch) error {
	err := ws.PatchFile(p.path, p.before, p.after)
	if errors.Is(err, workspacefs.ErrContentChanged) {
		return fmt.Errorf("%w: %s changed while it was synced; import the specs again", domain.ErrConflict, p.path)
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", p.path, err)
	}
	return nil
}

// specItemWriter returns the first spec provider that writes items back.
func (s *RoadmapService) specItemWriter() (specprovider.Provider, specprovider.ItemWriter) {
	for _, prov := range s.specProvs {
		if w, ok := prov.(specprovider.ItemWriter); ok {
			return prov, w
		}
	}
	return nil, nil
}

// newSpecFile is the spec file a roadmap without imported features is
// rendered into.
const newSpecFile = "ROADMAP.md"

// renderIntoNewSpecFile renders the roadmap into a new ROADMAP.md, written
// through workspacefs (KI-95). An existing spec file is never replaced
// (KI-203): its content would be lost.
func renderIntoNewSpecFile(workspacePath string, rm *roadmap.Roadmap) error {
	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		return fmt.Errorf("open workspace: %w", err)
	}
	defer func() { _ = ws.Close() }()
	if existing := findSpecFile(ws); existing != "" {
		return fmt.Errorf("%w: %s exists and no roadmap feature is imported from it; import the specs to sync statuses into it (the roadmap is never rendered over an existing file)",
			domain.ErrConflict, existing)
	}
	// Shared with the worker's tool user (KI-71).
	if err := ws.CreateExclusive(newSpecFile, []byte(renderMarkdown(rm)), project.WorkspaceFilePerm); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s exists; the roadmap is never rendered over an existing file", domain.ErrConflict, newSpecFile)
		}
		return fmt.Errorf("write spec file: %w", err)
	}
	slog.Info("rendered roadmap into a new spec file", "path", filepath.Join(workspacePath, newSpecFile))
	return nil
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

// findSpecFile returns the first existing spec file candidate (a regular
// file inside the workspace), or "" when there is none.
func findSpecFile(ws *workspacefs.Root) string {
	for _, name := range []string{
		"ROADMAP.md", "roadmap.md", "TODO.md", "todo.md",
		"docs/ROADMAP.md", "docs/roadmap.md", "docs/TODO.md", "docs/todo.md",
	} {
		info, statErr := ws.Stat(name)
		if statErr == nil && info.Mode().IsRegular() {
			return name
		}
	}
	return ""
}
