# Event setup and registration delivery

Approved PR sequence: branding (#43, #44); template workspace (#42); setup isolation (#46); forms (#49, #50); capacity (#51); registration journeys (#54); offers/status/withdrawal (#55, #57); applicant review (#56); teams (#59, #63); registration concurrency verification (#58).

## Branding (#43, #44)

The event-scoped branding API exposes only colours and raster images, not the
configuration snapshot's private forms, briefs or message drafts. Organisers save
under the event lock and expected revision; the change preserves other definitions
and records an audit event. Existing templates clone branding by value.

PNG/JPEG content is fully decoded after bounded dimension/type checks (512 KiB,
4096 pixels per side, one image per kind). SVG, scripts and arbitrary CSS are not
accepted. Text/background and primary/background require contrast of at least
4.5:1. Buttons derive a readable black/white text colour. Accent is decorative.

The workspace previews changes before save, protects unsaved edits, scopes theme
variables and portals to the current event, and clears its favicon on navigation.
`brandStyle` and `brandAsset` provide bounded rendering inputs for the later email
and room-display adapters; this PR does not implement those delivery features.

Verification: branding validation and real PostgreSQL isolation/revision tests;
OpenAPI HTTP request/response checks; desktop/mobile preview, save, event switching,
invalid palette and unsafe-image selection checks. Existing browser journeys are
included in the regression run.

## Template workspace and starters (#42)

The template workspace edits reusable event defaults, agenda offsets, forms,
tasks/dependencies, deliverables, role briefs, rubrics, message drafts and branding.
It previews changes against saved/published definitions. Saving creates a new
immutable draft; publication is separate and does not update existing events.
Cloning uses a chosen event-local live start and explicit DST resolution, and
requires acknowledging the lasting organiser competition restriction.

Two generic starters describe all-team judging and mentor qualification followed
by a fresh final. Both contain role briefs, owned tasks and equal-weight five-part
rubrics, and preserve the minimum-confirmed, admission, freeze, challenge and
submission order. They are reusable process definitions, not an implementation of
judging execution (#107 onward). Starter access requires event-creation authority.

Verification: both starters pass full domain validation; spring/autumn DST clones
preserve gate order. HTTP tests validate starter payloads and deny unauthorised
access. Desktop/mobile browser tests create, edit, compare, publish and clone a
template, preserving the earlier version and an existing event.
