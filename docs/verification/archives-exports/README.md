# Issue #119: archives, exports and cross-event reporting

Draft PR #268 is based on main `18b23f4e3201f9ab0d10ef3875628585f0ffd74f`.
The full issue remains in progress; this document distinguishes implemented source
from executed proof and outstanding acceptance checks.

## Implemented source

- Current-authority archived-event directory/detail; current organisers receive
  independent per-event cohort summaries and cursor-based cross-event comparison.
  Platform administration alone grants no private metrics/export access.
- Registration records, registered people, confirmed competitors, recorded
  attendance, entry/frozen-submission/unused/withdrawn/not-submitted records remain
  distinct. Recorded attendance has no unrelated registration denominator; zero
  recorded presence never establishes zero physical attendance. Physical census
  coverage and missing final judging/results are explicitly unavailable.
- Missing native formal outcomes are explicitly incomplete, never inferred open
  or not submitted. The real preparation → cancelled → archived path preserves
  the exact event/fact/outcome snapshot through later maintenance and reads.
  Existing assessment settlement and immutable submission ownership are retained.
- Audited no-store, formula-neutralised summary CSV includes source availability.
  Bounded JSON source exports compose logistics roster fields, explicit-audience
  formal submission evidence, own currently authorised assessment cards,
  separately published projects, published suppressed feedback and restricted
  objective/evidence observations inside the archive event transaction.
- Source owners retain current actor/resource/field/retention/publication policy:
  archive access does not confer logistics fields, another author's assessment,
  raw survey responses, unpublished outcomes or media byte access.
- Reporting retains exact objective description/version and measure
  baseline/target/unit/method/source/horizon, original evidence basis/date/source,
  nullable value/denominator and explicitly current follow-up status. Continued
  projects count native in-progress records; dated adoption observations count
  source-labelled evidence records, never unique adopted systems, ROI or awards.
- One native privacy retention helper evaluates the locked event/current policy
  using existing calendar-day deadlines. Native logistics, operational export,
  own/authorised profile reads and archive composition enforce expiry before
  deletion maintenance. No duration, role or archive competition-write exception
  was introduced; safe audit/history metadata is preserved.
- Canonical SQL, OpenAPI and generated Go/TypeScript contracts, HTTP adapters and
  runtime dependencies are maintained. No migration or real provider effect.

## Executed bounded proof

All PostgreSQL checks below used isolated native PostgreSQL containers with
`HACKBOX_INTEGRATION=1`, `GOMAXPROCS=2`, `GOFLAGS=-p=1`.

- Real reachable archive boundary RED: late maintenance created two
  `not_submitted` states/changes and altered gate facts after archival. GREEN:
  submissions 2.107s and archives 4.441s. Reads/history leave missing source absent.
- Native-source role/field/pagination/loss and safe-audit proof: archive package
  5.974s, including exclusion of photographer consent from logistics fields.
- Exact objective/evidence provenance: 2.357s. A later objective revision cannot
  retarget v1 description/unit/baseline/target; explicit zero differs from nil;
  self-report/estimate differ from dated sourced adoption evidence. Dedicated
  native archived follow-up leaves competition revision/write flags unchanged.
- Retention RED: expired native logistics fields remained readable. GREEN:
  3.092s, before/at the exact deadline for current 90-day and legitimately shortened
  one-day policies; expired rows physically remain, and no worker was invoked.
  Native logistics/export/profile reads deny, archive reports retention expiry.
- Real HTTP/OpenAPI response proof: 4.020s. Authentication/current role loss,
  metadata restriction, source scopes, no-store, download headers, source
  availability and hostile-cell CSV neutralisation are checked.
- Changed-owner native Go lint completed with `0 issues` before the latest
  retention source delta; a final lint/generation check remains required.

Historical local logs are under `/private/tmp/hackbox119-*`; they are development
receipts, not published independent-review or final-head CI claims.

## Remaining acceptance work

1. Finish positive archive source composition proof for formal audience/assessment
   authorship, released project event/cursor/media/privacy behaviour and actual
   frozen suppressed feedback; preserve existing owner tests rather than adding
   another score/result definition.
2. Complete the running capped PostgreSQL/race campaign, including held-event
   lock role loss, native lifecycle cancellation, worker execution eligibility
   and explicitly retained #117 reminder history. Record only terminal results.
3. Check exact generated parity, changed-owner lint and compile after final
   source changes, reconcile the PR description, then freeze for independent
   Standards/Spec review and every exact-head required CI check.

Native final judging (#108/#109) and published results (#113) do not exist yet.
Their absence is represented as unavailable; this issue cannot invent those
owners, a rank/award/result, transferred disbanded work or code-based performance.

### Terminal native race campaign

The selected native PostgreSQL/race campaign completed successfully in all ten
packages: archives 14.986s; privacy 4.642s; attendance 2.893s; submissions 2.952s;
scorecards 6.987s; outcomes 7.477s; surveys 3.087s; event 3.020s; jobs 2.919s;
HTTP adapters 4.792s. It includes actual event-lock blocking before role-loss
commit, native lifecycle/archive cancellation, current permission checks, exact
frozen scorecard evidence, published-project positive projection/withdrawal,
frozen suppressed aggregate correction, current queued-operation eligibility and
explicit archived follow-up delivery with retained history. Only deterministic
local/in-app test providers were used. Full command/regex is retained in the local
`/private/tmp/hackbox119-native-acceptance-race.log` campaign invocation; this is
selected acceptance proof, not a claim that every repository Go test ran.
