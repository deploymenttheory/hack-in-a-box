# Feedback report deadline regression (#325)

A report mutation validated its complete current qualification/judging/result
source, retained the authored report, then rebuilt the same source to assemble
its response under the same event lock. The response now uses that transaction’s
already validated source. Every new read/replay remains fresh; no lock, current
role, confirmation, sign-off, audience, score choice or API budget is relaxed.

The actual amended-release native HTTP case with real PostgreSQL, race detection,
4ms/query and observed maintenance contention went from503 at10.027s to200
at6.690s (1219 queries; the failed attempt stopped at1775 queries). It verifies
exact original acknowledgement, the authored amendment and deliberate withholding
of still-available confirmed scores. A separate zero-delay census measured2415
queries before the fix, rather than assuming the cause from source alone.

The eleven top-level report/source owner tests passed168.149s under the race
detector, including current-source invalidation, embargoes, API and receipt
contracts, concurrent writes, own-team privacy and completed/archived access.
Standards and Spec source reviews each found zero implementation defects.

The original connected native journey passed244.90s at clean `c86b6b75`, image
`sha256:b959ebc556b66d2ad1ac1cd6ccae25b32fe7ba6a5398492b1333cf08d4ff2992`.
Its production tree was compared against fix `bc42e351` and is identical. Both
modes passed the previously failing actual feedback path, then outcomes, archive
and actual retained30/60/90-day queue effects. Private original journal, database
and logs are retained and hashed in [the bounded receipt](proof.json); generated
resources were cleaned. The receipt audit found the newer final-repository and
expiry helpers were not invoked in that execution. Those assertions are not
claimed here; their separate corrected fixture is being rerun for #120.

This is a source-pinned fix regression, not full #120/browser/release acceptance.
All current PR CI gates remain required before merge.
