package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
)

func (s *Store) CreateChannel(ctx context.Context, ch *channel.Channel) (*channel.Channel, error) {
	tid := tenantFromCtx(ctx)
	var created channel.Channel
	err := s.pool.QueryRow(ctx,
		`INSERT INTO channels (tenant_id, project_id, name, type, description, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, project_id, name, type, description, created_by, created_at`,
		tid, nullIfEmpty(ch.ProjectID), ch.Name, ch.Type, ch.Description, nullIfEmpty(ch.CreatedBy),
	).Scan(&created.ID, &created.TenantID, &created.ProjectID, &created.Name,
		&created.Type, &created.Description, &created.CreatedBy, &created.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return &created, nil
}

func (s *Store) GetChannel(ctx context.Context, id string) (*channel.Channel, error) {
	var ch channel.Channel
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_id, name, type, description, created_by, created_at
		 FROM channels WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx),
	).Scan(&ch.ID, &ch.TenantID, &ch.ProjectID, &ch.Name,
		&ch.Type, &ch.Description, &ch.CreatedBy, &ch.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "get channel %s", id)
	}
	return &ch, nil
}

func (s *Store) ListChannels(ctx context.Context, projectID string) ([]channel.Channel, error) {
	tid := tenantFromCtx(ctx)
	var rows pgx.Rows
	var err error

	if projectID == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id, tenant_id, project_id, name, type, description, created_by, created_at
			 FROM channels WHERE tenant_id = $1 ORDER BY created_at DESC`,
			tid)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, tenant_id, project_id, name, type, description, created_by, created_at
			 FROM channels WHERE project_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`,
			projectID, tid)
	}
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (channel.Channel, error) {
		var ch channel.Channel
		err := r.Scan(&ch.ID, &ch.TenantID, &ch.ProjectID, &ch.Name,
			&ch.Type, &ch.Description, &ch.CreatedBy, &ch.CreatedAt)
		return ch, err
	})
}

func (s *Store) DeleteChannel(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM channels WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete channel %s", id)
}

func (s *Store) CreateChannelMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error) {
	var created channel.Message
	err := s.pool.QueryRow(ctx,
		`INSERT INTO channel_messages (channel_id, sender_id, sender_type, sender_name, content, metadata, parent_id)
		 VALUES ($1, $2, $3, $4, $5, COALESCE($6::jsonb, '{}'::jsonb), $7)
		 RETURNING id, channel_id, sender_id, sender_type, sender_name, content, metadata, parent_id, created_at`,
		msg.ChannelID, nullIfEmpty(msg.SenderID), msg.SenderType, msg.SenderName,
		msg.Content, nullIfEmpty(msg.Metadata), nullIfEmpty(msg.ParentID),
	).Scan(&created.ID, &created.ChannelID, &created.SenderID, &created.SenderType,
		&created.SenderName, &created.Content, &created.Metadata,
		&created.ParentID, &created.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create channel message: %w", err)
	}
	return &created, nil
}

func (s *Store) ListChannelMessages(ctx context.Context, channelID, cursor string, limit int) ([]channel.Message, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id, channel_id, sender_id, sender_type, sender_name, content, metadata, parent_id, created_at
			 FROM channel_messages WHERE channel_id = $1
			 ORDER BY created_at DESC LIMIT $2`,
			channelID, limit)
	} else {
		// Parse cursor as time for cursor-based pagination.
		cursorTime, parseErr := time.Parse(time.RFC3339Nano, cursor)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid cursor: %w", parseErr)
		}
		rows, err = s.pool.Query(ctx,
			`SELECT id, channel_id, sender_id, sender_type, sender_name, content, metadata, parent_id, created_at
			 FROM channel_messages WHERE channel_id = $1 AND created_at < $2
			 ORDER BY created_at DESC LIMIT $3`,
			channelID, cursorTime, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list channel messages: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (channel.Message, error) {
		var m channel.Message
		err := r.Scan(&m.ID, &m.ChannelID, &m.SenderID, &m.SenderType,
			&m.SenderName, &m.Content, &m.Metadata, &m.ParentID, &m.CreatedAt)
		return m, err
	})
}

func (s *Store) AddChannelMember(ctx context.Context, m *channel.Member) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, user_id, role, notify)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (channel_id, user_id) DO NOTHING`,
		m.ChannelID, m.UserID, m.Role, m.Notify)
	if err != nil {
		return fmt.Errorf("add channel member: %w", err)
	}
	return nil
}

func (s *Store) UpdateChannelMemberNotify(ctx context.Context, channelID, userID string, notify channel.NotifySetting) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE channel_members SET notify = $1 WHERE channel_id = $2 AND user_id = $3`,
		notify, channelID, userID)
	return execExpectOne(tag, err, "update channel member notify %s/%s", channelID, userID)
}
