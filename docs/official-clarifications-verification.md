# Official clarification verification

This #161 implementation supplies the native private-question and common answer
contract. Merge remains subject to exact-head CI and review.
The complete question and staff workspace remains #162.

Questions require current confirmed admission and published team membership;
explicit current organisers can triage them. Current authority and actual common
challenge/rubric publication are rechecked inside the event transaction. Archive
keeps authorised historical reads and freezes writes. Private attachment bytes
additionally require exact question/file/team association; staff byte access
opens only at live start. Verified storage reads do not widen generic file policy.

Answer authoring, exact-version approval and publication are distinct immutable
records. Corrections retain prior published versions. Public queries and DTOs
exclude the private question ID, team, body and attachments. Material-change kinds
require real owning released versions; rubric changes also require the owning
impact review and reviewed notice. The consuming source resolver and the common
rubric reader share exact publication/document/digest validation.

Publication and fixed-label native inbox references commit together. A notification
contains answer ID/version only and rechecks current confirmed status on list,
open and read. The canonical read-only route is
`/events/<event-id>/clarifications/<answer-id>`. Its current and historical bodies
use the same safe Markdown renderer as released resources. The account-scoped
cache is ephemeral, clears on navigation or denial, and never persists to browser
storage. Older corrections and native private histories have bounded keyset pages.

## Evidence

- Real PostgreSQL/race tests exercise actual T−7 content release, private team and
  staff isolation, request replay/conflict, withdrawal, staff revocation, archive,
  bounded private/history continuation and common-source validation.
- The actual shared production runtime exercises native publication and inbox
  list/open, correction history and concurrent duplicate publication. A fault
  after actual recipient inserts rolls back publication and every notification;
  retries create one retained notification per revision/current recipient.
- Actual owning rubric/challenge changes require their native released versions,
  reviewed impact and attendee notice. A reproduced retained-evidence regression
  is fixed: a later challenge-only release cannot erase the exact earlier rubric
  publication's reviewed notice. Foreign event/challenge/rubric, version, digest,
  notice and unreleased-review evidence all fail closed. Three PostgreSQL/race
  runs also exercise publication against current-source and native organiser-role
  changes; either serial order leaves complete publication/inbox state or none.
- Publication also races actual registration withdrawal, including frozen-team
  removal. Both serial orders retain one publication and a valid current recipient
  set; the departed attendee cannot read the answer or retained inbox references.
- HTTP tests validate responses against the generated OpenAPI contract and cover
  authentication, CSRF, malformed input, no-store, private history/triage/drafts,
  distinct approve/publish, common search and generic log privacy. Positive team
  and post-live staff attachment reads return exact bytes; other teams, pre-live
  staff and revoked readers receive no bytes or private metadata.
- A positive attachment regression first reproduced two HTTP500 responses from
  wrapping a nil storage error. Success now returns nil after verification;
  guarded error wrapping remains intact.
- Thirty combined guidance/help/inbox/recovery/canonical-reader browser cases
  pass on desktop/mobile in each independently built public and admin surface.
  They cover actual route
  navigation, author/time/source labels, superseded/corrected history continuation,
  denied copied links, sanitisation, active-authority clearing and no persistence.
- Both web builds, web lint, scoped Go lint and Go formatting pass.
- The actual normal schema33→34 upgrade preserves a checkpoint and mentor card
  created by the native scorecard services before upgrading, their exact native
  reader projection, common rubric/run, private help and observation history,
  inbox, event, authority and audit records. Empty rollback preserves that state;
  populated rollback refuses to discard a private question or its source history.
- Final actual-main focused PostgreSQL/race checks pass for the native question
  service, actual runtime/material/current-role/withdrawal cases and strict HTTP
  routes. The full HTTP PostgreSQL/race suite also passed before the final
  scorecard-only ancestry rebase.

## Remaining release gates

The concrete migration34 is based on actual merged scorecard schema33. Recovery
session/draft-scope cache behavior, help/runtime/inbox owner maps and scorecard
contracts are preserved. Exact-head CI and review remain merge gates.
Source adapters for #82 must derive metadata and current audience from
actual immutable publications with source-ID/keyset filtering before bounds;
orchestration is not implemented here.
