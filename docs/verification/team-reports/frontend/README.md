# Issue 113 frontend verification

Native browser source: `d9c7bf57`, on the published report/API checkpoint
`1ace2f6091375f61b12e4cd8945862d6295ff311` (including #112's typed current
judge-run ballot scope and room projection parser).

Both production portals and the independently served guide build successfully.
Frontend ESLint reports no errors. Six focused unit checks cover original receipt
ownership, exact publication identity, vote run/tie/candidate identity and current
navigation capabilities.

## Real PostgreSQL, HTTP and browser matrix

```sh
HACKBOX_INTEGRATION=1 HACKBOX_REPORT_BROWSER=1 \
  go test ./internal/app -run '^TestTeamReportsUIBrowsers$' -count=1 -timeout=15m
```

The final run passed in **88.328 seconds**. Two serial Go viewport editions each
own two isolated PostgreSQL fixtures and run the same two Playwright specs:
**four browser cases, no retries, no skips and no flaky results**. See
[native-results.json](native-results.json) for measured case durations, source,
artifact hashes and matrix results. Individual action/navigation/assertion limits
remain 15 seconds; each browser case has a 120-second limit. The harness obtains
actual native sessions through an owner-private Unix socket; it does not install
successful mock API responses or run external providers.

The actual native journeys prove:

- An attendee cannot open the private preparation route or API. Private award
  confirmation remains embargoed until a separate award announcement. A finalist
  announcement releases finalists alone before awards.
- An independently paired room receives the actual typed SSE projection, keeps
  the current cue, timer and next cue, and renders already-announced results after
  reconnection without fetching private results or reports.
- Assigned judges read candidates and frozen deliverables from the exact native
  judge run, retain only their own vote prose, require all votes and a strict
  majority, and explicitly replace a split ballot. A lost real vote response is
  reconciled through the actor-exact original receipt without replay.
- A post-completion prose report saves privately, releases separately, defaults
  scores off, and preserves its old release while an explicitly score-bearing
  correction remains a draft. A lost committed correction-release response does
  not become success through a current read.
- Guarded in-app departure preserves only original request metadata. Returning
  offers an explicit fresh event read when the departed read was cancelled;
  neither that read nor restored private presentation acknowledges the original
  command. Unsent reasons are not restored. Exact native receipt lookup settles
  the original release and PostgreSQL retains four versions and two releases.
- Current own-team membership, foreign-team denial, organiser-only preparation,
  author/version/audience history, private print exclusion, mobile overflow, and
  serious/critical Axe findings are checked in the actual portals. The organiser
  editor, own report and ballot are checked in light and dark system preferences.
  Event palette inputs remain authoritative in either preference.

## Deterministic UI faults and independent guide

```sh
cd web
npx playwright test --config playwright.reports.config.ts
npx playwright test --config playwright.judging-guide.config.ts results.spec.ts
```

Eight synthetic browser cases passed (19.2 seconds), covering unconfirmed
prose-only authoring, changed-source preflight, actual opener restoration,
temporary read failure with retained unsent wording, definite denial and fresh
restoration, session end during held preflight, and archived history with locked
writes on desktop/mobile. These fault callbacks are synthetic and are not native
publication evidence.

The independent guide's two viewport cases passed (4.6 seconds), importing actual
production reading and review components. Cookie and private storage access are
blocked and no API requests occur. Correction retains the prior synthetic release
version; pending dismissal/print locks, actual opener focus and unknown original
callbacks remain separate from current observations. Guide callbacks never create
native receipts or publications.

## Visual evidence

All images contain synthetic native fixtures. Authenticated traces and pairing
keys are excluded from this published evidence.

| Surface | Desktop | Mobile |
| --- | --- | --- |
| Own released feedback, light | [Image](desktop-own-team-light.png) | [Image](mobile-own-team-light.png) |
| Own released feedback, dark | [Image](desktop-own-team-dark.png) | [Image](mobile-own-team-dark.png) |
| Organiser editor, light | [Image](desktop-organiser-editor-light.png) | [Image](mobile-organiser-editor-light.png) |
| Organiser editor, dark | [Image](desktop-organiser-editor-dark.png) | [Image](mobile-organiser-editor-dark.png) |
| Final ballot, light | [Image](desktop-final-ballots-light.png) | [Image](mobile-final-ballots-light.png) |
| Final ballot, dark | [Image](desktop-final-ballots-dark.png) | [Image](mobile-final-ballots-dark.png) |
| Announced results | [Image](desktop-participant-announcements.png) | [Image](mobile-participant-announcements.png) |
| Paired room projection | [Image](desktop-room-announcements.png) | [Image](mobile-room-announcements.png) |

The all-team category tie authority decision remains with the parent delivery;
this frontend exposes the settled overall-final judge ballot workflow and adds
no inferred category casting action. The parent owns required CI wiring and final
integration into draft PR #286. This evidence does not close that policy gate.
