# Optional certificate backend verification

This is the M5 #164 backend; #165 owns its separate interface. It adds no M4 gate,
notification sender, public credential directory or accreditation claim.

The five native parents in [proof.json](proof.json) exercise real PostgreSQL and
production services. Award positives use actual independent judge cards and the
native prepare → confirm → explicit publish owner in both judging modes. HTTP
uses synthetic authenticated sessions and actual routing/CSRF/contract validation;
it does not claim OIDC sign-in proof.

The first schema-head omission and the subsequent test-only downgrade API mistake
are retained as actual RED evidence. The latter command passed four parents but
failed the final rollback assertion in the attendance parent. Its corrected,
source-frozen parent passes with actual Goose `DownTo`; production/API/app/HTTP
bytes and the other test bodies are unchanged. The receipt identifies each source
and command independently, rather than calling the earlier whole command green.

The HTTP parent can export its authorized print response to a private directory:
set `HACKBOX_CERTIFICATE_PRINT_ARTIFACT` to an absolute HTML filename. It exports
only synthetic certificate HTML plus filtered CSP/cache headers, never cookies.
From the repository root, render that exact output with:

```sh
node docs/verification/certificates/print-browser.cjs /absolute/private/export-directory
```

The directory contains `certificate.html` and `certificate.html.headers.json`.
Four Chromium cases cover desktop/mobile screen and print, accessible structure,
no horizontal overflow, no serious/critical Axe findings, no external requests,
and actual response-policy rejection of injected scripts/images. This verifies
the real HTTP-produced document in an isolated server; it is not an application
session or future certificate UI walkthrough. Private raw logs stay outside Git.

The rebase snapshot reconciles all 18 historical files; the final documentation
link/status change is separate. All 17 code/API/test files remain byte-equal.
Independent reviews and final-head CI remain required before this Draft is ready. No release acceptance follows from this
receipt alone.

Final-head CI at `fb0012fb` exposed an inherited historical-schema test that
validated migration55 retention and then incorrectly required runtime schema55
after the new head became56. The test-only correction keeps every pinned54→55
rollback/history assertion, upgrades explicitly to current head before runtime
compatibility checking, and rechecks retained correction history. The exact
additional native owner passes at `9db8318c`; its source/log/resource hashes are
recorded separately. The same CI also hit the existing whole HTTP-package
10-minute budget; integration of the separately reviewed exhaustive batching
fix in #303 and fresh final-head CI remain required. Neither failure is erased.
