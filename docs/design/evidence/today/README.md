# Matched Today UI evidence

These 24 screenshots render the actual application with the same deterministic synthetic API fixtures. They contain no real people, secrets or provider effects. The before baseline is main `c759a42e3d8561326b976db1ecf1966c315599d6`; after is product source `37be520a3e82e191f6fce852e618660b59dc09e0`. Both independent portals use 1280×900 desktop or 390×844 mobile viewports at device scale 1, the same light theme, event, actor fixture, published room cues, released guidance and server observation 14 June 2027.

Before Today had released guidance only. After adds native current-role contexts, current/next published room programme, assigned preparation, owning next actions and separate preparation/live boundaries. Competitor images additionally show the frozen smaller team, private drafts before live formal acceptance, current-account repository observations and controlled credential metadata. Judge images show current assigned preparation/released rubric reading and explicitly unavailable final-scoring assignment facts; future #108/#109 work is not fabricated or claimed as proven.

| Portal | Role fixture | Width | Before | After |
| --- | --- | --- | --- | --- |
| administrator | competitor | desktop | [Before](administrator-competitor-desktop-before.png) | [After](administrator-competitor-desktop-after.png) |
| administrator | competitor | mobile | [Before](administrator-competitor-mobile-before.png) | [After](administrator-competitor-mobile-after.png) |
| administrator | mentor | desktop | [Before](administrator-mentor-desktop-before.png) | [After](administrator-mentor-desktop-after.png) |
| administrator | mentor | mobile | [Before](administrator-mentor-mobile-before.png) | [After](administrator-mentor-mobile-after.png) |
| administrator | judge | desktop | [Before](administrator-judge-desktop-before.png) | [After](administrator-judge-desktop-after.png) |
| administrator | judge | mobile | [Before](administrator-judge-mobile-before.png) | [After](administrator-judge-mobile-after.png) |
| regular | competitor | desktop | [Before](regular-competitor-desktop-before.png) | [After](regular-competitor-desktop-after.png) |
| regular | competitor | mobile | [Before](regular-competitor-mobile-before.png) | [After](regular-competitor-mobile-after.png) |
| regular | mentor | desktop | [Before](regular-mentor-desktop-before.png) | [After](regular-mentor-desktop-after.png) |
| regular | mentor | mobile | [Before](regular-mentor-mobile-before.png) | [After](regular-mentor-mobile-after.png) |
| regular | judge | desktop | [Before](regular-judge-desktop-before.png) | [After](regular-judge-desktop-after.png) |
| regular | judge | mobile | [Before](regular-judge-mobile-before.png) | [After](regular-judge-mobile-after.png) |

Reproduce with `web/playwright.today-visual.config.ts` and `web/e2e/today-visual.capture.ts`, after building the pinned baseline checkout at `/private/tmp/hackbox-today-before-c759` and the current portal artifacts. Capture execution passed 24/24, with no horizontal overflow or API writes. Desktop judge and mobile competitor before/after images were visually inspected; the layouts use the real Fleet controls and responsive event navigation. These captures are separate from behavioural tests and do not claim backend policy validation.
