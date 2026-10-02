package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// knowledgeLayout builds a content root with docs/guide.md and notes.md, an
// outside directory with a secret, and symlinks in the root leading out.
func knowledgeLayout(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "knowledge")
	outside = filepath.Join(base, "outside")
	for _, dir := range []string{filepath.Join(root, "docs"), outside} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(root, "docs", "guide.md"), "inside guide")
	writeTestFile(t, filepath.Join(root, "notes.md"), "inside notes")
	writeTestFile(t, filepath.Join(outside, "secret.md"), "outside secret")
	for name, target := range map[string]string{
		"abs-dir":  outside,
		"abs-file": filepath.Join(outside, "secret.md"),
		"rel-dir":  "../outside",
		"rel-file": "../outside/secret.md",
		"in-link":  "docs/guide.md",
	} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	return root, outside
}

func TestKnowledgeContent_Relative(t *testing.T) {
	root, outside := knowledgeLayout(t)
	k := newKnowledgeContent(root)

	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "docs", want: "docs"},
		{in: "./docs/../docs/guide.md", want: "docs/guide.md"},
		{in: filepath.Join(root, "docs"), want: "docs"},
		{in: root + "/docs/../notes.md", want: "notes.md"},
		{in: "../outside", wantErr: true},
		{in: "docs/../../outside", wantErr: true},
		{in: outside, wantErr: true},
		{in: "/etc", wantErr: true},
		{in: root + "-other/x", wantErr: true},
	}
	for _, tt := range tests {
		got, err := k.relative(tt.in)
		if tt.wantErr {
			if !errors.Is(err, domain.ErrValidation) {
				t.Errorf("relative(%q) = %q, %v; want a validation error", tt.in, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("relative(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestKnowledgeContent_AcceptsTheResolvedRoot(t *testing.T) {
	root, _ := knowledgeLayout(t)
	link := filepath.Join(t.TempDir(), "kb-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	k := newKnowledgeContent(link)
	for _, in := range []string{filepath.Join(link, "docs"), filepath.Join(root, "docs")} {
		if got, err := k.relative(in); err != nil || got != "docs" {
			t.Errorf("relative(%q) = %q, %v; want docs", in, got, err)
		}
	}
}

// kbStore serves fixed knowledge bases.
type kbStore struct {
	mockStore
	kbs map[string]*knowledgebase.KnowledgeBase
}

func (s *kbStore) GetKnowledgeBase(_ context.Context, id string) (*knowledgebase.KnowledgeBase, error) {
	kb, ok := s.kbs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return kb, nil
}

func (s *kbStore) CreateKnowledgeBase(_ context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error) {
	kb := &knowledgebase.KnowledgeBase{ID: "kb-new", Name: req.Name, Category: req.Category, ContentPath: req.ContentPath}
	s.kbs[kb.ID] = kb
	return kb, nil
}

func newKBTestService(t *testing.T, root string, kbs ...*knowledgebase.KnowledgeBase) (*KnowledgeBaseService, *fakeQueue) {
	t.Helper()
	store := &kbStore{kbs: map[string]*knowledgebase.KnowledgeBase{}}
	for _, kb := range kbs {
		store.kbs[kb.ID] = kb
	}
	queue := &fakeQueue{}
	svc := NewKnowledgeBaseService(store, root)
	svc.SetRetrieval(NewRetrievalService(store, queue, &noopBroadcaster{}, &config.Orchestrator{}, &config.Limits{}))
	return svc, queue
}

func TestKnowledgeBaseService_CreateStoresTheRelativePath(t *testing.T) {
	root, outside := knowledgeLayout(t)
	svc, _ := newKBTestService(t, root)
	ctx := context.Background()

	for in, want := range map[string]string{"docs": "docs", filepath.Join(root, "notes.md"): "notes.md"} {
		kb, err := svc.Create(ctx, &knowledgebase.CreateRequest{Name: "kb", Category: "custom", ContentPath: in})
		if err != nil || kb.ContentPath != want {
			t.Fatalf("Create(%q) = %+v, %v; want content_path %q", in, kb, err, want)
		}
	}
	for _, in := range []string{outside, "/etc", "../outside"} {
		if _, err := svc.Create(ctx, &knowledgebase.CreateRequest{Name: "kb", Category: "custom", ContentPath: in}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("Create(%q) error = %v, want a validation error", in, err)
		}
	}
}

func TestKnowledgeBaseService_RequestIndexStaysInsideTheContentRoot(t *testing.T) {
	root, outside := knowledgeLayout(t)
	kb := func(id, contentPath string) *knowledgebase.KnowledgeBase {
		return &knowledgebase.KnowledgeBase{ID: id, Name: id, ContentPath: contentPath}
	}
	svc, queue := newKBTestService(t, root,
		kb("dir", "docs"), kb("file", "notes.md"), kb("in-link", "in-link"),
		kb("abs-dir", "abs-dir"), kb("abs-file", "abs-file"), kb("rel-dir", "rel-dir"), kb("rel-file", "rel-file"),
		kb("legacy", outside), kb("legacy-etc", "/etc"), kb("missing", "nope"), kb("dotdot", "../outside"),
	)
	ctx := context.Background()

	for id, want := range map[string]string{"dir": "docs", "file": "notes.md", "in-link": "in-link"} {
		queue.published = nil
		if err := svc.RequestIndex(ctx, id); err != nil {
			t.Fatalf("RequestIndex(%s): %v", id, err)
		}
		if len(queue.published) != 1 || queue.published[0].subject != messagequeue.SubjectRetrievalIndexRequest {
			t.Fatalf("RequestIndex(%s) published %v", id, queue.published)
		}
		var payload messagequeue.RetrievalIndexRequestPayload
		if err := json.Unmarshal(queue.published[0].data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.KnowledgePath != want || payload.WorkspacePath != "" || payload.ProjectID != "kb:"+id {
			t.Errorf("RequestIndex(%s) payload = %+v, want knowledge_path %q and no workspace_path", id, payload, want)
		}
	}

	for _, id := range []string{"abs-dir", "abs-file", "rel-dir", "rel-file", "legacy", "legacy-etc", "missing", "dotdot"} {
		queue.published = nil
		err := svc.RequestIndex(ctx, id)
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("RequestIndex(%s) error = %v, want a validation error", id, err)
		}
		if len(queue.published) != 0 {
			t.Errorf("RequestIndex(%s) published %d messages, want none", id, len(queue.published))
		}
		if err != nil && strings.Contains(err.Error(), outside) {
			t.Errorf("RequestIndex(%s) error names the outside path: %v", id, err)
		}
	}
}

func TestKnowledgeBaseService_RefusedRowsAreLoggedOnce(t *testing.T) {
	root, outside := knowledgeLayout(t)
	svc, _ := newKBTestService(t, root, &knowledgebase.KnowledgeBase{ID: "legacy", ContentPath: outside})
	for range 3 {
		_ = svc.RequestIndex(context.Background(), "legacy")
	}
	n := 0
	svc.content.logged.Range(func(_, _ any) bool { n++; return true })
	if n != 1 {
		t.Fatalf("logged %d knowledge bases, want 1", n)
	}
}

func TestProcessKnowledgeBase_FallbackReadsOnlyBelowTheContentRoot(t *testing.T) {
	root, outside := knowledgeLayout(t)
	writeTestFile(t, filepath.Join(root, "big.md"), strings.Repeat("ä", 10000))
	kbSvc, _ := newKBTestService(t, root)
	svc := NewContextOptimizerService(&mockStore{}, &config.Orchestrator{}, &config.Limits{})

	kb := func(contentPath string) *knowledgebase.KnowledgeBase {
		return &knowledgebase.KnowledgeBase{ID: contentPath, Name: "kb", ContentPath: contentPath, Status: "indexed"}
	}
	if got := svc.processKnowledgeBase(context.Background(), kb("notes.md"), "q"); got != nil {
		t.Fatalf("without the knowledge service: %v, want no entries", got)
	}
	svc.SetKnowledgeBases(kbSvc)

	got := svc.processKnowledgeBase(context.Background(), kb("notes.md"), "q")
	if len(got) != 1 || got[0].Content != "inside notes" {
		t.Fatalf("notes.md entries = %v, want the inside notes", got)
	}
	got = svc.processKnowledgeBase(context.Background(), kb("in-link"), "q")
	if len(got) != 1 || got[0].Content != "inside guide" {
		t.Fatalf("in-root symlink entries = %v, want the inside guide", got)
	}
	big := svc.processKnowledgeBase(context.Background(), kb("big.md"), "q")
	if len(big) != 1 || len(big[0].Content) > 8192 || !strings.HasPrefix(strings.Repeat("ä", 10000), big[0].Content) {
		t.Fatalf("big.md: %d entries, want one entry cut at 8192 bytes on a rune boundary", len(big))
	}

	for _, contentPath := range []string{
		filepath.Join(outside, "secret.md"), "/etc/passwd", "abs-file", "rel-file", "../outside/secret.md", "docs",
	} {
		for _, e := range svc.processKnowledgeBase(context.Background(), kb(contentPath), "q") {
			t.Errorf("content_path %q gave entry %q, want none", contentPath, e.Content)
		}
	}
}
