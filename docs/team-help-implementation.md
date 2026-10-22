# Team help request boundary (#159)

This isolated implementation is not yet ready to merge. Its schema is explicitly
`internal/help/schema.provisional.sql`, included by sqlc for development fixtures.
It has no Goose number and changes no production expected schema version. Before
opening its PR, move that schema into the actual next migration after merged
provider/inbox/automation/rubric dependencies and prove the populated ordered
upgrade. Migration numbering follows readiness, not this draft's creation time.

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
#81 must register the help resource owner and explicit destination allowlist only
after its core and this actual route are merged. No unavailable future handler is
activated in this draft. The notification sink accepts source IDs/revision and
authoritative recipients, never an arbitrary URL or private prose.

The independent read-only `/events/{event}/help/{request}` page uses the real GET
contract, current access and event timezone. It renders private context as inert
text and follows only approved HTTPS meeting routes. #160 still owns the request
form, full mobile queue and claim/transfer/resolve controls. This minimal reader
avoids making inbox destinations depend on those later controls or dashboards.

## Current evidence and remaining work

Synthetic provisional-schema PostgreSQL/race coverage proves concurrent claims,
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
production shared factory wiring preserving SMTP/Teams; real #81 help owner/sink
and explicit browser destination activation; #82 maintenance/source adapter;
focused real delivery/read-revocation evidence; final generated reproduction and
exact-head CI. No real contact message or provider broadcast has been sent.

Checkpoint execution: final help PostgreSQL/race14.051s and HTTP4.185s pass;
metadata keyset regression exceeds1,000 sources; six final reader browser
cases pass2.8s. Runtime graph/native inbox/automation and final ordered migration
proof are deliberately unfinished. The current bounded queue/maintenance batch
queries also need final pagination review before the implementation PR.
