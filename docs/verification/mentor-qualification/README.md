# Mentor qualification #110 — active implementation ledger

This is a full-scope draft, not an acceptance or release claim. It depends on the
actual #108 native owner and migration51; migration52 belongs only to this lane.
No provisional checkpoint is promoted into a final card. API/generated-client,
full cohort/tie/privacy/managed-evidence, migration parity and runtime proof are
still pending at this first observable checkpoint.

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
