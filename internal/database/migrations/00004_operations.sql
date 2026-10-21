-- +goose Up
CREATE TABLE operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES events(id),
    operation_key text NOT NULL CHECK (length(operation_key) BETWEEN 1 AND 200),
    kind text NOT NULL CHECK (length(kind) BETWEEN 1 AND 64),
    target_id uuid NOT NULL,
    due_at timestamptz NOT NULL,
    approved boolean NOT NULL DEFAULT false,
    cancelled boolean NOT NULL DEFAULT false,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','running','delivered','permanent','ambiguous','cancelled')),
    attempts integer NOT NULL DEFAULT 0,
    lease_until timestamptz,
    lease_token uuid,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, operation_key)
);
CREATE INDEX operations_due ON operations(due_at) WHERE state = 'pending';
CREATE TABLE delivery_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id uuid NOT NULL REFERENCES operations(id),
    lease_token uuid NOT NULL UNIQUE,
    outcome text NOT NULL CHECK (outcome IN ('running','delivered','retryable','permanent','ambiguous')),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE TABLE reconciliation_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recovered_leases integer NOT NULL,
    enqueued integer NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE reconciliation_runs;
DROP TABLE delivery_attempts;
DROP TABLE operations;
