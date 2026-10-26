package loadprofile

import (
	"strconv"
	"testing"
)

func TestOnlyContendingOwnersValidatedRevisionConflictIsExpected(t *testing.T) {
	for _, status := range []int{403, 409, 429, 500} {
		for _, code := range []string{"revision_conflict", "idempotency_conflict", "rate_limited"} {
			t.Run(strconv.Itoa(status)+"-"+code, func(t *testing.T) {
				var ledger AttemptLedger
				spec := syntheticWrite()
				id, err := ledger.Begin(spec)
				if err != nil {
					t.Fatal(err)
				}
				response := Response{
					Status: status,
					Body: []byte(
						`{"error":{"code":"` + code + `","message":"Synthetic component-only rejection","request_id":"synthetic-request"}}`,
					),
				}
				writer := NativeWrites{Attempts: &ledger}
				outcome, err := writer.finish(id, response, nil, false, true)
				if status == 409 && code == "revision_conflict" {
					if outcome != ExpectedConflict || err != nil ||
						ledger.ActorBlocked(spec.Actor) {
						t.Fatal("native precommit conflict not accounted", outcome, err)
					}
				} else if outcome != Unexpected || err == nil || !ledger.ActorBlocked(spec.Actor) {
					t.Fatal("unexpected denial/error was hidden", outcome, err)
				}
			})
		}
	}
	var ledger AttemptLedger
	spec := syntheticWrite()
	id, err := ledger.Begin(spec)
	if err != nil {
		t.Fatal(err)
	}
	writer := NativeWrites{Attempts: &ledger}
	outcome, err := writer.finish(
		id,
		Response{
			Status: 409,
			Body: []byte(
				`{"error":{"code":"revision_conflict","message":"Synthetic","request_id":"synthetic"}}`,
			),
		},
		nil,
		false,
		false,
	)
	if outcome != Unexpected || err == nil {
		t.Fatal("registration form mismatch hidden as write contention", outcome, err)
	}
}
