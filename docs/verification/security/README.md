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
| SSRF attempts through URLs/webhooks | Existing `teamsworkflow` pinned TLS/DNS/proxy/redirect tests; new adversarial transport cases planned; genuine webhook authentication under `httpapi` and native encrypted configuration/history under `githubwebhooks` | Pending full scoped execution and additional abuse assertions |
| Malicious Markdown/assets/uploads | Production `SafeMarkdown` renderer, protected upload MIME/size/path rules, attachment/download headers; new adversarial browser and asset cases planned | Pending |
| Cross-event object references | Current event/resource policies in native HTTP/file/export/stream/jobs tests; add a composed abuse matrix where a valid actor for one event guesses another event's objects | Pending |
| Revoked organiser/early-access eligibility; conflicting registration/grant races | Native `registration/TestOfferAcceptanceAndOrganiserGrantCannotCreateCompetingOrganiser`, eligibility history and access-grant race assertions | Pending execution/current assertion audit |
| Direct early-theme URLs; roster freeze; premature submission/review/scoring | Native content/objective/rubric release gates, registration freeze serialization, scorecards/judging pre-live and frozen-final guards | Pending execution/current assertion audit |
| Help claiming/access and private-to-common Q&A | Native help concurrent claims/revocation and clarification publication, attachment and current audience assertions | Pending execution/current assertion audit |
| Goal/rubric early disclosure; practice isolation | Native objectives/rubric common publication and both-mode practice/current assessor assertions | Pending execution/current assertion audit |
| Showcase search/media leakage | New #163 projection/browser proof must be integrated; current main lacks that completed dependency | Dependency pending; no substitute proof claimed |
| Post-archive follow-up | Native audited outcome/review/reminder path with ordinary competition writes denied and retention enforced | Pending execution/current assertion audit |
| Credential encryption, log redaction, retention | Native credentials recovery/rotation/authority; request/provider error redaction and current retention/worker boundaries | Pending execution/current assertion audit |
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
