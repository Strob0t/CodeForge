package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
)

func (s *Store) CreateConversation(ctx context.Context, c *conversation.Conversation) (*conversation.Conversation, error) {
	tid := tenantFromCtx(ctx)
	var created conversation.Conversation
	err := s.pool.QueryRow(ctx,
		`INSERT INTO conversations (tenant_id, project_id, title, mode, model)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, project_id, title, mode, model, created_at, updated_at`,
		tid, c.ProjectID, c.Title, c.Mode, c.Model,
	).Scan(&created.ID, &created.TenantID, &created.ProjectID, &created.Title,
		&created.Mode, &created.Model, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return &created, nil
}

func (s *Store) GetConversation(ctx context.Context, id string) (*conversation.Conversation, error) {
	var c conversation.Conversation
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_id, title, mode, model, created_at, updated_at
		 FROM conversations WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx),
	).Scan(&c.ID, &c.TenantID, &c.ProjectID, &c.Title, &c.Mode, &c.Model, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get conversation %s", id)
	}
	return &c, nil
}

func (s *Store) ListConversationsByProject(ctx context.Context, projectID string) ([]conversation.Conversation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, project_id, title, mode, model, created_at, updated_at
		 FROM conversations WHERE project_id = $1 AND tenant_id = $2 ORDER BY updated_at DESC`,
		projectID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (conversation.Conversation, error) {
		var c conversation.Conversation
		err := r.Scan(&c.ID, &c.TenantID, &c.ProjectID, &c.Title, &c.Mode, &c.Model, &c.CreatedAt, &c.UpdatedAt)
		return c, err
	})
}

func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM conversations WHERE id = $1 AND tenant_id = $2`, id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete conversation %s", id)
}

func (s *Store) CreateMessage(ctx context.Context, m *conversation.Message) (*conversation.Message, error) {
	// Normalise nil tool_calls to SQL NULL.
	var toolCallsJSON []byte
	if len(m.ToolCalls) > 0 {
		toolCallsJSON = []byte(m.ToolCalls)
	}

	// Normalise nil images to SQL NULL.
	var imagesJSON []byte
	if len(m.Images) > 0 {
		imagesJSON = []byte(m.Images)
	}

	var created conversation.Message
	err := s.pool.QueryRow(ctx,
		`INSERT INTO conversation_messages (conversation_id, role, content, tool_calls, tool_call_id, tool_name, tokens_in, tokens_out, model, images)
		 SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		 FROM conversations WHERE id = $1 AND tenant_id = $11
		 RETURNING id, conversation_id, role, content, tool_calls, tool_call_id, tool_name, tokens_in, tokens_out, model, images, created_at`,
		m.ConversationID, m.Role, m.Content, toolCallsJSON, m.ToolCallID, m.ToolName, m.TokensIn, m.TokensOut, m.Model, imagesJSON, tenantFromCtx(ctx),
	).Scan(&created.ID, &created.ConversationID, &created.Role, &created.Content,
		&created.ToolCalls, &created.ToolCallID, &created.ToolName,
		&created.TokensIn, &created.TokensOut, &created.Model, &created.Images, &created.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("create message: conversation %s not found: %w", m.ConversationID, domain.ErrNotFound)
		}
		return nil, fmt.Errorf("create message: %w", err)
	}
	// Update conversation's updated_at
	_, _ = s.pool.Exec(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = $1 AND tenant_id = $2`, m.ConversationID, tenantFromCtx(ctx))
	return &created, nil
}

func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.conversation_id, m.role, m.content, m.tool_calls, m.tool_call_id, m.tool_name, m.tokens_in, m.tokens_out, m.model, m.images, m.created_at
		 FROM conversation_messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.conversation_id = $1 AND c.tenant_id = $2
		 ORDER BY m.created_at ASC`,
		conversationID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (conversation.Message, error) {
		var m conversation.Message
		err := r.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.ToolCalls, &m.ToolCallID, &m.ToolName,
			&m.TokensIn, &m.TokensOut, &m.Model, &m.Images, &m.CreatedAt)
		return m, err
	})
}

func (s *Store) DeleteConversationMessages(ctx context.Context, conversationID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM conversation_messages WHERE conversation_id = $1 AND conversation_id IN (SELECT id FROM conversations WHERE tenant_id = $2)`,
		conversationID, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("delete conversation messages: %w", err)
	}
	return nil
}

// UpdateConversationMode persists a mode override for a conversation.
func (s *Store) UpdateConversationMode(ctx context.Context, conversationID, mode string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE conversations SET mode = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		mode, conversationID, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update conversation mode %s", conversationID)
}

// UpdateConversationModel persists a model override for a conversation.
func (s *Store) UpdateConversationModel(ctx context.Context, conversationID, model string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE conversations SET model = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		model, conversationID, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update conversation model %s", conversationID)
}

// SearchConversationMessages performs full-text search across conversation messages
// within the tenant, optionally filtered by project IDs.
func (s *Store) SearchConversationMessages(ctx context.Context, query string, projectIDs []string, limit int) ([]conversation.Message, error) {
	tid := tenantFromCtx(ctx)

	baseQuery := `SELECT m.id, m.conversation_id, m.role, m.content, m.tool_calls,
		m.tool_call_id, m.tool_name, m.tokens_in, m.tokens_out, m.model, m.created_at
		FROM conversation_messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE c.tenant_id = $1
		AND m.content IS NOT NULL AND m.content != ''
		AND to_tsvector('english', m.content) @@ plainto_tsquery('english', $2)`

	args := []any{tid, query}
	argIdx := 3

	if len(projectIDs) > 0 {
		baseQuery += fmt.Sprintf(" AND c.project_id = ANY($%d)", argIdx)
		args = append(args, projectIDs)
		argIdx++
	}

	baseQuery += fmt.Sprintf(" ORDER BY ts_rank(to_tsvector('english', m.content), plainto_tsquery('english', $2)) DESC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search conversation messages: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (conversation.Message, error) {
		var m conversation.Message
		err := r.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.ToolCalls, &m.ToolCallID, &m.ToolName,
			&m.TokensIn, &m.TokensOut, &m.Model, &m.CreatedAt)
		return m, err
	})
}

// CreateToolMessages inserts multiple tool-related messages (assistant messages
// with tool_calls and tool result messages) in a single batch operation.
func (s *Store) CreateToolMessages(ctx context.Context, conversationID string, msgs []conversation.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for i := range msgs {
		var toolCallsJSON []byte
		if len(msgs[i].ToolCalls) > 0 {
			toolCallsJSON = []byte(msgs[i].ToolCalls)
		}
		// Use ON CONFLICT DO NOTHING for messages with a tool_call_id to
		// prevent duplicates from NATS redeliveries.  Assistant messages
		// (which have tool_calls JSON but no tool_call_id) always insert.
		query := `INSERT INTO conversation_messages (conversation_id, role, content, tool_calls, tool_call_id, tool_name, tokens_in, tokens_out, model)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		if msgs[i].ToolCallID != "" {
			query += ` ON CONFLICT (conversation_id, tool_call_id) WHERE tool_call_id IS NOT NULL AND tool_call_id != '' DO NOTHING`
		}
		batch.Queue(
			query,
			conversationID, msgs[i].Role, msgs[i].Content, toolCallsJSON,
			msgs[i].ToolCallID, msgs[i].ToolName, msgs[i].TokensIn, msgs[i].TokensOut, msgs[i].Model,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()

	for range msgs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("create tool message batch: %w", err)
		}
	}

	// Update conversation's updated_at
	_, _ = s.pool.Exec(ctx, `UPDATE conversations SET updated_at = NOW() WHERE id = $1 AND tenant_id = $2`, conversationID, tenantFromCtx(ctx))
	return nil
}
