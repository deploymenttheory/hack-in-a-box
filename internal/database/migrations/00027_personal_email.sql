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
CREATE UNIQUE INDEX smtp_campaign_person_source ON smtp_campaign_messages(event_id,campaign_id,campaign_version,approval_binding,occurrence_id,person_id);
CREATE TRIGGER smtp_campaign_identity BEFORE UPDATE ON smtp_campaign_messages FOR EACH ROW EXECUTE FUNCTION protect_smtp_campaign_identity();

-- Each network attempt has an immutable prepared barrier and at most one
-- append-only bounded receipt. Snapshots retain the current UI projection.
CREATE TABLE smtp_submission_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id uuid NOT NULL REFERENCES operations(id),
    source_kind text NOT NULL CHECK(source_kind IN ('test','campaign')),
    attempt_number integer NOT NULL CHECK(attempt_number>0),
    prepared_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(operation_id,attempt_number)
);
CREATE TABLE smtp_submission_receipts (
    attempt_id uuid PRIMARY KEY REFERENCES smtp_submission_attempts(id),
    outcome text NOT NULL CHECK(outcome IN ('accepted','retryable','permanent','ambiguous')),
    receipt_code text NOT NULL CHECK(receipt_code IN ('relay_accepted','relay_temporary_rejection','relay_rejection','acceptance_unknown','relay_unavailable','tls_verification_failed','tls_required','authentication_unavailable','invalid_message','smtp_not_configured','unknown')),
    recorded_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementBegin
CREATE FUNCTION protect_smtp_submission_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'SMTP submission evidence is append-only';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER smtp_attempt_append_only BEFORE UPDATE OR DELETE ON smtp_submission_attempts FOR EACH ROW EXECUTE FUNCTION protect_smtp_submission_evidence();
CREATE TRIGGER smtp_receipt_append_only BEFORE UPDATE OR DELETE ON smtp_submission_receipts FOR EACH ROW EXECUTE FUNCTION protect_smtp_submission_evidence();

CREATE TABLE smtp_campaign_omissions (
    event_id uuid NOT NULL REFERENCES events(id),
    intent_id uuid NOT NULL,
    campaign_id uuid NOT NULL,
    campaign_version bigint NOT NULL,
    approval_binding text NOT NULL,
    occurrence_id uuid NOT NULL,
    occurrence_binding text NOT NULL,
    person_id uuid NOT NULL REFERENCES people(id),
    reason text NOT NULL CHECK(reason='address_unverified'),
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,intent_id),
    FOREIGN KEY(event_id,campaign_id,campaign_version) REFERENCES communication_campaign_versions(event_id,id,version)
);
CREATE TRIGGER smtp_omission_append_only BEFORE UPDATE OR DELETE ON smtp_campaign_omissions FOR EACH ROW EXECUTE FUNCTION protect_smtp_submission_evidence();

-- +goose Down
DROP TABLE smtp_campaign_omissions;
DROP TABLE smtp_submission_receipts;
DROP TABLE smtp_submission_attempts;
DROP FUNCTION protect_smtp_submission_evidence();
DROP TABLE smtp_campaign_messages;
DROP FUNCTION protect_smtp_campaign_identity();
DROP TABLE email_calendar_sequences;
DROP TABLE smtp_message_snapshots;
DROP FUNCTION protect_smtp_snapshot_identity();
DROP TABLE email_verifications;
DROP TABLE person_email_contacts;
