-- name: DeliveryReview :one
SELECT * FROM communication_delivery_reviews WHERE event_id=$1 AND id=$2;

-- name: RecordDeliveryReview :exec
INSERT INTO communication_delivery_reviews(id,event_id,operation_id,actor_id,expected_revision,action,reason,prior_attempt_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: PendingDeliveryResend :one
SELECT r.* FROM communication_delivery_reviews r
WHERE r.operation_id=$1 AND r.prior_attempt_id=$2 AND r.action='resend_acknowledged'
AND NOT EXISTS(SELECT 1 FROM communication_delivery_resend_uses u WHERE u.review_id=r.id)
ORDER BY r.created_at DESC,r.id DESC LIMIT 1;

-- name: ConsumeDeliveryResend :exec
INSERT INTO communication_delivery_resend_uses(review_id,attempt_id) VALUES($1,$2);

-- name: DeliveryReviews :many
SELECT * FROM communication_delivery_reviews WHERE event_id=$1 AND operation_id=$2
AND (sqlc.narg(before_id)::uuid IS NULL OR id<sqlc.narg(before_id)::uuid)
ORDER BY id DESC LIMIT sqlc.arg(page_size)::int;

-- name: ReviewRetryOperation :execrows
UPDATE operations SET state='pending',revision=revision+1,updated_at=now()
WHERE id=$1 AND revision=$2 AND state IN ('pending','ambiguous','delivered') AND approved AND NOT cancelled AND lease_token IS NULL;

-- name: ResolveUnknownDelivery :execrows
UPDATE operations SET cancelled=true,revision=revision+1,updated_at=now()
WHERE id=$1 AND revision=$2 AND state='ambiguous' AND lease_token IS NULL;
