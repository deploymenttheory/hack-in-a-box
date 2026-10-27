# Native challenge dispatch boundary and recovery

This is part of #120's original timing matrix. It does not replace the complete
connected event journey or establish release readiness. The new native cases
are implemented but have not yet executed; compilation and the independent DST
calendar oracle passed. Record actual source/image/results below after running.

From a clean checkout with Docker, Git and Python 3:

```sh
sh scripts/verify-lifecycle-dispatch.sh
```

The command builds the explicit verification image and runs two fresh isolated
Compose projects sequentially. Each uses both judging modes, actual normal OIDC,
native registration/go/under-three remediation/frozen teams, reviewed challenge
and common rubric. No production provider account, user stack, public host port
or SQL setup is used. The only controlled clock is native domain eligibility;
HTTP/authentication, worker startup and River leases retain real wall time.

The host verifies that no worker exists, waits for the runner to advance the
private clock and write its ready barrier, then starts the actual native worker.
Neither test invokes a dispatcher/maintenance handler or edits a job's due time.

| Case | Actual domain instant at worker start | Required native outcome |
| --- | --- | --- |
| `last-on-time` | Scheduled T−7 plus exactly60seconds | The original approved version releases once and records the actual instant; confirmed readers receive that version. |
| `first-late` | Scheduled T−7 plus60seconds and1microsecond | Original version remains missed/unreleased; route/rubric/search embargo holds; manual live entry and reapproval at the past schedule fail. |

The late case then uses normal organiser APIs to postpone to a future T−7
schedule without reopening frozen teams. An explicitly saved/reviewed/approved
new version must remain private until that new instant, then release through
the actual worker while preserving the original missed history. An attendance
exception never substitutes for challenge publication. The fixture's exact
one-minute boundary is independent of the implementation constant.

Private evidence goes to a new owner-only temporary directory printed by the
runner: build/startup/service logs, original intent journal, ready barrier,
bounded Go receipt, actual exit status, source/image ledger and final database.
It is captured once and retained even for failures. Only the generated projects'
containers, volumes and networks are removed, and cleanup is checked explicitly.
Do not publish raw databases, auth state, logs or domain prose. Publish reviewed
positive-projection receipts/hashes and retain failed attempts honestly.

These are focused native scheduling/recovery cases. The original #120 twelve
criteria and L01–L18 still require the remaining retired-repository, complete
Fleet/browser/role/expiry/provider and normal-image execution and final audit.
