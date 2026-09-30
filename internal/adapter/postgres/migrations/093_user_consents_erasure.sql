-- +goose Up
-- GDPR erasure failed for every user with a consent record: user_consents.user_id
-- referenced users without an ON DELETE action. Consent records are proof of
-- consent (Art. 7(1)) and stay after erasure, anonymized: the erasure clears the
-- IP address and user agent, and deleting the user sets user_id to NULL here.
ALTER TABLE user_consents ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE user_consents DROP CONSTRAINT IF EXISTS user_consents_user_id_fkey;
ALTER TABLE user_consents ADD CONSTRAINT user_consents_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
-- The anonymized records of erased users cannot be linked to a user again, so
-- they are removed to restore NOT NULL.
DELETE FROM user_consents WHERE user_id IS NULL;
ALTER TABLE user_consents DROP CONSTRAINT IF EXISTS user_consents_user_id_fkey;
ALTER TABLE user_consents ADD CONSTRAINT user_consents_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE user_consents ALTER COLUMN user_id SET NOT NULL;
