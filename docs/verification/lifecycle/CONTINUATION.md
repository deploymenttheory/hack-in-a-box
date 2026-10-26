# Complete lifecycle continuation after PR #291

PR #291 merged `e485545c` as `6764e89a`. Its source and published receipts
explicitly retain `complete_lifecycle=false`; merging did not run the remaining
scenario or satisfy the full acceptance contract. Issue #120 was automatically
closed, with all twelve acceptance checkboxes still unchecked when audited.
This continuation preserves that full contract and the L01–L18 matrix in
[README.md](README.md).

## Source and evidence boundaries

The continuation starts on main `6764e89a` and carries only four unpublished
post-`e485545c` changes: chronological finalist/award publication, its causal
judge-start regression, the completion-owner correction and explicit keyed HTTP
test intents. The recovered checkout's final tree is byte-identical to original
`9d3fa421`; the original refs remain available. No changed public endpoint or
schema is introduced.

| Evidence | Actual result and limit |
|---|---|
| Frozen `dbe2304a`, runner7896 | Both connected native modes passed383.22s through separate award confirmation. Twenty-seven result versions/commands, two advisory versions/commands, two judge approvals and zero publications. This is the merged checkpoint, not full lifecycle acceptance. |
| Frozen `a98924ed`, runner12337 | Failed316.71s. Explicit private finalist preparation, actual finalist publication, original acknowledgement recovery, duplicate rejection and authorised recipient reads passed. The following existing guard found independent judging had already started. No complete release-phase pass is claimed. |
| Frozen `f5194948` native regression | Failed4.04s/package4.360s: both modes had no started run or judge cards before organiser results preview; preview alone set `started_at` while cards remained zero. |
| Frozen `8a94dd7f` correction | Native regression passed4.28s/package4.781s. Preview and qualified preparation/publication remain non-starting; the intended judge's actual brief still starts assessment. Five affected native owner parents passed22.207s without skips/failures. Independent Standards and Spec reviews each reported zero findings for the exact bounded correction. This component fixture is not the full HTTP scenario. |
| Frozen `9d3fa421` HTTP prerequisite | Two real loopback HTTP controls passed0.447s, configured lint zero. A controlled old keyed transport caused two accepted effects after connection loss; the corrected explicit-intent helper preserves one effect and caller uncertainty. This is not a live help flow or physical Wi-Fi test. |

Historical source/image/schema and raw-artifact hashes remain attached to each
published receipt. Later rebases or fixes do not promote earlier executions to a
new head. Full raw runtime exports, private assessment/advisory prose, provider
configuration and authentication state stay local. Public receipts use explicitly
validated synthetic IDs, hashes, enums, counts and original acknowledgement
metadata. All completed owned Compose and native test resources were removed.

## Actual issue #120 acceptance audit

Every row remains open for final integrated acceptance; the middle column is
supporting checkpoint evidence.

| Original acceptance | Supporting evidence | Remaining native action/assertion |
|---|---|---|
| Both judging templates; two differently branded concurrent events with overlapping people | Both native templates, separate brands/objectives/team IDs and overlapping normal OIDC people reach confirmation. | Retain both event contexts through release, feedback, archive and outcomes on final merged owners; prove current event/resource isolation throughout. |
| Withdrawal, failed message, private resource, Wi-Fi interruption and unresolved/reconciled freeze | Real withdrawals, queued SMTP fault/recovery, private drafts/cards, controlled reply loss and explicit frozen-work reconciliation. | Actual browser offline/reconnect recovery with original intent and no accepted-write replay; complete current-source/audience fault variants. Controlled reply loss alone is not Wi-Fi acceptance. |
| Each role sees only authorised content; intended evidence in final archive | Many actual participant/mentor/judge/organiser denial and private-author checks. | Complete role/file/export/audience matrix; archived evidence, consent and expiry boundaries; do not publish raw private assessments. |
| Synthetic fixtures without production provider accounts | Existing private deterministic OIDC/GitHub/SMTP fixtures and native PostgreSQL/worker composition. | Reproducible final clean Compose/browser run; final image/config/source ledger and cleanup. No production provider success or real Git history claim. |
| Explicit minimum-confirmed go, surviving waitlist offers, pre-publication freeze and target-five allocation | Real36-person admission,29/30 go boundary,35 capacity, FIFO acceptance and four-to-six published teams. | Prove an outstanding offer survives to the actual admission cutoff, cutoff rejection/no backfill and final invariant census. Staff never count toward the configurable competitor floor. |
| Under-three pre-freeze reassignment; smaller/withdrawn teams after freeze; no moves/backfill; unused disbanded repos | Real pre-freeze depletion, unlock/reassignment/republication and freeze move denial. | After actual repository provision/publication, keep a smaller team's identity without reassignment; fully withdraw another; review any resulting access changes through native owners. Assert its existing repo is unused/unscored and post-freeze moves/post-cutoff replacement fail. |
| T−7 local-calendar publication, early work, live-only formal/review, missed release and reasoned exceptions | Real worker DST/T−7 publication, private preparation work/drafts and early formal/review denial. | Actual missed/+60-second boundary, source/current audience gates, reasoned post-publication postponement and below-minimum attendance exception. Use event-specific dates and a single monotonic domain clock; do not rewind time or stretch windows. |
| Former-organiser/privileged competitor rejection; cutoff freeze before stage advancement; assessment with an explicit outcome for every team | Native cutoff sources/retry/reconciliation while events remain live, including frozen/not-submitted/withdrawn outcomes. | Actual privilege history survives grant revocation; enforce same-event disqualification and correct cross-event scope. Reach native assessment only after complete current-team outcome census. |
| Objectives, common rubric, exact acknowledgements, concurrent help, reviewed clarification and correction notices | Native objective/common rubric/acknowledgement and live mentor checkpoints. | Race two authorised helpers against one current revision: one claim wins. Rehearse private support, reviewed common clarification and corrected-answer current-audience notifications through actual queued native effects. |
| Practice versus real scoring, non-winning showcase and owned30/60/90 reviews with estimated/evidenced outcomes | Isolated actual practice, new mentor/final judge cards; native retained-review components. | Reviewed consented non-winner publication/search/filter/page/withdrawal; actual owned review plans and queued30/60/90 effects. Distinguish estimates, self-reports and evidenced adoption. |
| Archive retains only explicitly approved follow-up; audited updates leave competition immutable; M5 excluded | Exclusive default90-day expiry and explicit native120-day synthetic retention components. | Native completion/archive; approved retained follow-up only; audit/export/current owner access; compare frozen submission, rubric, scoring, decisions and publication hashes before/after authorised outcome updates. Default90 stays unchanged. |
| Completed Fleet screens/guide and full action-to-next-action journeys | API lifecycle and separate historical UI/normal-image proofs. | Final merged Fleet routes in both actual portals, real backend actions, keyboard/narrow recovery, independent published guide and public-app outage/admin availability. Prototype/mocked-browser checks do not satisfy this row. |

The applicable product rules also remain in scope: configurable native feedback
surveys and approved scheduled follow-up; ideas from every team rather than only
winners; follow-up owner/status/optional backlog URL; participant media consent
and organiser review; anonymised archive learning without expired personal data.

## Next connected implementation sequence

1. Rebuild the final carried source and rerun both modes through chronological
   private finalist preparation/release, fresh judging, confirmation and separate
   award publication. Keep each exact original acknowledgement and current safe
   recipient projection; reject duplicate source publication without mutation.
2. Author own-team feedback explicitly: private draft, optional current confirmed
   own-team scores, deliberate release and amendment. A newer private draft must
   not leak or replace the last released report. Other teams and former members
   remain denied; an unscored reserve gets prose without fabricated zero scores.
3. Add live help contention, private-to-common reviewed clarification, correction
   delivery and real browser disconnection recovery. Help uses its required
   caller-owned `Idempotency-Key`; claims are not automatically replayed.
4. Integrate admission cutoff/privilege history/post-freeze and publication
   exception variants into coherent event chronology. Keep historical approved
   access receipts distinct from later reviewed removal/coverage plans. Capture
   actual current-source failures before choosing any production change.
5. Exercise consented media review and configurable native feedback surveys with
   actual approved queued notifications, then reviewed non-winning project
   publication and organisational discovery/revocation.
6. Complete/archive through native transitions; retain explicit owned follow-up
   plans, perform30/60/90 reviews and audited outcomes/export checks while proving
   competition evidence remains immutable. Show the negative default90 boundary
   separately from the reasoned native120-day synthetic policy.
7. Run final Fleet keyboard/narrow/full-route journeys, independent guide and
   normal production-image OIDC/TLS/calendar/restart/restore/admin-isolation
   proofs. Publish the final source/image/schema/action/assertion ledger and
   finish independent full Standards/Spec plus final-head CI.

The scenario continues to use normal authenticated HTTP/native owners and real
queued workers/PostgreSQL. SQL is a read-only evidence oracle, not a way to seed
registration, admission, freeze, challenge, rubric, scores or successful effects.
The verification entrypoint remains distinct from the production image. Neither
the continuation Draft nor a green CI run alone completes these acceptance rows.
