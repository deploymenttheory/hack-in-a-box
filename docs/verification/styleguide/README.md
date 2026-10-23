# Interactive guide verification — DS-04 (#233)

This records scoped development evidence; final exact-head CI and review remain
required before merge. All records, people, notes and receipts are synthetic.

Before this change, main `1e3b9d12d586ff315d227eee7cf9d5d6d2b4368a` had no
style-guide static entry/build/host, publication service or portal support link.
Its accepted visual baseline is [the Fleet reference](../../design/fleet-reference)
and [visual contract](../../design/visual-contract.md). This new artifact imports
the production system rather than restyling a copy of the old portal.

## Local evidence

- 103 unit checks passed, including the existing shared primitive/pattern tests
  and the public guide URL contract; full web lint and both portal builds passed.
- 18 standalone guide desktop/mobile journeys passed against the actual Go host.
  They deny all cookie and Web Storage access and record any unexpected API/network
  request, runtime error or console error, including nested example documents.
- Journeys exercise actual 360px/narrow viewport rendering, labelled controls,
  field focus, dialog Escape/return focus, all mode/palette combinations with a
  portalled dialog, reset versus retained appearance state, all100 guidelines,
  exact receipt success/unknown/latest-only reconciliation with no write replay,
  definite failure, empty/no-match, stale review, paused authority, account/portal
  replacement and a late response after revocation.
- Both portal support links passed on desktop/mobile (4 cases); the existing
  admin shell journeys passed as well (6 cases). An initial admin run reused an
  abandoned preview in a trashed worktree. Its exact PID/cwd were verified and
  that stale process was stopped; checks passed against the current built output.
- Focused Go race tests cover static bytes/MIME/cache, health/readiness, GET/HEAD,
  denied API/auth/method paths, outside-root symlinks and absence of application
  configuration. Focused Go lint passes after error/context/shutdown fixes.

Visual inspection found old global mobile navigation ordering leaking through
an application stylesheet import. The guide now imports the same production
foundations/primitives/pattern styles directly, excluding old domain layouts.
A narrow-screen navigation-order assertion prevents this regression. The separate
Tailwind entry explicitly scans production source, and a computed-style assertion
checks that assistive-only status text retains its actual hidden utility. The final 18-case rerun passed with this correction, and all six screenshots
were captured again from that output.

## Visual artifacts

The PNGs in this directory show actual standalone-host output at1440×1000 and
390×844: production component examples, collection and selected review. Compare
compact aligned records, text-labelled icon actions and the flat task canvas with
the accepted reference. These are new guide views, not claims that later portal
screen migrations are finished. Example previews are real nested viewports with
independent scrolling; a full-page outer screenshot need not reveal all example
content below the frame's scroll position.

Docker runtime verification and CI artifact download remain pending in this draft.
