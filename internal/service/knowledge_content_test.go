package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	kbTenantA = "11111111-1111-4111-8111-111111111111"
	kbTenantB = "22222222-2222-4222-8222-222222222222"
)

var (
	ctxTenantA = tenantctx.WithTenant(context.Background(), kbTenantA)
	ctxTenantB = tenantctx.WithTenant(context.Background(), kbTenantB)
)

// knowledgeLayout builds a content root with one area per tenant: tenant A
// has docs/guide.md and notes.md plus symlinks that lead out of its area
// (into tenant B's area, out of the root); tenant B has secret.md; the root
// itself holds top.md; an outside directory holds a secret.
func knowledgeLayout(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "knowledge")
	outside = filepath.Join(base, "outside")
	areaA := filepath.Join(root, kbTenantA)
	areaB := filepath.Join(root, kbTenantB)
	for _, dir := range []string{filepath.Join(areaA, "docs"), areaB, outside} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(areaA, "docs", "guide.md"), "inside guide")
	writeTestFile(t, filepath.Join(areaA, "notes.md"), "inside notes")
	writeTestFile(t, filepath.Join(areaB, "secret.md"), "tenant B secret")
	writeTestFile(t, filepath.Join(root, "top.md"), "root level")
	writeTestFile(t, filepath.Join(outside, "secret.md"), "outside secret")
	for name, target := range map[string]string{
		"abs-dir":  outside,
		"abs-file": filepath.Join(outside, "secret.md"),
		"rel-dir":  "../../outside",
		"rel-file": "../../outside/secret.md",
		"cross":    "../" + kbTenantB + "/secret.md", // into tenant B's area
		"up":       "..",                             // the content root
		"in-link":  "docs/guide.md",
	} {
		if err := os.Symlink(target, filepath.Join(areaA, name)); err != nil {
			t.Fatal(err)
		}
	}
	return root, outside
}

func TestKnowledgeContent_Relative(t *testing.T) {
	root, outside := knowledgeLayout(t)
	k := newKnowledgeContent(root)
	areaA := filepath.Join(root, kbTenantA)

	for in, want := range map[string]string{
		"docs":                       "docs",
		"./docs/../docs/guide.md":    "docs/guide.md",
		".":                          ".",
		filepath.Join(areaA, "docs"): "docs",
		areaA:                        ".",
	} {
		if got, err := k.relative(kbTenantA, in); err != nil || got != want {
			t.Errorf("relative(A, %q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"..", "../" + kbTenantB + "/secret.md", filepath.Join(root, kbTenantB, "secret.md"),
		root, filepath.Join(root, "top.md"), outside, "/etc", root + "-other/x",
	} {
		if got, err := k.relative(kbTenantA, in); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("relative(A, %q) = %q, %v; want a validation error", in, got, err)
		}
	}
	for _, tenant := range []string{"", ".", "..", "a/b", "not-a-uuid", "urn:uuid:" + kbTenantA, "AAAAAAAA-1111-4111-8111-111111111111"} {
		if got, err := k.relative(tenant, "docs"); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("relative(%q, docs) = %q, %v; want a validation error", tenant, got, err)
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
	for _, in := range []string{filepath.Join(link, kbTenantA, "docs"), filepath.Join(root, kbTenantA, "docs")} {
		if got, err := k.relative(kbTenantA, in); err != nil || got != "docs" {
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

// refusals runs fn for each input and checks that every answer is the same
// validation error: no answer tells an existing path of another tenant (or
// of the root) apart from a missing one.
func refusals(t *testing.T, inputs []string, fn func(string) error) {
	t.Helper()
	var first string
	for _, in := range inputs {
		err := fn(in)
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("%q: error = %v, want a validation error", in, err)
			continue
		}
		if first == "" {
			first = err.Error()
		} else if err.Error() != first {
			t.Errorf("%q: error %q differs from %q (tells paths apart)", in, err, first)
		}
	}
}

func TestKnowledgeBaseService_CreateStoresTheTenantRelativePath(t *testing.T) {
	root, outside := knowledgeLayout(t)
	svc, _ := newKBTestService(t, root)
	areaA := filepath.Join(root, kbTenantA)

	for in, want := range map[string]string{"docs": "docs", filepath.Join(areaA, "notes.md"): "notes.md", ".": "."} {
		kb, err := svc.Create(ctxTenantA, &knowledgebase.CreateRequest{Name: "kb", Category: "custom", ContentPath: in})
		if err != nil || kb.ContentPath != want {
			t.Fatalf("Create(%q) = %+v, %v; want content_path %q", in, kb, err, want)
		}
	}
	refusals(t, []string{
		filepath.Join(root, kbTenantB, "secret.md"), filepath.Join(root, kbTenantB, "missing.md"),
		"../" + kbTenantB, "../" + kbTenantB + "/missing", root, outside, "/etc",
	}, func(in string) error {
		_, err := svc.Create(ctxTenantA, &knowledgebase.CreateRequest{Name: "kb", Category: "custom", ContentPath: in})
		return err
	})
}

func TestKnowledgeBaseService_RequestIndexStaysInsideTheTenantArea(t *testing.T) {
	root, outside := knowledgeLayout(t)
	kb := func(id, contentPath string) *knowledgebase.KnowledgeBase {
		return &knowledgebase.KnowledgeBase{ID: id, Name: id, ContentPath: contentPath}
	}
	svc, queue := newKBTestService(t, root,
		kb("dir", "docs"), kb("file", "notes.md"), kb("in-link", "in-link"), kb("area", "."), kb("b-secret", "secret.md"),
		kb("abs-dir", "abs-dir"), kb("abs-file", "abs-file"), kb("rel-dir", "rel-dir"), kb("rel-file", "rel-file"),
		kb("cross", "cross"), kb("up", "up"), kb("up-top", "up/top.md"), kb("legacy", outside),
		kb("legacy-b", filepath.Join(root, kbTenantB, "secret.md")), kb("legacy-etc", "/etc"),
		kb("missing", "nope"), kb("climb", "../"+kbTenantB+"/secret.md"), kb("climb-missing", "../"+kbTenantB+"/nope"),
		kb("root-relative-b", kbTenantB+"/secret.md"), // a round-2 row: relative to the content root
	)

	published := func(ctx context.Context, id string) messagequeue.RetrievalIndexRequestPayload {
		t.Helper()
		queue.published = nil
		if err := svc.RequestIndex(ctx, id); err != nil {
			t.Fatalf("RequestIndex(%s): %v", id, err)
		}
		var payload messagequeue.RetrievalIndexRequestPayload
		if len(queue.published) != 1 || json.Unmarshal(queue.published[0].data, &payload) != nil {
			t.Fatalf("RequestIndex(%s) published %v", id, queue.published)
		}
		return payload
	}
	for id, want := range map[string]string{"dir": "docs", "file": "notes.md", "in-link": "in-link", "area": "."} {
		payload := published(ctxTenantA, id)
		if payload.KnowledgePath != want || payload.WorkspacePath != "" || payload.TenantID != kbTenantA {
			t.Errorf("RequestIndex(%s) payload = %+v, want knowledge_path %q for tenant A", id, payload, want)
		}
	}
	if payload := published(ctxTenantB, "b-secret"); payload.TenantID != kbTenantB || payload.KnowledgePath != "secret.md" {
		t.Errorf("tenant B's own file: payload = %+v", payload)
	}

	queue.published = nil
	refusals(t, []string{
		"b-secret", "abs-dir", "abs-file", "rel-dir", "rel-file", "cross", "up", "up-top",
		"legacy", "legacy-b", "legacy-etc", "missing", "climb", "climb-missing", "root-relative-b",
	}, func(id string) error { return svc.RequestIndex(ctxTenantA, id) })
	if len(queue.published) != 0 {
		t.Errorf("refused requests published %d messages", len(queue.published))
	}
}

func TestKnowledgeBaseService_RefusedRowsAreLoggedOnce(t *testing.T) {
	root, outside := knowledgeLayout(t)
	svc, _ := newKBTestService(t, root, &knowledgebase.KnowledgeBase{ID: "legacy", ContentPath: outside})
	for range 3 {
		_ = svc.RequestIndex(ctxTenantA, "legacy")
	}
	n := 0
	svc.content.logged.Range(func(_, _ any) bool { n++; return true })
	if n != 1 {
		t.Fatalf("logged %d knowledge bases, want 1", n)
	}
}

func TestProcessKnowledgeBase_FallbackReadsOnlyTheTenantArea(t *testing.T) {
	root, outside := knowledgeLayout(t)
	writeTestFile(t, filepath.Join(root, kbTenantA, "big.md"), strings.Repeat("ä", 10000))
	kbSvc, _ := newKBTestService(t, root)
	svc := NewContextOptimizerService(&mockStore{}, &config.Orchestrator{}, &config.Limits{})

	kb := func(contentPath string) *knowledgebase.KnowledgeBase {
		return &knowledgebase.KnowledgeBase{ID: contentPath, Name: "kb", ContentPath: contentPath, Status: "indexed"}
	}
	if got := svc.processKnowledgeBase(ctxTenantA, kb("notes.md"), "q"); got != nil {
		t.Fatalf("without the knowledge service: %v, want no entries", got)
	}
	svc.SetKnowledgeBases(kbSvc)

	for ctx, want := range map[context.Context]map[string]string{
		ctxTenantA: {"notes.md": "inside notes", "in-link": "inside guide"},
		ctxTenantB: {"secret.md": "tenant B secret"},
	} {
		for contentPath, content := range want {
			got := svc.processKnowledgeBase(ctx, kb(contentPath), "q")
			if len(got) != 1 || got[0].Content != content {
				t.Errorf("%s entries = %v, want %q", contentPath, got, content)
			}
		}
	}
	big := svc.processKnowledgeBase(ctxTenantA, kb("big.md"), "q")
	if len(big) != 1 || len(big[0].Content) > 8192 || !strings.HasPrefix(strings.Repeat("ä", 10000), big[0].Content) {
		t.Fatalf("big.md: %d entries, want one entry cut at 8192 bytes on a rune boundary", len(big))
	}

	for _, contentPath := range []string{
		kbTenantB + "/secret.md", "secret.md", "cross", "up/top.md", "../" + kbTenantB + "/secret.md", filepath.Join(root, kbTenantB, "secret.md"),
		filepath.Join(outside, "secret.md"), "/etc/passwd", "abs-file", "rel-file", "docs",
	} {
		for _, e := range svc.processKnowledgeBase(ctxTenantA, kb(contentPath), "q") {
			t.Errorf("tenant A, content_path %q gave entry %q, want none", contentPath, e.Content)
		}
	}
}

// answeringQueue answers every retrieval search with one indexed chunk, as a
// worker with a built index would.
type answeringQueue struct {
	fakeQueue
	retrieval *RetrievalService
	searches  int
}

func (q *answeringQueue) Publish(ctx context.Context, subject string, data []byte) error {
	if subject == messagequeue.SubjectRetrievalSearchRequest {
		q.searches++
		var req messagequeue.RetrievalSearchRequestPayload
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}
		go q.retrieval.HandleSearchResult(ctx, &messagequeue.RetrievalSearchResultPayload{
			ProjectID: req.ProjectID, RequestID: req.RequestID,
			Results: []messagequeue.RetrievalSearchHitPayload{{Content: "indexed chunk of " + req.ProjectID}},
		})
	}
	return q.fakeQueue.Publish(ctx, subject, data)
}

// A knowledge base indexed while its content_path was usable keeps an index
// in memory; once the path lies outside the tenant's area, its chunks are no
// longer served (KI-105 round 3).
func TestProcessKnowledgeBase_StaleIndexIsNotServed(t *testing.T) {
	root, outside := knowledgeLayout(t)
	queue := &answeringQueue{}
	retrieval := NewRetrievalService(&mockStore{}, queue, &noopBroadcaster{}, &config.Orchestrator{},
		&config.Limits{SearchTimeout: 5 * time.Second})
	queue.retrieval = retrieval
	svc := NewContextOptimizerService(&mockStore{}, &config.Orchestrator{}, &config.Limits{})
	svc.SetRetrieval(retrieval)
	svc.SetKnowledgeBases(NewKnowledgeBaseService(&mockStore{}, root))

	kb := func(id, contentPath string) *knowledgebase.KnowledgeBase {
		payload := &messagequeue.RetrievalIndexResultPayload{ProjectID: "kb:" + id, Status: "ready"}
		if err := retrieval.HandleIndexResult(ctxTenantA, payload); err != nil {
			t.Fatal(err)
		}
		return &knowledgebase.KnowledgeBase{ID: id, Name: id, ContentPath: contentPath, Status: "indexed"}
	}

	got := svc.processKnowledgeBase(ctxTenantA, kb("usable", "notes.md"), "q")
	if len(got) != 1 || got[0].Content != "indexed chunk of kb:usable" {
		t.Fatalf("usable knowledge base: entries = %v, want the indexed chunk", got)
	}
	queue.searches = 0
	for id, contentPath := range map[string]string{
		"legacy": outside,
		"cross":  filepath.Join(root, kbTenantB, "secret.md"),
		"climb":  "../" + kbTenantB,
	} {
		if got := svc.processKnowledgeBase(ctxTenantA, kb(id, contentPath), "q"); len(got) != 0 {
			t.Errorf("%s (%s): entries = %v, want none", id, contentPath, got)
		}
	}
	if queue.searches != 0 {
		t.Errorf("stale knowledge bases were searched %d times", queue.searches)
	}
}
