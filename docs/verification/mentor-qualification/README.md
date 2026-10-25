# Mentor qualification #110 — active implementation ledger

This is a full-scope draft, not an acceptance or release claim. It depends on the
actual #108 native owner and migration51; migration52 belongs only to this lane.
No provisional checkpoint is promoted into a final card. Native read/write,
original-receipt/history APIs, generated contracts and real runtime policy wiring
are implemented. Complete managed-evidence/correction/history, fresh-final consumer,
exclusion/concurrency/schema preservation proof and final validation remain pending.
Earlier checkpoint statements below retain their historical source attribution.

## Native source and owner

The new qualification owner consumes #104's canonical signed final huddle and
#101's immutable independent-card signoff through a metadata-only transaction
seam. It rechecks original intended moderator, required independent cards,
evidence/gaps, native final run and frozen submission revision; it exports no
independent scores, raw notes or private source prose. Moderated category values
are explicitly entered into a separate one-per-team/run card with evidence.
They are never summed or averaged from live checkpoints.

The draft owns explicit panel configuration, moderated draft/sign actions,
freeze-evidence eligibility decisions, total/category proposals, recorded
mentor votes and organiser casting decisions, panel signoff and the real
`rubric.FinalistPolicy` seam. Category proposals consider every eligible team,
including non-finalists; multiple category awards are permitted. A reserve or
category winner receives no finalist grant merely from that designation.
These are internal private proposals; award confirmation, finalist publication,
participant feedback and announcement remain separate owning workflows.

## Executed proof on actual ae24005 parent

- The native source-consumption test first failed compilation because the new
  public seam was absent. It then passed actual PostgreSQL/race **6.273s**, using
  the real native #104/#101 owners and actual migration51. This is a new-feature
  test, not a claimed production defect reproduction.
- First normal52 workflow run failed before its journey because SchemaVersion
  still targeted51. Updating the real runtime schema version to52 and using the
  actual51→52 migration chain corrected fixture setup; no placeholder51 exists.
- Native panel→unsigned-huddle denial→canonical independent signoff→explicit
  moderated draft→complete final-card signoff passed **5.882s**. The final has no
  managed-repository evidence and is classified `not_applicable`; this does not
  substitute for the still-required real managed/URL/file cases.
- A five-team real PostgreSQL/native-owner journey passed **10.988s**: the first
  team wins multiple categories, the fifth wins a category despite not reaching
  the fresh final, configured three finalists plus one reserve follow explicit
  final totals, and the actual finalist-policy seam denies reserve/non-finalist
  teams. Checkpoint values remain independent and unused.
- The new native packages and touched rubric/mentor owners compile. These
  bounded cases do not claim full native concurrency, privacy or HTTP coverage.

Fixtures contain only synthetic people, evidence and teams. They use test-owned
PostgreSQL containers, normal Goose/River migration setup, controlled native time,
GOMAXPROCS2/GOFLAGS-p1 and no user process/container teardown.

## Complete acceptance still owned here

Required independent reviews/evidence gaps must block final cards; all eligible
competing teams remain category candidates; only configured finalists reach the
fresh judged final; disbanded/inactive entries and unused repositories are not
scored. Equal-score records and affected cutoff/category ties require recorded
evidence review, mentor votes and organiser casting decisions before selection.
Missing/unresolved managed freeze evidence requires an explicit exact-source
eligibility decision, with not-submitted/withdrawn outcomes distinct. Same-final
managed SHA/decision/source changes must never silently reuse old moderated
cards/signoffs/selection. Native current-role/conflict/audience/lifecycle,
historical privacy, real #108 policy wiring, strict authenticated API/generated
contracts and actual populated51→52 preservation/guarded rollback remain part
of the deliverable. Final CI and independent full-scope reviews remain required.

The first compiling checkpoint is explicitly stacked on actual corrected #108
`096ab6a9b06dcb928acd6ae306b2d598b8951300` (itself based on merged mainb2f7).
Rebase preserves actual migration51 and does not replay a squashed parent.
Qualification/moderation/mentor/rubric/HTTP/application compile checks pass after
that rebase; earlier native timings remain attributed to the actual ae24005
parent on which they executed. Canonical sqlc models include migration52.

## Durable corrected-parent recovery and bounded integrity corrections

The recovered clean checkpoint `383edaef2ed9aa56aaf67c02427288d0e80b2606`
is stacked on actual corrected #108 `b57d0829ca8c13d0f23ab0573161f1966373e2f5`,
including its complete factual-summary migration51 and recipient-filtered evidence
projections. Bounded HTTP/application compilation passed0.371s/0.387s, with no
tests selected. Earlier native timings above retain their original attribution.

Actual new real PostgreSQL/race regressions on the owned extension reproduced:

- Direct qualification-source TRUNCATE was accepted (RED2.585s). Statement
  UPDATE/DELETE/TRUNCATE guards then rejected both direct sources truncation and
  rounds truncation with CASCADE (GREEN3.225s). These use actual51→52 migrations.
- A named qualification-panel mentor without native observation access received
  a private observation UUID and its attached note (RED3.116s; native notes read
  was independently403). Per-recipient copied evidence projections removed both,
  retained the separately authored panel-facing note and preserved the owner's
  full authored references (GREEN4.483s). This does not claim full mixed-document
  history/eligibility/vote privacy coverage yet.
- A mismatched captured freeze identity was accepted for a new moderated card
  (RED6.123s). Card save/sign and eligibility decisions now require the exact
  captured opaque freeze binding, separately from native decision UUID/head
  checks. The bounded owned native suite passed20.918s before the subsequent
  behavior-preserving lint refactor; final post-refactor proof remains separate.

All owned test PostgreSQL containers terminated normally. Logs are task-owned
`/private/tmp/hackbox110-{panel-privacy-red,panel-privacy-green,binding-red,owner-green}.log`.
These are synthetic native owners, not providers or a completed HTTP deliverable.

The actual published383 CI lint job121112516827 identified formatting, error
wrapping and two oversized methods. Scoped native owner refactoring and contextual
error wrapping corrected them; configured local golangci-lint2.14.0 reports0issues
for qualification/moderation/mentor/rubric packages. Full API/runtime/history,
tied-cutoff/category votes, managed-amendment, concurrency and migration preservation
proof remain required; no acceptance scope has been narrowed.

The clean immutable `781619483e82bdb5000e6e98042a08e0d77a1043` owned suite
passed16.838s with actual PostgreSQL/-race after the lint refactor.

## Actual tied-selection performance RED

Production7816194 plus the exact new tied-cutoff/category regression and shared
native fixture helper (retained in the next test commit after execution) reached
the fixed five-minute timeout: FAIL300.329s, handle79770. It uses five real native
independent/huddle/moderated final cards, four selected total-tie seats and five
category ties, including missing/duplicate ballot and casting-authority checks.
The source was unchanged during execution. No timeout/assertion was increased.
Actual Go stacks identify `cohort → ballots → resolved → resolveTotals → proposal
→ page → Change` while recording an organiser casting resolution. A bounded1s
process sample and timeout stacks remain at `/private/tmp/hackbox110-ties-actual.sample.txt`
and `/private/tmp/hackbox110-ties.log`. Testcontainers' reaper removed owned
PostgreSQL3583405e62cd; no labelled testcontainers remained at the post-run check.
This is a native validation-performance failure, not successful tie acceptance.

On the patch-equivalent rebase onto merged #108 main
`b2b09b6c35fa6cdc8ae6e9fa30acfe3ca5fbd063`, the unchanged tie regression passed
with the original5m timeout: **PostgreSQL/-race127.530s**, handle83441. Only
redundant full-cohort reads inside the same event-locked transaction were removed;
ballot/decision actor authority, native evidence authorization, source identity,
required ballots, duplicate rejection and casting checks remain. No cross-command,
cross-reader or cross-transaction cache exists. This is a bounded full five-team
all-ties workflow proof, not an event-scale performance certification. The actual
300.329s RED and sample retain their original source attribution.

## Exact original qualification commands

Before exposing the new API, migration52 now retains an immutable metadata-only
request ledger. A client-authored request UUID and reviewed event/round revisions
bind a native command fingerprint; exact repeated requests return their original
ACK metadata with a separately current private page. A fresh read carries no ACK.
Recovery returns only request/run/action/expected revisions/result version/hash,
under current event/panel/actor authority, never authored reasons/cards/code.

The real PostgreSQL/-race request suite passed **11.460s** on the owned extension
of main-based25deb7c before the subsequent command-dispatch refactor. It proves
immutable original lookup across a later round, same UUID/different intent denial,
missing original remaining unknown, actor/role-loss privacy, duplicate concurrent
requests producing one retained effect, and distinct competing heads producing
one success/one conflict. Actual native schema52 is used; no synthetic receipt
projection, HTTP success claim or provider effect is inferred. Log:
`/private/tmp/hackbox110-receipts.log`. Canonical sqlc regeneration includes the
real complete migration51 and additive52 command table. Public API/runtime/history
and current-source broad proof remain pending.

## Native API/runtime and current audience checkpoint

The owned extension of `2d664a0b` on mainb986 adds strict versioned native
read/write/original-receipt/history APIs and generated Go/TypeScript contracts.
Runtime composition passes the same actual qualification owner into both
`Rubrics.Finalists` and `Judging.Finalists`; it shares actual moderation,
scorecard, freeze and pool/clock owners, with no placeholder selection policy.

A genuine separate-panel current-conflict regression failed14.806s: after an
actual `mentors.Change` reporting conflict, the panel-only mentor retained other
private cards/cohort facts and a stored proposal. The same regression passed
16.185s after checking the reader's native scope across the eligible cohort
before emitting global private state. Safe metadata/public criteria remain.
History applies this current scope and its original audience separately.

The initial native history audience/exclusive-version test passed5.946s.
Historical snapshots are explicitly distinct from current selection and original
request receipts. Per-recipient history evidence copying and historical source
withdrawal/amendment need the expanded current-source proof still listed below.

The actual authenticated native runtime test reached strict OpenAPI/CSRF,
publication, real preparation→live transition, cutoff/not-submitted settlement,
reviewed panel write, both independent portal reads and immutable original
receipt recovery. Two fixture expectations were corrected: missing required CSRF
header is400 at schema validation, and an unpublished foreign event is403 before
run lookup. An asserted edit/readback corrected a missed formatted-line change;
those expectation failures are not product REDs.

A separate authorised-event unknown-run probe then reproduced a new endpoint
500 (RED6.741s). Native missing-row normalization corrected it to404; the unchanged
full HTTP/runtime assertions passed **PostgreSQL/-race8.413s**. They also retain
role-loss denial, private no-store responses, missing original/fresh-read
separation and exactly one persisted command despite denied probes. No usable
complete moderated-card HTTP/cohort or live-provider claim is inferred from this
bounded metadata journey. Logs: `/private/tmp/hackbox110-http-notfound-{red,green}.log`.

Both touched API/runtime packages compile; canonical Go/TypeScript generation,
TypeScript no-emit checking and configured lint across all six touched owner/API/
runtime packages pass (0issues). Shared complete web dependencies are reused
read-only with matching lockfile; no global dependency install. The complete
managed amendment/history/native fresh-final integration, excluded outcomes,
required-review/gap/concurrency and actual populated51→52/rollback cases remain
owned here before final source-wide verification, CI and independent reviews.

## Required native consumer projections and results source

The #111 consumer preflight identified genuine read gaps: panel mentors could
not use an organiser-only rubric catalogue as run discovery, and action authority
must not be inferred from roles or an old signed projection. The additive native
collection now discovers actual current published mentor-final runs under the
actor's organiser/intended-panel authority. Exact readers remain authoritative.
Native per-page/per-team/tie readiness mirrors existing command checks, including
record lock, current canonical moderator/source, captured card version, eligibility
review, complete required ballots and organiser casting choices. No new write
policy or schema52 change is introduced. Source IDs never grant source access.

`SelectionInTransaction(ctx,tx,eventID)` supplies #112 a revalidated exact current
private signoff's run/round version, rubric reference, signed binding, signer/time,
finalist/reserve/category IDs only. It returns no mentor total/rank/prose and grants
no publication. The results owner must enforce its current actor/recipient/command
authority in that same transaction. The existing real finalist policy now delegates
to that same source, retaining its selected-team membership check.

Working-tree native proof on the owned extension of2abe: discovery/current panel/
read-only3.609s; canonical moderator/draft/signed-record and exact five-team metadata
handoff14.667s; affected total-tie required ballots/duplicate prevention/casting
readiness22.461s; strict authenticated runtime discovery/capabilities5.501s, all
PostgreSQL/-race. A handoff extraction compile-only failure was corrected before
execution; it is not a product RED. Current generated TypeScript/no-emit and six-
package configured lint pass0issues. The first lint attempt encountered another
lane's process lock; the successful run uses a private task cache, leaving shared
caches untouched. Existing reader scope was extracted unchanged for lint.

Full remaining source/managed amendment/history/fresh-judge/schema acceptance
proof and final integrated verification/reviews/CI remain required.

## Final-evidence acceptance expansion and retained-run recovery

This checkpoint adds real native PostgreSQL acceptance coverage; it does not add
another schema/API/command field or change finalist/award policy. Actual protected
file upload and formal URL submission both yield `not_applicable`, while missing,
withdrawn, inactive-entry and disbanded teams remain unscored and distinct from
unresolved managed code. The disbanded unused repository schedules no operation.
The bounded URL/file outcome run passes **20.264s/-race**. Fixture-only corrections
used the actual `pending` repository state and settled formal output before native
independent assessment; failed fixture attempts are not product REDs.

The managed-repository journey uses the real capture handler with no configured
provider, explicit unresolved eligibility/ineligibility decisions, native freeze
reconciliation, explicit new independent reviews/native run and new moderated
values. It reproduced an actual recovery defect: superseding the run made its
retained history and original ACK unreadable (**RED4.275s**). A separate historical
read mode retains current event/actor/intended-panel/native publication/evidence
checks, but cannot authorize current writes, current selection or scoring. The
same journey passes **GREEN5.168s**; its expanded authority suite passes **10.999s**,
including completed same-native-final SHA amendment, original historical values,
exact original receipt, superseded current-read/write denial and role-loss denial.
Logs: `/private/tmp/hackbox110-managed-history-{red,green}.log` and
`/private/tmp/hackbox110-history-authority-green.log`.

Native pending second-review, unresolved formal gap, done support action without
formal resolution, and explicit partial/pending final values remain unable to
produce signed final cards. The actual native case passed in the first combined
run (whose overall result failed the separately documented outcome fixture).
It remains included in final integrated owner verification below.

Actual #107/#108 fresh-judge consumers with this real finalist policy pass
**11.441s/-race**: selected teams get empty fresh judge cards; reserve/category-only
teams are denied, with no mentor score/rank payload. Populated actual schema51
(all public native tables, including retained factual summaries) survives52,
empty52 rollback and reapply byte-for-byte (**4.095s/-race**). Populated52 rollback,
UPDATE/DELETE/TRUNCATE rejection and exact original ACK preservation pass
**4.822s/-race**. A fixture initially used an organiser as factual-summary author;
the corrected proof uses its actual assigned mentor, preserving native policy.

The actual authenticated generated-router positive-card journey passes
**7.645s/-race**: formal URL final, independent native reviews and huddle signoff,
reviewed configure/save/sign commands, exact original ACK, separate current GET,
other-portal intended-panel read and retained history. One raw-domain test body
serialized optional evidence as null and was correctly rejected by strict schema;
the proof now uses the real generated request DTO, whose absent array stays
omitted. No contract relaxation was made. The test-only before-cutoff callback
lets #111 reuse this actual app/publication/runtime without rewinding time.
Configured lint across qualification/rubric/judging/app passes **0 issues**.

Final integrated source-wide owner/API/runtime proof, generation/type parity,
current-head CI and independent full Spec/Standards reviews remain pending.
