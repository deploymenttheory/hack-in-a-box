# Reviewed project showcase verification (#163)

This delivery extends the existing reviewed outcome publication owner. The
showcase never publishes submissions or awards by reading them. Default project
sharing remains authenticated organisation access; event/team publication remains
an explicit organiser choice. Published awards additionally require the actual
current native result audience. Missing metadata is neutral, not an inferred loss.

## Scope and assertions

- Native two-team independent judging produces a real winner and non-winner;
  their project publications are separate, positive organiser decisions.
- Permission-filtered event/status/search and cursor pages span two events.
  Facets use only explicitly shared reviewed names, not current private metadata.
- Private award confirmation remains absent; only actual explicit announcement
  can supply this team's safe award names/metadata. Organisation-only readers do
  not inherit native event result or media access from a shared project.
- Reviewed media resolves through the actual owner on every read, including
  archived consent withdrawal. Source repository links retain provider access.
- Unpublished guesses, changed current audience, explicit withdrawal, corrections
  and held old responses do not restore private content or cached records.
- Actual archive retains safe reading and shows a read-only showcase. The
  separate outcomes owner retains its existing explicitly audited post-archive
  follow-through exception; this browsing surface adds no mutation route.
- Proposed/accepted follow-up is distinct from demonstrated adoption. A current
  source-record link is conditional on that record's own native reader.
- Pure production `ShowcaseRecords`/`ShowcaseReading` exports are exercised in the
  independently built guide with synthetic local controls and no API/storage.

## Commands

```sh
HACKBOX_INTEGRATION=1 go test -v -race -p 1 -parallel 1 ./internal/app \
  -run '^TestShowcaseNativePublicationFiltersAwardsAndCurrentAudience$' \
  -count=1 -timeout=5m
HACKBOX_INTEGRATION=1 HACKBOX_SHOWCASE_BROWSER=1 \
  go test -v -race -p 1 -parallel 1 ./internal/app \
  -run '^TestShowcaseBrowsers$' -count=1 -timeout=10m
npm --prefix web run build:styleguide
cd web && npx playwright test --config playwright.styleguide.config.ts showcase.spec.ts
```

The browser matrix uses actual PostgreSQL, native API composition, separate portal
builds/sessions and an owner-private Unix control socket for synthetic fixture
transitions. Authority withdrawal is a synthetic stored input fact; it verifies
native current-audience readers, not the registration withdrawal operation.
Project publication, award release, consent, correction and archive use native
owners. There are no fabricated successful domain/API responses. The only held
browser response is an actual formerly authorised native read, to test late reply
fencing after withdrawal/navigation.

## Current evidence

Initial static checks: affected Go packages compile, TypeScript compiles, both
portals and the independent guide build; configured affected Go lint reports zero
issues and frontend ESLint reports zero issues. Native and browser commands above
are **pending** at this checkpoint. Their presence is not proof of execution.
Exact tested source refs/results will be appended after terminal runs.


## Native publication and retention proof

The first composed run at `64ca8fbb` reached actual archive and correctly exposed
a fixture mismatch: two actual settled submissions but the original lifecycle
team count remained one. The fixture now derives facts from the actual frozen
roster through `event.RecordTeamFacts`; it never bypasses the lifecycle transition.
The targeted diagnostic recorded exactly `total teams=1 settled=2`.

The next run at `8431c91f` reached actual archive and exposed a separate native
clock mismatch: `SetEventStage` used database wall time for `completed_at`, while
retention reads used the injected domain clock. Native lifecycle completion and
cancellation regressions reproduced that mismatch (**RED 4.518s**).

At `f96fb69af58076bbd1747de25c06a736d84e5d9c`, one trusted transition clock now
sets the first completion/cancellation timestamp, preserves existing retention
origins, clears on explicit assessment reopen, and leaves database audit/update
timestamps unchanged. No public time input, retention grant or schema migration
is added. Results:

- Native lifecycle package: **8.806s**, injected completion/cancellation and
  default server-time cases; stale transitions, explicit reopen, subsequent
  cancellation and archive retain the appropriate origin.
- Composed native showcase: **8.897s** (test **7.35s**), actual two-event search,
  cursor pages, independent non-winner publication, private versus released awards,
  narrower result/media/source audience, correction/withdrawal with safe retained
  versions, actual archive, consent decline, current audience loss and the exact
  unchanged 90-day retention boundary.

The shared CI-only commits through `67ca74a3` preserve those Go/API source bytes.
The four-case browser matrix and independent guide browser checks remain pending.
Earlier duplicate red fixture invocations are not counted as passing proof.
