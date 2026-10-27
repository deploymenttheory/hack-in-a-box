# Native result request budget (#330)

Two connected six-team lifecycle runs returned503 while preparing results at
the unchanged10-second deadline. A smaller native HTTP regression uses actual
frozen submissions, mentor qualification, independent final cards/deliberation,
a real PostgreSQL pool and6ms test-only query delay. It concurrently invokes
the native submissions Maintain method and observes PostgreSQL's blocking graph
to prove that maintenance waits on the API's event lock. This focused check is
not a substitute for the separately deployed River-worker lifecycle.

Before the correction, the6ms case returned503 after10.026s (1227queries before
cancellation). The complete4ms diagnostic path had1326queries and took8.298s.
The same6ms case now returns200 in5.442s with674queries, preserving exact native
preparation/source/acknowledgement and no inferred confirmation or announcement.
Original diagnostic/failed/success logs remain private. A separate earlier
reaper-startup failure never reached the API and is not counted as a red result.

Result writes already validate their grading source under the event lock. The
response now reuses that source only within that same transaction and reloads
the persisted round/event revision. New reads and original-request replays still
calculate a current source. Result decisions, current organiser authority,
source invalidation, independent majority rules, signoff, embargo and request
timeouts remain intact. No across-request source cache is introduced.

Fifteen selected native owner tests passed under the race detector (148.295s),
covering current sources/authority, replacement/reopen invalidation, explicit
releases and original receipts, concurrent publication, independent embargoes,
all required ballots/strict majority/revote, mentor advisory scope, downstream
publication privacy and rollback. The complete pure results suite passed1.232s.
The configured CI lint scope reports0issues. Separate Standards and Spec reviews
found0standards findings and0implementation defects; full CI and connected
lifecycle acceptance are distinct from these bounded receipts.

The rebased production/test tree is identical to reviewed a6a9641b; the main
6c52e4e1 predecessor adds the independent Compose readiness fix. Exact results
are in proof.json. These are native correctness/budget results, not deployment
capacity, full Fleet role/offline coverage or release acceptance.

## Ballot extension and final owner checks

The connected rerun atddd7f2e1 passed result preparation and first-event result
confirmation, then failed on a second-event judge ballot (408.47s whole run).
It remains a failed full lifecycle; it does not establish end-to-end acceptance.

The same6ms native regression model reproduced a ballot503 after10.034s
(1268queries before cancellation). Removing only its post-write projection was
insufficient: that intermediate attempt still returned503. The final correction
checks each unique tied candidate once within the fixed actor/run/source event
transaction and rebuilds the successful write response from that authorised
page. Every candidate still passes full native assignment, conflict, frozen
final and deliberation checks; each evidence reference is separately authorised.
Counts and this judge's own vote are recalculated with the original all-judges/
strict-majority rules. Advisory scope is retained; other judges' prose is absent.
Fresh requests and receipt replays still read current sources/authority.

The ballot now returns200 in6.123s with660queries and observed native maintenance
contention. The expanded18-test native owner suite passed430.521s under-race,
including both budget regressions and the additional current-target/visible-
evidence cases. The complete pure result suite passes (cached), configured CI
lint reports0issues, and both extended review axes report0implementation defects.
The later source change only wraps two lines for the configured formatter.

Run both budget regressions with the ordinary integration setup:

```sh
HACKBOX_INTEGRATION=1 go test -race ./internal/app \
  -run '^TestResult(Preparation|Ballot)WithDatabaseRoundTripCost$' -count=1
```

The next full connected rerun uses the exact final production tree, with its
originals retained separately. Full CI, complete Fleet role/offline acceptance
and load capacity remain separate gates; this bounded receipt claims none of
those results by implication.
