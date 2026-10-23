# Portal design-system verification

These are synthetic registration fixtures. The same screen before this change
was captured from merged baseline `e75cb4b`; after captures use this PR's shared
system and pinned browser tooling. No real account, mailbox or provider was used.

| Screen | Before | After |
| --- | --- | --- |
| Registration under admin shell, desktop | [Baseline](before-admin-desktop.png) | [Shared system](after-admin-desktop.png) |
| Registration under admin shell, mobile | — | [Shared system](after-admin-mobile.png) |
| Attendee portal, desktop | — | [Shared system](after-participant-desktop.png) |
| Attendee portal, mobile | — | [Shared system](after-participant-mobile.png) |
| Organiser event setup, desktop | — | [Event details, dates and calendar](after-organiser-setup-desktop.png) |
| Organiser event setup, mobile | — | [Event details, dates and calendar](after-organiser-setup-mobile.png) |

The committed `design-system.spec.ts` matrix checks actual rendered normal,
system-dark, custom-light, custom-dark and near-threshold grey palettes on both
viewports. It measures input, border, action/hover, card, menu and error contrast;
checks keyboard focus, native color scopes/portals, reduced motion, overflow,
read-only/disabled states, private draft retention and native checkbox labels.

The room regression was red on the prior partial room role mapping: a native
light room under system dark produced primary-role contrast **1:1**, and a
near-threshold grey room produced **2.7307:1**, against a required **4.5:1**.
All four desktop/mobile regressions passed after room controls derived the full
semantic palette and native color-scheme through the shared `brandStyle` owner.
The targeted participant theme/room run passed **30 cases**; the admin normal
palette/checkbox-label run passed **four cases**.

The final full admin and participant suites run against the frozen commit. An
initial build passed 580 admin cases; an initial high-concurrency participant run
had 383 passes and three timing failures (room QR, room reconnect/revocation and
short-lived credential copy). Those results are not presented as a final green
run. Logs and traces are retained locally while exact-head PR CI is the complete
merge gate. No workflow timing or permissions were changed to make these checks
pass.
