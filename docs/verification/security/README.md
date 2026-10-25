# Security and privacy verification (#121)

This is a release verification workstream, not a security certification. The
first checkpoint is based on main `2cfbce4046625e719bfec3a442641a70fda54cdc`.
No release sign-off or absence of critical/high findings is claimed until the
complete evidence below, current dependencies and independent review are settled.

All people, providers, tokens, domains and data in the fixtures are synthetic.
Evidence reports scenario names, fixed error codes and assertions, never bearer
cookies, signed tokens, credentials or private payloads. Native tests require
`HACKBOX_INTEGRATION=1`; a skipped fixture is not verification.

## Acceptance and threat matrix

| Issue #121 boundary | Executable proof and current scope | Status |
| --- | --- | --- |
| SSO, session, CSRF | New `identity/TestSecurity121OIDCTokenBrowserAndSessionAbuse`: actual local RS256/JWKS discovery and PKCE exchange; forged issuer/audience/authorised party/signature/algorithm/payload/nonce; browser-bound and expired/replayed state; anonymous-session fixation; exact Origin/CSRF; current session revocation; same email with distinct subjects | Native PASS at initial checkpoint |
| SSRF attempts through URLs/webhooks | Pinned TLS/DNS/proxy/redirect cases plus new zero-lookup/zero-dial malicious destinations; genuine exact-raw-byte webhook authentication and provenance in canonical HTTP | Native matrix PASS; exact cases in JSON |
| Malicious Markdown/assets/uploads | Real protected upload MIME/size rules and attachment/nosniff downloads; production `SafeMarkdown` browser with raw HTML/SVG/iframe/style, active/data/credential links, remote image request traps and actual current403 | Native matrix PASS and4 browser cases PASS; browser API is mocked |
| Cross-event object references | New valid event-A actor guesses event-B file/audit/export IDs and substitutes event scope; direct current-revocation download denial and native role/resource matrix | Native matrix PASS; not a claim about every possible route |
| Revoked organiser/early-access eligibility; conflicting registration/grant races | Native lasting exclusion after grant revocation, conflicting early-access/registration races, actual offered-place versus organiser grant | Native matrix PASS |
| Direct early-theme URLs; roster freeze; premature submission/review/scoring | Canonical HTTP embargo/privileged exclusion, native registration freeze serialization, submission, scorecard and judge guards | Native matrix PASS |
| Help claiming/access and private-to-common Q&A | Native help concurrent claims/revocation and canonical clarification publication, private attachment and current audience assertions | Native matrix PASS |
| Goal/rubric early disclosure; practice isolation | Native objectives/rubric common publication and exact-version correction, both-mode practice and current independent judge authority | Native matrix PASS |
| Showcase search/media leakage | Merged #163 source is integrated; exact native public-reader/source/publication abuse and native publication/search/consent owner | PASS2 exact parents at9d787fe2; scope/source below |
| Post-archive follow-up | Native audited outcome/review/reminder path with ordinary competition writes denied, current-source cancellation and retention enforced | Native matrix PASS |
| Credential encryption, log redaction, retention | Native credential reveal/rotation/key-fault/audit-failure and context-bound encryption; new request/provider error redaction, retention and archived consent assertions | Native matrix PASS |
| New mentor contribution | Follow-up feature is owned separately; assess its implemented authority, private evidence and publication boundary after integration | Dependency pending |
| Manual local synthetic deployment; automated advisories and full required checks | Actual configured normal-command abuse:55 API outcomes, immutable main076 images, exact OIDC/PKCE, private current-role controls and zero-resource cleanup | PASS atc2f7c69d; final integrated feature/current-head checks remain pending |
| Findings with reproduction, boundary and remediation | Findings ledger will distinguish reproduced issues, fixes, remaining risks and untested boundaries; sensitive values excluded | No findings conclusion yet |

## Initial native checkpoint

```sh
HACKBOX_INTEGRATION=1 GOMAXPROCS=2 go test -v -race -p 1 -parallel 1 \
  ./internal/identity -run '^TestSecurity121OIDCTokenBrowserAndSessionAbuse$' \
  -count=1 -timeout=3m
```

Terminal PASS, package **3.887s**, parent **2.54s**, 16 adversarial subtests,
followed by trusted-login/fixation/CSRF/revocation/identity-separation positive
controls in the parent. The isolated PostgreSQL container stopped and terminated
normally. This checkpoint verifies the real verifier/session/store; it does not
exercise reverse-proxy configuration, browser rendering, all event APIs or the
remaining release threats. The test changes no production authentication policy.

## Additional executable abuse scenarios

New transport assertions call the real Teams Workflows adapter with malicious
URL forms and loopback/link-local/private/mixed DNS answers. They require zero
network dials for every rejected destination; the public-address positive control
requires one lookup and one dial to its pinned IP. Provider/destination details
must not enter returned receipts. Native HTTP tests additionally exercise valid
stored sessions against cross-event file IDs, audit and exports, malicious MIME
claims/oversized uploads, attachment/nosniff downloads and current organiser
revocation. The actual OIDC front door is proved separately above; these stored
sessions do not pretend to repeat that authentication protocol.

The new non-database transport/redaction tests passed with the race detector:
`teamsworkflow` **1.365s**, `httpapi` **3.113s**. The new canonical HTTP/object
fixture passed against isolated PostgreSQL in **13.772s** (test11.67s), with
normal container termination. No production policy was changed.

`cases.json` maps exact existing and new test names to the required threat
boundaries. Run `HACKBOX_INTEGRATION=1 python3 scripts/verify-security.py
--output-dir /private/tmp/hackbox121-security-evidence`. It requires every named
parent to execute and pass, rejects any skip/failure, and records only names,
source, commands and private-log digests in its summary. Its explicit scope is
not full CI, browser/manual deployment verification or pending feature proof.

## Agent-executed manual configured-deployment observations

Root authorised read-only anonymous probes of the existing synthetic preview,
source `2cfbce4046625e719bfec3a442641a70fda54cdc`, on localhost8080/8081. On
both independently configured portals, `/healthz` returned200; `/api/v1/session`,
`/api/v1/events`, `/api/v1/projects` and a guessed event/file route returned401;
`/ops/metrics` returned403. Each retained `Cache-Control: no-store`,
`X-Content-Type-Options: nosniff` and no cross-origin allow header for an attacker
Origin. No request mutated data, supplied a session, restarted a process or
retained response bodies/cookies. These observations cover the anonymous deployed
boundary only, on the stated historical source. Privileged manual abuse and
integrated final-source deployment checks remain pending.

## Complete scoped checkpoint at6bab692b

All **17 native groups/51 exact top-level cases PASS**, no skipped or failed
cases, on clean source `6bab692bbf34eb2d88926b0e4887ff870551ccfc`.
[`native-matrix.json`](native-matrix.json) retains the actual commands, every
case, source identity and private-log SHA256. The runner parsed the real Go JSON
events and required every manifest parent exactly once; it did not infer passes
from names, compilation or a package-only exit code. All fixtures were serial
with `GOMAXPROCS=2`, `-p1`, `-parallel1`, `-race`, `-count1`; no shared app or user
container was stopped or modified.

The new production-renderer browser test passes **2/2 administration(3.9s)**
and **2/2 participant(4.5s)** at desktop/mobile, workers1, zero retries. It asserts
no active elements, executable schemes, credentials in links, remote image/style
requests or script/dialog side effects; the positive ordinary link remains and
current403 removes the rendered private text. Responses in this browser test are
synthetic API fixtures, not native PostgreSQL proof. An initial harness attempted
an unsupported query-client `focus` event; no denied GET occurred. The corrected
test uses the client's visibility event and explicitly observes the actual403
before checking disposal. No production behavior or timeout was weakened.

Both portal builds, configured affected Go lint, frontend ESLint, the package/test
advisory scan and lockfile installation pass. The advisory limitation and precise
trust boundaries are in [threat-model.md](threat-model.md).

This checkpoint does not clear the integrated showcase/mentor feature,
privileged configured-deployment abuse, independent review or final CI gates.

## Supplemental delivery proof at43250218

The separately committed delivery manifest covers actual HTTP display
capabilities and SSE revocation, policy-before-payload subscription checks,
native queued-job current role/assignment/event checks, overlapping file roles,
room projection, approved multi-channel confidentiality and GitHub App
least-privilege operation tokens. On clean source
`432502185d422016eb0cae19f54e23044d641500`, all **7 groups/12 exact parents
PASS**, with no skips/failures; see
[`native-delivery-matrix.json`](native-delivery-matrix.json) for the actual source
record, cases, commands and log hashes.

Reproduce this additional scope with:

```sh
HACKBOX_INTEGRATION=1 python3 scripts/verify-security.py \
  --manifest docs/verification/security/delivery-cases.json \
  --output-dir /private/tmp/hackbox121-security-delivery
```

The two explicit manifests together executed **24 groups/63 distinct top-level
cases**, retaining their separate source identities. No case or current feature
is inferred from another package's passing label. The actual rendering suite
also passed49 files/237 unit tests; the four browser cases, lint, both portal
builds and advisory/dependency-graph log digests are retained separately in
[`frontend-advisory-summary.json`](frontend-advisory-summary.json).

## Remaining delivery gates

- Preserve the now-executed integrated #163 native abuse evidence and its exact
  source; validate any later production changes against that bounded proof.
- Integrate the separately owned mentor-contribution feature and verify its
  current authority, original private evidence, tie/confirmation and publication
  boundaries. The absent feature cannot be certified by current judge tests.
- Preserve the now-executed55-outcome normal-command privileged walkthrough
  against main076. Reconcile final integrated feature changes with this scope;
  the future/live competition and mentor boundaries still require their native
  owners. Historical preview observations retain their narrower source/scope.
- Execute the supported production HTTPS/Secure-cookie profile with a local
  trusted synthetic issuer/proxy; the completed private HTTP/development walk
  does not establish that transport scope.
- Obtain independent reviews and complete final current-head required CI. Keep
  this PR Draft and #121 open; no critical/high-severity release sign-off or full
  release completion is claimed by these scoped results.

## Isolated normal-command manual walkthrough

`compose.yaml` derives its private network/provider composition from #291's
normal-image probe. Both portals run the production `/app/api` command, normal
migrations and real time. The independent Python probe signs in through actual
RS256 discovery/token exchange and browser-bound PKCE callbacks; no SQL, test
clock or fabricated application cookie supplies authority. Only the explicitly
synthetic provider offers actor selection.

Pin the normal public/admin images built from the stated application source,
the reviewed synthetic provider image and the Python runner by immutable image
ID. Set `HACKBOX_SECURITY_SOURCE` and a private absolute
`HACKBOX_SECURITY_ARTIFACTS` directory; `compose.yaml` requires those values.
Use a unique Compose project name and never an existing user deployment:

```sh
docker compose -p YOUR_UNIQUE_SYNTHETIC_PROJECT \
  -f docs/verification/security/compose.yaml up -d --wait public admin
docker compose -p YOUR_UNIQUE_SYNTHETIC_PROJECT \
  -f docs/verification/security/compose.yaml run --rm probe
docker compose -p YOUR_UNIQUE_SYNTHETIC_PROJECT \
  -f docs/verification/security/compose.yaml down --volumes --remove-orphans
```

The agent-executed walkthrough covers privileged/ordinary identities, exact
CSRF and Origin rejection, cross-event files/exports, active MIME spoofing,
unreleased content/preview URLs, current role grants and revocation with the
same authenticated session, and independent portal logout. Its JSON receipt
retains scenario labels, statuses and source only. It explicitly excludes future
competition progression, showcase browser rendering and the unintegrated mentor
feature; those retain their separate native/browser/integration gates. A script
or successful image build is not a recorded execution result.

## Rebased integration checkpoint

The security branch now includes merged showcase #289 through main
`076f6162558df2ac3d7fb1ff7c23b3d35dc21784`. The original63 native proofs above
retain their original source identities; rebasing does not re-execute them.
`showcase-cases.json` explicitly selects the new public-reader/source/publication
abuse case and the existing full native publication/search/consent owner.

On source `a64fb6be`, the existing owner passed **86.83s**, but the package failed:
the new abuse fixture incorrectly used POST for the actual PUT publication
mutation, receiving405. A405 is not authority proof. The corrected fixture uses
actual PUT, the actor's own valid session/CSRF/Origin, exact current source and
If-Match; checks that denial changes no current/published version; then executes
the identical command as organiser and requires real publication withdrawal.
Its native rerun remains pending. No successful matrix artifact is published for
this failed package execution.

The separate normal-command deployment preparation built fresh public image
`sha256:529088d0e4ee70be5e55d90a5b4d9594d724f36cd59f717ca21f560139733f90`
from source `a64fb6be`. Its isolated PostgreSQL/provider/public startup became
healthy. Shared Docker daemon loss then interrupted the administration image
build with RPC EOF before the walkthrough was executed. Image build and healthy
startup do not establish any privileged abuse result. Root coordinates shared
Docker recovery; only this uniquely named synthetic project may subsequently be
cleaned/restarted by this lane. The normal user deployments remain outside this
walkthrough's mutation scope.

`verify-security-deployment.py` now wraps this walkthrough with a frozen Git
archive, immutable image IDs, explicit application/verification source records,
a fresh UUID-suffixed project, all-resource preflight and scoped final cleanup.
It accepts existing normal images only when their retained build-source record
matches their embedded revision (when present) and the verification source has
no differing production code. An absent image revision label is recorded as
absent; image IDs and a supplied source string alone do not prove a build origin.
The separately retained build record remains required.

```sh
python3 scripts/verify-security-deployment.py \
  --application-source FULL_BUILD_SOURCE_SHA \
  --public-image sha256:FULL_PUBLIC_IMAGE_ID \
  --admin-image sha256:FULL_ADMIN_IMAGE_ID \
  --provider-image sha256:FULL_SYNTHETIC_PROVIDER_IMAGE_ID \
  --python-image sha256:FULL_PYTHON_RUNNER_IMAGE_ID \
  --artifacts /private/tmp/NEW_PRIVATE_SECURITY_ARTIFACT_DIRECTORY
```

The walkthrough separately asserts missing required CSRF header400, invalid
32-byte token403 and invalid Origin403. The driver itself is not an execution
result; publish its measured receipt and zero-resource cleanup before claiming
that scope passed.

## Executed showcase abuse and normal-command walkthrough

At clean verification source `9d787fe231e599803cdd7ff07fba1f1582b06070`, both
exact showcase manifest parents passed with the race detector, **16.263s**,
no skips/failures. The corrected native request is actual PUT with current source,
If-Match and the actor's own session/CSRF/Origin. The outsider denial changes no
publication version; the identical organiser command succeeds and really removes
the published card. Both portal reads, literal adversarial search, restricted
media/source separation and native consent/withdrawal/archive assertions pass.
The source/commands/parent names/log digest are retained in
[native-showcase-matrix.json](native-showcase-matrix.json).

At clean verification source `c2f7c69db067ef5fd350323a7dcb1b358355a77b`, the
agent-executed normal-command walkthrough passed **55 observed API outcomes**.
Application images are independently built from clean tracked main
`076f6162558df2ac3d7fb1ff7c23b3d35dc21784`; the runner verified their exact
immutable IDs, embedded source labels and production-code equivalence. Both
portals use normal `/app/api`, actual RS256 discovery/token exchange, browser-bound
PKCE/nonce callbacks and native PostgreSQL sessions. No SQL, test clock, invented
application cookie or successful mocked application response supplies authority.

The observed outcomes include unauthenticated reads on both portals; real owner
and stranger logins; native event/file/draft positive controls; missing/invalid
CSRF and invalid Origin; guessed/private/foreign-event file and export denials;
active MIME spoofing415; direct unreleased content and preview denial; self-grant
rejection; same-session mentor and organiser access/revocation; fresh-login
revocation persistence; and public logout401 with the admin session still valid.
Only labels/statuses and source are retained in
[normal-deployment-observations.json](normal-deployment-observations.json).
[normal-deployment-summary.json](normal-deployment-summary.json) records pinned
images, log digests and zero remaining project containers/volumes/networks.
Normal user deployments were untouched.

The first walkthrough at9d787fe2 failed because its script expected400 for the
correct canonical unsupported-MIME415. That failed attempt remains separately
retained with `passed:false` and zero-resource cleanup. Only the expected exact
status changed; no application behavior, timeout or broad accepted-status range
was changed. It is not an application security finding or successful full run.

These proofs remain scoped: future/live progression, standalone showcase browser
rendering and the separately owned mentor feature are not inferred from this
normal-command walkthrough. `mentor-advisory-cases.json` specifies the actual
new owner assertion to execute after #292 merges and is explicitly **pending**;
its absent test on an older source must fail the runner rather than count as a
pass. Final integrated source, independent review and latest-head required CI
remain release-signoff gates.

The completed55-outcome deployment used private-network HTTP and development
configuration. It does not prove the production HTTPS issuer/origin or Secure
cookie transport profile. #120's SMTP TLS proof is a different boundary and is
not substituted for browser/API HTTPS. That supported-profile execution is an
explicit remaining #121 gate.
