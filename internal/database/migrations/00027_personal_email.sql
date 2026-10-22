-- +goose Up
CREATE TABLE person_email_contacts (
    person_id uuid PRIMARY KEY REFERENCES people(id),
    mailbox text NOT NULL CHECK (length(mailbox) BETWEEN 3 AND 254),
    provenance text NOT NULL CHECK (provenance IN ('oidc_verified','code_confirmed')),
    verified_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE email_verifications (
    id uuid PRIMARY KEY,
    person_id uuid NOT NULL REFERENCES people(id),
    session_revision bigint NOT NULL,
    mailbox text NOT NULL CHECK (length(mailbox) BETWEEN 3 AND 254),
    mailbox_hash bytea NOT NULL,
    code_hash bytea NOT NULL,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    requested_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    delivery_state text NOT NULL DEFAULT 'prepared' CHECK (delivery_state IN ('prepared','accepted','retryable','permanent','ambiguous')),
    receipt_code text NOT NULL DEFAULT '',
    CHECK (expires_at > requested_at)
);
CREATE INDEX email_verifications_person_time ON email_verifications(person_id,requested_at DESC);
CREATE INDEX email_verifications_destination_time ON email_verifications(mailbox_hash,requested_at DESC);

CREATE TABLE smtp_message_snapshots (
    operation_id uuid PRIMARY KEY REFERENCES operations(id),
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    test_request_id uuid NOT NULL,
    mailbox text NOT NULL,
    message_id text NOT NULL,
    snapshot jsonb NOT NULL,
    state text NOT NULL DEFAULT 'prepared' CHECK (state IN ('prepared','accepted','retryable','permanent','ambiguous')),
    receipt_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(event_id,test_request_id) REFERENCES communication_test_requests(event_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_smtp_snapshot_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.operation_id,NEW.event_id,NEW.person_id,NEW.test_request_id,NEW.mailbox,NEW.message_id,NEW.snapshot,NEW.created_at)
       IS DISTINCT FROM ROW(OLD.operation_id,OLD.event_id,OLD.person_id,OLD.test_request_id,OLD.mailbox,OLD.message_id,OLD.snapshot,OLD.created_at) THEN
        RAISE EXCEPTION 'SMTP attempt identity and content are immutable';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER smtp_snapshot_identity BEFORE UPDATE ON smtp_message_snapshots FOR EACH ROW EXECUTE FUNCTION protect_smtp_snapshot_identity();

CREATE TABLE email_calendar_sequences (
    event_id uuid NOT NULL REFERENCES events(id),
    test boolean NOT NULL,
    fingerprint bytea NOT NULL,
    sequence integer NOT NULL DEFAULT 0 CHECK (sequence >= 0),
    PRIMARY KEY(event_id,test)
);

CREATE TABLE smtp_campaign_messages (
    operation_id uuid PRIMARY KEY REFERENCES operations(id),
    event_id uuid NOT NULL REFERENCES events(id),
    intent_id uuid NOT NULL UNIQUE,
    campaign_id uuid NOT NULL,
    campaign_version bigint NOT NULL,
    approval_binding text NOT NULL,
    occurrence_id uuid NOT NULL,
    occurrence_binding text NOT NULL,
    provider_revision text NOT NULL,
    person_id uuid NOT NULL REFERENCES people(id),
    mailbox text NOT NULL,
    snapshot jsonb NOT NULL,
    state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','prepared','accepted','retryable','permanent','ambiguous')),
    receipt_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(event_id,campaign_id,campaign_version) REFERENCES communication_campaign_versions(event_id,id,version)
);
-- +goose StatementBegin
CREATE FUNCTION protect_smtp_campaign_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.operation_id,NEW.event_id,NEW.intent_id,NEW.campaign_id,NEW.campaign_version,NEW.approval_binding,NEW.occurrence_id,NEW.occurrence_binding,NEW.provider_revision,NEW.person_id,NEW.mailbox,NEW.snapshot,NEW.created_at)
       IS DISTINCT FROM ROW(OLD.operation_id,OLD.event_id,OLD.intent_id,OLD.campaign_id,OLD.campaign_version,OLD.approval_binding,OLD.occurrence_id,OLD.occurrence_binding,OLD.provider_revision,OLD.person_id,OLD.mailbox,OLD.snapshot,OLD.created_at) THEN
        RAISE EXCEPTION 'SMTP campaign identity and content are immutable';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER smtp_campaign_identity BEFORE UPDATE ON smtp_campaign_messages FOR EACH ROW EXECUTE FUNCTION protect_smtp_campaign_identity();

-- +goose Down
DROP TABLE smtp_campaign_messages;
DROP FUNCTION protect_smtp_campaign_identity();
DROP TABLE email_calendar_sequences;
DROP TABLE smtp_message_snapshots;
DROP FUNCTION protect_smtp_snapshot_identity();
DROP TABLE email_verifications;
DROP TABLE person_email_contacts;
