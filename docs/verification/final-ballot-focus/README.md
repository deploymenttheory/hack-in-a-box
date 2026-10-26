# Final ballot review focus recovery

## Observed CI failure

Run `40234734654`, job `121293752202`, source `bc595ff8`, failed the
native desktop category-ballot journey at `final-ballots.spec.ts:42`, inside
`vote` line13. After Cancel, the unchanged strict expectation observed the
original Review tie vote button inactive34 times across its15-second bound.
The mobile case passed. This is a real keyboard-focus failure; neither the
mobile pass nor later unrelated production-image proof dismisses it.

The failed native-results-report-proof artifact was inspected privately. Raw
traces contain authenticated fixture transport and remain outside the repository.
Only the following safe timing/method/status facts are recorded:

- Cancel begins at trace17184.55ms.
- A current ballot GET starts at17188.036ms and returns native200 after593.972ms;
  its current candidate-brief GETs finish around17903ms.
- The original-opener focus expectation begins at17245ms while that read is
  pending, and fails at32247ms.

Three hypotheses were considered before the correction:

1. The current read temporarily disables the opener, so returnFocus chooses its
   fallback and never returns after the read settles.
2. A changed current-source signature remounts the editor/opener.
3. Dialog teardown races the hidden form becoming visible even without a read.

## Causal regression and correction

The actual production Alert dialog, useResultDecision, useTodayRead private
scope and separate command-history consumer reproduce hypothesis1 deterministically
with a held same-context synthetic unit read: **RED1.96s**. After the read
settles, the opener is enabled but focus remains on the Decision status heading.
An earlier setup query failed because modal background controls are correctly
hidden; that harness error is not causal RED evidence.

An exact same-render-batch read-start/Cancel regression then exposed the other
part of this seam (**RED1.99s**, earlier four controls passing): dialog teardown
can invoke finalFocus before the form-unhide commit. The original opener may
still be hidden while not yet disabled, so an immediate focus request can leave
focus on body. This demonstrates the combined read/teardown timing rather than
claiming ordinary cancellation alone fails.

The correction reconciles its captured return target after the DOM commit and
leaves an immediate accessible fallback while the opener is disabled. After the same private owner's current read settles, it returns to
the captured opener only if the user has left focus on that fallback or the otherwise unfocused body. It never
restores a detached opener, targets a replacement-source editor, steals focus
from another destination, infers acknowledgement, repeats a write or alters
read/permission/command lifetimes. There is no new timer or retry.

At intermediate source `a88539c2e7a8550d53ededccd341efa64a36bdb7`, four
actual-dialog unit regressions passed **1.15s**. The final five-test correction
also covers same-batch read-start and cancellation (**703ms**). The ordinary
cancel control succeeds without read overlap; same-signature reproduction needs
no remount and distinguishes hypothesis1 from hypothesis2. Current private
scope, connected/enabled/unhidden source and unchanged user destination remain
mandatory before any deferred focus. No temporary diagnostic logging is kept.

## Native verification

The native ballot helper retains its original strict opener-focus expectation.
For each real vote, a supported window-focus current read now overlaps Cancel
deterministically. It holds only the actual fetched native200 response, checks
the accessible fallback while paused, then releases that same response and
requires the original opener to regain focus. No fake successful domain response,
weakened assertion, disabled polling, timer increase or flaky retry is used.

The complete desktop/mobile native reports and overall/category-ballot matrix
remains required before this candidate can be integrated into the cutover PR.


Native verification is explicitly held while the user's latest merged Docker
preview is built and opened. No native/Docker/browser proof was started for this
isolated correction, and no passing-native claim is made. Independent review
and the full native matrix remain integration gates. The frozen mentor advisory
PR and the root cutover worktree were not modified.

Final source checkpoint: `4941ab7b073f6c41e383249fb052bc2fea3f4897`, based on
`076f6162558df2ac3d7fb1ff7c23b3d35dc21784`. Both portal builds/TypeScript
and frontend ESLint pass. The affected focus/current-reader/event-authority
suites pass **10/10 (1.10s)** across three files. These are host unit/static
checks and do not substitute for the pending actual native matrix.


## Completed isolated native focus matrix

At exact isolated source `be49f37df1f39ec8abfd92cc9ee34a08980a5cbd`, all
four actual reports/overall-and-category ballot browser cases passed, package
**110.351s**, test **108.76s**: desktop two cases **52.94s**, mobile two cases
**55.82s**. The deterministic held native200 current-read/cancel overlap retained
strict focus expectations. Fixtures cleaned up normally. This is the isolated
focus proof; later combined PR292 corrections require their own final-source
verification.

## Separate final PR292 CI failures

The final `44089cc0` native failure was a distinct input-acceptance problem,
not the results Cancel helper. Qualification final-card save5 returned native200
with a Technical approach score10 and no evidence, while its other four scores
had one evidence note each. The current source correctly reported an incomplete
card and withheld the signing action. Trace Ctrl+A at46952.745ms preceded a
current-panel GET at46955.4ms (native200757.364ms); global insertText at46975.366ms
ran while the field was paused. The old helper never checked the accepted value.

A second actual CI source `5d8fd041`, run40244553309/job121323256896, missed
a Pending checkbox toggle: Space started31668.432ms, current-panel GET started
31669.666ms (native200615.677ms), then the dependent score correctly remained
disabled. Its other seven qualification cases passed. These are trusted-input
acceptance seams, not permission to force disabled controls or loosen domain
readiness.

The helper's deterministic synthetic browser probes reproduced both failures
before correction: the text stayed at its earlier value and the checkbox stayed
checked. These are browser-helper regressions, not native-domain success proof.
The bounded correction preserves native polling and pauses, records an actual
disabled/inert transition and trusted input-event count, and permits one local
text/checkbox re-entry only after zero input was accepted and that exact element
is enabled again. Replaced elements, unobserved failures and accepted-but-wrong
values fail; command buttons never use this recovery.

The mocked administrator category-source-drift assertion also expected the old
private held dialog and retained text. It now requires disposal, zero POSTs and
a fresh explicit selection with blank inputs, matching the independently reviewed
P1 successful-source-disposal contract. Its responses remain explicitly mocked
and do not substitute for native authority or domain proof.
