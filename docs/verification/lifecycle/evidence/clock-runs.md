# Native clock seam checkpoints

These are focused real-PostgreSQL component/composition regressions. Their
completed-event fixtures are **not** the HTTP lifecycle's setup, and they do
not complete #120. They start real River clients; no test calls a delivery
handler, `Execute`, `JobRetry`, or a virtual queue clock.

## Retained eligibility

- Session `92575`: behavioral RED, package 3.831s. A genuine native approved
  thirty-day review failed ordinary automation maintenance at retained enqueue:
  `enqueue durable communication occurrence: access denied`. Domain time was
  `2026-06-27T20:47:45Z`; wall time was `2026-10-25T19:47:45Z`. The historical
  source was domain-current, but jobs supplied wall time to its retained SQL
  predicate. No queued effect is attributed to this failed enqueue.
- Session `58827`: GREEN, test 33.70s/package 34.117s. The same native owner
  approval enqueued; actual River attempted before domain due and remained
  scheduled without an inbox effect. After monotonic domain advance, its actual
  thirty-second wall snooze resumed into the native inbox. No delivery shortcut
  or deadline/retention widening was used.
- Fix and regression: `5dbeec08`. The observed RED/GREEN preceded consumption
  of the separate event timestamp prerequisite. The eligibility changes were
  limited to jobs due/retained-source checks and the existing private factory
  clock. River scheduling, snoozes, HTTP/auth, attempts, leases and reconciliation
  retain wall time. Nil preserves the previous production behavior.

## Default exclusive retention boundary

- A first fixture attempt failed with a PostgreSQL parameter-type conflict;
  that setup error was corrected before claiming behavioral RED.
- Session `35254`: behavioral RED, test 1.84s/package 2.251s. Under the unchanged
  default ninety-day policy, native approval incorrectly accepted a plan due at
  exact exclusive expiry: January 1, 2026 12:00 UTC → April 1 12:00 UTC.
- Session `20309`: GREEN, test 3.05s/package 3.486s. The one-comparison fix
  rejects equality; native owner access still works one second before expiry
  and returns not-found at expiry. No default or SQL privacy deadline changed.
- Fix and regression: `4a34ef8f`.

## Due eligibility, real leases and nil default

These jobs-module cases use a synthetic external-delivery boundary to inspect
an actual committed lease from inside a real River callback. They supplement,
and do not replace, the native outcome-owner/inbox composition proof above.

- Session `89332`: FAIL, package 37.506s. The test incorrectly additionally
  expected operation state `in_flight`; the repository state is `running`.
  Removing this unnecessary internal-state assertion after compilation did not
  change that running binary. Its failure remains attributed to the old source.
  The raw log pathname was reused by the next run; this record preserves the
  captured tool-output result, not a purported complete original raw log.
- Session `17530`: PASS, package 39.625s; nil default 3.41s, historical domain
  35.94s. Source SHA256 was
  `5aac8426d252558c7472d02640290829ee8f2d5c500e93cec9b4c1752d328027`.
  Its raw log was preserved separately as `hackbox120-domain-lease-17530.log`.
  This version checked the latest attempt after callback; the next run adds
  the stronger matching-token assertion during callback.
- Session `68956`: PASS, package 40.420s; nil default 6.66s, historical domain
  33.47s. Source stayed frozen, SHA256
  `77454ee1ea0583b19e3288c2c0d47a05ce68a1ccb6bcdab94c9b3578a85ce3e1`,
  committed unchanged as `03c763b4`. It confirms no early eligibility call,
  effect, application attempt or lease, a future wall-time River snooze, natural
  resume after domain advance, one delivery, a real two-minute wall lease and
  the matching running attempt by lease token **during** callback. Observed:

  | Mode | Due | Matching native attempt | Lease expiry |
  |---|---|---|---|
  | Nil default | 2026-10-25T20:00:21.919715Z | 2026-10-25T20:00:26.294655Z | 2026-10-25T20:02:26.297104Z |
  | Historical domain | 2026-07-27T19:00:27.883226Z | 2026-10-25T20:00:59.362213Z | 2026-10-25T20:02:59.365331Z |

Each run used one sequential Go fixture process with `GOMAXPROCS=2`, `-p=2`;
native PostgreSQL fixtures terminated through their normal cleanup. There was
no concurrent lifecycle Compose project or second lifecycle Go fixture process.

## Event timestamp prerequisite

`f78a8e85` consumes exactly the four core files from
`f96fb69af58076bbd1747de25c06a736d84e5d9c`: database event query/generated query,
event service and completion-clock regression. A four-file diff against the
source commit was empty. Its showcase-specific app assertion was excluded;
event timestamp logic was not independently reimplemented.

Configured positive 30/60/90 queued effects, source invalidation and the full
HTTP/Compose lifecycle remain pending; none is inferred from these checkpoints.
