# Registration workflow evidence

These are actual production portal screens with synthetic fixtures, not replacement mockups. Before uses frozen parent commit `2258c7e68de2c89cad3013dc175ee6ddcc730c6b`; after uses the registration branch. Desktop is Chromium Desktop Chrome, mobile is Chromium iPhone 13. The evidence journey also asserts no horizontal overflow.

| Surface | Desktop | Mobile |
| --- | --- | --- |
| Participant application | [Before](before-application-desktop.png) · [After](after-application-desktop.png) | [Before](before-application-mobile.png) · [After](after-application-mobile.png) |
| Applicant review | [Before](before-applicant-review-desktop.png) · [After](after-applicant-review-desktop.png) | [Before](before-applicant-review-mobile.png) · [After](after-applicant-review-mobile.png) |
| Versioned form builder | [Before](before-form-builder-desktop.png) · [After](after-form-builder-desktop.png) | [Before](before-form-builder-mobile.png) · [After](after-form-builder-mobile.png) |

The surrounding event workspace belongs to UI-02. These captures intentionally show the actual composition on the frozen shell prerequisite, including that existing context. UI-03 owns the registration, applicant and form surfaces below it.

Run from `web` after building both portals:

```sh
npx playwright test --config playwright.registration-evidence.config.ts
```

For baseline evidence, build both portals in a detached checkout, then set `REGISTRATION_EVIDENCE_BASELINE=1` and `REGISTRATION_EVIDENCE_CHECKOUT` to its absolute `web` directory. The same synthetic journey runs against both builds.
