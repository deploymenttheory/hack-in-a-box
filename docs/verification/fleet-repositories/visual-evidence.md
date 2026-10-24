# Repository task visual evidence

The actual production baseline is `bc350d8c3145e8cb1787a840568a8f7b42cf349f`.
It has no repository consumer or Repositories destination. The before images
therefore show the existing adjacent task context: admin Team allocation and
regular Team access. They do not pretend an old repository page existed.

The after images initially used production implementation `871fed83aeffa3a226cd227bf1191db411377405`.
The two approval-review images were recaptured at runtime source
`bea455107aab4b90f915b349cdc9105942c3e817` after adding captured human-readable
event/team/repository labels. The other ten after scenes are unaffected by that
review-only composition change.
Only the new test harness and verification documentation were uncommitted during
capture; no product styles or runtime code were changed for screenshots.

`web/e2e/repository-evidence.spec.ts` is the identical synthetic harness in both
isolated checkouts. Both use the same event, team, person, native numeric repository,
plan, pending invitation, App and template. Native domain POST is forbidden and
asserted absent; no installation, provisioning or invitation is sent. The baseline
additions are untracked test harness/config only, not product changes.

Desktop uses the standard Desktop Chrome project (1280 CSS pixels wide); mobile
uses the standard iPhone 13/Chromium project (390 CSS pixels wide). Browser
viewport height is extended by the actual capture region height plus 500 pixels
so complete reading/action regions fit; width, device pixel ratio and product
styles are unchanged. The captured target is the actual planning layout,
inspector or review region, not reconstructed markup. Every scene asserts no
horizontal document overflow.

Run each portal with `REPOSITORY_EVIDENCE_DIR` pointing to this directory’s
`images` subdirectory. Add `REPOSITORY_BASELINE=1` only for the baseline checkout.
Build both portals in the respective checkout before running the same test via
its normal Playwright administrator and regular configurations. Actual terminal
results, image dimensions/hashes and visual review are recorded with the final
candidate below; a harness alone is not evidence of executed screenshots.

## Executed captures and visual review

- Both actual production baseline builds PASS: `/private/tmp/hackbox94-baseline-build.log`.
- Current captures: administrator **2/2**, regular **2/2** PASS, with all twelve
  after images and zero domain POST. Logs `/private/tmp/hackbox94-after-{admin,public}-evidence.log`.
- Actual baseline captures: administrator **2/2**, regular **2/2** PASS, with four
  before images and explicit missing-Repositories assertions. Logs
  `/private/tmp/hackbox94-before-{admin,public}-evidence.log`.
- After the bounded review-label correction, both affected approval scenes were
  recaptured **2/2 PASS**, using `REPOSITORY_EVIDENCE_ONLY=approval-review`.
  Log `/private/tmp/hackbox94-label-evidence.log`; other scenes were not replaced.
- All sixteen final PNGs were opened and inspected. Reading regions and action
  labels remain visible, long identifiers wrap, regular UI has no administration,
  and installation review shows saved App/organisation metadata without secrets.
  Approval now leads with captured readable labels; immutable IDs remain visible.
  The runtime and same-source label/authority/receipt browser proof are recorded
  in [the contract map](contract-map.md). Final independent reviews and CI remain
  separate merge gates.

| Context | Before: real adjacent task | After: new repository directory |
| --- | --- | --- |
| Admin desktop | [Team allocation](images/before-admin-desktop-adjacent-task-context.png) | [Repositories](images/after-admin-desktop-directory.png) |
| Admin mobile | [Team allocation](images/before-admin-mobile-adjacent-task-context.png) | [Repositories](images/after-admin-mobile-directory.png) |
| Regular desktop | [Team access](images/before-participant-desktop-adjacent-task-context.png) | [Repositories](images/after-participant-desktop-directory.png) |
| Regular mobile | [Team access](images/before-participant-mobile-adjacent-task-context.png) | [Repositories](images/after-participant-mobile-directory.png) |

## Artifact identity

| PNG | Pixels | SHA-256 |
| --- | --- | --- |
| [after-admin-desktop-access-plan.png](images/after-admin-desktop-access-plan.png) | 335×1373 | `c12a62edd434b538c695be7b17c91aa5189ca8f44cb3784856d824d53fc55459` |
| [after-admin-desktop-approval-review.png](images/after-admin-desktop-approval-review.png) | 335×543 | `1543aa1ed5273165668f21db38cf71021f4a7e2e502e5d69b7f6447baef18f08` |
| [after-admin-desktop-directory.png](images/after-admin-desktop-directory.png) | 1232×898 | `9f265fd3911b50e5e5807f1ce5ffd13ac83a86b690b45e5c017ea69ae82974c0` |
| [after-admin-desktop-installation-review.png](images/after-admin-desktop-installation-review.png) | 1000×142 | `d512039a4af53b4c70d7e19830545c69fa8384150c935754209cc6d5fd11ae51` |
| [after-admin-mobile-access-plan.png](images/after-admin-mobile-access-plan.png) | 1074×4134 | `c75bd29dcb0b8273e8b05fe027f2aad27dff0da94eedb754e927097ca333ba2d` |
| [after-admin-mobile-approval-review.png](images/after-admin-mobile-approval-review.png) | 1074×1488 | `42497738a73064d57e2e571b4fbf5fa492d414f186c4ea2f2132cd49552e778f` |
| [after-admin-mobile-directory.png](images/after-admin-mobile-directory.png) | 1074×3015 | `e34f6d5d94cbc012a2b061838d0a0ec2fcfc8d5c19a9ee52a35248b2f74c132f` |
| [after-admin-mobile-installation-review.png](images/after-admin-mobile-installation-review.png) | 1074×984 | `4a7ac2802581daa0467578e2b35c0cbf58856a198e447d82f0dd3cc433e8cf5b` |
| [after-participant-desktop-directory.png](images/after-participant-desktop-directory.png) | 1232×352 | `323b2914e435779e13ca21604f7fce9422d80e0dca3af00efebefc98cb0fab57` |
| [after-participant-desktop-own-invitation.png](images/after-participant-desktop-own-invitation.png) | 335×827 | `c5828bcb435500e52fd312cd7b05990046e126910abe17c542f4adb476b099d1` |
| [after-participant-mobile-directory.png](images/after-participant-mobile-directory.png) | 1074×1695 | `e7b60fb110b2b912a096243f3ae60751ebf28516f02d6eecc46ac7951a297657` |
| [after-participant-mobile-own-invitation.png](images/after-participant-mobile-own-invitation.png) | 1074×2364 | `6af42f1147387a012579887c49673cf8caa56d01c8323cac04f1477e9f4de4cc` |
| [before-admin-desktop-adjacent-task-context.png](images/before-admin-desktop-adjacent-task-context.png) | 1232×862 | `b76512033f0ed6044afd0b5407a20864fb001d8c6ca98d852cf8588a71ebf370` |
| [before-admin-mobile-adjacent-task-context.png](images/before-admin-mobile-adjacent-task-context.png) | 1074×2712 | `281388378f66c6d8719d119b02d9dd81c4e55e1480afd14a9aa75477566690fc` |
| [before-participant-desktop-adjacent-task-context.png](images/before-participant-desktop-adjacent-task-context.png) | 1232×446 | `699e5abab70639140dd43a33d8e5bdb661fcdbfe9e7affb6a34cb08fbc586cea` |
| [before-participant-mobile-adjacent-task-context.png](images/before-participant-mobile-adjacent-task-context.png) | 1074×1926 | `35450bb5887c70caf60fe24b310703dc725946ddfa641ce8d7b0cc0428ebb272` |
