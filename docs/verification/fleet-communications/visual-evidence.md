# Production communications UI: before and after

These are actual Chromium screenshots from repository production builds using deterministic synthetic fixtures only. The original source is `e159ebfd4f54ee8a67e5924eb216cb2bbd8372ec`, built in an isolated detached checkout; the redesigned source is `199a13c` plus the screenshot-only inbox test addition. No screenshots were generated or edited. No email, Teams or live provider request was made.

Both sources run the same synthetic campaign approval scenario: saved reminder, confirmed competitor audience, changed message version, explicit retained preview and approval. The redesigned workflow displays its Review & TEST step; the original displays all editing/setup controls in one page. Inbox pairs show the same synthetic recipient's original HTML-escaped message after toggling read then unread. The new collection is also shown after returning keyboard focus to the actual Open action. Regular-portal checks use the same inbox fixture; administrator campaign tools retain their portal boundary.

Viewports are Playwright Desktop Chrome (1280 × 720) and iPhone 13 (390 × 664 CSS pixels, Chromium), with full-page capture and the fixture's Europe/London time zone.

| Actual UI state | Original desktop | Redesigned desktop | Original mobile | Redesigned mobile |
| --- | --- | --- | --- | --- |
| Approved campaign | [Before](images/before-campaign-desktop.png) | [After](images/after-campaign-desktop.png) | [Before](images/before-campaign-mobile.png) | [After](images/after-campaign-mobile.png) |
| Unread message detail | [Before](images/before-inbox-desktop.png) | [After](images/after-inbox-desktop.png) | [Before](images/before-inbox-mobile.png) | [After](images/after-inbox-mobile.png) |
| Return to inbox collection | — | [After](images/after-inbox-collection-desktop.png) | — | [After](images/after-inbox-collection-mobile.png) |

Reproduction: run the matching `approval binds saved content and timing` and `keyboard opens safe preview` scenarios from `web/e2e/campaigns.spec.ts` and `web/e2e/inbox.spec.ts` against each source's production bundle. The original source uses its own original action labels; synthetic source values, recipient, workflow outcome and viewports are the same. Baseline capture passed six applicable portal/device checks; redesigned capture passed the broader 102-check suite plus the four matching detail captures. These images do not prove backend/provider delivery or replace final accessibility/state checks.
