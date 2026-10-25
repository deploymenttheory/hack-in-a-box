# Interactive style guide

The guide imports production Tailwind v4 tokens, Lucide registry, primitives and
collection/detail/review/action-queue patterns. It is a separate static output,
not an authenticated application route. Every person, event, note, request and
receipt in its examples is synthetic; scenario state lives in mounted memory.
No browser API, cookie, private store, identity provider or database is needed.

## Browse independently

From the repository root, with Docker Desktop running:

```sh
docker compose -f compose.styleguide.yaml --profile styleguide up -d --build --wait
```

Open <http://127.0.0.1:8082>. This uses its own `hackbox-styleguide` project and
network with one service, without application/database/worker dependencies or
volumes. It works while either portal is unavailable. No `.env` initialization,
OIDC settings or database password is required. Stop this service with:

```sh
docker compose -f compose.styleguide.yaml --profile styleguide down
```

`HACKBOX_STYLEGUIDE_PORT` changes the local host port;
`HACKBOX_STYLEGUIDE_IMAGE` changes the image tag;
`HACKBOX_STYLEGUIDE_REVISION` embeds a commit identifier during the build.
The non-root read-only service serves only guide documents and static assets.
`/healthz` checks the host; `/readyz` requires all three static entry documents.
It denies `/api`, `/auth` and application routes, and its CSP denies connections.
The only allowed frames are its own component/task examples.

Without Docker:

```sh
npm --prefix web ci
npm --prefix web run build:styleguide
HACKBOX_WEB_DIR=web/dist-styleguide HACKBOX_HTTP_ADDR=127.0.0.1:8082 go run ./cmd/styleguide
```

`npm --prefix web run preview:styleguide` is also a static Vite preview, not the
production Go host. Browser checks use the actual Go host, deny cookie/private
storage access, observe requests, and fail on console/runtime errors.

## Publish a versioned artifact

The `styleguide` CI job builds, exercises and uploads `styleguide-<full-head-sha>`
for every successful head. Download that workflow artifact and extract its
contents at the root of an approved docs origin, or build Docker's `styleguide`
target. Retain `index.html`, `samples.html`, `patterns.html`, `assets/` and the
Inter license together. Serve correct MIME types, no-store HTML, immutable
fingerprinted assets and the same CSP/security headers as `internal/styleguide`.
The guide currently assumes an origin root; a path-prefix mount needs deliberate
base/entry routing changes and tests. External hosting, DNS and credentials are
not provisioned by this feature. Do not use production portal cookies on the docs
origin. Promote the artifact corresponding to the production component version.

Both portal footers expose “About this interface · Style guide”. The single public
build setting `VITE_STYLEGUIDE_URL` defaults to `http://127.0.0.1:8082`. Set it to
the approved HTTP(S) docs origin before building both portals, for example:

```sh
VITE_STYLEGUIDE_URL=https://docs.example.test docker compose build api admin
```

Compose passes the same setting as a Docker build argument to each portal.
Changing it requires rebuilding; setting a runtime environment variable alone
does not rewrite static HTML. URLs with credentials or executable schemes fail.
Do not put secrets in this or any `VITE_` build setting.

## Use and extend the system

Start with [the visual contract](visual-contract.md), [primitives](primitives.md)
and [patterns](patterns.md). The guide's Components section exercises the actual
controls; Task patterns exercises prepare → review/correct/cancel → exact outcome
→ next action. Use the labelled synthetic transport control for empty, delayed,
rejected, stale, read-failure, unknown and latest-only responses. Unknown outcomes
permit a status read without another write. Reset clears all mounted example
state; changing account/event/portal drops the corresponding synthetic scope.
Appearance and density changes retain drafts. Viewport controls resize a genuine
iframe viewport; available width can constrain a requested size on small screens.

Task patterns also imports the actual production `ProtectedUploadControls` shared
by content attachments, `TaskEvidence` and team deliverables. Its owner-controlled
example chooses synthetic file metadata, captures one key/revision/team/reason,
permits deliberate identical retries after an unknown callback, clears metadata
on denial and focuses the relevant feedback. A synthetic owner callback is not a
file upload, attachment acceptance or formal submission receipt. The example has
no FileReader, byte transfer, account or API transport; it can display selected
local metadata without reading the file. Reset restores the labelled native
input. Usage/reference entries explain owner scope barriers, privacy, pending
locks, focus, and the separate native `ProtectedUpload` adapter.

Every future UI PR must update production code, its interactive examples,
copyable snippets and intended-use/variant/semantics/accessibility/anti-pattern
guidance together when behaviour changes. Keep the actual icon registry as the
catalogue source. Use fixtures from `components/patterns/scenario`, never copied
participant data. Keep examples out of production entry points; never add a fake
receipt or transport to a production pattern. Shared visual state cannot authorize
a domain command or replace generated API contracts, CSRF and version checks.

Run meaningful checks from the `web` directory:

```sh
npm run lint
npm test
npm run build
npm run test:e2e:styleguide
```

To run the same suite against the opt-in Docker publication instead:

```sh
STYLEGUIDE_TEST_ORIGIN=http://127.0.0.1:8082 npx playwright test --config playwright.styleguide.config.ts
```

The guide browser suite normally starts only its static Go host, has no API/DB service,
checks real narrow viewports and keyboard focus, and makes cookie/private-store
access fail. Existing portal journeys and full repository CI still apply. Capture
synthetic desktop/mobile visual evidence and compare it with the accepted Fleet
reference; guide verification is not certification of every future domain screen.

The media decision history example uses the production `MediaHistoryDisclosure`
and `MediaHistoryContent` with a local synthetic owner. Open starts in loading;
explicit controls supply metadata-only cursor pages, read failure or access denial.
Retry and reopen return to loading without reusing earlier rows. Close restores
focus to the mounted disclosure button; expanded/controls attributes describe the
region and the whole disclosure is hidden in print. Ready rows accept only version,
action and recorded time. Loading, failure and denial states cannot carry rows.
Use this beside a current native `can_read_history` record, including read-only
archives. The native adapter owns fresh per-mount scope, current authorization,
no-store transport and late-response fences. Never pass historical documents,
consent tokens, captions, reasons or byte URLs. History is neither a byte grant,
renewed approval nor an original command receipt. The example has no session,
private persistence or transport. Its copyable snippet documents the owner seam.

The media lifecycle example imports production `MediaIntentReview` and
`PhotoConsentReview`. Its synthetic local owner captures exact native record,
event/item source, action, metadata, reason and review attestations before showing
confirmation. Correct details restores the unchanged editor and actual opener.
Approval uses the native reviewed state; explicit release and withdrawal remain
separate actions. Local unknown callbacks are never described as native receipts.
Reading current synthetic media leaves the original unknown barrier in place.
Self-consent is separately reviewed and has no idempotent retry contract. Guidance
covers native current authority/consent incarnation, dirty versus pending versus
unknown navigation, source comparison, private print/URL lifetimes, signed-in
metadata-only root capsules and the distinction between original acknowledged
version and newer current safe projection. It makes no API, file, storage or
session request and does not fabricate receipt, consent or protected-byte grants.

Media submission and metadata correction use the same field-specific validator and
`useMediaValidation` owner hook. Pass each result to production `Field.error`;
invalid submission focuses the first usable invalid control after error markup is
rendered. Errors remain local to that private editor. No per-keystroke alert or
transport failure is used as a field error. The interactive media lifecycle example
uses these actual helpers and demonstrates an over-byte-limit title, its associated
accessible description and corrective focus before the existing review flow.


The outcomes example imports production `OutcomeDecision` and `useOutcomeReview`
with a bounded synthetic local owner. It demonstrates evidence basis and exact
zero versus unknown, separate positive-field publication drafts, explicit owner
review timing/template, captured review focus/correction, pending and unknown
barriers. Simulated current facts never settle the original outcome. No account,
API, private payload or provider is involved. The copyable reference documents
current authority/CSRF/version and exact native receipt ownership; local guide
feedback is explicitly not a server receipt or publication.

Keep the actual initiating control and preparation form mounted with `hidden`
while reviewing. Pass the clicked button (`event.currentTarget`) or native
`SubmitEvent.submitter` into `setReviewed`; correction then returns focus to that
same control. Current authority loss still unmounts and scrubs the entire private
editor. An unrelated new control with matching text is not the original opener.
## Survey decision ownership

`SurveyDecisionControls`, `SurveyDefinitionReview` and `SurveyAnswerSummary` are
controlled production presentation used by response, versioned authoring and
report adapters. The interactive guide imports those exports with three synthetic
backend-free owners. Keyboard users can review/correct/cancel, hold a callback
pending, explicitly retry a captured response/text identity, read current facts,
select a separately reviewed base and revoke private wording. Pending blocks
Escape/cancel. A removed/disabled opener falls back to the current enabled action.

The native owner supplies current actor/session/event/resource authority, exact
payload/revision matching and scope fences. Raw answers/review prose must remain
in that mounted owner. Store only safe original command metadata across routes;
current reads do not prove original effects. Only native response UUID and exact
reviewed-text identity contracts permit captured retry. Configuration and report
approval/publication unknowns are current-read-only. The guide makes no API,
authentication, storage or real provider calls and never fabricates backend receipts.

Private survey reviews opt into `ConfirmationDialog.printExcluded` through the
actual production `SurveyDecisionControls`. The optional prop hides the themed
portal viewport and backdrop during print; a private page ancestor does not cover
its sibling portal. Other dialog defaults are unchanged. Open and pending reviews
retain their captured payload and original command metadata when print media
changes. Printing never dismisses, confirms, cancels or retries a native command.
The interactive production survey examples demonstrate print → screen while open
and pending, preserving focus/dismissal and explicit owner callback behavior.

## MC and independent room presentation

The presentation example imports production `CuePreview` and `RoomCanvas` with a
synthetic complete projection and fixed server clock. Use private cues only in the
MC preview. The public canvas receives the allowlisted current/next projection,
clock, connection state and already-authorised asset slot; it never loads an
account or credential. A held timer stays fixed, zero never advances a cue and
reconnection labels retained state as last-known. Keep the QR and visible canonical
HTTPS destination identical, distance-readable type, explicit keyboard actions and
print exclusion. The interactive snippet shows the production boundary. Do not
substitute contact metadata for confirmed rendering or a current read for an
original command acknowledgement. The decision example below shares the actual
production review and deliberately retains an unknown original outcome.

`PresentationReview` is also imported by the synthetic decision owner. Capture
an immutable action, source revision, target cue/countdown or display, and reason
before opening it. Correction/cancellation restore the initiating control and
unchanged inputs. Native adapters check the current session and console before
sending; their endpoints offer no idempotent replay or original-command lookup.
Keep only action/revision metadata above route lifetimes, never cue text, reasons,
labels or one-time pairing keys. A direct matching response may settle that safe
metadata after private teardown; current reads and later commands never settle
an earlier lost response. The guide demonstrates unknown and changed-source
barriers without calling an API or inventing a receipt. Use a deliberate fresh
starting point for a different reviewed action. Current denial clears private
snapshots and held continuations; temporary reads retain unsent work with writes
paused. Pairing keys are shown once in local memory and scrubbed on departure,
hiding, print and session end; contact records never prove rendering.


The repository workflow example imports production `RepositoryActionReview` and
`RepositoryPlan` from the pure presentation module. Its synthetic local owner
captures exact event/team/numeric repository, people, permission, plan and reason
before explicit confirmation. Keep the original preparation form mounted with
`hidden`; correction restores its actual initiating control. Pending/unknown
locks prevent replacement. Current reads never supply an original receipt.
The example has no API/session/storage/provider access. Production adapters retain
only safe original metadata above private editor lifetimes, erase private options
and credentials on authority loss, and validate CSRF/IfMatch/native receipts.
No secret-bearing App command is automatically resent. Disbanded repositories
remain unused and unscored; repository recovery cannot reopen frozen teams.
## Role-specific Today

`TodayView` and `TodayLifecycle` are controlled production exports. The independent
guide provides a backend-free synthetic read owner for context switching, current
read versus temporary failure/denial, explicit owner navigation and focus return,
and separate preparation/release/submission/cutoff observations. A changed clock
is not permission to publish, submit, review or reopen a frozen roster.

Owners derive action rows from current authorised APIs; Today owns no parallel
task/notification store or writes. Only currently held role contexts are offered.
A definite denial clears private summaries and cancels/fences late reads above
disposed children. Temporary failure keeps the last observations but pauses
actions. Native deep links and guarded local navigation preserve existing
pending/unknown command owners; current observations do not confirm an earlier
command. Labels communicate urgency without colour; role controls retain native
keyboard semantics. Private Today projections are excluded from print.

## Optional session places

`SessionCollection`, `SessionFacts` and `SessionDecision` are production exports
used by both portals and the independent synthetic guide. They display compact
discovery/personal agenda facts, separate session waitlists, capacity shortfalls,
current event-local dates and captured reviews. Keep the initiating control
mounted/hidden and return focus on cancellation. Pending locks dismissal; unknown
permits an explicit original-acknowledgement read without automatic write replay.
The owner captures source revision, own booking/attendance revision, original UUID
and exact current subject. Current reads never replace an original receipt.

The native adapter derives roster visibility from `can_read_attendance` and
attendance changes from the distinct `can_record_attendance`; neither grants
logistics or assessment access. Current roster denial must unmount its private
names and held review even while the general session remains readable. Calendar
sequences, source/queue history, provider acceptance and delivery remain distinct.
No session place grants event admission, minimum attendance or team membership.
The static guide's callbacks are synthetic local observations, never native
receipts, API calls, private storage or provider effects.

## Submitted builds and code cutoff evidence

`FreezeEvidence` and `FreezeReview` are production exports shared with the static
guide. Label formal outcome, actual cutoff, observation instant and reviewed SHA
separately. Only a completed current binding to the exact native frozen final may
show the immutable code link. Live URLs are mutable; a repository reference or
current branch head is not verified cutoff evidence.

Capture the native source/head/final revisions and original request UUID before
review. Exact request metadata/fingerprint may survive in signed-in memory; branch
drafts, SHA candidates, reasons and evidence bodies remain in private owners.
Current GET never acknowledges a prior command. Unknown outcomes permit original
receipt lookup or deliberate fresh-base review without replay. Current role,
assignment or audience loss clears held private reviews and late continuations.
Use native fields, a named confirmation and actual opener focus; preserve unsent
inputs on cancelled review/departure. The guide uses synthetic callbacks only.

Editable submitted-build forms allow invalid submission attempts while current
permission, pending and unresolved-command locks still disable unavailable work.
Use associated Field errors for branch/SHA/reason and the supporting-evidence
alternatives, then focus the first invalid control before opening a review.
Fresh-base recovery captures its own opener; cancellation returns there or to
the connected local controls fallback without releasing the unknown barrier.

## Independent judging

The judging example imports production `JudgeBriefing`, `JudgeRubric`,
`JudgeScoreFields`, `JudgeScoreSummary` and `JudgeReview` with fixtures from the
shared synthetic scenario module. It demonstrates retained field input, captured
review/correction and opener focus, pending dismissal locks, lost original
callback versus current observations, explicit synthetic original completion and
private denial/print teardown. It has no account, API, storage or file transport.
The usage snippet documents current native source/version/UUID/fingerprint
ownership, isolated practice, exact receipts and metadata-only organiser reopen.
See [judging workflows](judging-workflows.md) for the production boundary.

## Moderation and private selection

The production `QualificationCardComparison` keeps current native source
references and recorded reasons beside the moderated card. Source-specific
huddle/build reads retain their own authority; panel membership does not confer
private observation or submission access. Treat absent/withheld evidence as
unavailable, never as a zero score. Current-source changes clear selected private
inspectors and preparation views before another deliberate action.

`QualificationPreparationPreview` displays separate finalist or category local
preparation only from an actual current signed private source. Omit category
selections from finalist preparation. This example has no approval, send or
publication capability; #112/#113 own later result/publication/feedback actions.
Use the actual native discovery/action capabilities before enabling commands,
keep editable invalid forms actionable, preserve exact version/request recovery,
and return focus to the actual review opener or an enabled local fallback.

The moderation example also imports the controlled production final-card fields,
including pending categories, descriptive anchors, retained evidence references,
reason validation and first-invalid focus. Its separate memory-only callback
shows reviewed values, pending dismissal locks, unknown outcomes and private
scope disposal. These fictional callbacks never imply a saved native card,
selection signoff or published announcement. Keep sent-command metadata and
its departure guard above the private editor; a current read or a deliberate new
base cannot acknowledge an earlier request.

## Results and deliberate team feedback

`AnnouncedResults` accepts only the typed native announcement projection. It selects
one latest publication per kind for participant and room reading; MC cue, countdown,
QR destination and next cue remain independent. It never loads private preparation
or looks up an unpublished result. `ResultReview`, `TeamFeedbackContent` and
`TeamFeedbackReview` are the same production presentations used in the independent
synthetic guide, with local callbacks and no account, API or storage access.

Native owners keep report body, review and recipients in disposable private memory,
exclude private reading/reviews from print and retain only original request metadata
above route and authority lifetimes. Capture exact actor, event, team, source,
version, authoring body and explicit score inclusion before review. Re-read current
source and audience before sending. An original acknowledgement must match actor,
request, action, version and native fingerprint; current GET never settles a lost
response. Correction saves preserve the previous release until a separate deliberate
versioned release records author and confirmed published-team audience. Score choice
starts off; only exact own-team category averages and total can be included.

Keep the initiating form mounted with `hidden` during review so cancellation restores
the actual opener. Pending locks dismissal; unknown offers original receipt lookup
or deliberate current-base review, never an automatic replay. Current denial disposes
private report, audience and held continuation; restoring permission starts a fresh
read. The guide distinguishes synthetic callback completion from a native receipt.
