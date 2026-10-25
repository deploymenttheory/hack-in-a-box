# GitHub recovery verification — issue 98

This is a full-scope verification delivery, not live GitHub acceptance. The
approved organisation, App registration/installation, credentials and sandbox
event do not exist in this environment; #91's live gate remains pending. No
external repository/invitation was created, no attendee code was executed and no
user container was changed. Real PostgreSQL tests use synthetic identities and
fault only the external provider boundary. No schema or production policy change
is planned. #94 is closed; #97's real UI exists in its reviewed separate PR and
remains a dependency until merged. This backend test lane does not copy its UI.

## Complete acceptance ledger

A row is complete only when its named current-source execution is recorded.
Existing tests are coverage sources, not claimed fresh passes until executed.

| Requirement | Actual native sources and new verification | Current state |
| --- | --- | --- |
| Revoked App / missing permissions / rate limits | New encrypted App → durable capture fault matrix; new native provisioning fault fixture. Existing `githubapp` transport tests exercise installation/token diagnostics including actual HTTP 429. | New native cases and full related transport/owner run pass; exact sources below. |
| Invitation failure / uncertain response / no duplicates | New definitely-unsent failure requires a new reviewed plan, preserving one invitation; original lost-response, commit-failure and concurrent-worker cases in `repositoryaccess`. | New native cases and full related owner run pass; exact sources below. |
| Uncertain create / unrelated resource collision | New encrypted App → actual provisioning queue/worker/reconcile cases; original exact provenance, numeric identity and concurrency cases in `repositories`. | New native fixture and related original owner run pass; exact sources below. |
| Forged / signed cross-event or cross-team activity cannot change submissions | New signed activity test uses two event/team/repository mappings sharing one App secret; tampered body and wrong explicit event reject, with actual submission-state/revision/decision snapshots. | Native case and related original owner run pass; exact sources below. |
| Uncertainty remains unresolved; explicit audited correction; immutable original and amendment | New App recovery matrix retains original source and attempt history; later current ref never verifies historical cutoff. New signed delayed/replayed push cannot alter original completed receipt or formal records. Original `submissionfreeze` tests exercise explicit amendment and history. | New native matrix/cross-owner cases and related original owner run pass; exact sources below. |
| Pre-live work versus formal submission/review | Separate future-dated event verifies an actual pre-live webhook receipt and private draft using real wall time, with formal submit/review blocked. Historical fixture separately verifies delayed pre-live-dated commits, controlled formal gates and actual cutoff dispatch. | Native case and related original owner run pass; exact sources below. |
| Cutoff independent from lifecycle advancement | Same native flow captures/final-settles while lifecycle remains preparation, without changing the deadline. | Native case and related original owner run pass; exact sources below. |
| Stale collaborator work cannot bypass current freeze/retirement | New queued approved grant is rechecked against unfrozen, disbanded and withdrawn current native facts, with zero provider writes/effect claims; original native frozen-reassignment and stale identity/mentor-window cases retained. | New native cases and related original owner run pass; exact sources below. |
| Disbanded repository unused/unscored | New real judge consumer rejects unresolved source, permits only audited binding, then rejects retired-team brief/new scores/result completion while preserving authored bytes. Existing rubric, webhook and freeze owners retain their disband guards. | Native case and related original owner run pass; exact sources below. |
| One approved live sandbox event | [Existing setup](../../github-app-setup.md) and [operator execution record](SANDBOX-RECORD.md), including current real UI/API source and exact evidence fields. | **Pending: approved sandbox/App not provisioned. Synthetic tests cannot satisfy this row.** |

## Finite reproducible synthetic verification

From the repository root, with Docker available for test-owned pinned PostgreSQL:

```sh
HACKBOX_INTEGRATION=1 go test -race -p 1 ./internal/githubapp ./internal/repositories ./internal/repositoryaccess ./internal/githubwebhooks ./internal/submissionfreeze ./internal/submissions ./internal/rubric ./internal/judging ./internal/access ./internal/jobs -count=1 -timeout=15m
```

This command explicitly enables integration tests; a run without that environment
variable can skip PostgreSQL and is not native acceptance evidence. Provider
transport fixtures use only local HTTP; all databases/listeners belong to the
tests and clean up through their existing owners. No extra real provider test is
triggered by running it. Record the executable SHA, dirty status, command, result
and per-package output. Do not reattribute old runs after a rebase.

## Source and execution ledger

- Starting source: merged main `b2b09b6c35fa6cdc8ae6e9fa30acfe3ca5fbd063`.
- Initial working-tree native `TestRecoveryFaults...` matrix passed with real
  PostgreSQL and `-race`, package **6.053s**. It uses the actual encrypted App,
  native final settlement, durable capture and audited correction owners.
- Initial working-tree `repositoryaccess` `TestRecovery...` cases passed with
  real PostgreSQL and `-race`, package **8.045s**. Retired facts are explicitly
  fault-injected current native records; they do not claim a complete human
  withdrawal/disband UI journey or a live provider effect.
- Remaining new cross-owner tests, related owner suites, configured lint,
  current-main rebase and final exact-head CI are pending.

Primary references checked 25 October 2026: GitHub's [signature verification](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries),
[template creation contract](https://docs.github.com/en/rest/repos/repos#create-a-repository-using-a-template)
and [invitation endpoints](https://docs.github.com/en/rest/collaborators/invitations).
HMAC authenticates the original body; it is not permission to target another
native repository/event or to claim historical cutoff state. An invitation is
not proof of accepted collaborator access. The existing setup document remains
the source for approved App permissions and protected credential injection.

## Current-main finite checkpoint — e61363b

The owned checkpoint rebased cleanly onto actual main
b986148ac2798e79790f26db887caee5df4bb70a without touching the independent UI
worktree or changing a migration/dependency lockfile. On clean executable
e61363b2a6c20cbc55d74b738d6b21df77d9c92d all new native recovery tests passed
with real PostgreSQL and -race: submissionfreeze 6.812s, repositoryaccess 7.917s,
githubwebhooks 9.565s and judging 4.152s. Configured lint over the four affected
owners returned 0 issues. No production defect or RED is claimed from these
initially passing verification additions. The complete related original owner
suites and exact final CI remain pending, as does the genuine live sandbox gate.

## Full related proof and clock-attribution correction

On clean unchanged ec95b5d7851a0cfde8cf6cc7ea4da3cbffdc6739, all ten related
original owner suites passed with real PostgreSQL and -race: githubapp 21.396s,
repositories 7.777s, repositoryaccess 23.134s, githubwebhooks 88.185s,
submissionfreeze 79.866s, submissions 23.071s, rubric 20.206s, judging 33.399s,
access 4.847s and jobs 26.454s. The five actual HTTP App/repository/access/webhook/
freeze authentication, CSRF, current-authority and original-receipt cases passed
with PostgreSQL/-race in 50.917s. These are local results, not final Linux CI.

A source/evidence audit clarified the historical combined fixture: provider
commit time is pre-live and native submission time is controlled, while webhook
receipt time is actual wall time. It proves delayed pre-live-dated activity, not
a genuinely pre-live receipt. A separate future-dated scenario now checks actual
received_at before live start with the default real submission clock, preserving
a private draft while formal/review/scoring records stay absent. No job due date
is rewritten or production timing policy changed. The extra missing-permission
fixture code is also aligned with the native `permissions_missing` diagnostic.
Current focused execution/lint and exact-head CI remain pending for this addition.

The first focused addition run exposed a test-only timestamp type mismatch:
`GithubWebhookDelivery.ReceivedAt` is already `time.Time`, while optional
`ProviderAt` is nullable. The fixture now uses the actual generated field type;
this compile failure is not a product RED. The aligned native diagnostic matrix
already passed at 178a5eb in 7.380s. The real-wall-clock receipt case and lint
remain pending until the corrected fixture executes.

The corrected genuine pre-live receipt case passed on clean unchanged executable
ed9be2e82e7b2d079196d8e6f9b19773a03e1f73 with real PostgreSQL/-race in
5.413s. Actual default-clock submission/review and received-at boundaries are
asserted, with operational activity present, private work retained and no formal
revision or score. Affected-owner configured lint returned 0 issues. The native
`permissions_missing` matrix remains separately attributed to 178a5eb (7.380s);
the full related owner/HTTP run remains attributed to ec95. No unrelated full
suite is claimed rerun after these additive test-only corrections.

All authorised synthetic/native verification and the concrete operator record
are now delivered. The exact-head CI/independent reviews and the genuinely
unavailable approved sandbox event remain open gates. This Draft does not
complete live #91/#98 acceptance or relax any first-release requirement.
