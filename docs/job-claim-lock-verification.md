# Job claim and recovery lock ordering

Event commands lock the event before cancelling its pending/running operations. Previously, job claiming locked the operation and then synchronously called `Handler.Eligible`; recovery similarly held the operation while calling `Handler.Reconcile`. Those callbacks can open independent transactions that lock the event.

The resulting cycle crosses a client wait:

```text
claim/recovery transaction holds Operation
  awaits provider callback on another transaction
    waits for Event held by an event command
      waits for Operation held by claim/recovery
```

PostgreSQL sees the two database waits, but cannot detect the synchronous client dependency as a complete database deadlock. The event command and worker stall until a context timeout or other interruption.

Claiming now reads a candidate and calls eligibility without row locks, then locks Event followed by Operation and checks the candidate revision, delivery identity, due time, approval, cancellation and event write eligibility before claiming. Recovery retains its advisory guard while calling reconciliation without row locks, then locks Event followed by Operation and revalidates identity, revision, running state, expired lease and exact lease token before recording the result. A changed candidate remains available for a fresh check. Existing worker snoozing, advisory exclusion, attempt/lease ownership and provider effect barriers remain in place.

## Reproduction and verification

The tests use isolated real PostgreSQL databases and the public job service. No River client is started for the bounded lock reproduction; the existing real-River suite independently verifies worker scheduling and retained uniqueness.

- Before: `TestEligibilityCannotBlockEventCancellation` and `TestRecoveryCannotBlockEventCancellation` both fail on unchanged main. Each observes two PostgreSQL sessions blocked simultaneously, then the event command reaches its explicit two-second deadline. Combined run: **FAIL 9.554s**.
- After: both bounded regressions pass with the race detector: **PASS 4.757s**. Event cancellation commits, the callback resumes, and no cancelled operation is delivered or newly attempted.
- `TestApprovalChangedDuringEligibilityRequiresFreshCheck` revokes and reapproves while the callback is paused. The stale check cannot claim the newly approved revision; the next current check delivers once.
- Full `HACKBOX_INTEGRATION=1 go test -race ./internal/jobs -count=1`: **PASS 21.250s**, including real River deferred eligibility/approval/cancellation, overlapping worker/recovery exclusion, crash recovery, current role/resource/event authority and provider result handling.
- Scoped repository formatter and `golangci-lint run ./internal/jobs/...`: **0 issues**.

The fix requires no schema, API or provider payload changes. Deployment can roll back the code, although doing so reintroduces the lock cycle. Provider handlers still own rechecking current authority at the effect boundary; generic eligibility is not permission to bypass those barriers.
