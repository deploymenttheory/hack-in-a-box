-- +goose Up
CREATE TABLE sessions (
    token text PRIMARY KEY,
    data bytea NOT NULL,
    expiry timestamptz NOT NULL
);
CREATE INDEX sessions_expiry ON sessions(expiry);
-- One-use state is consumed atomically, including when a callback is invalid.
CREATE TABLE login_attempts (
    state_hash text PRIMARY KEY,
    browser_hash text NOT NULL,
    nonce text NOT NULL,
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX login_attempts_expiry ON login_attempts(expires_at);

-- +goose Down
DROP TABLE login_attempts;
DROP TABLE sessions;
