# MC reviewed-preflight read race

An inherited admin-browser failure on PR277 ae24005 left the return-to-schedule
review open with “The change was not sent.” Root inspected actual completed CI
job121095742818, its error snapshot and network trace. Two close presentation
GETs overlapped the explicit reviewed preflight; no return POST was issued.
The ordinary polling/focus reader incremented the shared latest-request serial,
so the otherwise valid preflight was discarded as superseded.

A deterministic test holds that explicit GET, triggers the existing focus refresh,
then releases it. On unchanged production mainb2f7f5a plus only test/config overlay,
it failed on desktop5.8s and mobile5.6s with the same missing planned-cue outcome.
The assertions and five-second limit are unchanged after correction.

The bounded reader correction reuses the current projection only when the held
response confirms the same revision and both projections still permit control,
within the original root scope and authority epoch. It never installs the
superseded private payload. Different revisions, permission loss and scope
expiry keep their existing rejection/teardown behavior. Background reads continue
while a preflight is held, so they can still reveal revoked authority. This read
is not a mutation receipt and cannot settle or replay an unknown room command.

After rebuilding both production portals, the complete MC browser file passed
36/36 across desktop/mobile viewport sizes in1.2minutes, one worker/no retries.
This includes the original CI failure, deterministic race, held-preflight
permission downgrade, transient outage, private teardown, unknown room changes,
SPA return, once-only pairing secrets, print/hidden/pagehide and late ACK cases.
These browser scenarios use deterministic synthetic HTTP fixtures, not a native
database or actual room display. Focused ESLint passed; production build includes
actual TypeScript build-mode checking.

Local proof ran on the working-tree correction before its commit. Diagnostic
configuration uses isolated preview4191; source test runs in ordinary CI through
the existing MC file. RED artifacts are retained outside the repository at
/private/tmp/hackbox-mc-priority-red; the original CI trace/snapshot at
/private/tmp/hackbox277-mc-trace.zip and hackbox277-mc-error-context.md.

Final immutable-head CI and independent Standards/Spec reviews remain required
before merge. This scoped correction references122 but does not complete its
whole accessibility/device/final-route acceptance.
