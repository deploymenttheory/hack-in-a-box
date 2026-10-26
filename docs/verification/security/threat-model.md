# Supported deployment and abuse boundaries

The supported installation is one organisation, separate participant/admin
origins and API processes, PostgreSQL sessions/domain state, private file
storage, a durable worker and external TLS termination. These tests do not
replace the operator's TLS, OIDC tenant/client, external encryption-key,
backup/restore or host configuration responsibilities.

| Untrusted input or actor | Boundary and required observation |
| --- | --- |
| Browser callback, ID token, pre-existing cookie | Real OIDC discovery/JWKS RS256 verification, issuer/audience/authorised party/expiry/nonce, one-use browser-bound state and PKCE. Invalid attempts cannot create an identity. Authentication rotates anonymous sessions. |
| Cross-origin mutation or stale signed-in cookie | Current native session/person revision and exact Origin/CSRF are independently checked. A previously valid cookie does not survive explicit session revocation. |
| Legitimate person with guessed event/team/file IDs | Authentication is not a resource grant. Current roles, assignments, team membership and event scope control direct reads, files, exports, API and background consumers. A platform administrator gains no implicit private assessment access. |
| Revoked organiser/advance-access holder | Lasting competition exclusion cannot be removed by revoking the privileged role. Grant/registration races must serialize; an unaccepted offer never silently supplies competition eligibility. |
| Early or frozen competition work | Common challenge/rubric publication, live start, team freeze and frozen final evidence remain distinct native gates. A direct URL, practice score, working draft or later stage cannot bypass them. |
| Markdown, file bytes, filenames and links | Production rendering ignores raw HTML and active URLs, never automatically loads Markdown images, and preserves deliberate ordinary links. Upload size/detected MIME/declared MIME and protected attachment/nosniff download rules remain independent. A safe display is not permission to access another team's bytes. |
| Signed webhook or outbound destination | GitHub signs exact raw bytes and native provenance is rechecked before mutation. Teams URLs are credentials: allowlisted paths/hosts, all resolved IPs, pinned dial address, TLS name, no proxy/redirect and safe receipts must hold. Ordinary reviewed reference links are not server fetch commands. |
| Operational helper or published clarification reader | Claiming help grants no private scoring/repository access. Public clarification is deliberately authored common material, never implicit publication of the original private question or attachment. |
| Showcase, result and media reader | Separate publication and current audiences apply independently. Published project access grants no private result, submission, original media or restricted outcome access. Consent/withdrawal/correction remove future access; prior downloaded copies cannot be recalled. |
| Archived follow-up owner or queued worker | Only the dedicated authorised/audited follow-up path may update outcome evidence or approved retained reminders. Ordinary competition writes remain closed; current source, recipient and retention policy are rechecked at execution. |

The executable native case manifest and browser tests identify which of these
observations were actually exercised. A package name, passing compilation,
current source review or anonymous deployment request is not proof of every
operation in that package or every supported deployment configuration.

## Findings and remediation record

No application security finding has been established by the completed initial
abuse scenarios. This is an interim observation, not release sign-off. Record any
new reproduced finding with a safe scenario identifier, precise entry point,
current actor/scope, expected/actual boundary, severity rationale, failing test
and remediation commit; never paste credentials, bearer tokens, private raw
assessment or provider response bodies into the issue/PR evidence.

The required package/test advisory scan exited successfully with zero affected
packages. It reported module-only [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932),
which affects the deprecated `golang.org/x/crypto/openpgp` package family and has
no fixed version. The complete application and test dependency graph (`go list
-deps -test ./cmd/... ./internal/...`) contains zero imports in that family.
Keep this module advisory visible; it is not a discovered reachable application
vulnerability and does not justify swapping unrelated cryptographic code.

The lockfile-pinned `npm ci` audit at this checkpoint reported zero
vulnerabilities. Both dependency observations are time/source scoped and must
be revisited by final current-head required checks.
