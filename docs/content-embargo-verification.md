# Content embargo verification

Issue [#72](https://github.com/deploymenttheory/hack-in-a-box/issues/72) verifies
the author-to-reader boundary supplied by #69–#71. No endpoint, generated contract
or database migration changes are needed.

## Real PostgreSQL and HTTP coverage

`internal/httpapi/content_embargo_test.go` uses the real router, PostgreSQL,
authenticated sessions, protected local storage, and a shared controllable domain
clock. Fixtures contain two frozen teams of three confirmed competitors, an
organiser, a mentor and a judge. They use synthetic titles, bodies, filenames and
search canaries.

| Test | Boundary proved |
| --- | --- |
| `TestContentEmbargoHTTPRoleTeamAndCacheMatrix` | Europe/London spring and autumn preparation weeks are 167 and 169 elapsed hours. Immediately before publication, list/search/cursors, detail/history requests, acknowledgements and copied or guessed file URLs reveal no embargoed content. Manual and early queued attempts cannot publish. |
| Same matrix, at publication | A stale version-one job cannot publish approved version two. A fresh worker's actual startup maintenance hook releases persisted approved content; repeated startup and concurrent duplicate dispatches produce one release record per item. Every confirmed competitor receives the same challenge version and release instant. Mentors, judges and each team receive only their current audience's pages and attachments. |
| Same matrix, after authority changes | Original session cookies and copied URLs stop working after grant revocation, attendance withdrawal or resource withdrawal. Conditional and range headers on every private file GET cannot produce a cached-response bypass. Successful and denied private responses use `no-store` and `nosniff`. |
| Same matrix, exports and assets | Before and after publication, organiser audit exports, configuration snapshots and HTML shells contain no native content canaries. Unsupported content export paths and guessed file IDs or actual private object keys under public asset paths return errors without content. A real public application bundle remains immutable-cacheable as a positive control. Request logs contain no canaries. |
| `TestContentEmbargoHTTPGatesAndMissedPostponement` | Missing confirmed minimum, incomplete freeze and dispatch beyond the 60-second allowance prevent publication. Fixing a gate later does not catch up. Postponement, a new future calendar schedule and fresh version approval are required; stale jobs cannot bypass them. |
| `TestContentEmbargoHTTPPrivilegedGrantsAndLastingExclusion` | Actual staff endpoints reject organiser/early-brief grants to confirmed and waitlisted competitors. Permitted preview grants can be revoked, removing new access while retaining permanent competition exclusion. |

The startup fixture runs `jobs.Service.Run` with a newly constructed content
service and worker. It stops after the real local startup maintenance callback;
it does not claim to exercise external provider clients or a process crash. The
existing content, schedule and jobs suites supply additional transaction,
cancellation, dispatch-allowance and durable-operation coverage.

The matrix also verifies a production correction: a blocked replacement must
not disclose its private workflow diagnostics to readers of the released version.
Authors retain the blocked diagnostic, but an already published challenge is not
described as a missed initial publication requiring postponement.

## Browser coverage

`web/e2e/embargo.spec.ts` runs on desktop and mobile with deterministic API
fixtures. It verifies withdrawal and revocation remove visible cached bodies,
titles and download links on refresh and subsequent navigation; copied download
requests are denied; private bodies are absent from local/session storage and
CacheStorage; and ordinary readers never request author previews. It also holds
an old account's response across a session/account switch and proves it cannot
populate the next account's reader. Warm navigation and search remain empty until
release. Real server authorization and release transactions are established by
the PostgreSQL matrix above, rather than by these browser mocks.

Already delivered bytes cannot be recalled. These checks cover future application
reads and downloads, application state and response caching; they do not promise
to erase files a person previously saved.

## Run

```sh
HACKBOX_INTEGRATION=1 go test -race -timeout 5m ./internal/httpapi -run TestContentEmbargoHTTP
npm --prefix web run build
npm --prefix web run test:e2e -- embargo.spec.ts
```

Integration mode requires Docker and starts isolated pinned PostgreSQL fixtures.
Without `HACKBOX_INTEGRATION=1`, database tests explicitly skip. The implementation
PR records actual results from CI, including the complete PostgreSQL race suite,
browser suite, lint, generated-contract check and Docker persistence smoke test.
