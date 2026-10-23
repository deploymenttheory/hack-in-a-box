# Fleet UI study: 100 qualities and their application

Studied 23 October 2026. Source pinned to Fleet commit `dc2161d163e3391bf671e0a28daff9477fe59186`. This studies the application UI, not GitHub's repository chrome or Fleet's marketing website. Public screenshots were visually inspected; current component/style source was read. Fleet is not claimed to use Tailwind. The prototypes translate the observed patterns into a new Tailwind v4 token system.

Evidence keys below link to primary sources. “Application” describes our design adaptation, not an assertion that Fleet implements Hack-in-a-Box's domain rules. Shared qualities apply to A, B and C; form/table-specific qualities apply wherever that component appears in either portal. The three layouts use the same behavior layer, not three disconnected mockups.

## Sources

- **S1 — visual system:** [colours](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/styles/var/colors.scss), [type scale](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/styles/var/fonts.scss), [spacing](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/styles/var/padding.scss), [font faces](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/styles/global/_fonts.scss).
- **S2 — application chrome:** [CoreLayout](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/layouts/CoreLayout/CoreLayout.tsx), [SiteTopNav](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/top_nav/SiteTopNav/SiteTopNav.tsx).
- **S3 — icon system:** [Icon](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/Icon/Icon.tsx), [button icon and accessibility behavior](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/buttons/Button/Button.tsx).
- **S4 — tables/toolbars:** [TableContainer](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/TableContainer/TableContainer.tsx), [data table styling](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/TableContainer/DataTable/_styles.scss).
- **S5 — collection journeys:** [ManageHostsPage](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/pages/hosts/ManageHostsPage/ManageHostsPage.tsx), [Pagination](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/Pagination/Pagination.tsx).
- **S6 — forms, API states, routing and command conventions:** [frontend patterns](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/docs/patterns.md).
- **S7 — action styling:** [button styles](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/buttons/Button/_styles.scss).
- **S8 — focused decisions:** [Modal](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/Modal/Modal.tsx), [modal styles](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/Modal/_styles.scss).
- **S9 — state communication:** [EmptyState](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/EmptyState/EmptyState.tsx), [StatusIndicator](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/StatusIndicator/StatusIndicator.tsx), [TooltipWrapper](https://github.com/fleetdm/fleet/blob/dc2161d163e3391bf671e0a28daff9477fe59186/frontend/components/TooltipWrapper/TooltipWrapper.tsx).
- **S10 — visual evidence:** [published Hosts screenshot](https://fleetdm.com/images/articles/enrolling-linux-devices-for-fleet-management-4-951x223%402x.png), [Add Hosts screenshot](https://fleetdm.com/images/articles/enrolling-linux-devices-for-fleet-management-2-605x332%402x.png), from [Fleet's Linux enrollment guide](https://fleetdm.com/articles/enrolling-linux-devices-for-fleet-management).

The screenshots are illustrative published product states, not proof of the precise current deployment. Where they differ, the pinned frontend source is the current implementation evidence. Images are retained locally as study references; Fleet branding/assets are not copied into our proposals.

## Visual hierarchy — Q01–Q10

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 01 | A quiet main canvas gives data priority. | S1, S10 | One continuous page surface; remove dashboard-card grids. |
| 02 | Secondary surfaces are only slightly different. | S1 | Low-contrast navigation and table headers. |
| 03 | Navy/neutral text carries most of the interface. | S1 | Colour does not decorate every section. |
| 04 | Strong colour identifies interaction. | S1, S7 | One recognisable primary action colour. |
| 05 | Status colours are separate from action colours. | S1, S9 | Labelled status icons, with independent primary buttons. |
| 06 | A small type scale establishes hierarchy. | S1 | 24px page title, 16px section title, 14px body, 12px metadata. |
| 07 | Inter is the main interface face. | S1 | Shared sans-serif type; remove oversized editorial greetings. |
| 08 | Regular and semibold weights do the work. | S1 | Avoid heavy headings on every row. |
| 09 | Spacing follows a repeatable scale. | S1 | Shared 4/8/12/16/24/32px roles. |
| 10 | Controls have modest corner radii. | S7 | 4px controls; no repeated rounded containers. |

## Navigation and scope — Q11–Q20

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 11 | Main destinations occupy a compact top bar. | S2, S10 | Visible icon-and-label navigation in both portals. |
| 12 | The active destination has a restrained underline. | S2, S10 | Current section stays obvious without a large filled tile. |
| 13 | The current collection/scope sits beside its heading. | S5, S10 | Event context directly above the teams or task list. |
| 14 | A dropdown changes scope in place. | S5, S10 | Event picker clears stale selection and pagination. |
| 15 | Account tools are separated from main work. | S2 | Compact profile menu with account and sign-out actions. |
| 16 | Local section navigation stays local. | S6, S8 | Overview/access/activity tabs belong inside a team detail. |
| 17 | Returning to a collection is explicit. | S6 | Named back link preserves the collection filter. |
| 18 | Global navigation supports keyboard search. | S6 | Command search routes to relevant screens. |
| 19 | Commands use verb-first, sentence-case labels. | S6 | “Review access”, “Save draft”, “Confirm submission”. |
| 20 | Entity actions stay with the selected entity. | S6 | Team changes appear on rows/details, not global search. |

## Icons and action hierarchy — Q21–Q30

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 21 | One shared icon component keeps shapes consistent. | S3 | Use the same Lucide stroke set throughout. |
| 22 | Action icons use a restrained size. | S3 | 16px navigation, search, export and row-action icons. |
| 23 | Icon colour follows the action's text. | S3 | CurrentColor icons remain readable in every variant. |
| 24 | Icons identify the kind of action. | S3, S10 | Search, filter, export, columns, external reference and overflow are distinct. |
| 25 | Important actions include text labels. | S3, S10 | No icon-only primary actions. |
| 26 | Icon-only controls need an accessible name. | S3 | Label close, more, command and clear controls. |
| 27 | The page's main action is easy to locate. | S5, S10 | Primary action sits beside the title, not inside an arbitrary card. |
| 28 | Secondary actions have less visual weight. | S7 | Export/columns are quiet text-and-icon controls. |
| 29 | Hover, press and focus are distinct. | S7 | Shared interaction variants, visible keyboard focus. |
| 30 | A loading action retains its dimensions and variant. | S3, S6 | Spinner and loading text replace content without a layout jump. |

## Data table anatomy — Q31–Q40

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 31 | A table is the collection's main content. | S4, S10 | Teams and people are rows, not a grid of cards. |
| 32 | Headers use a subtle contrasting surface. | S4, S10 | Quiet header band and fine separators. |
| 33 | Rows align comparable information into columns. | S4 | Team, members, repository and access state share baselines. |
| 34 | Entity names act as the detail entry point. | S5, S10 | Named team links open the selected detail journey. |
| 35 | Supporting metadata sits below the main identifier. | S5 | Small published/draft context under team names. |
| 36 | Sort affordances live in the column heading. | S4 | Click name/member headers to change actual row order. |
| 37 | The sort direction remains visible. | S4 | Directional arrow and aria-sort reflect the active order. |
| 38 | Row selection has its own column. | S4, S10 | Labelled native checkboxes, separate from entity links. |
| 39 | Selection changes the available actions. | S4 | Selected-count toolbar exposes a scoped review action. |
| 40 | Clearing selection is explicit. | S4 | A visible “Clear selection” action restores the normal toolbar. |

## Finding and managing records — Q41–Q50

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 41 | A result count anchors the toolbar. | S4, S10 | Count updates with filters and search. |
| 42 | Search belongs next to the collection. | S4, S10 | Search team/person names without leaving the page. |
| 43 | Search has an identifiable magnifier. | S4, S10 | Visible icon and a programmatic input label. |
| 44 | Status filtering is a compact dropdown. | S4, S10 | All / needs attention / ready / unused states. |
| 45 | Filters and search can combine. | S4 | Apply both predicates and show the resulting count. |
| 46 | Columns can be adjusted. | S4, S10 | A column menu changes optional visible columns. |
| 47 | Export is available from the collection toolbar. | S5, S10 | Preview the exact safe filtered CSV fields; production download uses authorised data. |
| 48 | Pagination has named previous/next actions. | S5 | Real page slices with boundary-disabled controls. |
| 49 | Filter changes reset an obsolete page. | S4 | Return to the first page after narrowing results. |
| 50 | Single-page collections omit unnecessary paging. | S5 | Hide pagination when all matching records fit. |

## Forms and validation — Q51–Q60

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 51 | Fields have concise labels. | S6 | Keep labels visible in draft and review forms. |
| 52 | Help text is separate from error text. | S6 | URL guidance remains distinct from validation feedback. |
| 53 | Pristine fields do not start in an error state. | S6 | No red warnings on first opening the form. |
| 54 | Edited fields validate at a sensible boundary. | S6 | Validate a touched field on blur. |
| 55 | Submit validates the whole form. | S6 | Surface all required evidence errors together. |
| 56 | An invalid submit does not call the operation. | S6 | Keep the form open and focus the first invalid input. |
| 57 | An editable invalid form still allows a submit attempt. | S6 | Let the action reveal how to fix it rather than silently disabling it. |
| 58 | One actionable error appears per field. | S6 | “Enter a valid demo URL”; no stacked generic warnings. |
| 59 | Optional empty fields are valid. | S6 | An optional note does not block the next step. |
| 60 | Irrelevant dependent errors clear. | S6 | Clear stale format errors when a field is removed or no longer required. |

## Form lifecycle — Q61–Q70

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 61 | Whitespace is handled consistently. | S6 | Trim URL and label inputs before validation/review. |
| 62 | Input length follows the contract. | S6 | Bound evidence labels and URLs; do not introduce unbounded fields. |
| 63 | A native form owns submission. | S6 | Enter, the local form event and the review action share one validation boundary in the preview. |
| 64 | In-flight actions prevent duplicate submission. | S6 | Both button state and an intent guard prevent double clicks. |
| 65 | Fields freeze while a request is in flight. | S6 | Submitted values cannot change behind a pending confirmation. |
| 66 | Failures restore editing. | S6 | Restore fields and retain the same draft after a known failure. |
| 67 | Field errors and transport errors differ. | S6 | Inline validation versus a clear save failure message. |
| 68 | Cancelling a focused form is available. | S6, S8 | Cancel returns to the source context with the draft intact. |
| 69 | Successful work gives visible confirmation. | S6 | Completion message, updated row, and next relevant action. |
| 70 | Sensitive values stay out of ordinary display. | S6 | Never render installation tokens or App private keys. |

## Focused decisions and continuity — Q71–Q80

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 71 | A modal has one clear task title. | S8, S10 | “Review repository recovery” or “Confirm submission”. |
| 72 | Close has a predictable position. | S8, S10 | Labelled close control at the top right. |
| 73 | Escape exits a dismissible task. | S8 | Keyboard escape and focus return work. |
| 74 | The background remains recognisable. | S8, S10 | Keep source-list context behind a focused decision. |
| 75 | Task width matches its content. | S8 | Narrow confirmation, wider form, no full-screen tiny dialog. |
| 76 | Local options sit inside the current task. | S8, S10 | Choose an operation before reviewing its consequences. |
| 77 | Main and cancellation actions have clear hierarchy. | S7, S8 | Quiet Back/Cancel and one specific confirmation action. |
| 78 | Modal contents are separated from the action footer. | S8 | Stable footer with the next step and current step count. |
| 79 | The task remains bound to the selected entity. | S5, S8 | Team name stays in list, detail, review and confirmation. |
| 80 | Task completion preserves the working context. | S5, S8 | Return to the source list/detail; its record updates and filters remain intact. |

## States and feedback — Q81–Q90

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 81 | Status includes a text label. | S9, S10 | “Needs review” remains understandable without colour. |
| 82 | Status and warning icons reinforce meaning. | S9 | Check, clock and warning icons distinguish conditions. |
| 83 | An empty collection has a purposeful next action. | S9 | Draft event explains publication and offers “Review allocation”. |
| 84 | No matches is different from no records. | S4, S9 | A clear-filters path restores an existing collection. |
| 85 | Collection headings survive state changes. | S4 | Keep scope and toolbar visible during load/no-match states. |
| 86 | Loading indicates an operation is underway. | S3, S9 | Spinner and action text are announced; no fake success. |
| 87 | Disabled actions have contextual explanations. | S6, S9 | Explain frozen allocation or a not-yet-open submission window beside the control. |
| 88 | Feedback appears near the completed work. | S6 | Toast/status message plus updated row or accepted revision. |
| 89 | Explanatory tooltips supplement visible text. | S9 | Icon hints are optional; essential instructions stay visible. |
| 90 | Details reveal evidence progressively. | S5, S8 | Activity and recovery evidence appear after selecting a team. |

## System consistency and behavior — Q91–Q100

| # | Fleet quality | Evidence | Application to A/B/C |
|---|---|---|---|
| 91 | Shared colour roles have a single owner. | S1 | CSS-first Tailwind v4 semantic tokens feed every proposal. |
| 92 | Dark mode changes roles, not arbitrary components. | S1 | Paired light/dark tokens keep the same hierarchy. |
| 93 | A reusable button variant system keeps actions consistent. | S3, S7 | Shared primary, secondary, ghost, destructive and link roles. |
| 94 | API-derived permission gates match the destination. | S6 | Prototype phase capabilities model, rather than bypass, backend gates. |
| 95 | Navigation keeps current scope in mind. | S6 | Account/event changes clear stale selected entities. |
| 96 | Query/search and local editing have different lifecycles. | S6 | Search does not overwrite an in-progress submission draft. |
| 97 | User-supplied strings are treated as data. | S6 | Escape labels and notes before inserting them into the prototype. |
| 98 | Wide data is contained, not allowed to break the page. | S4 | Reflow rows on phones and constrain table overflow where needed. |
| 99 | Keyboard handling respects native controls. | S3, S6, S8 | Native buttons, fields and form submit; scoped command shortcut. |
| 100 | Tests focus on visible outcomes. | S6 | Verify complete journeys, validation, retained context and changed rows. |

## What changes materially in this iteration

All three use a flat application canvas, an icon-led navigation/toolbar system and deliberate list → detail → action → review → confirmation journeys. A uses a wide collection with a focused modal; B keeps a detail inspector alongside the collection; C uses an action queue and a full working step. The difference is the work arrangement, not just the accent colour.

Complete local journeys to try in each proposal:

1. Admin: Teams → Team Atlas → Review recovery → inspect evidence → reconcile → return to the updated team.
2. Admin: Teams → Team Orion → Review access → inspect the exact invitations → approve → see queued/pending state, not a fabricated provider acceptance.
3. Participant: Submission → edit evidence → validate → save draft; select the Live hack scenario → review → confirm → see the accepted revision.
4. Filter/search/select records → review the selected scope → clear selection; use columns, sorting and actual pagination.

The Tailwind compiler produces a namespaced utility layer from `fleet-tokens.css`; semantic product classes own the prototype's component anatomy. The artifact is a design discussion surface with synthetic data and no API calls or external mutation. GitHub access planning remains dependent on the real #93 contract. It is not deployed product code.

Intentional adaptations: keep labels visible when errors appear, retain 44px targets on touch, support phone layouts instead of copying Fleet's unsupported-screen treatment, and preserve this product's stricter uncertain-write and publication rules. No Fleet logo, source component implementation or private data is reused.
