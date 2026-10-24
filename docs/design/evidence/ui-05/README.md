# UI-05 actual-screen evidence

Synthetic actual application screens, captured with Chromium at the project’s desktop (1280px) and mobile (iPhone13) projects. Before is main `01958dd01660482b7f8964c4da0349612021aba7`; after is this PR’s main01958-based resource implementation. The screenshots are rendered production builds with native API fixtures. No real event, account, credential or provider was accessed.

Both metadata screenshots show masked values before any audited request. Tests assert zero reveals and no rendered synthetic private value. Revealed credentials are intentionally absent from these published images.

| Journey | Before desktop | After desktop | Before mobile | After mobile |
|---|---|---|---|---|
| Resource directory | [Before](before/resource-directory-desktop.png) | [After](after/resource-directory-desktop.png) | [Before](before/resource-directory-mobile.png) | [After](after/resource-directory-mobile.png) |
| Resource reader | [Before](before/resource-reader-desktop.png) | [After](after/resource-reader-desktop.png) | [Before](before/resource-reader-mobile.png) | [After](after/resource-reader-mobile.png) |
| Content authoring | [Before](before/content-authoring-desktop.png) | [After](after/content-authoring-desktop.png) | [Before](before/content-authoring-mobile.png) | [After](after/content-authoring-mobile.png) |
| Team deliverables | [Before](before/team-deliverables-desktop.png) | [After](after/team-deliverables-desktop.png) | [Before](before/team-deliverables-mobile.png) | [After](after/team-deliverables-mobile.png) |
| Private metadata | [Before](before/private-metadata-desktop.png) | [After](after/private-metadata-desktop.png) | [Before](before/private-metadata-mobile.png) | [After](after/private-metadata-mobile.png) |

Content now uses focused mounted authoring sections and a separate exact-version review/release path; released reading uses the shared directory and focused prose; team work distinguishes personal preparation, private draft and formal acceptance; private access shows current metadata and requires explicit audited reveal review. Desktop and mobile images were inspected for readable actions, wrapping, private-value masking and horizontal overflow. Inspection caught the desktop Version/type split, corrected by narrow private-access table styles and a final24-case secret/evidence run.

Reproduction: build both portals with `npm run build`; run `playwright.resources.config.ts` resource/content/deliverable and private-metadata cases. Public examples use `RESOURCE_PORTAL=participant`. Native owner fixtures preserve current audience, versions, CSRF, published/live/cutoff capability, protected downloads and no-replay assertions. Baseline captures used the same synthetic journeys in a detached main checkout with isolated preview ports4336/4337.
