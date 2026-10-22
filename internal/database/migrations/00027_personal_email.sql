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
    mailbox text NOT NULL,
    message_id text NOT NULL,
    snapshot jsonb NOT NULL,
    state text NOT NULL DEFAULT 'prepared' CHECK (state IN ('prepared','accepted','retryable','permanent','ambiguous')),
    receipt_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE email_calendar_sequences (
    event_id uuid NOT NULL REFERENCES events(id),
    test boolean NOT NULL,
    fingerprint bytea NOT NULL,
    sequence integer NOT NULL DEFAULT 0 CHECK (sequence >= 0),
    PRIMARY KEY(event_id,test)
);

-- +goose Down
DROP TABLE email_calendar_sequences;
DROP TABLE smtp_message_snapshots;
DROP TABLE email_verifications;
DROP TABLE person_email_contacts;
