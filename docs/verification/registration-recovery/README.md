# Registration recovery follow-up (#236)

Merged PR #252 left command recovery inside mounted editors. The actual event
section control could discard an unknown write and recreate an idle editor.
This follow-up uses the existing event-scoped, memory-only planning history.
Pending writes prevent departure. Unknown attempts survive local replacement;
current GETs cannot settle them or permit replay. Apply's explicit captured UUID
retry remains its separate native contract.

## Reproduction and initial checkpoint

The actual built admin portal on merged main `22a42bc` failed all four focused
`registration-recovery.spec.ts` desktop cases: unknown form and policy reentry,
long-name tablet overflow (1697px document at 1024px viewport), and focused
collection controls becoming hidden at the 72rem canvas breakpoint.

The same four cases now pass (3.5s). TypeScript and both portal builds pass.
The fix retains the original operation/resource/revision in existing scoped
memory, narrows nowrap to column headers, and synchronizes owner focus with
its existing 72rem CSS boundary. Further pending, denial, account-scope, native
request and both-portal regression verification remains in progress. This is
an early draft, not a completed CI/review claim.

Local logs: `/private/tmp/registration-recovery-red.log`,
`/private/tmp/registration-recovery-green.log`, and
`/private/tmp/registration-recovery-build.log`. Browser artifacts are synthetic;
request headers and full traces are not published.
