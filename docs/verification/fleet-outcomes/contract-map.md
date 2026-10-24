# Ideas and outcome follow-through — #242 / UI-09

This report describes the complete present UI implementation in PR #261, based
on main `6e8ab511f00f3c3e8b63c91200a52637bee65119`. The native idea, outcome,
owner-plan, publication and released-project surfaces are rebuilt with the
accepted Fleet foundations. Local acceptance work is complete; the frozen
candidate still requires independent review and exact-head repository CI before
merge. The issue remains open until those gates complete.

## Source and native ownership

| Owner | Implemented responsibility |
| --- | --- |
| `outcome-workspace.tsx` | Own/organiser-review directories, native cursor paging, proposal/source selection, current source and archived-owner detail, owner-review destinations, existing native positive-field project search, current released-project detail and consent-aware native media destinations. Native internal media links use the router, preserving session intent metadata; external reviewed links retain normal external-link semantics. No invented search, totals, routes or authority. |
| `outcome-controls.tsx` | Revision, explicit adoption, deliberate moderation and exact measure references, eligible owner/due-date assignment, owner progress, evidence and bounded reasoned history. Evidence basis, exact decimal strings and unknown/null remain distinct. Private evidence/history disclosures own fresh per-opening readers. |
| `outcome-reviews.tsx` | Optional 30/60/90-day plans with actual reviewed local date/time, IANA timezone/DST choice, native owner selector, exact message/source/channel/provider/privacy binding and separate save/approve/cancel decisions. Native archived-owner exceptions remain unchanged. Teams is never an owner-reminder channel. Rescheduling never resolves an unknown external delivery. |
| `outcome-publication.tsx` | Separately authored positive fields/audience, deliberate source-name sharing, exact current media references and separate draft/publish/withdraw review. A correction remains a draft until explicitly released. Every opening owns its private read/editor; only a known-existence boolean remains above the disclosure. Private source, assessments, owner details and evidence are never prefilled into publication fields. |
| `outcome-decision.tsx` | Production immutable local review, friendly exact-field summaries, explicit confirmation, pending/unknown/stale barriers and actual initiating-control correction focus. Preparation controls stay mounted but hidden during review. Current denial still removes the entire private editor. |
| `outcome-command.tsx` | Current session/preflight, CSRF, original UUID/expected version/payload, exact native receipt, explicit contract-supported same-command retry and read-only original recovery above capability-bound editors. Rejected retries do not settle the original unknown command. |
| `outcome-safety.tsx` / `app-shell.tsx` | Separate actor/draft-session metadata owner above sibling routes and private presentation lifetimes. Whitelisted kind/resource/review/command IDs, expected version, action, pending/unknown state and opaque live-editor identity only. No private bodies, reasons, fields, recipients, URLs, browser persistence or other owners' capsules. |
| `outcome-read.ts` | Fresh per-mount/current reads and child projection lifetimes, actor/draft scope, exact-key cleanup and late-response fencing. Parent permission restoration does not restore an earlier private projection. |
| `outcome-api.ts` | Existing generated native types and routes. Original receipts use `command_id`; current resource facts never establish an earlier acknowledgement. |
| `design-system/outcomes.css` / guide | Scoped flat token-based sections/dividers, native controls, responsive facts/records and private print exclusion. The independent guide imports actual production review/focus controls with synthetic local owners, copyable usage and accessibility/anti-pattern guidance. |

Native services remain the authority: `internal/outcomes/{service,read,model,
measurements,receipts,publication,projects,plans,reviews,review_model,
review_delivery}.go`, supporting automation/communications owners, the generated
API and current database policy. No provider, schema, generated-contract or
backend-policy expansion was introduced.

The only supporting backend correction is the existing HTTP list adapter's UUID
argument order. It now forwards `(actor, cursor, mode, originalOptionalEventID,
limit)` in the declared native service order. Omitted/nil event semantics remain
intact. Real HTTP/PostgreSQL regression tests cover distinct own/default pages,
selected-event review authorisation/filter/cursor, wrong-event/role denial and
missing-event validation. Direct service tests alone did not expose this defect.

## Preserved boundaries

- General organisation ideas remain available without event-directory/config
  authority. Team output still requires the actual native release/live boundary.
- Status and proposed value never imply demonstrated adoption, ROI, awards or
  publication. Self-report, estimate and dated demonstrated evidence are explicit.
- Owners and exact source versions remain native. Assignment does not approve a
  reminder; archive does not grant competition, submission or assessment changes.
- A known original response/receipt can settle its command. Current GETs, editor
  replacement, source versions, capability changes and rejected retries cannot.
  Raw immutable retry payload remains only in its authorised mounted editor.
- Private source, publication, evidence, history and media choices disappear on
  current denial. Reopening and partial restoration require fresh native reads.
  All private outcome/project output is excluded from print.
- Published projections follow the current approved audience. Their native media
  URLs are resolved through current owning consent; omitted links disappear on
  the next current projection. Navigation does not grant media or source access.
- Existing independent portals/origins/sessions and ADR-0001–0006 are unchanged.
  Existing CommunicationSafety and other private command capsules are untouched.

## Executed evidence

All examples use synthetic people and deterministic fixtures. No real provider,
credential, sandbox configuration or external send is involved.

| Check | Actual terminal result and scope |
| --- | --- |
| Latest lint and both production builds | Pass: `/private/tmp/fleet242-candidate-lint.log`, `fleet242-candidate-build.log`. Includes final native-media route and scoped fieldset/shared source-select changes. |
| Latest complete web units | **148/148 pass**, 25 files: `/private/tmp/fleet242-candidate-units.log`. Includes metadata-only lifetime isolation, immutable review, zero/unknown, actual opener focus and pending/unknown/stale barriers. |
| Full administrator outcome suite at checkpoint 4161d27 | **56/56 pass**: `/private/tmp/fleet242-opening-admin3.log`. Native source/owner/archive, field rejection, uncertainty/retry, held reads, per-opening disclosures, capability restoration, checkpoint replacement, print and keyboard/mobile. |
| Full participant outcome suite at checkpoint 4161d27 | **44/44 pass**: `/private/tmp/fleet242-opening-public3.log`. Same applicable native journeys, including ordinary no-event-role owners. |
| Final changed composition paths | **6/6 admin + 4/4 participant pass**: `/private/tmp/fleet242-final-composition-{admin,public}.log`. Original unknown outcome → project → current native media → return retains the document and original barrier, with no replacement; current consent omission removes links; correction requires explicit release. |
| Final shared source selector/current denial | **2/2 pass**, desktop/mobile, original 20-second assertion unchanged: `/private/tmp/fleet242-candidate-native-controls.log`. No timeout/retry increase. |
| Actual matching production captures | **6 before + 6 after capture cases pass**, 24 PNGs: [visual index](visual-evidence.md), [hash/source manifest](visual-manifest.json). Four current admin cases and two participant cases cover directory/source plus complete admin publication/plan preparation regions at matching CSS widths/DPR (viewport height is temporarily extended only to fit the app scrollport). No domain writes in capture cases. |
| Independent production guide | **40/40 pass in actual CI at 4161d27**, job `120929458805`; root-retained log `/private/tmp/hackbox261-416-guide-ci.log`. Covers the merged media example and actual outcome focus/unknown example, without private cookies/storage/API access. Final CI reruns after the candidate changes. |
| Native supporting HTTP correction | **2/2 real PostgreSQL/race tests pass**: `/private/tmp/fleet242-http-green.log`; previous failing adapter recorded in `fleet242-http-red.log`. Existing native private publication journey remains included. Native consent/owner/reminder contracts retain their existing integration coverage. |

The checkpoint-wide browser counts are attributed to their executed source;
they are not claimed as a rerun of every browser test at the final candidate.
Final exact-head CI remains the required complete repository gate.

Actual red evidence also reproduced cached private owner options after capability
loss/restoration on the pre-refactor build (desktop/mobile), and the raw native
media link's document reload/lost original barrier at 4161d27 (desktop/mobile).
The retained assertions now pass. The original guide focus failure was fixed by
preserving the real initiating control, not by weakening focus assertions.

## Final review boundary

No implementation or local acceptance item above is deferred to a future UI
slice. Remaining gates are the paired independent frozen-source review and all
exact-head CI checks, followed by the authorised merge. Concrete review findings
must be fixed and verified before readiness.

#119 still owns missing archive/export backend work; #163 owns the additional
cross-event showcase; #113 owns result/feedback release. This UI work does not
claim any of those separate product deliverables complete.
