# Matched survey UI evidence

All people, answers, clocks and API responses are deterministic synthetic fixtures.
No provider is contacted. The before build is main commit
`6e8ab511f00f3c3e8b63c91200a52637bee65119`; the after build is this PR's
completed survey implementation. Both builds use the same
`web/e2e/survey-visual-evidence.spec.ts` and survey-workspace fixture.

The response is an unsent conditional answer. The authoring view is the same
unapproved questionnaire with two questions. Viewports and portal builds match;
the pointer is moved away before capture. Actual Playwright screenshots are
included at desktop and mobile sizes for both separately built portals.

| Portal / surface | Before | After |
| --- | --- | --- |
| admin / response / desktop | [before](before-admin-response-desktop.png) | [after](after-admin-response-desktop.png) |
| admin / response / mobile | [before](before-admin-response-mobile.png) | [after](after-admin-response-mobile.png) |
| admin / authoring / desktop | [before](before-admin-authoring-desktop.png) | [after](after-admin-authoring-desktop.png) |
| admin / authoring / mobile | [before](before-admin-authoring-mobile.png) | [after](after-admin-authoring-mobile.png) |
| participant / response / desktop | [before](before-participant-response-desktop.png) | [after](after-participant-response-desktop.png) |
| participant / response / mobile | [before](before-participant-response-mobile.png) | [after](after-participant-response-mobile.png) |
| participant / authoring / desktop | [before](before-participant-authoring-desktop.png) | [after](after-participant-authoring-desktop.png) |
| participant / authoring / mobile | [before](before-participant-authoring-mobile.png) | [after](after-participant-authoring-mobile.png) |

Compared with the accepted Fleet reference: flat canvas, one focused editor,
icon-labelled actions, compact native controls, clear current section and
review-before-write continuity. The old combined schedule/privacy/question
fieldset is replaced by focused question/schedule/privacy/preview sections.
No decorative report charts or fabricated aggregate values are added.

Reproduce after building the desired checkout:

```sh
cd web
SURVEY_EVIDENCE_DIR=/private/tmp/survey-admin-evidence npx playwright test --config playwright.survey-evidence.config.ts
SURVEY_PORTAL=participant SURVEY_EVIDENCE_DIR=/private/tmp/survey-participant-evidence npx playwright test --config playwright.survey-evidence.config.ts
```

The evidence suite is explicit and separate from behavioral CI. It waits for the
actual production controls and asserts narrow-screen overflow; owning journeys
cover authority, focus, native mutations and private asynchronous lifetimes.
