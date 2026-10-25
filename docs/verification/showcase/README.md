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
