# Repository readiness administration: native ownership and verification

Issue #94. Initial implementation checkpoint, based on main
`bc350d8c3145e8cb1787a840568a8f7b42cf349f`. This draft is incomplete and does not
claim acceptance, provider setup or the separately open live sandbox work #91.

## Complete native surface inventory

| Surface | Owning contract and constraints | UI checkpoint / remaining work |
| --- | --- | --- |
| Repository collection | GET `/events/{event_id}/repositories`, generated `TeamRepository[]`; native `internal/repositories/service.go List` returns staff array or only current active member's published team after challenge publication. No pagination, search or total contract. | Actual Fleet collection in both portals; no inferred URLs/ownership. Current mapping count only. |
| Selected repository | Mapping identifies event/team/organisation/name/numeric provider ID; `unused` derives disbanded state. | Actual responsive Inspector; selected row uses current collection; own fresh read gate on every mount. Return to initiating control. |
| App organisation/template | GET and POST `/github-app`, generated `GitHubAppStatus`/`GitHubAppCommand`; StaffRead metadata, EventEdit revision-guarded configure/rotate/disconnect. Only private local editor may contain private_key/webhook_secret. No safe GET exposes them. | Remaining: current diagnostics, configuration forms, reviewed intent, secret erasure and unknown barrier. |
| Installation | POST `/github-app/authorize`; existing native URL uses session/event/organiser-bound single-use state. Configuration/linking is separate from participant OAuth. | Remaining: explicit installation review and safe native redirect. No real installation during tests. |
| Provisioning | POST `/repositories` action queue has NO team_id; queues all actual published teams idempotently, retains successes and provenance. Omitted team IDs cannot become selected retries. | Remaining: explicit event-wide initial queue review showing exact visible published teams; no fake selected retry. |
| Unknown repository create | POST `/repositories` action reconcile REQUIRES selected team_id; `Recover` is lookup-only under current organiser/revision authority, missing result never grants another create. | Remaining: selected uncertain/failed repository recovery, original unknown command retained separately from current mapping reads. |
| Access planning | GET `/repositories/{team_id}/access` returns plan/grants/execution/can_manage/grants_open/flags/observed_at. StaffRead only; numeric ready repository required. | First current readonly inspector projection; remaining complete exact action/member/flags/diagnostic composition. |
| Access change | POST access `review`, `approve`, `refresh`; EventEdit/CSRF/IfMatch. review uses original plan_id+reason; repeat returns exact retained review receipt. approve binds current plan ID/version/fingerprint+reason; worker rechecks current identity/membership. `RepositoryAccessResult.receipt` is distinct from current. | Remaining: deliberate prepared/reviewed exact operations, original receipt checks/unknown barrier/exact retry only proven safe; no GET-as-receipt. |
| Own invitations | GET `/repositories/{team_id}/invitations`, generated OwnRepositoryInvitations, current person's verified GitHub ID/link revision and private grants. | Remaining: participant own invitation/readiness inspector; never staff people/grants cache. |
| Session lifetime | Existing authenticated CommunicationSafety root stores whitelisted metadata only. PrivateRequestScope fences late callbacks. | Remaining: original pending/unknown metadata above selected detail/tab/current-authority lifetimes; no raw reason/people/key/webhook/config command in shared state or storage. |

## Domain constraints

ADR-0005 disbanded repositories remain unused/unscored. Repository controls never
change team assignment, reopen freeze, execute participant code or delete/transfer
repositories. Provider creation/acceptance, queued work, pending invitations,
accepted grants, ambiguous effects and original command acknowledgement remain
separate. Operational activity never accepts a submission or changes a score.

Authority is always native current event/resource policy. Frontend event flags
only control discoverability; current reads and guarded native writes decide
access. Every private owner uses fresh per-opening current GET, cancellation and
late-response fences. Current denial removes private detail before restoration;
restoring a parent alone cannot reuse an old access projection. No report/export
capability is invented. No private keys or installation tokens are previewed,
reported, cached globally or persisted in the browser.

## Remaining before completion

All command/configuration/installation/own-invitation work above; actual independent
production guide examples, copyable usage/accessibility/unknown-recovery guidance;
meaningful both-portal desktop/mobile keyboard, freeze/disbanded, exact original
receipt/unknown, partial provisioning, identity/permission/provider distinction,
current denial/partial restoration/held reads and late writes. Capture matched
synthetic before/after evidence. Run lint, units, both production builds, focused
native integrations and complete final CI, followed by independent frozen review.
No real provider effects or real credentials are used in verification.

Native PostgreSQL/race coverage already exists in
`internal/repositories/service_test.go TestDurableProvisioningPostgres`,
`internal/repositoryaccess/service_test.go TestReviewedRepositoryAccessRealPostgres`
and `internal/httpapi/repositoryaccess_test.go` current-authority/privacy/exact-review
journeys. Those are contracts to retain, not unexecuted proof for this draft.

## Executed first checkpoint proof

- Lint PASS: `/private/tmp/hackbox94-first-lint.log`.
- Both production portal builds PASS: `/private/tmp/hackbox94-first-build.log`.
  Initial shared older node_modules lacked merged QR dependencies; isolated locked
  `npm ci` resolved that dependency mismatch without source/package/lock changes.
- Actual synthetic native collection→inspector→keyboard return: desktop/mobile
  **2/2 admin + 2/2 regular PASS**, logs `/private/tmp/hackbox94-first-{admin,public}.log`.
  Exact team/numeric repository, staff gaps, disbanded state, no regular-portal
  staff access read, no domain/provider writes and no horizontal overflow checked.

This is first-slice proof only. No command/guide/full-acceptance proof is claimed.

## Expanded source checkpoint — not completed UI verification

The draft now contains App setup/rotation/disconnect/installation review,
event-wide published-team queue and selected-team lookup-only reconciliation,
exact access review/approval and distinct provider/grant status, own invitations,
private drafts and safe actor/session/resource metadata above private owners.
Regular UI projects the current own team and native own-invitation authority;
organiser flags do not expose administration. Pure production review controls
are imported by the synthetic independent guide. Expanded UI/guide source still
needs fresh compile/lint/unit/browser proof; first-slice proof does not cover it.

A bounded authorised native addition closes the original access receipt gap:
GET `/repositories/{team_id}/access/receipts/{plan_id}?action=review|approve`.
It requires current organiser StaffRead and original creator/approver, binds the
immutable numeric repository and returns the requested historical definition or
approval. It never calls GitHub or writes audit/jobs/revisions. A missing receipt
never proves no effect. Original write bodies and IfMatch are unchanged. Updating
IfMatch could derive a different original review, so it is never an exact retry.
No migration. App/provisioning unknown outcomes retain safe metadata and are not
blindly resent; their current state reads are not original receipts.

Command body identity is SHA-256 of a versioned UTF-8 byte-length-framed tuple:
action, original actor/event/team, numeric repository, plan ID, native review
command version zero or approval plan version, and Unicode-trimmed reason.
Shared synthetic golden vectors cover Unicode, literal markup and delimiters.
Optional `intentFingerprint` metadata accepts only 64 lowercase hex characters.
No raw reason, people, keys or command body enters shared memory/storage. Browser
capture occurs before POST and exact receipt recovery recomputes the body digest.

Executed native proof: `HACKBOX_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=1 go test
-race -p 1 -parallel 1 ./internal/repositoryaccess ./internal/httpapi -run
'TestOriginalRepositoryReceiptCurrentAuthorityAndNoEffectsPostgres|TestRepositoryCommandFingerprintBrowserParity|TestRepositoryAccessHTTPPrivacyCurrentAuthorityAndExactReview'
-count=1` PASS. Real PostgreSQL service/history/privacy/no-effect and Go shared
fingerprint parity pass (9.447s package); actual HTTP historical receipt,
committed/lost reply → changed revision → original tuple conflict → exact GET,
role/member/other-actor privacy and zero event/audit/jobs/provider effects pass
(7.044s package). Log `/private/tmp/hackbox94-native-receipt-proof.log`.
Go/TypeScript contract generation PASS: `/private/tmp/hackbox94-generate.log`.
No SQL definitions changed. Browser-side vector/unit proof remains pending.

An obsolete interim compiler reported nullable-plan accesses now fixed in source;
no current-source compile is claimed. Its preceding lint passed only at that
intermediate executed source. The compiler naturally ended with that failure
before a proposed stop; no running test was abandoned. All proof handles terminal.
Remaining: fresh serial compile/lint/units, complete command/secret/unknown/current
role/partial restoration/held-read/late-response browser journeys in both portals,
independent guide proof and matched actual visuals, full exact-head checks and
paired review. No readiness, merge or issue closure is claimed.
