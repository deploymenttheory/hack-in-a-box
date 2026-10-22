# Readiness exports

Issue #62 adds authenticated `GET /api/v1/events/{event_id}/exports/readiness.csv` and `GET /api/v1/events/{event_id}/exports/run-sheet.html`, both requiring `audience=organiser|mentor|participant`. The selected audience narrows current authority; it cannot grant a role. Organiser downloads appear in the admin application, while participant and mentor downloads appear in the regular application.

Each export resolves active roles, confirmed admission and current team/resource assignments under the event row lock. Participant withdrawal, revoked staff roles and replaced mentor assignments affect subsequent downloads immediately. Organisers see operational counts and all preparation tasks; mentors see their assigned tasks and permitted teams' formal deliverables; participants see their assigned tasks and their own published team's deliverables. Raw configuration agenda text retains the organiser-only boundary of the event API. Challenge definitions remain absent from participant exports until actual challenge publication. Mentors cannot inspect formal team work before live start. Private drafts do not count as submitted evidence.

The report includes generation time in UTC, the event timezone, distinct admission/team-freeze/challenge-publication/live/submission milestones, organiser-only raw configuration agenda, overdue/unassigned tasks, required formal evidence gaps and explicit submission outcomes. Organiser counts distinguish confirmed competitors and their configured minimum, staff (including overlap), unaccepted reservations and competitors without published teams. Staff and reservations never increase the competitor minimum count.

Exports deliberately omit task descriptions and evidence, form answers, contact/dietary/accessibility information, credentials, private drafts, review reasons and scores. HTML uses escaped text, embedded print styles and a restrictive offline content security policy, without external assets or executable scripts. The table scrolls horizontally on narrow screens and resets for landscape printing. All CSV cells, including metadata, neutralise formula prefixes and embedded spreadsheet control characters.

Verification:

- `HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./internal/preparation ./internal/submissions` passes against isolated real PostgreSQL databases.
- `HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./internal/httpapi -run TestReadinessDownloadContracts` checks generated download contracts, media types, attachments, no-store, anonymous rejection, audience restrictions and revoked access.
- The PostgreSQL export scenario checks all three audiences, an unrelated platform administrator, early challenge embargo, publication, assigned-team isolation, mentor resource removal, participant withdrawal and explicit not-submitted outcomes.
- Hostile CSV/HTML and empty-event cases pass. Synthetic long/hostile values were rendered in a browser with networking disabled, at desktop and 390px mobile widths, and printed to landscape PDF. All three outputs were visually inspected; the document contains no script/link/image/frame elements and the mobile body does not overflow its viewport.
- Go lint, generated contract checks, frontend lint, both independent frontend production builds and component tests pass.

Downloaded files are point-in-time snapshots and cannot revoke information already downloaded. Their generation time and privacy reminder make this explicit. No schema migration or outbound provider call is introduced.
