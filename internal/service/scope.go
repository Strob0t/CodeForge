package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ScopeService manages retrieval scope CRUD and cross-project search fan-out.
type ScopeService struct {
	store          database.Store
	retrieval      *RetrievalService
	graph          *GraphService
	knowledgeBases *KnowledgeBaseService
}

// NewScopeService creates a ScopeService.
func NewScopeService(store database.Store) *ScopeService {
	return &ScopeService{store: store}
}

// SetRetrieval wires the retrieval service for cross-project search.
func (s *ScopeService) SetRetrieval(r *RetrievalService) { s.retrieval = r }

// SetGraph wires the graph service for cross-project graph search.
func (s *ScopeService) SetGraph(g *GraphService) { s.graph = g }

// SetKnowledgeBase wires the knowledge base service for scope-based KB search.
func (s *ScopeService) SetKnowledgeBase(kb *KnowledgeBaseService) { s.knowledgeBases = kb }

// --- CRUD ---

// Create validates and creates a new retrieval scope.
func (s *ScopeService) Create(ctx context.Context, req cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validate scope: %w", err)
	}
	return s.store.CreateScope(ctx, req)
}

// Get returns a scope by ID.
func (s *ScopeService) Get(ctx context.Context, id string) (*cfcontext.RetrievalScope, error) {
	return s.store.GetScope(ctx, id)
}

// List returns all scopes for the current tenant.
func (s *ScopeService) List(ctx context.Context) ([]cfcontext.RetrievalScope, error) {
	return s.store.ListScopes(ctx)
}

// Update applies partial updates to a scope.
func (s *ScopeService) Update(ctx context.Context, id string, req cfcontext.UpdateScopeRequest) (*cfcontext.RetrievalScope, error) {
	return s.store.UpdateScope(ctx, id, req)
}

// Delete removes a scope by ID.
func (s *ScopeService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteScope(ctx, id)
}

// AddProject adds a project to a scope.
func (s *ScopeService) AddProject(ctx context.Context, scopeID, projectID string) error {
	return s.store.AddProjectToScope(ctx, scopeID, projectID)
}

// RemoveProject removes a project from a scope.
func (s *ScopeService) RemoveProject(ctx context.Context, scopeID, projectID string) error {
	return s.store.RemoveProjectFromScope(ctx, scopeID, projectID)
}

// ResolveProjectIDs returns all project IDs belonging to a scope.
func (s *ScopeService) ResolveProjectIDs(ctx context.Context, scopeID string) ([]string, error) {
	sc, err := s.store.GetScope(ctx, scopeID)
	if err != nil {
		return nil, err
	}
	return sc.ProjectIDs, nil
}

// --- Cross-project search ---

const maxScopeFanOut = 5

// SearchScope performs hybrid retrieval search across all projects in a scope.
func (s *ScopeService) SearchScope(
	ctx context.Context,
	scopeID, query string,
	topK int,
	bm25Weight, semanticWeight float64,
) ([]messagequeue.RetrievalSearchHitPayload, error) {
	if s.retrieval == nil {
		return nil, fmt.Errorf("retrieval service not configured")
	}

	pids, err := s.ResolveProjectIDs(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("resolve scope projects: %w", err)
	}

	// Resolve knowledge base IDs attached to this scope.
	var kbIDs []string
	if s.knowledgeBases != nil {
		kbs, err := s.knowledgeBases.ListByScope(ctx, scopeID)
		if err != nil {
			slog.Warn("failed to list scope knowledge bases", "scope_id", scopeID, "error", err)
		} else {
			for i := range kbs {
				if kbs[i].Status == "indexed" {
					kbIDs = append(kbIDs, "kb:"+kbs[i].ID)
				}
			}
		}
	}

	// Combine project IDs and KB "project" IDs for fan-out search.
	allIDs := make([]string, 0, len(pids)+len(kbIDs))
	allIDs = append(allIDs, pids...)
	allIDs = append(allIDs, kbIDs...)
	if len(allIDs) == 0 {
		return nil, nil
	}

	type result struct {
		hits []messagequeue.RetrievalSearchHitPayload
		err  error
	}

	sem := make(chan struct{}, maxScopeFanOut)
	results := make([]result, len(allIDs))
	var wg sync.WaitGroup

	for i, pid := range allIDs {
		wg.Add(1)
		go func(idx int, projectID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := s.retrieval.SearchSync(ctx, projectID, query, topK, bm25Weight, semanticWeight, scopeID)
			if err != nil {
				slog.Warn("scope search failed for project", "project_id", projectID, "error", err)
				results[idx] = result{err: err}
				return
			}
			// Tag each hit with its source project.
			for j := range resp.Results {
				resp.Results[j].ProjectID = projectID
			}
			results[idx] = result{hits: resp.Results}
		}(i, pid)
	}
	wg.Wait()

	var merged []messagequeue.RetrievalSearchHitPayload
	for _, r := range results {
		if r.err != nil {
			continue // partial failure — include results from successful projects
		}
		merged = append(merged, r.hits...)
	}

	// Sort descending by score.
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}

// SearchScopeGraph performs graph search across all projects in a scope.
func (s *ScopeService) SearchScopeGraph(
	ctx context.Context,
	scopeID string,
	seedSymbols []string,
	maxHops, topK int,
) ([]messagequeue.GraphSearchHitPayload, error) {
	if s.graph == nil {
		return nil, fmt.Errorf("graph service not configured")
	}

	pids, err := s.ResolveProjectIDs(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("resolve scope projects: %w", err)
	}
	if len(pids) == 0 {
		return nil, nil
	}

	type result struct {
		hits []messagequeue.GraphSearchHitPayload
		err  error
	}

	sem := make(chan struct{}, maxScopeFanOut)
	results := make([]result, len(pids))
	var wg sync.WaitGroup

	for i, pid := range pids {
		wg.Add(1)
		go func(idx int, projectID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := s.graph.SearchSync(ctx, projectID, seedSymbols, maxHops, topK, scopeID)
			if err != nil {
				slog.Warn("scope graph search failed for project", "project_id", projectID, "error", err)
				results[idx] = result{err: err}
				return
			}
			for j := range resp.Results {
				resp.Results[j].ProjectID = projectID
			}
			results[idx] = result{hits: resp.Results}
		}(i, pid)
	}
	wg.Wait()

	var merged []messagequeue.GraphSearchHitPayload
	for _, r := range results {
		if r.err != nil {
			continue
		}
		merged = append(merged, r.hits...)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}
