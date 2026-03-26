package database

import (
	"context"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/knowledgebase"
)

// ContextStore defines database operations for context packs, shared context,
// repo maps, retrieval scopes, and knowledge bases.
type ContextStore interface {
	// Context Packs
	CreateContextPack(ctx context.Context, pack *cfcontext.ContextPack) error
	GetContextPack(ctx context.Context, id string) (*cfcontext.ContextPack, error)
	GetContextPackByTask(ctx context.Context, taskID string) (*cfcontext.ContextPack, error)
	DeleteContextPack(ctx context.Context, id string) error

	// Shared Context
	CreateSharedContext(ctx context.Context, sc *cfcontext.SharedContext) error
	GetSharedContext(ctx context.Context, id string) (*cfcontext.SharedContext, error)
	GetSharedContextByTeam(ctx context.Context, teamID string) (*cfcontext.SharedContext, error)
	AddSharedContextItem(ctx context.Context, req cfcontext.AddSharedItemRequest) (*cfcontext.SharedContextItem, error)
	DeleteSharedContext(ctx context.Context, id string) error

	// Repo Maps
	UpsertRepoMap(ctx context.Context, m *cfcontext.RepoMap) error
	GetRepoMap(ctx context.Context, projectID string) (*cfcontext.RepoMap, error)
	DeleteRepoMap(ctx context.Context, projectID string) error

	// Retrieval Scopes
	CreateScope(ctx context.Context, req cfcontext.CreateScopeRequest) (*cfcontext.RetrievalScope, error)
	GetScope(ctx context.Context, id string) (*cfcontext.RetrievalScope, error)
	ListScopes(ctx context.Context) ([]cfcontext.RetrievalScope, error)
	UpdateScope(ctx context.Context, id string, req cfcontext.UpdateScopeRequest) (*cfcontext.RetrievalScope, error)
	DeleteScope(ctx context.Context, id string) error
	ListScopesByProject(ctx context.Context, projectID string) ([]cfcontext.RetrievalScope, error)
	GetScopesForProject(ctx context.Context, projectID string) ([]cfcontext.RetrievalScope, error)
	AddProjectToScope(ctx context.Context, scopeID, projectID string) error
	RemoveProjectFromScope(ctx context.Context, scopeID, projectID string) error

	// Knowledge Bases
	CreateKnowledgeBase(ctx context.Context, req *knowledgebase.CreateRequest) (*knowledgebase.KnowledgeBase, error)
	GetKnowledgeBase(ctx context.Context, id string) (*knowledgebase.KnowledgeBase, error)
	ListKnowledgeBases(ctx context.Context) ([]knowledgebase.KnowledgeBase, error)
	UpdateKnowledgeBase(ctx context.Context, id string, req knowledgebase.UpdateRequest) (*knowledgebase.KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, id string) error
	UpdateKnowledgeBaseStatus(ctx context.Context, id, status string, chunkCount int) error
	AddKnowledgeBaseToScope(ctx context.Context, scopeID, kbID string) error
	RemoveKnowledgeBaseFromScope(ctx context.Context, scopeID, kbID string) error
	ListKnowledgeBasesByScope(ctx context.Context, scopeID string) ([]knowledgebase.KnowledgeBase, error)
}
