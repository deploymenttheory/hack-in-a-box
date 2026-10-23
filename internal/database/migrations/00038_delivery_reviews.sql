-- +goose Up
CREATE TABLE communication_delivery_reviews (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES events(id),
    operation_id uuid NOT NULL REFERENCES operations(id),
    actor_id uuid NOT NULL REFERENCES people(id),
    expected_revision bigint NOT NULL CHECK(expected_revision>0),
    action text NOT NULL CHECK(action IN ('retry','cancel','cancel_resend','resolve_no_resend','resend_acknowledged')),
    reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
    prior_attempt_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK(action<>'resend_acknowledged' OR prior_attempt_id IS NOT NULL)
);
CREATE INDEX communication_delivery_review_operation ON communication_delivery_reviews(operation_id,created_at DESC,id);
CREATE TABLE communication_delivery_resend_uses (
    review_id uuid PRIMARY KEY REFERENCES communication_delivery_reviews(id),
    attempt_id uuid NOT NULL UNIQUE,
    used_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER communication_delivery_reviews_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_delivery_reviews FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER communication_delivery_resend_uses_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_delivery_resend_uses FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM communication_delivery_reviews) OR EXISTS(SELECT 1 FROM communication_delivery_resend_uses) THEN
        RAISE EXCEPTION 'Delivery review and resend evidence must be retained';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE communication_delivery_resend_uses;
DROP TABLE communication_delivery_reviews;
