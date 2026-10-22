# Participant credential reveal verification

Issue #75 adds a private Team access page to the ordinary participant application. It uses the resource inventory metadata and audited reveal endpoint from #74. The organiser application remains a separate build.

Only a current member of a published team can enter the resource view. The server remains authoritative for confirmed attendance, current assignment, availability, expiry and reveal permission. Scheduled, expired and unassigned resources are absent from the ordinary member inventory. Empty-state guidance directs participants to an organiser without promising that external accounts work.

The shared query cache contains an explicit projection of safe resource metadata. Reveal and copy call the server directly, outside both query and mutation caches. A value lives only in the mounted component's temporary state and DOM; there is no draft, browser storage or service-worker persistence. Copy performs another audited reveal against the current revision and reports success only after the clipboard write resolves.

A value hides after at most 30 seconds, or at its recorded expiry if sooner. Resource/version/revision changes and team membership changes replace the disclosure component. Navigation, logout initiation, session expiry, browser blur, hidden-page transitions and printing clear its display, abort pending requests and ignore late responses. Team and resource metadata refresh on focus, mount and every five seconds while active; a remote authority change is observed at that refresh, while each reveal/copy is checked immediately by the server. There is no room-display or export route for the page, and print CSS excludes the whole private resource section.

## Evidence

- `web/e2e/team-access.spec.ts`: eight scenarios run against the **public** application in desktop and mobile projects (16 passing cases). Ordinary non-admin sessions cover deliberate reveal/copy, storage absence, clipboard denial, 30-second expiry, version/assignment/membership changes, session expiry, a late reveal after navigation, print/hide cleanup, separate team accounts and logout.
- `web/src/team-access.test.tsx`: a real QueryClient is inspected after disclosure. Neither the revealed value nor an unexpected value field in metadata enters query data; the mutation cache stays empty. Unmount removes the displayed value.
- The browser fixture uses separate ordinary identities and synthetic values. It does not claim to test real PostgreSQL authorization; the server integration suite belongs to the credential lifecycle implementation and #74.
- Local verification: 16 public browser cases and all seven component tests passed; frontend lint and both production builds passed. CI verifies the final rebased PR head separately.

The browser cannot erase a user's system clipboard after an intentional copy. The UI promises temporary onscreen visibility only, and never promises clipboard revocation or that a copied credential will continue to work.
