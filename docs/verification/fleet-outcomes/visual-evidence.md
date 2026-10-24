# Actual Fleet outcomes before/after evidence

Before: production app at `e9d7c4665a582feca6543718861ff56308d82fe9`.
After: final PR #261 candidate based on `ea88901f7204a0a3a441ef03610e97e6b5297740`.

Both builds use identical synthetic fixture data and device configurations. The
baseline production source is unchanged; only fixture/capture files were copied.
Directory/source captures are actual portal viewports. Publication/owner-plan
captures show complete actual workflow regions. They retain CSS width/DPR and
temporarily extend viewport height to fit the app scrollport, then restore it;
input focus is cleared only for capture. No product styles or data are changed. Capture cases send no domain writes.
These are rendered production components, not proposed wireframes.

| Portal / viewport / task | Before | After |
| --- | --- | --- |
| admin / desktop / directory | [Before](images/before-admin-desktop-directory.png) | [After](images/after-admin-desktop-directory.png) |
| admin / desktop / source | [Before](images/before-admin-desktop-source.png) | [After](images/after-admin-desktop-source.png) |
| admin / desktop / publication | [Before](images/before-admin-desktop-publication.png) | [After](images/after-admin-desktop-publication.png) |
| admin / desktop / owner-plan | [Before](images/before-admin-desktop-owner-plan.png) | [After](images/after-admin-desktop-owner-plan.png) |
| admin / mobile / directory | [Before](images/before-admin-mobile-directory.png) | [After](images/after-admin-mobile-directory.png) |
| admin / mobile / source | [Before](images/before-admin-mobile-source.png) | [After](images/after-admin-mobile-source.png) |
| admin / mobile / publication | [Before](images/before-admin-mobile-publication.png) | [After](images/after-admin-mobile-publication.png) |
| admin / mobile / owner-plan | [Before](images/before-admin-mobile-owner-plan.png) | [After](images/after-admin-mobile-owner-plan.png) |
| participant / desktop / directory | [Before](images/before-participant-desktop-directory.png) | [After](images/after-participant-desktop-directory.png) |
| participant / desktop / source | [Before](images/before-participant-desktop-source.png) | [After](images/after-participant-desktop-source.png) |
| participant / mobile / directory | [Before](images/before-participant-mobile-directory.png) | [After](images/after-participant-mobile-directory.png) |
| participant / mobile / source | [Before](images/before-participant-mobile-source.png) | [After](images/after-participant-mobile-source.png) |

Review focus: flat white/navy/teal canvas and dividers, production field borders
and focus, icon/text primary actions, explicit preparation/review stages, separate
proposed/evidenced/published information, truthful native states and mobile wrapping.
The screenshot fixtures deliberately include inert hostile-looking source text.

The [manifest](visual-manifest.json) records source/fixture/image SHA-256 values.
Independent source/state/authority checks are recorded in the [contract map](contract-map.md).

Final review delta: refreshed only AFTER directories (both portals/widths) and
AFTER publication/owner-plan regions for the scoped one-line Open action and
plain-language approval/media copy. BEFORE and AFTER source images remain the
original matching captures. The manifest distinguishes this recapture generation.
