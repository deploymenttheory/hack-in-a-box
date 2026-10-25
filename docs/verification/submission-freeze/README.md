# Submission cutoff repository evidence — issue #96

Implementation in progress against main `2d82ac402fc68e39af0c49708fa6f363742bbd66`.
This is backend work; #97 owns the Fleet journey and #108 owns final judge
assignment/scoring. Neither consumer is represented as complete by this work.
No real GitHub, SMTP or Teams operations are used by the proof.

## Native ownership and evidence policy

`internal/submissions.Settle` remains the sole final-outcome owner. Its existing
shared cutoff settles frozen, not-submitted and withdrawn outcomes independently
of organiser stage advancement. The new native `CutoffInTransaction` accessor
reads that original immutable settlement history and logical `frozen_at`; it does
not settle or infer an outcome. A delayed worker therefore cannot accidentally
bind a later organiser replacement as the original cutoff final.

The separate `internal/submissionfreeze` module records the original event, team,
entry, native final revision/outcome, managed numeric repository/provenance,
canonical repository URL, exact branch, logical cutoff and actual source-record
instant. Existing permitted file/URL-only evidence remains valid and records
`not_applicable`; a managed GitHub repository is not a new global requirement.
The routine branch default is `main`, visible before configuration. Current team
members or organisers may explicitly choose another valid branch before cutoff.
The source/configuration snapshot is immutable once captured. No code execution,
branch rewrite, repository deletion or automatic write-access revocation occurs.

Every successful ref lookup is a **current observation**. Its actual observation
time and SHA are retained, while the source remains `unresolved` until a current
authorised organiser explicitly reconciles evidence with a recorded reason.
Even a lookup completing at the logical cutoff is not treated as proof of an
unbroken historical timeline. An outage-delayed successful lookup cannot silently
become the cutoff head. Missing branches, empty repositories and inaccessible
repositories have explicit observation statuses; missing managed identity,
not-submitted, withdrawn and non-repository evidence have explicit source states.

Authenticated retained push evidence is queried directly by exact source/ref,
with bounded cursor pages. The staff status endpoint's latest-100 projection is
not used as a history source. UUID page order is storage order, not occurrence
order. Separate provider and received times survive. Commit author timestamps,
missing deliveries, receive order and informational activity counts never prove
cutoff contents or become scores. The existing selected webhook fields do not
include complete before/forced/deleted history; no such facts are inferred.

Reconciliation records source identity, reviewed head revision, exact current
native state/final revisions, selected retained observation/delivery IDs, SHA,
current organiser, actual decision time, reason and optional external review
provenance. Selecting a candidate is not automatic verification. Amendments are
new immutable decisions retaining the original source and every earlier receipt.
An amendment can deliberately bind a legitimately replaced current native final
in the same entry. It requires an explicit reviewed target, current native/head
versions and exact prior receipt identity. A later amendment is never labelled
original cutoff contents. Current-binding mismatch remains explicit until that
decision; GET never copies or moves a receipt to another native final.

The internal `EvidenceForFinalInTransaction` seam returns a code SHA only for an
exact matching current frozen final with completed reviewed evidence. The future
assessment owner must first enforce its own current assignment/window/conflict
policy in the same locked transaction. There is no new public judge shortcut.
Current parent event and native team/submission audience policy apply to public
reads. Decision reasons and external review prose are organiser-only. Archived
competition state cannot be amended or acquire new capture jobs/effects.

## Queue, provider and request lifetimes

API and worker composition register the same capture handler. Maintenance queues
one operation per entry/cutoff and deduplicates repeated scheduling. Native event
locks establish source facts and current authority; the existing durable job
claim protects live duplicate execution. Remote read-only provider I/O runs
outside event/operation row transactions. A second native transaction rechecks
current event/configuration/credential provenance, team/entry/final and resource
authority before publishing. Invalidated SHA payloads are discarded. A slow
provider does not block another team's native settlement/read. Retry is safe
because the handler performs no external write; immutable attempt history remains.

The existing GitHub App owner decrypts and clears its credentials privately. The
narrow branch seam requests a repository-scoped Contents-read installation token,
checks actual numeric repository/organisation/private identity, and reads one
exact `heads/<branch>` ref. It never fetches arbitrary participant URLs or uses
participant tokens. Provider response bodies and keys are never public fields.

Versioned `/api/v1/events/{event}/teams/{team}/submission-freeze` operations expose
current projection, explicit configure/reconcile/amend, organiser evidence pages
and actor-bound exact original-command recovery. Commands require CSRF and
observed revision. Reuse of request ID for different content conflicts; exact
original replay retains the original ACK under current read authority. A fresh
current GET is not the ACK of an uncertain original command. A null recovery
receipt means unknown, not failure or permission to submit a replacement.

Migration 50 adds guarded immutable evidence/command storage. Recovery 48 and
session 49 remain intact; their own populated guards must be tested at their
owned schema levels rather than assuming an unrelated empty migration remains.

## Primary provider contract

Checked 25 October 2026 against the installed go-github/v89 and
`ghinstallation/v2` dependencies; no SDK dependency upgrade is needed.

- [Git references](https://docs.github.com/en/rest/git/refs#get-a-reference):
  exact refs, App installation Contents-read permission, commit object, 404 and
  409 statuses. References are mutable, so a current lookup is not historical
  cutoff proof (inference from this contract).
- [Push event](https://docs.github.com/en/webhooks/webhook-events-and-payloads#push):
  `after` belongs to one push/ref, with documented created/deleted/forced cases
  and delivery limits. A locally retained subset is not complete history
  (inference); no discarded flags are reconstructed.
- [Webhook troubleshooting](https://docs.github.com/en/webhooks/testing-and-troubleshooting-webhooks/troubleshooting-webhooks):
  delivery may be delayed or out of order. This module retains uncertainty and
  requires an explicit organiser decision rather than inventing a time guarantee.

## Verification ledger — not yet final acceptance

- Original native owner suite: 14 scenarios passed with real PostgreSQL/race,
  9.455 s, `/private/tmp/hackbox96-native-second.log`. An earlier incomplete run
  failed because the reserved migration existed while SchemaVersion still
  selected 49; the selector is now 50.
- Expanded original-cutoff/replacement and request-lifetime suite passed with
  real PostgreSQL/race, 19.355 s, `/private/tmp/hackbox96-native-third.log`.
- Held-provider regression was RED: another team's native read timed out while
  the provider held LockEvent, 7.639 s,
  `/private/tmp/hackbox96-provider-lock-red.log`. After moving network I/O outside
  broad event transactions, the unchanged assertion passed, 9.590 s,
  `/private/tmp/hackbox96-provider-lock-green.log`. The provider is not released
  until after the unrelated native read has completed.
- Canonical SQL/Go/TypeScript generation succeeds. Initial npm generation lacked
  worktree dependencies; locked `npm ci --ignore-scripts` restored them.
- Expanded signed-webhook, HTTP, provider transport and guarded migration runs
  are pending. Configured focused lint is not yet clean. No final-head CI,
  independent review, ready-for-review, merge or issue closure is claimed here.

## Observable draft checkpoint

Subsequent pre-reference-hardening bounded runs passed with race enabled:
`submissionfreeze` 57.448 s, deterministic App transport 1.877 s, authenticated
HTTP 9.710 s and actual registered API/worker graph 7.674 s, in
`/private/tmp/hackbox96-native-checkpoint.log`. These were earlier uncommitted
source checkpoints; they are not an attestation of every later draft edit.

The current draft adds explicit managed-reference correspondence: matching
native deliverable IDs, uncaptured additional repository references, current
reference applicability and an honest mismatch exception. Recognised GitHub
repository identity (including a recognised tree/commit URL) supplies no cutoff
SHA proof. An unrelated reference cannot grant an audience access to managed
code. Native correction plus a reasoned version-bound amendment is required to
repair a mismatch. File/URL-only current finals remain not applicable; no new
GitHub gate is inferred from historical code evidence.

Draft publication is for visibility. Latest reference/audience/assessment seam
proof, remaining lint cleanup, full generation parity, owned migration guard
compatibility, full final-head CI and independent reviews remain required before
Ready/merge/closure. Source compilation is checked separately; integration tests
are skipped without `HACKBOX_INTEGRATION=1` and are not claimed by that compile.

## Operator recovery

1. Inspect the native final outcome, source binding, current-reference status,
   cutoff and actual observation instants. Check the explicit current-binding
   flag; a historical receipt is not automatically the current native final.
2. Inspect current App health without exposing keys or raw provider bodies.
   Retry read-only capture under existing job recovery policy where appropriate.
   A later successful read remains later evidence and unresolved.
3. Page exact retained push evidence and inspect selected observation IDs. Record
   independent supporting evidence where available. Do not treat author dates,
   latest-100 staff status or missing webhooks as complete historical proof.
4. A current organiser reviews the exact source/head/native/final revisions and
   records an explicit reconciliation or separate amendment with reason and
   provenance. To repair a wrong native repository reference, first make the
   existing authorised native-final correction with its required reason, then
   explicitly amend onto that exact current final. No arbitrary repository swap,
   branch edit after cutoff or automatic rebind is available.
5. Preserve request ID and reviewed versions for uncertain mutations. Look up the
   actor-bound original receipt; a fresh current projection cannot acknowledge
   that original command. A null receipt is still unknown. Do not create a new
   request or automatic write replay because an acknowledgement was lost.
6. Keep original source, receipt and amendment history. Archived competition is
   read-only; do not bypass its policy or roll back populated migration 50.
