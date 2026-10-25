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
| Showcase search/media leakage | New #163 projection/browser proof must be integrated; current main lacks that completed dependency | Dependency pending; no substitute proof claimed |
| Post-archive follow-up | Native audited outcome/review/reminder path with ordinary competition writes denied, current-source cancellation and retention enforced | Native matrix PASS |
| Credential encryption, log redaction, retention | Native credential reveal/rotation/key-fault/audit-failure and context-bound encryption; new request/provider error redaction, retention and archived consent assertions | Native matrix PASS |
| New mentor contribution | Follow-up feature is owned separately; assess its implemented authority, private evidence and publication boundary after integration | Dependency pending |
| Manual local synthetic deployment; automated advisories and full required checks | Actual configured deployment abuse scenarios and current-head required checks must be recorded separately | Pending |
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

- Integrate the completed #163 showcase source, then execute its native
  search/direct-route/current-consent/current-audience/stale-response abuse proof
  as part of this release verification, not as an unexecuted reference.
- Integrate the separately owned mentor-contribution feature and verify its
  current authority, original private evidence, tie/confirmation and publication
  boundaries. The absent feature cannot be certified by current judge tests.
- Execute privileged abuse scenarios against the final integrated local
  synthetic deployment. Historical anonymous-only preview observations and
  native HTTP fixtures retain their actual narrower scope.
- Obtain independent reviews and complete final current-head required CI. Keep
  this PR Draft and #121 open; no critical/high-severity release sign-off or full
  release completion is claimed by these scoped results.
