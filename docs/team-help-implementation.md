# Team help request boundary (#159)

This isolated implementation uses Goose schema32 after the concrete observations31
checkpoint, retaining SMTP, Teams, rubric and inbox contracts. It still needs final
restacking onto merged main and fresh exact-head CI before merge. Numbered tests
use the actual migration; there is no provisional-schema fixture injection.

## Owning domain

Help requests are team/event scoped operational records. A current active
confirmed member of a published team creates/reads only that team's requests.
Current organisers read the event queue. Current logistics staff help with setup
and logistics; mentors require current published assignment, assignment time,
availability and no declared current-team conflict. No request claim creates a
resource assignment, private observation, assessment, repository grant or score.

Waiting, claimed, resolved and cancelled states have a request revision and an
append-only transition history. Release/reopen/unavailable-assignee reconciliation
start a new waiting cycle. Every command has a fixed actor and durable digest of
its idempotency key/input; reusing an identity for different input conflicts.
Event locking serialises claims with membership/assignment writers. Archived
requests remain readable according to current authority, but cannot mutate.

Setup/logistics support works before live start. Competition-output creation,
reading and transitions require both actual common publication and the scheduled
live-start instant. Help resolution never implies formal review completion.

Queue responses omit private problem/location and apply current scope before
counts. Requested, first claimed, resolved and updated times describe operational
response/completion, without mentor/team rankings. Event-configured thresholds
retain the concrete threshold in each follow-up record and deduplicate by request,
waiting cycle and settings revision. A due follow-up appends a real `overdue`
transition/new revision. The domain method owns this local transaction; #82 owns
unattended scheduling and provider execution.

## Safe source and destination seams

The help policy is shared by request HTTP access, per-person communications fact
authorization, inbox resource ownership and automation metadata. Help registers a
native policy resource with its request UUID; it does not create generic resource
assignments. The registered communications owner validates actual event/request,
resource kind/ID, current revision, requester and safe status. Unknown/stale data
never falls back to a supplied owner ID. Without a registered owner, #77's existing
owner-only behavior is preserved. The safe fact maps waiting/claimed to `open`,
resolved to `resolved` and cancellation to `withdrawn`; it contains no question,
location, meeting route or confidential assessment prose.

Exported metadata methods are:

- `MaintainInTransaction(ctx, tx, eventID) error`.
- `DiscoverNotificationSourcesInTransaction` returns actual latest transition
  references: request UUID, revision, occurred time and action. Its additive
  `DiscoverNotificationSourcesPageInTransaction` filters event/request before
  bounded UUID keyset pages; the wrapper has no permanent source-history cap.
- `ResolveNotificationAudienceInTransaction` validates the exact current source
  and derives current recipient UUIDs plus safe fact/resource identity/status.

#82 supplies a thin adapter without importing automation into the help domain.
This isolated implementation stacks the concrete #81 core contract and registers
only the actual help resource owner and explicit help destination allowlist. The
shared API/worker factory preserves SMTP/Teams and delegates current recipients to
the native help policy. Unknown or stale native source references deny access;
there is no fallback to a forged owner ID. Other future owners stay unavailable. The notification sink accepts source IDs/revision and
authoritative recipients, never an arbitrary URL or private prose.

The independent read-only `/events/{event}/help/{request}` page uses the real GET
contract, current access and event timezone. It renders private context as inert
text and follows only approved HTTPS meeting routes. #160 still owns the request
form, full mobile queue and claim/transfer/resolve controls. This minimal reader
avoids making inbox destinations depend on those later controls or dashboards.

## Current evidence and remaining work

Synthetic PostgreSQL/race coverage proves concurrent claims,
idempotency, team/event isolation, current withdrawal/conflict/departure denial,
pre-live logistics and competition-output gating, immutable history, scoped
counts, exact threshold/dedup, approved meeting hosts and revocation across an
event-locked read. Native source tests prove team/helper routing and deny unknown
or stale owner-forged facts without private prose. Existing communications tests
continue to pass with the default policy.

The real HTTP fixture proves session/team boundaries, no-store, private context
exclusion from queue/logs, safe settings access, CSRF and pre-live output denial.
Six desktop/mobile reader cases exercise hostile text, keyboard refresh after
access loss, and cross-event resource references. The synthetic mobile capture
was visually inspected for readable wrapping without automatic external loads.
Both frontend bundles, web lint and Go lint pass at the checkpoint.

Still required before the implementation PR: final migration/upgrade proof;
#82 maintenance/source adapter; final generated reproduction and
exact-head CI. No real contact message or provider broadcast has been sent.

Checkpoint execution: final help PostgreSQL/race14.051s and HTTP4.185s pass;
metadata keyset regression exceeds1,000 sources; six final reader browser
cases pass2.8s. Final ordered migration and automation composition remain unfinished.

On the concrete provider/rubric/inbox stack, the real shared-factory regression
creates pre-live logistics help, records a fixed safe inbox update, opens the
actual help resource and denies both after withdrawal. Integrated help/HTTP/app/
inbox race checks pass18.616/9.496/4.556/17.251s. Eight actual participant desktop/
mobile cases pass2.4s including inbox-to-help navigation with no private question
copied into the preview. Full Go lint reports0 issues; both bundles and web lint
pass. Native queue/maintenance reads use bounded UUID keyset queries without a
permanent1,000-request cap; the populated1,005-source queue regression passes.
Concrete31→32 migration tests retain earlier private resources and verify empty, settings-only and populated rollback behavior. Final main rebase and exact-head CI remain gates.


Numbered schema32 proof: help15.777 s, HTTP6.647 s, shared runtime3.942 s and
inbox12.854 s pass with PostgreSQL/race. Historical credential/rubric/SMTP/Teams/
database migration suites pass3.670/3.614/3.327/3.561/3.402 s. The observation31
historical fixture exposed the expected partial-Down/latest assertion edge under32;
its owning200 PR is supplying explicit31 retention plus latest restoration.
That parent correction and final main restacking remain required. No claim of
full stacked CI green is made before those concrete changes are integrated.
