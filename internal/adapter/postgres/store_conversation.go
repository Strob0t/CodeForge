package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
)

func (s *Store) CreateConversation(ctx context.Context, c *conversation.Conversation) (*conversation.Conversation, error) {
	tid := tenantFromCtx(ctx)
	var created conversation.Conversation
	err := s.pool.QueryRow(ctx,
		`INSERT INTO conversations (tenant_id, project_id, title)
		 VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, project_id, title, created_at, updated_at`,
		tid, c.ProjectID, c.Title,
	).Scan(&created.ID, &created.TenantID, &created.ProjectID, &created.Title, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return &created, nil
}

func (s *Store) GetConversation(ctx context.Context, id string) (*conversation.Conversation, error) {
	var c conversation.Conversation
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_id, title, created_at, updated_at
		 FROM conversations WHERE id = $1`,
		id,
	).Scan(&c.ID, &c.TenantID, &c.ProjectID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("get conversation %s: %w", id, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("get conversation %s: %w", id, err)
	}
	return &c, nil
}

func (s *Store) ListConversationsByProject(ctx context.Context, projectID string) ([]conversation.Conversation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, project_id, title, created_at, updated_at
		 FROM conversations WHERE project_id = $1 AND tenant_id = $2 ORDER BY updated_at DESC`,
		projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var result []conversation.Conversation
	for rows.Next() {
		var c conversation.Conversation
		if err := rows.Scan(&c.ID, &c.TenantID, &c.ProjectID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete conversation %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete conversation %s: %w", id, domain.ErrNotFound)
	}
	return nil
}

func (s *Store) CreateMessage(ctx context.Context, m *conversation.Message) (*conversation.Message, error) {
	var created conversation.Message
	err := s.pool.QueryRow(ctx,
		`INSERT INTO conversation_messages (conversation_id, role, content, tokens_in, tokens_out, model)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, conversation_id, role, content, tokens_in, tokens_out, model, created_at`,
		m.ConversationID, m.Role, m.Content, m.TokensIn, m.TokensOut, m.Model,
	).Scan(&created.ID, &created.ConversationID, &created.Role, &created.Content,
		&created.TokensIn, &created.TokensOut, &created.Model, &created.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	// Update conversation's updated_at
	_, _ = s.pool.Exec(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = $1`, m.ConversationID)
	return &created, nil
}

func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, conversation_id, role, content, tokens_in, tokens_out, model, created_at
		 FROM conversation_messages WHERE conversation_id = $1 ORDER BY created_at ASC`,
		conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var result []conversation.Message
	for rows.Next() {
		var m conversation.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.TokensIn, &m.TokensOut, &m.Model, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
