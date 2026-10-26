# Operator documentation verification (#125)

The documentation now has an operator handbook, event-day checklist and real
index instead of the inherited project template. It describes current Compose
service/origin/configuration boundaries and links the owned integration/recovery
procedures. README statements that all integrations are future work are removed;
the recovery guide no longer recommends a stale hard-coded schema.

The fresh isolated Docker walkthrough now passes at `fa92e01b`; see
[fresh-installation evidence](fresh-fa92e01b/proof.json). It follows the handbook's
normal Compose commands in a new tracked checkout, uses generated private local
credentials and fresh database/files volumes, migrates schema55, performs the
first normal OIDC login, removes bootstrap and recreates the APIs/worker, then
creates and brands an event through the UI. Both portals decode the saved logo
and inherit the saved primary colour; the matching independent guide and separate
logout pass. Both disposable projects leave zero containers, networks or volumes.
This uses the existing Docker host, rather than claiming a new physical machine.
Full #120 lifecycle, final integrated reviews/CI and release signoff remain open.

| Required proof | Current status |
| --- | --- |
| Local links and command/settings source review | 57 local targets across seven pages exist; independent reviewers checked the documented Compose/configuration/recovery boundaries against source |
| Secret examples contain placeholders only | Initial independent source reviews found no credential/example issue; no live provider values introduced |
| Fresh isolated Docker installation, normal sign-in, branded test event, matching guide | PASS at fa92e01b: normal production Compose, new checkout/volumes, real OIDC/PKCE with synthetic issuer, bootstrap removed, native preview/create/branding200, saved logo/theme on both portals, matching guide and separate logout |
| Correct any steps discovered by the actual walkthrough | Initial browser-fixture dependency setup failed before login; installing locked verification dependencies in the fresh checkout corrected it. Private cleanup guard was corrected after independent review and fault-injected controls; original RED and successful raw hashes remain recorded |
| ADR-0001–0005, retention/archive exceptions, integration/manual recovery and optional M5 accuracy | Initial independent Spec review: zero correctness findings |
| Independent Standards review | Initial source review: zero findings |
| Final integrated source review and candidate CI | Pending |

The fresh walkthrough uses separate projects, volumes, ports and synthetic
identity/provider fixtures, preserves existing local installations, and records its
source/image digests, actual commands/outcomes, portal logins, event branding and
cleanup. A Compose health-only smoke does not satisfy the branded-event criterion.
A fixture using seeded sessions is not normal OIDC evidence. Source reviews check
written instructions; they do not replace execution by the walkthrough operator. Live GitHub/Teams
checks require their separately approved external prerequisites.

PR #295 now targets main after the Fleet cutover and PostgreSQL restart fix.
The recorded walkthrough retains its original source and image digests; rebasing
does not make that execution evidence apply to a later runtime. The issue's #120
dependency and final integrated reviews/CI remain open; the scoped branded-event
proof does not establish the complete event lifecycle.
