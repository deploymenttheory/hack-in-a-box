# Observed defects and retests

## Admin brand accessible name omits visible portal identity

- Actual native PostgreSQL/production admin MC scan failed
  `label-content-name-mismatch` on `.fleet-shell-brand` (serious).
- Visible text was `Hack in a Box · Admin`; the explicit accessible label was
  `Hack in a Box home`. The regular portal's visible wording did not fail.
- Correction: preserve visible wording in the admin label plus the home action;
  regular label remains explicit. Existing account navigation checks require the
  exact corresponding portal label, not a loose substring.
- Same-scope native admin scan passed after correction; no scanner rule is suppressed. Existing exact home-link/account regression passed administrator 2/2 (9.1s) and regular 2/2 (7.6s), desktop/narrow Chromium. The combined native keyboard/scans then passed 3/3 (11.8s), real-PG/race owner 30.828s. These diagnostics were executed with a dirty worktree; clean-source proof remains required.
- Diagnostic first native run: 2 passed, 1 failed; native/race owner 13.454s.
  This dirty worktree run is not attributed to a clean final source.

## Guide keyboard harness selection

The initial guide test used a Home/ArrowDown sequence that did not select the
intended dark option on this host. This is a harness failure, not evidence of a
product theme defect. Native keyboard type-ahead selected dark correctly; combined guide 2/2 passed in 5.8s.
No direct DOM mutation or forced selection was used.

A separate native MC test initially omitted its mandatory room-change reason. It now checks the actual required-field error, scans that state, enters the reason by keyboard, and proceeds to review/correction. This was a test omission, not a product defect. One early attempted correction command used the wrong working directory; its following guide run repeated unchanged source and is not counted as new corrected proof.

Full browser/device/screen-reader/zoom and other final first-release findings
remain to be evaluated. This register is not exhaustive or a no-defects claim.

## Review-inventory extraction correction

The initial standards inventory parser crossed the obsolete 4.1.1 section into
4.1.2's level marker, retaining the former and omitting the latter while its total
count still equalled 55. Manual identifier inspection caught this before draft
publication. The corrected extraction bounds each criterion's level to its own
section, excludes obsolete 4.1.1 and includes current 4.1.2. All rows remain
not evaluated; a correct count alone is not completeness proof.

## Registration native-select test sequence

The new pre-admission test reached real field rejection and correction, then its
Enter key on the team-preference select implicitly submitted the native form into
review. Its next step incorrectly expected the earlier Review application button.
This was a harness sequence error, not a product defect. Native type-ahead commits
the choice, then Tab reaches the explicit review action. The targeted corrected
registration run passed; the team case separately passed its actual native
create/join/approval and committed-but-lost-response assertions. No assertion,
domain gate or product authority was weakened.
