-- +goose Up
-- Convert users.id from TEXT to UUID for type consistency across all tables.

-- 1. Drop existing foreign keys referencing users(id).
ALTER TABLE api_keys DROP CONSTRAINT api_keys_user_id_fkey;
ALTER TABLE password_reset_tokens DROP CONSTRAINT password_reset_tokens_user_id_fkey;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_user_id_fkey;
ALTER TABLE user_llm_keys DROP CONSTRAINT user_llm_keys_user_id_fkey;

-- 2. Convert users.id from TEXT to UUID.
ALTER TABLE users ALTER COLUMN id DROP DEFAULT;
ALTER TABLE users ALTER COLUMN id SET DATA TYPE UUID USING id::uuid;
ALTER TABLE users ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- 3. Convert all FK columns from TEXT to UUID.
ALTER TABLE api_keys ALTER COLUMN user_id SET DATA TYPE UUID USING user_id::uuid;
ALTER TABLE password_reset_tokens ALTER COLUMN user_id SET DATA TYPE UUID USING user_id::uuid;
ALTER TABLE refresh_tokens ALTER COLUMN user_id SET DATA TYPE UUID USING user_id::uuid;
ALTER TABLE user_llm_keys ALTER COLUMN user_id SET DATA TYPE UUID USING user_id::uuid;

-- 4. Re-add foreign keys.
ALTER TABLE api_keys ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE user_llm_keys ADD CONSTRAINT user_llm_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);

-- +goose Down
ALTER TABLE api_keys DROP CONSTRAINT api_keys_user_id_fkey;
ALTER TABLE password_reset_tokens DROP CONSTRAINT password_reset_tokens_user_id_fkey;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_user_id_fkey;
ALTER TABLE user_llm_keys DROP CONSTRAINT user_llm_keys_user_id_fkey;

ALTER TABLE users ALTER COLUMN id DROP DEFAULT;
ALTER TABLE users ALTER COLUMN id SET DATA TYPE TEXT USING id::text;
ALTER TABLE users ALTER COLUMN id SET DEFAULT (gen_random_uuid())::text;
ALTER TABLE api_keys ALTER COLUMN user_id SET DATA TYPE TEXT USING user_id::text;
ALTER TABLE password_reset_tokens ALTER COLUMN user_id SET DATA TYPE TEXT USING user_id::text;
ALTER TABLE refresh_tokens ALTER COLUMN user_id SET DATA TYPE TEXT USING user_id::text;
ALTER TABLE user_llm_keys ALTER COLUMN user_id SET DATA TYPE TEXT USING user_id::text;

ALTER TABLE api_keys ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE password_reset_tokens ADD CONSTRAINT password_reset_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE user_llm_keys ADD CONSTRAINT user_llm_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
