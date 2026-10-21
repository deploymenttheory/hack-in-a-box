-- +goose Up
ALTER TABLE registration_history ALTER COLUMN actor_id DROP NOT NULL;
CREATE TABLE registration_settings (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 automated boolean NOT NULL DEFAULT false,
 working_days integer NOT NULL DEFAULT 2 CHECK(working_days BETWEEN 1 AND 30),
 approved_by uuid NOT NULL REFERENCES people(id),
 revision bigint NOT NULL DEFAULT 1
);
CREATE TABLE registration_offers (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id uuid NOT NULL,
 registration_id uuid NOT NULL,
 expires_at timestamptz NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','accepted','declined','expired')),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,registration_id) REFERENCES registrations(event_id,id)
);
CREATE UNIQUE INDEX one_pending_registration_offer ON registration_offers(registration_id) WHERE state='pending';
-- +goose Down
DROP TABLE registration_offers;
DROP TABLE registration_settings;
