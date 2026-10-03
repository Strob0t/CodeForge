package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
)

// CreateChannel creates a channel in the caller's tenant. Its creator is
// linked only when it has a row in users: a caller without one (the default
// user while auth is disabled, the internal service key user) has no user to
// reference, and the channel is created without a creator instead of failing
// on the foreign key (KI-89).
func (s *Store) CreateChannel(ctx context.Context, ch *channel.Channel) (*channel.Channel, error) {
	tid := tenantFromCtx(ctx)
	var created channel.Channel
	err := s.pool.QueryRow(ctx,
		`INSERT INTO channels (tenant_id, project_id, name, type, description, created_by)
		 VALUES ($1, $2, $3, $4, $5, (SELECT u.id FROM users u WHERE u.id = $6::uuid))
		 RETURNING id, tenant_id, COALESCE(project_id::text,''), name, type, description, COALESCE(created_by::text,''), created_at`,
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
		`SELECT id, tenant_id, COALESCE(project_id::text,''), name, type, description, COALESCE(created_by::text,''), created_at,
		        webhook_key_hash IS NOT NULL
		 FROM channels WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx),
	).Scan(&ch.ID, &ch.TenantID, &ch.ProjectID, &ch.Name,
		&ch.Type, &ch.Description, &ch.CreatedBy, &ch.CreatedAt, &ch.HasWebhookKey)
	if err != nil {
		return nil, notFoundWrap(err, "get channel %s", id)
	}
	return &ch, nil
}

// ListChannels lists the tenant's channels, of one project when projectID is
// set. With a user, each channel carries the number of top-level messages of
// others after the user's read position (0 for a user without an account row,
// whose read position is not tracked).
func (s *Store) ListChannels(ctx context.Context, projectID, userID string) ([]channel.Channel, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.tenant_id, COALESCE(c.project_id::text,''), c.name, c.type, c.description,
		        COALESCE(c.created_by::text,''), c.created_at, c.webhook_key_hash IS NOT NULL,
		        CASE WHEN NOT EXISTS (SELECT 1 FROM users u WHERE u.id = $3::uuid) THEN 0 ELSE (
		          SELECT COUNT(*) FROM channel_messages m
		          WHERE m.channel_id = c.id AND m.parent_id IS NULL
		            AND m.sender_id IS DISTINCT FROM $3::uuid
		            AND m.created_at > COALESCE(rs.last_read_at, '-infinity'::timestamptz))
		        END
		 FROM channels c
		 LEFT JOIN channel_read_state rs ON rs.channel_id = c.id AND rs.user_id = $3::uuid
		 WHERE c.tenant_id = $1 AND ($2::uuid IS NULL OR c.project_id = $2::uuid)
		 ORDER BY c.created_at DESC`,
		tenantFromCtx(ctx), nullIfEmpty(projectID), nullIfEmpty(userID))
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (channel.Channel, error) {
		var ch channel.Channel
		err := r.Scan(&ch.ID, &ch.TenantID, &ch.ProjectID, &ch.Name,
			&ch.Type, &ch.Description, &ch.CreatedBy, &ch.CreatedAt, &ch.HasWebhookKey, &ch.UnreadCount)
		return ch, err
	})
}

// SetChannelWebhookKeyHash stores the hash of the webhook key of a channel of
// the caller's tenant.
func (s *Store) SetChannelWebhookKeyHash(ctx context.Context, channelID string, hash []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE channels SET webhook_key_hash = $3 WHERE id = $1 AND tenant_id = $2`,
		channelID, tenantFromCtx(ctx), hash)
	return execExpectOne(tag, err, "set webhook key of channel %s", channelID)
}

// GetChannelWebhookKeyHash returns the tenant and webhook key hash of a
// channel.
//
// INTENTIONALLY CROSS-TENANT: a webhook call carries no user and no tenant;
// the key it presents is checked against this hash and the message is then
// handled in the returned tenant.
func (s *Store) GetChannelWebhookKeyHash(ctx context.Context, channelID string) (tenantID string, hash []byte, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT tenant_id, webhook_key_hash FROM channels WHERE id = $1`, channelID,
	).Scan(&tenantID, &hash)
	if err != nil {
		return "", nil, notFoundWrap(err, "get webhook key of channel %s", channelID)
	}
	return tenantID, hash, nil
}

// MarkChannelRead moves the user's read position in a channel of the
// caller's tenant to one of its messages; an older message does not move it
// back. A channel or message that is not the tenant's, or not the channel's,
// is not found; a user without an account row gets
// channel.ErrReadStateNotTracked.
func (s *Store) MarkChannelRead(ctx context.Context, channelID, userID, messageID string) (*channel.ReadState, error) {
	tid := tenantFromCtx(ctx)
	var messageFound, hasAccount bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM channel_messages m JOIN channels c ON c.id = m.channel_id
		                WHERE m.id = $1 AND m.channel_id = $2 AND c.tenant_id = $3),
		        EXISTS (SELECT 1 FROM users u WHERE u.id = $4::uuid)`,
		messageID, channelID, tid, userID,
	).Scan(&messageFound, &hasAccount)
	switch {
	case err != nil:
		return nil, fmt.Errorf("mark channel %s read: %w", channelID, err)
	case !messageFound:
		return nil, fmt.Errorf("mark channel %s read at message %s: %w", channelID, messageID, domain.ErrNotFound)
	case !hasAccount:
		return nil, fmt.Errorf("mark channel %s read: %w", channelID, channel.ErrReadStateNotTracked)
	}

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO channel_read_state (channel_id, user_id, tenant_id, last_read_message_id, last_read_at)
		 SELECT m.channel_id, $2, m.tenant_id, m.id, m.created_at
		 FROM channel_messages m JOIN channels c ON c.id = m.channel_id
		 WHERE m.id = $3 AND m.channel_id = $1 AND c.tenant_id = $4
		 ON CONFLICT (channel_id, user_id) DO UPDATE
		   SET last_read_message_id = EXCLUDED.last_read_message_id,
		       last_read_at = EXCLUDED.last_read_at, updated_at = now()
		   WHERE channel_read_state.last_read_at <= EXCLUDED.last_read_at`,
		channelID, userID, messageID, tid); err != nil {
		return nil, fmt.Errorf("mark channel %s read: %w", channelID, err)
	}
	var state channel.ReadState
	err = s.pool.QueryRow(ctx,
		`SELECT channel_id, user_id, COALESCE(last_read_message_id::text,''), last_read_at
		 FROM channel_read_state
		 WHERE channel_id = $1 AND user_id = $2 AND tenant_id = $3`,
		channelID, userID, tid,
	).Scan(&state.ChannelID, &state.UserID, &state.LastReadMessageID, &state.LastReadAt)
	if err != nil {
		return nil, notFoundWrap(err, "mark channel %s read at message %s", channelID, messageID)
	}
	return &state, nil
}

// ListChannelReadStates returns the read positions in a channel of the
// caller's tenant.
func (s *Store) ListChannelReadStates(ctx context.Context, channelID string) ([]channel.ReadState, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT channel_id, user_id, COALESCE(last_read_message_id::text,''), last_read_at
		 FROM channel_read_state WHERE channel_id = $1 AND tenant_id = $2
		 ORDER BY last_read_at DESC`,
		channelID, tenantFromCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("list read states of channel %s: %w", channelID, err)
	}
	return scanRows(rows, func(r pgx.Rows) (channel.ReadState, error) {
		var rs channel.ReadState
		err := r.Scan(&rs.ChannelID, &rs.UserID, &rs.LastReadMessageID, &rs.LastReadAt)
		return rs, err
	})
}

func (s *Store) DeleteChannel(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM channels WHERE id = $1 AND tenant_id = $2`,
		id, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "delete channel %s", id)
}

// CreateChannelMessage stores a message in a channel of the caller's tenant;
// the message takes the channel's tenant. A channel of another tenant is not
// found, and so is a thread parent that is not a message of the same channel.
// The sender's kind and name are always kept; its user ID only when it has a
// row in users (like CreateChannel's creator, KI-89).
func (s *Store) CreateChannelMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error) {
	var created channel.Message
	err := s.pool.QueryRow(ctx,
		`INSERT INTO channel_messages (channel_id, tenant_id, sender_id, sender_type, sender_name, content, metadata, parent_id)
		 SELECT c.id, c.tenant_id, (SELECT u.id FROM users u WHERE u.id = $2::uuid), $3, $4, $5,
		        COALESCE($6::jsonb, '{}'::jsonb), $7::uuid
		 FROM channels c WHERE c.id = $1 AND c.tenant_id = $8
		   AND ($7::uuid IS NULL OR EXISTS (
		     SELECT 1 FROM channel_messages p WHERE p.id = $7::uuid AND p.channel_id = c.id))
		 RETURNING id, channel_id, COALESCE(sender_id::text,''), sender_type, sender_name, content, COALESCE(metadata,'{}'), COALESCE(parent_id::text,''), created_at`,
		msg.ChannelID, nullIfEmpty(msg.SenderID), msg.SenderType, msg.SenderName,
		msg.Content, nullIfEmpty(msg.Metadata), nullIfEmpty(msg.ParentID), tenantFromCtx(ctx),
	).Scan(&created.ID, &created.ChannelID, &created.SenderID, &created.SenderType,
		&created.SenderName, &created.Content, &created.Metadata,
		&created.ParentID, &created.CreatedAt)
	if err != nil {
		return nil, notFoundWrap(err, "create channel message in channel %s", msg.ChannelID)
	}
	return &created, nil
}

func (s *Store) ListChannelMessages(ctx context.Context, channelID, cursor string, limit int) ([]channel.Message, error) {
	var rows pgx.Rows
	var err error

	if cursor == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT m.id, m.channel_id, COALESCE(m.sender_id::text,''), m.sender_type, m.sender_name, m.content, COALESCE(m.metadata,'{}'), COALESCE(m.parent_id::text,''), m.created_at
			 FROM channel_messages m
			 JOIN channels c ON c.id = m.channel_id
			 WHERE m.channel_id = $1 AND c.tenant_id = $2
			 ORDER BY m.created_at DESC LIMIT $3`,
			channelID, tenantFromCtx(ctx), limit)
	} else {
		// Parse cursor as time for cursor-based pagination.
		cursorTime, parseErr := time.Parse(time.RFC3339Nano, cursor)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid cursor: %w", parseErr)
		}
		rows, err = s.pool.Query(ctx,
			`SELECT m.id, m.channel_id, COALESCE(m.sender_id::text,''), m.sender_type, m.sender_name, m.content, COALESCE(m.metadata,'{}'), COALESCE(m.parent_id::text,''), m.created_at
			 FROM channel_messages m
			 JOIN channels c ON c.id = m.channel_id
			 WHERE m.channel_id = $1 AND c.tenant_id = $2 AND m.created_at < $3
			 ORDER BY m.created_at DESC LIMIT $4`,
			channelID, tenantFromCtx(ctx), cursorTime, limit)
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

// AnonymizeChannelMessagesForUser replaces the sender name of the user's
// messages in the current tenant with channel.ErasedSenderName. Called before
// the user is deleted (GDPR Art. 17); the foreign key then sets sender_id to
// NULL, and the messages stay in their channels.
func (s *Store) AnonymizeChannelMessagesForUser(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE channel_messages SET sender_name = $3 WHERE sender_id = $1 AND tenant_id = $2`,
		userID, tenantFromCtx(ctx), channel.ErasedSenderName)
	if err != nil {
		return 0, fmt.Errorf("anonymize channel messages for user: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) AddChannelMember(ctx context.Context, m *channel.Member) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO channel_members (channel_id, user_id, role, notify, tenant_id)
		 SELECT $1, $2, $3, $4, tenant_id
		 FROM channels WHERE id = $1 AND tenant_id = $5
		 ON CONFLICT (channel_id, user_id) DO NOTHING`,
		m.ChannelID, m.UserID, m.Role, m.Notify, tenantFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("add channel member: %w", err)
	}
	return nil
}

func (s *Store) UpdateChannelMemberNotify(ctx context.Context, channelID, userID string, notify channel.NotifySetting) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE channel_members SET notify = $1
		 WHERE channel_id = $2 AND user_id = $3 AND tenant_id = $4`,
		notify, channelID, userID, tenantFromCtx(ctx))
	return execExpectOne(tag, err, "update channel member notify %s/%s", channelID, userID)
}
