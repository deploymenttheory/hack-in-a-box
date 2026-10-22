-- +goose Up
ALTER TABLE team_credentials
 ADD COLUMN resource_type text NOT NULL DEFAULT 'credential' CHECK(resource_type IN ('credential','account','licence','access_code','other')),
 ADD COLUMN available_at timestamptz,
 ADD COLUMN expires_at timestamptz,
 ADD CONSTRAINT credential_availability_interval CHECK(available_at IS NULL OR expires_at IS NULL OR expires_at>available_at);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM team_credentials WHERE available_at IS NOT NULL OR expires_at IS NOT NULL) THEN
  RAISE EXCEPTION 'explicitly remove credential availability and expiry metadata before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE team_credentials DROP CONSTRAINT credential_availability_interval,DROP COLUMN expires_at,DROP COLUMN available_at,DROP COLUMN resource_type;
