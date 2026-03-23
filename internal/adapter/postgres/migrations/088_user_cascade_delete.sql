-- +goose Up
-- Ensure all user-referencing FKs cascade on delete for GDPR compliance (Article 17).
-- Migration 070 re-added these FKs without CASCADE; this migration fixes that.

-- API keys
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_user_id_fkey;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Refresh tokens
ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_user_id_fkey;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Password reset tokens
ALTER TABLE password_reset_tokens DROP CONSTRAINT IF EXISTS password_reset_tokens_user_id_fkey;
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- User LLM keys
ALTER TABLE user_llm_keys DROP CONSTRAINT IF EXISTS user_llm_keys_user_id_fkey;
ALTER TABLE user_llm_keys ADD CONSTRAINT user_llm_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- Channel tables: SET NULL on user deletion (shared resources should not be cascade-deleted)
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_created_by_fkey;
ALTER TABLE channels ADD CONSTRAINT channels_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE channel_messages DROP CONSTRAINT IF EXISTS channel_messages_sender_id_fkey;
ALTER TABLE channel_messages ADD CONSTRAINT channel_messages_sender_id_fkey
    FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE channel_members DROP CONSTRAINT IF EXISTS channel_members_user_id_fkey;
ALTER TABLE channel_members ADD CONSTRAINT channel_members_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- +goose Down
-- Revert to default NO ACTION (safe -- just removes CASCADE/SET NULL)
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_user_id_fkey;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_user_id_fkey;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE password_reset_tokens DROP CONSTRAINT IF EXISTS password_reset_tokens_user_id_fkey;
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE user_llm_keys DROP CONSTRAINT IF EXISTS user_llm_keys_user_id_fkey;
ALTER TABLE user_llm_keys ADD CONSTRAINT user_llm_keys_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_created_by_fkey;
ALTER TABLE channels ADD CONSTRAINT channels_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);

ALTER TABLE channel_messages DROP CONSTRAINT IF EXISTS channel_messages_sender_id_fkey;
ALTER TABLE channel_messages ADD CONSTRAINT channel_messages_sender_id_fkey
    FOREIGN KEY (sender_id) REFERENCES users(id);

ALTER TABLE channel_members DROP CONSTRAINT IF EXISTS channel_members_user_id_fkey;
ALTER TABLE channel_members ADD CONSTRAINT channel_members_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
