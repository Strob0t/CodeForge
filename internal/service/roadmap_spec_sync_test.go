package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/markdownspec"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// roadmapMemStore keeps one project's roadmap, milestones, features and spec
// file records in memory.
type roadmapMemStore struct {
	mockStore
	rm         *roadmap.Roadmap
	milestones []roadmap.Milestone
	features   []roadmap.Feature
	specFiles  map[string]roadmap.SpecFile
	nextID     int
}

func newRoadmapMemStore(workspace string) *roadmapMemStore {
	return &roadmapMemStore{
		mockStore: mockStore{projects: []project.Project{{ID: "p1", Name: "app", WorkspacePath: workspace}}},
		specFiles: map[string]roadmap.SpecFile{},
	}
}

func (m *roadmapMemStore) id(prefix string) string {
	m.nextID++
	return fmt.Sprintf("%s-%d", prefix, m.nextID)
}

func (m *roadmapMemStore) CreateRoadmap(_ context.Context, req roadmap.CreateRoadmapRequest) (*roadmap.Roadmap, error) {
	m.rm = &roadmap.Roadmap{ID: m.id("rm"), ProjectID: req.ProjectID, Title: req.Title}
	r := *m.rm
	return &r, nil
}

func (m *roadmapMemStore) GetRoadmapByProject(_ context.Context, _ string) (*roadmap.Roadmap, error) {
	if m.rm == nil {
		return nil, domain.ErrNotFound
	}
	r := *m.rm
	return &r, nil
}

func (m *roadmapMemStore) CreateMilestone(_ context.Context, req roadmap.CreateMilestoneRequest) (*roadmap.Milestone, error) {
	ms := roadmap.Milestone{ID: m.id("ms"), RoadmapID: req.RoadmapID, Title: req.Title, Description: req.Description}
	m.milestones = append(m.milestones, ms)
	return &ms, nil
}

func (m *roadmapMemStore) ListMilestones(_ context.Context, _ string) ([]roadmap.Milestone, error) {
	return slices.Clone(m.milestones), nil
}

func (m *roadmapMemStore) FindMilestoneByTitle(_ context.Context, _, title string) (*roadmap.Milestone, error) {
	for i := range m.milestones {
		if m.milestones[i].Title == title {
			ms := m.milestones[i]
			return &ms, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *roadmapMemStore) CreateFeature(_ context.Context, req *roadmap.CreateFeatureRequest) (*roadmap.Feature, error) {
	f := roadmap.Feature{
		ID: m.id("f"), MilestoneID: req.MilestoneID, RoadmapID: m.rm.ID, Title: req.Title, Description: req.Description,
		Status: roadmap.FeatureBacklog, Labels: req.Labels, SpecRef: req.SpecRef, ExternalIDs: req.ExternalIDs, Version: 1,
	}
	m.features = append(m.features, f)
	return &f, nil
}

func (m *roadmapMemStore) FindFeatureBySpecRef(_ context.Context, milestoneID, specRef string) (*roadmap.Feature, error) {
	for i := range m.features {
		if m.features[i].MilestoneID == milestoneID && m.features[i].SpecRef == specRef {
			f := m.features[i]
			return &f, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *roadmapMemStore) ListFeatures(_ context.Context, milestoneID string) ([]roadmap.Feature, error) {
	var out []roadmap.Feature
	for i := range m.features {
		if m.features[i].MilestoneID == milestoneID {
			out = append(out, m.features[i])
		}
	}
	return out, nil
}

func (m *roadmapMemStore) ListFeaturesByRoadmap(_ context.Context, _ string) ([]roadmap.Feature, error) {
	return slices.Clone(m.features), nil
}

func (m *roadmapMemStore) UpdateFeature(_ context.Context, f *roadmap.Feature) error {
	for i := range m.features {
		if m.features[i].ID == f.ID {
			if m.features[i].Version != f.Version {
				return domain.ErrConflict
			}
			f.Version++
			m.features[i] = *f
			return nil
		}
	}
	return domain.ErrNotFound
}

func (m *roadmapMemStore) GetSpecFile(_ context.Context, _, path string) (*roadmap.SpecFile, error) {
	f, ok := m.specFiles[path]
	if !ok {
		return nil, domain.ErrNotFound
	}
	f.Checked = maps.Clone(f.Checked)
	return &f, nil
}

func (m *roadmapMemStore) SetSpecFile(_ context.Context, f *roadmap.SpecFile) error {
	stored := *f
	stored.Checked = maps.Clone(f.Checked)
	m.specFiles[f.Path] = stored
	return nil
}

// feature returns the stored feature titled title (it must be the only one).
func (m *roadmapMemStore) feature(t *testing.T, title string) *roadmap.Feature {
	t.Helper()
	var found *roadmap.Feature
	for i := range m.features {
		if m.features[i].Title == title {
			if found != nil {
				t.Fatalf("two features titled %q: %+v and %+v", title, *found, m.features[i])
			}
			found = &m.features[i]
		}
	}
	if found == nil {
		t.Fatalf("no feature titled %q in %+v", title, m.features)
	}
	return found
}

// setStatus changes a feature's status as the UI or the auto-agent would.
func (m *roadmapMemStore) setStatus(t *testing.T, title string, status roadmap.FeatureStatus) {
	t.Helper()
	f := m.feature(t, title)
	f.Status = status
	f.Version++
}

// specFixture is a TODO.md with what the old sync lost: an intro, a code
// block holding a heading and a checkbox, a numbered list, a table, a
// plain list item, an uppercase [X] and a line over 64 KiB.
const specFixtureHead = "# Project TODO\n\nSome intro with [a link](http://x).\n\n## Phase 1\n\n" +
	"- [x] Done item\n  details\n- [ ] Open item\n- plain list item\n\n" +
	"```sh\n# not a heading\n- [ ] not an item\n```\n\n" +
	"1. numbered step\n| a | b |\n|---|---|\n\n"

func specFixture() string {
	return specFixtureHead + strings.Repeat("long ", 20000) + "\n* [X] Upper done\n- [ ] Last item\n"
}

type specEnv struct {
	ws    string
	store *roadmapMemStore
	svc   *RoadmapService
}

func newSpecEnv(t *testing.T, files map[string]string) *specEnv {
	t.Helper()
	ws := t.TempDir()
	store := newRoadmapMemStore(ws)
	env := &specEnv{ws: ws, store: store, svc: NewRoadmapService(store, nil, []specprovider.Provider{&markdownspec.Provider{}}, nil)}
	for name, content := range files {
		env.write(t, name, content)
	}
	return env
}

func (e *specEnv) importSpecs(t *testing.T) *roadmap.ImportResult {
	t.Helper()
	res, err := e.svc.ImportSpecs(context.Background(), "p1")
	if err != nil || len(res.Errors) > 0 {
		t.Fatalf("ImportSpecs = %+v, %v", res, err)
	}
	return res
}

func (e *specEnv) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.ws, name)) //nolint:gosec // test workspace file
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (e *specEnv) write(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(e.ws, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, content)
}

// refs lists the stored features as "title@spec_ref=status".
func (e *specEnv) refs() []string {
	var out []string
	for i := range e.store.features {
		f := &e.store.features[i]
		out = append(out, fmt.Sprintf("%s@%s=%s", f.Title, f.SpecRef, f.Status))
	}
	slices.Sort(out)
	return out
}

func wantRefs(t *testing.T, got []string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("features:\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// KI-203 (R4-3): only checkbox items become features (headings, plain and
// numbered list items and lines in code blocks do not), with the status of
// their checkbox.
func TestImportSpecs_ChecklistItemsWithTheirStatus(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": specFixture()})
	res := env.importSpecs(t)
	if res.MilestonesCreated != 1 || res.FeaturesCreated != 4 || res.FeaturesUpdated != 0 {
		t.Fatalf("ImportSpecs = %+v", res)
	}
	wantRefs(t, env.refs(),
		"Done item@TODO.md#L7=done",
		"Open item@TODO.md#L9=backlog",
		"Upper done@TODO.md#L22=done",
		"Last item@TODO.md#L23=backlog",
	)
}

// Re-importing an unchanged file changes nothing; inserting lines above the
// items moves their references instead of creating duplicates.
func TestImportSpecs_ReimportMatchesByTitle(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": specFixture()})
	env.importSpecs(t)
	if res := env.importSpecs(t); res.FeaturesCreated != 0 || res.FeaturesUpdated != 0 || res.MilestonesCreated != 0 {
		t.Fatalf("re-import of an unchanged file = %+v", res)
	}

	env.write(t, "TODO.md", "Inserted line\n\n"+specFixture())
	res := env.importSpecs(t)
	if res.FeaturesCreated != 0 || res.FeaturesUpdated != 4 {
		t.Fatalf("re-import after inserting lines = %+v", res)
	}
	wantRefs(t, env.refs(),
		"Done item@TODO.md#L9=done",
		"Open item@TODO.md#L11=backlog",
		"Upper done@TODO.md#L24=done",
		"Last item@TODO.md#L25=backlog",
	)
}

// A repeated title is matched by occurrence: the n-th item with the title
// takes the feature with the title whose line comes n-th in the file. (Two
// items with one title cannot be told apart beyond their order.)
func TestImportSpecs_RepeatedTitles(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": "## A\n- [ ] Tests\n## B\n- [x] Tests\n"})
	env.importSpecs(t)
	env.write(t, "TODO.md", "## New\n- [ ] Tests\n## A\n- [ ] Tests\n## B\n- [x] Tests\n")
	res := env.importSpecs(t)
	if res.FeaturesCreated != 1 {
		t.Fatalf("re-import = %+v", res)
	}
	wantRefs(t, env.refs(), "Tests@TODO.md#L2=backlog", "Tests@TODO.md#L4=backlog", "Tests@TODO.md#L6=done")
}

// Three-way merge of a checkbox and its feature's status: the roadmap
// records the state each imported box had when it last imported or synced
// the file. A box that changed in the file since sets the status, both
// ways (an unchecked box reopens only a done feature). A box that did not
// change keeps the roadmap's status, even when other lines of the file
// changed. Without a record (a roadmap imported before KI-203), a checked
// box marks the feature done and an unchecked one never reopens it.
func TestImportSpecs_StatusFollowsAChangedBox(t *testing.T) {
	const checked, unchecked = "- [x] Item\n", "- [ ] Item\n"
	const elsewhere = "Intro\n" // another line of the file changed
	tests := []struct {
		name      string
		before    string // file at the first import
		dbStatus  roadmap.FeatureStatus
		after     string // file at the re-import
		noRecord  bool   // the first import left no record (pre-KI-203)
		wantState roadmap.FeatureStatus
	}{
		{"unchanged file keeps done", unchecked, roadmap.FeatureDone, unchecked, false, roadmap.FeatureDone},
		{"unchanged file keeps reopened", checked, roadmap.FeatureBacklog, checked, false, roadmap.FeatureBacklog},
		{"checked in the file", unchecked, roadmap.FeatureInProgress, checked, false, roadmap.FeatureDone},
		{"unchecked in the file", checked, roadmap.FeatureDone, unchecked, false, roadmap.FeatureBacklog},
		{"unchecked in the file keeps in progress", checked, roadmap.FeatureInProgress, unchecked, false, roadmap.FeatureInProgress},
		{"unchanged box keeps done", unchecked, roadmap.FeatureDone, elsewhere + unchecked, false, roadmap.FeatureDone},
		{"unchanged box keeps reopened", checked, roadmap.FeatureBacklog, elsewhere + checked, false, roadmap.FeatureBacklog},
		{"unchanged box keeps in progress", unchecked, roadmap.FeatureInProgress, elsewhere + unchecked, false, roadmap.FeatureInProgress},
		{"no record, checked", unchecked, roadmap.FeatureBacklog, checked, true, roadmap.FeatureDone},
		{"no record, unchecked keeps done", checked, roadmap.FeatureDone, unchecked, true, roadmap.FeatureDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newSpecEnv(t, map[string]string{"TODO.md": tt.before})
			env.importSpecs(t)
			env.store.setStatus(t, "Item", tt.dbStatus)
			if tt.noRecord {
				clear(env.store.specFiles)
			}
			env.write(t, "TODO.md", tt.after)
			env.importSpecs(t)
			if got := env.store.feature(t, "Item").Status; got != tt.wantState {
				t.Fatalf("status = %s, want %s", got, tt.wantState)
			}
		})
	}
}

// A status set in the UI survives a re-import of a file in which another
// box was checked: only the changed box takes the file's state.
func TestImportSpecs_UIStatusSurvivesAnotherBoxChange(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": "- [ ] A\n- [ ] B\n- [x] C\n"})
	env.importSpecs(t)
	env.store.setStatus(t, "A", roadmap.FeatureDone)
	env.store.setStatus(t, "C", roadmap.FeatureInProgress)
	env.write(t, "TODO.md", "- [ ] A\n- [x] B\n- [x] C\n")
	env.importSpecs(t)
	wantRefs(t, env.refs(), "A@TODO.md#L1=done", "B@TODO.md#L2=done", "C@TODO.md#L3=in_progress")
}

// Features the old import created (headings, list items, a duplicate per
// inserted line, all backlog) are matched by title where an item still
// has it; the rest lose their line, so "Sync to file" never writes to a
// line that now holds something else. Nothing is deleted.
func TestImportSpecs_LegacyFeaturesLoseTheirLine(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": "## Phase 1\n- [x] Done item\n- [ ] Open item\n- plain\n"})
	if _, err := env.svc.getOrCreateRoadmap(context.Background(), "p1", "app"); err != nil {
		t.Fatal(err)
	}
	ms, _ := env.store.CreateMilestone(context.Background(), roadmap.CreateMilestoneRequest{RoadmapID: env.store.rm.ID, Title: "Imported from markdown"})
	for _, legacy := range []struct{ title, ref string }{
		{"Phase 1", "TODO.md#L1"}, {"Done item", "TODO.md#L2"}, {"Open item", "TODO.md#L3"},
		{"plain", "TODO.md#L4"}, {"Open item", "TODO.md#L4"}, // a duplicate after a line was inserted
	} {
		if _, err := env.store.CreateFeature(context.Background(), &roadmap.CreateFeatureRequest{
			MilestoneID: ms.ID, Title: legacy.title, SpecRef: legacy.ref, Labels: []string{"markdown"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	res := env.importSpecs(t)
	if res.FeaturesCreated != 0 || res.MilestonesCreated != 0 {
		t.Fatalf("ImportSpecs = %+v", res)
	}
	wantRefs(t, env.refs(),
		"Phase 1@TODO.md=backlog",
		"Done item@TODO.md#L2=done",
		"Open item@TODO.md#L3=backlog",
		"plain@TODO.md=backlog",
		"Open item@TODO.md=backlog",
	)
}

// KI-203 (R4-2): "Sync to file" changes only the markers of changed
// statuses; everything else stays byte for byte, [x] stays [x], and the
// written file counts as synced (a re-import changes nothing).
func TestSyncToSpecFile_PatchesOnlyMarkers(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": specFixture()})
	env.importSpecs(t)
	env.store.setStatus(t, "Open item", roadmap.FeatureDone)
	env.store.setStatus(t, "Upper done", roadmap.FeatureDone)
	env.store.setStatus(t, "Last item", roadmap.FeatureInProgress)

	if err := env.svc.SyncToSpecFile(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(specFixture(), "- [ ] Open item", "- [x] Open item", 1)
	if got := env.read(t, "TODO.md"); got != want {
		t.Fatalf("synced file differs beyond the Open item marker (%d bytes, want %d)", len(got), len(want))
	}
	if res := env.importSpecs(t); res.FeaturesUpdated != 0 || res.FeaturesCreated != 0 {
		t.Fatalf("re-import after the sync = %+v", res)
	}
	if got := env.store.feature(t, "Last item").Status; got != roadmap.FeatureInProgress {
		t.Fatalf("Last item = %s after the re-import, want in_progress", got)
	}
}

// "Sync to file" records the boxes it wrote: a feature reopened after the
// sync stays open when the file changes elsewhere, since its box did not
// change in the file since the sync.
func TestSyncToSpecFile_RecordsTheWrittenBoxes(t *testing.T) {
	env := newSpecEnv(t, map[string]string{"TODO.md": "- [ ] Item\n"})
	env.importSpecs(t)
	env.store.setStatus(t, "Item", roadmap.FeatureDone)
	if err := env.svc.SyncToSpecFile(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	env.store.setStatus(t, "Item", roadmap.FeatureBacklog)
	env.write(t, "TODO.md", "Intro\n"+env.read(t, "TODO.md"))
	env.importSpecs(t)
	wantRefs(t, env.refs(), "Item@TODO.md#L2=backlog")
}

// The spec file is never overwritten when it cannot be patched safely: it
// changed since the last import or sync, it was never imported, a line no
// longer holds its feature's checkbox, or a spec_ref names a file that is
// not one of the workspace's markdown spec files.
func TestSyncToSpecFile_Refusals(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, env *specEnv)
		wantErr error
	}{
		{"file changed since the import", func(t *testing.T, env *specEnv) {
			env.write(t, "TODO.md", env.read(t, "TODO.md")+"- [ ] Added by hand\n")
		}, domain.ErrConflict},
		{"file never imported", func(_ *testing.T, env *specEnv) {
			clear(env.store.specFiles)
		}, domain.ErrConflict},
		{"line holds another title", func(t *testing.T, env *specEnv) {
			env.store.feature(t, "Open item").SpecRef = "TODO.md#L7"
		}, domain.ErrConflict},
		{"line out of the file", func(t *testing.T, env *specEnv) {
			env.store.feature(t, "Open item").SpecRef = "TODO.md#L999"
		}, domain.ErrConflict},
		{"spec_ref names a non-markdown file", func(t *testing.T, env *specEnv) {
			env.store.feature(t, "Open item").SpecRef = "go.mod#L1"
		}, domain.ErrValidation},
		{"spec_ref names a markdown file that is no spec", func(t *testing.T, env *specEnv) {
			env.store.feature(t, "Open item").SpecRef = "README.md#L1"
		}, domain.ErrValidation},
		{"spec_ref leaves the workspace", func(t *testing.T, env *specEnv) {
			env.store.feature(t, "Open item").SpecRef = "../TODO.md#L9"
		}, domain.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"TODO.md": specFixture(), "go.mod": "module x\n", "README.md": "- [ ] Open item\n"}
			env := newSpecEnv(t, files)
			env.importSpecs(t)
			env.store.setStatus(t, "Open item", roadmap.FeatureDone)
			env.store.setStatus(t, "Done item", roadmap.FeatureBacklog)
			tt.prepare(t, env)
			before := map[string]string{}
			for name := range files {
				before[name] = env.read(t, name)
			}

			err := env.svc.SyncToSpecFile(context.Background(), "p1")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SyncToSpecFile = %v, want %v", err, tt.wantErr)
			}
			for name, content := range before {
				if env.read(t, name) != content {
					t.Fatalf("%s was written despite the refusal", name)
				}
			}
		})
	}
}

// Without imported items the roadmap is rendered into a new ROADMAP.md, but
// never over an existing spec file (it used to replace ROADMAP/TODO files).
func TestSyncToSpecFile_RenderOnlyIntoANewFile(t *testing.T) {
	t.Run("existing spec file", func(t *testing.T) {
		env := newSpecEnv(t, map[string]string{"docs/todo.md": "# Mine\n\nNotes.\n"})
		if _, err := env.svc.getOrCreateRoadmap(context.Background(), "p1", "app"); err != nil {
			t.Fatal(err)
		}
		if err := env.svc.SyncToSpecFile(context.Background(), "p1"); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("SyncToSpecFile = %v, want ErrConflict", err)
		}
		if got := env.read(t, "docs/todo.md"); got != "# Mine\n\nNotes.\n" {
			t.Fatalf("docs/todo.md was overwritten: %q", got)
		}
	})
	t.Run("no spec file", func(t *testing.T) {
		env := newSpecEnv(t, nil)
		if _, err := env.svc.getOrCreateRoadmap(context.Background(), "p1", "app"); err != nil {
			t.Fatal(err)
		}
		if err := env.svc.SyncToSpecFile(context.Background(), "p1"); err != nil {
			t.Fatal(err)
		}
		if got := env.read(t, "ROADMAP.md"); !strings.HasPrefix(got, "# app Roadmap\n") {
			t.Fatalf("ROADMAP.md = %q", got)
		}
		// The next sync finds the file it wrote and does not render over it.
		if err := env.svc.SyncToSpecFile(context.Background(), "p1"); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("second SyncToSpecFile = %v, want ErrConflict", err)
		}
	})
}

// KI-203 (R4-12): a PM import reuses its milestone and updates the features
// it imported before (matched by external ID) instead of duplicating them.
func TestImportPMItems_Upserts(t *testing.T) {
	env := newSpecEnv(t, nil)
	prov := &itemsPMProvider{name: "gitlab", items: []pmprovider.Item{
		{ExternalID: "g/app#1", Title: "First", Description: "one"},
		{ExternalID: "g/app#2", Title: "Second"},
	}}
	env.svc.pmProvs = []pmprovider.Provider{prov}
	ctx := defaultTenantCtx()

	res, err := env.svc.ImportPMItems(ctx, "p1", "gitlab", "g/app")
	if err != nil || res.MilestonesCreated != 1 || res.FeaturesCreated != 2 {
		t.Fatalf("first import = %+v, %v", res, err)
	}
	prov.items[0].Title = "First (renamed)"
	prov.items = append(prov.items, pmprovider.Item{ExternalID: "g/app#3", Title: "Third"})
	res, err = env.svc.ImportPMItems(ctx, "p1", "gitlab", "g/app")
	if err != nil || res.MilestonesCreated != 0 || res.FeaturesCreated != 1 || res.FeaturesUpdated != 1 {
		t.Fatalf("second import = %+v, %v", res, err)
	}
	if len(env.store.milestones) != 1 || len(env.store.features) != 3 {
		t.Fatalf("milestones %d, features %v", len(env.store.milestones), env.refs())
	}
	if f := env.store.feature(t, "First (renamed)"); f.ExternalIDs["gitlab"] != "g/app#1" || f.Description != "one" {
		t.Fatalf("renamed feature = %+v", f)
	}
}

// itemsPMProvider lists a fixed set of items.
type itemsPMProvider struct {
	listingPMProvider
	name  string
	items []pmprovider.Item
}

func (p *itemsPMProvider) Name() string { return p.name }

func (p *itemsPMProvider) ListItems(context.Context, string) ([]pmprovider.Item, error) {
	return slices.Clone(p.items), nil
}
