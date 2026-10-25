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
issues and frontend ESLint reports zero issues. This paragraph records the initial static checkpoint, before the terminal native
runs documented below. Test presence alone was never treated as execution proof.


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
Earlier duplicate red fixture invocations are not counted as passing proof.
All Go/API/database source remains byte-identical from `f96fb69a` through the
final browser source `755d16f0`; later production corrections are frontend only.


## Native owner regression suites

At `4f0d163336710400d1f6f285968845a3552b0176`, all affected native owner suites
passed with `HACKBOX_INTEGRATION=1 GOMAXPROCS=2 go test -v -race -p 1 -parallel 1
./internal/event ./internal/outcomes ./internal/results -count=1 -timeout=10m`:

| Package | Terminal result |
| --- | --- |
| `internal/event` | PASS, 16.384s |
| `internal/outcomes` | PASS, 17.469s |
| `internal/results` | PASS, 6.326s |

These suites ran against real PostgreSQL with no skipped native cases. Their
production Go/API/SQL source is unchanged in the final browser checkpoint.

## Browser corrections and independent guide

The original four-case browser pass at `3e79a02f` (116.154s) preceded stronger
same-document cache navigation and directory accessibility assertions; it is
historical evidence. The stronger run exposed an actual row-action accessible
name mismatch. `1bab14cc` corrected the production accessible name to include its
visible “Open project” text. A subsequent audit caught a sample during the finite
160ms theme transition: the browser harness now awaits actual finite animation
completion, following the existing native pattern. No colors, axe rules,
assertion/action timeouts or retries were relaxed. The resulting complete run at
`5f83a90d` passed all four cases (52.632s), before the final search privacy change.

Typed search text now lives only in memory beneath the existing identity/session
private cache. Only event UUID/status remain document URL context. Current denial,
identity/session disposal or session invalidation clears remembered text. The
final native matrix also asserts that query text is absent from document URLs and
survives the same-session detail/back journey. No local/session storage or cookie
stores the query.

The independently hosted guide passed **2/2** desktop/mobile checks at
`d56dac1c8f87cc3583971f0218fe5fe72de2666f` (**1.6s**). The guide imports the actual
production collection/reading exports, uses distinct synthetic event/team facts,
restores the actual connected opener, and makes no private API/storage request.
An earlier guide-only failure was a strict locator matching the deliberately
retained hidden collection as well as the active article; the corrected locator
scopes the reading assertion to its active article without changing focus behavior.

Configured affected Go lint and frontend ESLint report zero issues. Both portal
builds and TypeScript pass. Canonical SQL/OpenAPI/Go/TypeScript regeneration is
clean at `557a0101`; subsequent changes through `755d16f0` are guide documentation
and one old presentation-label assertion only.


## Final actual browser matrix

At `755d16f0`, the complete strengthened matrix passed **4/4**, no retries,
flaky cases or skipped cases: Go package **50.225s**, test **48.66s**.

| Portal / viewport | Native case duration |
| --- | --- |
| Administration / desktop | 13.81s |
| Administration / mobile | 11.82s |
| Participant / desktop | 11.51s |
| Participant / mobile | 11.52s |

Each scope starts a fresh actual PostgreSQL/native-owner fixture. All cases prove
same-document back navigation, actual formerly authorised late response fencing,
search text absent from document URLs and retained on ordinary detail/back,
current publication/audience/award/media/consent checks, archive/correction and
light/dark directory plus reading accessibility. All assertions retain the native
configured bounds and zero retries. Screenshots were visually inspected at desktop
and mobile widths. The 16 original rendered PNGs and SHA-256 hashes are indexed in
[`native-browser-evidence.json`](native-browser-evidence.json); scope directories
contain their unedited screenshots.

Existing outcome regression coverage initially passed 46/48 administration cases;
the remaining desktop/mobile cases completed consent removal correctly and then
failed only on the previous “Separately reviewed publication” scope label. The
assertion now matches the rebuilt visible “Reviewed publication” label without
changing its consent/version/no-write assertions. At the unchanged production/test source `755d16f0`, both full portal reruns
passed: **48/48 administration (1.3m)** and **48/48 participant (1.3m)**, desktop
and mobile, no retries or skips. These synthetic API regressions cover command
receipt recovery, private read/navigation lifetimes and media-projection removal;
they supplement, rather than substitute for, the actual native matrix.
