# Connected native retention cases

Both cases execute the original two-event native journey through actual
registration, published/frozen teams, GitHub work/access, formal cutoff,
independent judging, feedback, reviewed outcomes and archive. They are not
standalone seeded policy fixtures or complete Fleet browser/release acceptance.

| Case | Native policy setup | Required retained effects | Exact expiry |
| --- | --- | --- | --- |
| `configured120` (default command) | Explicit organiser privacy update to120 days | Approved real queued30/60/90-day owner reviews | Owner/organiser outcome/history and last approved notification unreadable at day120; readable1second before |
| `default90` | Read and assert the untouched native90-day policy; no policy update | Actual30/60-day deliveries; save a day90 draft, reject its approval at the exclusive deadline without changing its version/state | Same read-time checks at day90; the unsendable reminder is never treated as an approved/delivered effect |

```sh
sh scripts/verify-lifecycle-journey.sh
HACKBOX_LIFECYCLE_RETENTION_CASE=default90 sh scripts/verify-lifecycle-journey.sh
```

The clean-source runner records the selected case, actual image/source/exit,
original intent journal, private database/logs and scoped cleanup. The final Go
receipt requires executed repository/expiry sentinels for both events; default90
also requires the rejected boundary approval and exactly two approved reviews.
No production retention default/window, approval rule or API timeout changes.
The private monotonic fixture clock permits the already-configured120-day
boundary after postponement; it is absent from production images.

The default90 case passes199.11s on clean `922ad7b2`; see
[actual receipt](default-retention-expiry-proof.json). The earlier120-day case
passed211.23s on `92f988ae`; its same-driver repeat is pending. Earlier
uninvoked-helper receipts do not supply these executed verdicts.
