# UI-02 event planning inventory

This is the #235 ownership and verification checklist, not a completed migration
claim. Backend policy and generated API contracts stay unchanged. The accepted
Fleet visual contract and production primitives/patterns are the presentation
foundation. The initial draft remains incomplete until the nested presentation
and browser evidence below are independently reviewed.

| Entry | Owned nested panels and decisions | Source / status |
|---|---|---|
| Event workspace, both portals | Event identity, current stage/revision/timezone, local navigation, dates, confirmed-attendance minimum, blockers and next planning action, scoped private exports | `events.tsx`, `participant-events.tsx`, `planning-layout.tsx`: implemented; compact native narrow selector added after visual review; fresh responsive matrix pending |
| Create / setup | Identity, description/venue, capacity/minimum, all event-local timestamps and DST choice, working days/excluded dates, lasting organiser acknowledgement, published postponement/reason, native preview, review/save/revision | `event-form.tsx`: implemented; stale/unknown/DST/private-authority journeys pass; full final proof pending |
| Templates | Starter/create/directory, immutable versions, private configuration, current-base comparison, differences, save/publish, published comparison, independent clone | `templates.tsx`: implemented; browser adaptation pending |
| Template nested configuration | Objectives/measures, event defaults/agenda/audience, form definitions/questions/branches, preparation dependencies, deliverable definitions, role briefs, rubrics/criteria, communication drafts, branding | `definition-editor.tsx`: mounted local section editors; nested item selection; proof pending. Shared registration/survey form editor keeps its prior mode outside the template owner. |
| Objectives | Private definition/owner/embargo, measures and null baselines/targets, reason/retirement, retained exact-version outcome, explicit current-base comparison, history and advisory gaps | `objective-workspace.tsx`, `objective-fields.tsx`: native owner retained, directory rebuilt; focused per-measure editors implemented; native history/outcome tests pass; final proof pending |
| Rules and contacts | Native released exact versions/acknowledgements and planning path to native authoring | `rules-planning.tsx`, opt-in `ContentEditor` planning composition and flat `EventGuidance`: implemented with actual native save/review/approval/release; released readers and participant acknowledgement remain #238 |
| Staff | Assign/revoke, role explanations, lasting exclusions, active/waitlisted conflicts, final-owner protection, explicit ownership transfer, assignment history in event timezone | `staff.tsx`: collection and focused editor implemented; browser proof pending |
| Schedule | Native event-local due times, draft/approved state, explicit approval and retained completed history | `events.tsx`: native actions retained, compact review rows; proof pending |
| Branding | Safe palette, contrast validation, logo/icon/event-image uploads/removal, production Theme preview/native controls, save/reset/conflict, scoped event assets | `branding.tsx`: focused sections implemented; browser proof pending. Preserve #233 pure palette extraction on restack. |
| Preparation planning | Current readiness/milestones, blocker destinations, complete-collection filters, table/narrow records/status board/run sheet, selected task and ownership/due/DST/dependency/checklist/evidence/decision/history/conflict review | `preparation-board.tsx`, `preparation-editor.tsx`: implemented; proof pending. Participant evidence/checklist readers belong to #238. |
| Operational readiness | Venue/AV/catering, owner/deadline/blockers/overdue/checklist progress/reasons/waivers and explicit latest-version comparison | `venue-readiness.tsx`: flat rows and native controls; production operational collection/inspector implemented; native authority/owner/deadline tests pass; final proof pending |
| Attendance | Confirmed arrivals vs admissions, staff/catering counts, roster cutoff, day/filter/search, check-in exact request identity, correction reason/history/latest review, catering configuration/conflict | `attendance.tsx`: production collection/inspector implemented; proof pending |
| Restricted logistics | Actual permitted projection only, bounded page filter/cursor, selected private details, roster cutoff and audited complete-page export | `logistics.tsx`: production collection/inspector implemented; proof pending |
| Configuration resource inventory | Masked metadata/readiness, missing team assignments, metadata/availability/expiry/DST, write-only create/replacement, reviewed assignment/revocation and provider follow-up | `resource-inventory.tsx`, `resource-inventory-editor.tsx`: production collection/inspector implemented; proof pending. No reveal/export is added. Released library/editor/readers/credentials/deliverables belong to #238. |
| Scoped export actions | Native organiser/participant readiness CSV and printable run sheet, current permissions/audit/private snapshot guidance | `export-downloads.tsx`: scoped native links with production iconography implemented; final proof pending |

Other event tabs remain mounted by their existing owners: #236 registration/forms/
applicants/offers; #237 teams/mentor allocation/access; #238 released resources and
working deliverables; #239 campaigns/messages/Inbox; #240 help/assessment/private questions/clarifications;
#241 surveys; #242 outcomes; #243 media; #244 MC/display. #88 owns full Today/live
aggregation. Hosting a component does not transfer its domain migration into #235.

No fake server-wide search/counts, export, provider acceptance or completion may
be inferred by a presentation component. All writes retain actual CSRF/revision,
private scope and explicit domain outcome ownership; unknown effects require a
read/review and never automatic replay. No backend migration or provider effects
are planned for this presentation work.
