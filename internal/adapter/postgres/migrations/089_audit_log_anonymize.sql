-- +goose Up
-- GDPR Art. 17 / ADR-009: Make admin_email nullable for anonymization on user deletion.
-- IP addresses are personal data per CJEU C-582/14 (Breyer). Retention: 180 days.
ALTER TABLE audit_log ALTER COLUMN admin_email DROP NOT NULL;
ALTER TABLE audit_log ALTER COLUMN admin_email SET DEFAULT NULL;
COMMENT ON COLUMN audit_log.ip_address IS 'Personal data per CJEU C-582/14. Retention: 180 days.';

-- +goose Down
UPDATE audit_log SET admin_email = 'unknown' WHERE admin_email IS NULL;
ALTER TABLE audit_log ALTER COLUMN admin_email SET NOT NULL;
