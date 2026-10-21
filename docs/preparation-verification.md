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
