# Portal route and nested-component migration inventory

This inventory is derived from the actual `app.tsx`, `participant-app.tsx` and
`main.tsx` route registrations. #234 owns shared shell, directory and account
presentation. Existing direct/bookmarked routes, search parameters and domain
validators remain registered. Navigation offers a bounded current-permission
subset; lack of a navigation link does not replace backend authorisation.

#245 is the final parity/removal gate for every row. The route inventory is an
assignment of remaining migration work, not a claim that every feature layout
has already been rebuilt. #88 owns complete Today/readiness product content.

## Registered routes

| Path | Actual component | Builds | Presentation owner |
|---|---|---|---|
| `/` | `EventDirectory` | Admin, Participant | #234 |
| `/account` | `AccountPage` | Admin, Participant | #234 |
| `/events/$eventId` | `EventWorkspace` | Admin, Participant | #235–244 (nested ownership below) |
| `/events/$eventId/clarifications` | `ClarificationWorkspacePage` | Admin, Participant | #240 |
| `/events/$eventId/clarifications/$answerId` | `ClarificationReader` | Admin, Participant | #240 |
| `/events/$eventId/communications/deliveries` | `DeliveryHistoryPage` | Admin, Participant | #239 |
| `/events/$eventId/communications/deliveries/$deliveryId` | `DeliveryHistoryPage` | Admin, Participant | #239 |
| `/events/$eventId/feedback-followups/$followupId` | `FeedbackReader` | Admin, Participant | #240 |
| `/events/$eventId/feedback/$feedbackId` | `FeedbackReader` | Admin, Participant | #240 |
| `/events/$eventId/help` | `HelpWorkspace` | Admin, Participant | #240 |
| `/events/$eventId/help/$requestId` | `HelpPage` | Admin, Participant | #240 |
| `/events/$eventId/media` | `MediaWorkspacePage` | Admin, Participant | #243 |
| `/events/$eventId/media/$mediaId` | `MediaWorkspacePage` | Admin, Participant | #243 |
| `/events/$eventId/judging` | `JudgeWorkspacePage` | Admin, Participant | #109 |
| `/events/$eventId/judging/practice` | `JudgePracticePage` | Admin, Participant | #109 |
| `/events/$eventId/mentor` | `MentorWorkspace` | Admin, Participant | #240 |
| `/events/$eventId/mentor/$teamId` | `MentorWorkspace` | Admin, Participant | #240 |
| `/events/$eventId/mentor/$teamId/scorecards` | `ScorecardWorkspace` | Admin, Participant | #240 |
| `/events/$eventId/photo-consent` | `OwnPhotoConsentPage` | Admin, Participant | #243 |
| `/events/$eventId/questions/$questionId` | `ClarificationWorkspacePage` | Admin, Participant | #240 |
| `/events/$eventId/resources/$resourceId` | `ResourcePage` | Admin, Participant | #238 |
| `/events/$eventId/review` | `ReviewDashboard` | Admin, Participant | #240 |
| `/events/$eventId/review/$teamId` | `TeamReviewPage` | Admin, Participant | #240 |
| `/events/$eventId/review/$teamId/summary-preview` | `JudgeSummaryPreview` | Admin, Participant | #240 |
| `/events/$eventId/surveys` | `SurveyDirectoryPage` | Admin, Participant | #241 |
| `/events/$eventId/surveys/$surveyId` | `SurveyReader` | Admin, Participant | #241 |
| `/events/$eventId/surveys/$surveyId/manage` | `SurveyManagePage` | Admin, Participant | #241 |
| `/events/$eventId/surveys/$surveyId/results` | `SurveyResultsPage` | Admin, Participant | #241 |
| `/events/$eventId/surveys/new` | `SurveyCreatePage` | Admin, Participant | #241 |
| `/events/new` | `NewEvent` | Admin | #235 |
| `/ideas` | `OutcomeWorkspace` | Admin, Participant | #242 |
| `/ideas/$outcomeId` | `OutcomePage` | Admin, Participant | #242 |
| `/ideas/reviews/$reviewId` | `OutcomeReviewPage` | Admin, Participant | #242 |
| `/inbox` | `InboxPage` | Admin, Participant | #239 |
| `/projects` | `PublishedProjects` | Admin, Participant | #242 |
| `/projects/$outcomeId` | `PublishedProjectPage` | Admin, Participant | #242 |
| `/register` | `RegistrationDirectory` | Admin, Participant | #234 |
| `/register/$eventId` | `RegistrationPage` | Admin, Participant | #236 |
| `/templates` | `TemplatesPage` | Admin | #235 |
| `/templates/$templateId` | `TemplatePage` | Admin | #235 |
| `/display/$eventId` (independent main entry; key is supplied only in a bearer header) | `room-display.tsx` | Participant only | #244 |
| Unknown/bookmarked forbidden route, login, expiry, connection and account menu/search overlays | Shell / existing auth owners | Both | #234 |

## Nested tabs, readers and overlays

| Entry | Nested components / states / overlays | Presentation owner |
|---|---|---|
| Event overview / setup | Dates, blockers, stage transition reason/attendance exception, event form, preview, schedule, branding editor, objectives, staff/ownership transfers, template editor/import/publish/clone, configuration inventory, venue/logistics/check-in, staff planning/preparation/readiness/export composition | #235 |
| Event repositories | `RepositoryWorkspace`: current repository collection and team/numeric-repository inspector; App configuration, installation review, published-team initial queue, selected lookup-only recovery, exact access planning/approval/original receipts, own invitation status; fresh private ownership and safe original-command metadata | #94; regular portal exposes only current own-team status, administration stays in the admin portal |
| Event Today | Current event guidance; complete readiness and action ownership remains #88 | #235 admin chrome/readiness composition, #238 participant preparation evidence/checklists, #88 full Today/live aggregation |
| Registration / applicants | Application chooser, branching questions, offers/deadlines, withdrawals, applicant review/detail/reasons, form editor/preview/publish acknowledgement | #236 |
| Teams / mentors / team access | Team requests/roster, allocation/freeze, team member readers, mentor roster/coverage/conflicts, retained repository/access status | #237 |
| Inventory / attendance / operations / logistics / staff preparation | Event-configuration inventory/assignment/returns, venue/logistics/check-in, staff planning/preparation/readiness exceptions and export composition | #235 |
| Released resources / participant preparation / deliverables | Released resource library/editor/readers, attachments, credential reveal, participant preparation evidence/checklists, working deliverable and formal submission boundaries | #238 |
| Rules setup | Focused rules authoring/review/publication using the native content editor; released version and acknowledgement readiness | #235 |
| Resource content authoring | General challenge/brief/resource content editor/preview/publication; released readers, rules acknowledgement and support contacts | #238 |
| Campaigns / Inbox | Campaign composer/message templates/review/approval/test, Teams confirmation, automation status/reconciliation, notification reader/mark-read, delivery history/receipt detail | #239 |
| Clarifications / private questions | Question author/read/review, explicit common-answer review/publication and released answer reader | #240 |
| Mentor / help / review / feedback | Mentor workspace/private draft recovery, scorecards/observations/evidence, helper queue/request thread, review dashboard, team inspector/judge summary, moderation/huddle/feedback readers and follow-ups | #240 |
| Surveys | Directory, audience/authoring/publish, respondent version, answer privacy, management/close/results/export and results review | #241 |
| Ideas / outcomes / projects | Idea create/detail/review, current team follow-through, project eligibility/published/private readers and explicit next actions | #242 |
| Media / consent | Gallery/detail/upload/review, own consent, protected image derivatives, withdrawal/current permission states | #243 |
| MC console / room display | Rehearsal, pairing, cue controls, operator confirmation/receipt, distance-readable output and token-only display recovery | #244 |
| Account | Organisation read-only ID, private email/code/confirmation, GitHub link/change/unlink acknowledgement and callback outcome; scoped account menu + destination search | #234 |

Domain readers retain their current query/CSRF/revision/receipt/private-draft
services. Their feature owners migrate nested forms, dialogs and inspectors
alongside their primary screen. No automatic replay, new grant, endpoint or
cross-portal runtime dependency is introduced by this assignment.

Shared parent components may host both admin configuration and released participant views: #235 owns the former and #238 owns the latter. Preserve the existing domain readers and audience boundaries while migrating each composition. #88 continues to own complete Today/live aggregation logic.
