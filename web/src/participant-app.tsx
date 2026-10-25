import { MentorAdvisoryPage } from './mentor-advisory'
import { ResultsBallotsPage } from './results-ballots'
import { TeamFeedbackPage, TeamFeedbackDirectoryPage } from './team-feedback'
import { AnnouncedResultsPage } from './results-workspace'
import { showcaseSearch } from './showcase'
import { QualificationPanelPage, QualificationDirectoryPage } from './qualification'
import { JudgeWorkspacePage, JudgePracticePage } from './judge-workspace'
import { AssignedBuildsPage } from './assigned-builds'
import { SubmittedBuildPage } from './submitted-build'
import { OutcomeWorkspace, OutcomePage, OutcomeReviewPage, PublishedProjects, PublishedProjectPage } from './outcome-workspace'
import { MediaWorkspacePage, OwnPhotoConsentPage } from './media-workspace'
import { ReviewDashboard, TeamReviewPage, JudgeSummaryPreview } from './review-dashboard'
import { SurveyDirectoryPage } from './survey-directory'
import { SurveyCreatePage, SurveyManagePage, surveyCreateSearch } from './survey-workspace'
import { SurveyResultsPage } from './survey-results'
import { SurveyReader } from './survey-reader'
import { FeedbackReader } from './feedback-reader'
import { DeliveryHistoryPage } from './delivery-history'
import { ScorecardWorkspace } from './scorecard-workspace'
import { HelpWorkspace } from './help-workspace'
import { ClarificationWorkspacePage } from './clarification-workspace'
import { MentorWorkspace } from './mentor-workspace'
import { InboxPage, ResourcePage } from './inbox'
import { ClarificationReader } from './clarification-reader'
import { QueryClientProvider } from '@tanstack/react-query'
import { createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { HelpPage } from './help-reader'
import { Failure, Loading } from './components/states'
import { Shell, AccountPage, NotFound, sessionCache } from './app-shell'
import { RegistrationDirectory, RegistrationPage } from './registration'
import { EventDirectory, EventWorkspace } from './participant-events'

const root = createRootRoute({ component: Shell, notFoundComponent: NotFound, errorComponent: ({ error, reset }) => <Failure error={error} retry={reset} /> })
const ideas = createRoute({ getParentRoute: () => root, path: '/ideas', validateSearch: (s: Record<string, unknown>): { event?: string } => ({ event: typeof s.event === 'string' && /^[a-f0-9-]{36}$/i.test(s.event) ? s.event : undefined }), component: OutcomeWorkspace })
const idea = createRoute({ getParentRoute: () => root, path: '/ideas/$outcomeId', component: OutcomePage })
const outcomeReview = createRoute({ getParentRoute: () => root, path: '/ideas/reviews/$reviewId', component: OutcomeReviewPage })
const projects = createRoute({ getParentRoute: () => root, path: '/projects', validateSearch: showcaseSearch, component: PublishedProjects })
const project = createRoute({ getParentRoute: () => root, path: '/projects/$outcomeId', validateSearch: showcaseSearch, component: PublishedProjectPage })
const inbox = createRoute({ getParentRoute: () => root, path: '/inbox', component: InboxPage })
const resource = createRoute({ getParentRoute: () => root, path: '/events/$eventId/resources/$resourceId', component: ResourcePage })
const deliveryHistory = createRoute({ getParentRoute: () => root, path: '/events/$eventId/communications/deliveries', component: DeliveryHistoryPage })
const deliveryReview = createRoute({ getParentRoute: () => root, path: '/events/$eventId/communications/deliveries/$deliveryId', component: DeliveryHistoryPage })
const clarificationWorkspace = createRoute({ getParentRoute: () => root, path: '/events/$eventId/clarifications', component: ClarificationWorkspacePage })
const privateQuestion = createRoute({ getParentRoute: () => root, path: '/events/$eventId/questions/$questionId', component: ClarificationWorkspacePage })
const surveyDirectory = createRoute({ getParentRoute: () => root, path: '/events/$eventId/surveys', component: SurveyDirectoryPage })
const surveyCreate = createRoute({ getParentRoute: () => root, path: '/events/$eventId/surveys/new', component: SurveyCreatePage, validateSearch: surveyCreateSearch })
const surveyManage = createRoute({ getParentRoute: () => root, path: '/events/$eventId/surveys/$surveyId/manage', component: SurveyManagePage })
const surveyResults = createRoute({ getParentRoute: () => root, path: '/events/$eventId/surveys/$surveyId/results', component: SurveyResultsPage })
const survey = createRoute({ getParentRoute: () => root, path: '/events/$eventId/surveys/$surveyId', component: SurveyReader })
const clarification = createRoute({ getParentRoute: () => root, path: '/events/$eventId/clarifications/$answerId', component: ClarificationReader })
const ownPhotoConsent = createRoute({ getParentRoute: () => root, path: '/events/$eventId/photo-consent', component: OwnPhotoConsentPage })
const mediaGallery = createRoute({ getParentRoute: () => root, path: '/events/$eventId/media', component: MediaWorkspacePage })
const mediaDetail = createRoute({ getParentRoute: () => root, path: '/events/$eventId/media/$mediaId', component: MediaWorkspacePage })
const assignedBuilds = createRoute({ getParentRoute: () => root, path: '/events/$eventId/builds', component: AssignedBuildsPage })
const submittedBuild = createRoute({ getParentRoute: () => root, path: '/events/$eventId/teams/$teamId/build', component: SubmittedBuildPage, validateSearch: (s:Record<string,unknown>):{judge_run?:string}=>({judge_run:typeof s.judge_run==='string'&&/^[a-f0-9-]{36}$/i.test(s.judge_run)?s.judge_run:undefined}) })
const qualificationDirectory = createRoute({ getParentRoute: () => root, path: '/events/$eventId/panel', component: QualificationDirectoryPage })
const qualificationPanel = createRoute({ getParentRoute: () => root, path: '/events/$eventId/panel/$runId', component: QualificationPanelPage })
const judgeDesk = createRoute({ getParentRoute: () => root, path: '/events/$eventId/judging', component: JudgeWorkspacePage })
const judgePractice = createRoute({ getParentRoute: () => root, path: '/events/$eventId/judging/practice', component: JudgePracticePage })
const teamFeedbackDirectory = createRoute({ getParentRoute: () => root, path: '/events/$eventId/team-feedback', component: TeamFeedbackDirectoryPage })
const teamFeedback = createRoute({ getParentRoute: () => root, path: '/events/$eventId/teams/$teamId/feedback-report', component: TeamFeedbackPage })
const mentorAdvisories = createRoute({ getParentRoute: () => root, path: '/events/$eventId/results/mentor-advisories', component: MentorAdvisoryPage })
const resultBallots = createRoute({ getParentRoute: () => root, path: '/events/$eventId/results/ballots', component: ResultsBallotsPage })
const publishedResults = createRoute({ getParentRoute: () => root, path: '/events/$eventId/results', component: AnnouncedResultsPage })
const home = createRoute({ getParentRoute: () => root, path: '/', component: EventDirectory })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
const reviewDashboard = createRoute({ getParentRoute: () => root, path: '/events/$eventId/review', component: ReviewDashboard })
const teamReview = createRoute({ getParentRoute: () => root, path: '/events/$eventId/review/$teamId', component: TeamReviewPage })
const judgeSummaryPreview = createRoute({ getParentRoute: () => root, path: '/events/$eventId/review/$teamId/summary-preview', component: JudgeSummaryPreview })
const mentor = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor', component: MentorWorkspace })
const scorecards = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor/$teamId/scorecards', component: ScorecardWorkspace })
const mentorTeam = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor/$teamId', validateSearch: (s: Record<string, unknown>) => ({ note: typeof s.note === 'string' && /^[a-f0-9-]{36}$/i.test(s.note) ? s.note : undefined, fresh: s.fresh === true || s.fresh === 'true' }), component: MentorWorkspace })
const feedback = createRoute({ getParentRoute: () => root, path: '/events/$eventId/feedback/$feedbackId', component: FeedbackReader })
const feedbackFollowup = createRoute({ getParentRoute: () => root, path: '/events/$eventId/feedback-followups/$followupId', component: FeedbackReader })
const helpQueue = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help', component: HelpWorkspace })
const help = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help/$requestId', component: HelpPage })
const registration = createRoute({ getParentRoute: () => root, path: '/register/$eventId', component: RegistrationPage })
const registrations = createRoute({ getParentRoute: () => root, path: '/register', component: RegistrationDirectory })
const account = createRoute({ getParentRoute: () => root, path: '/account', component: AccountPage })
export const router = createRouter({ routeTree: root.addChildren([mentorAdvisories, resultBallots, teamFeedbackDirectory, teamFeedback, publishedResults, qualificationDirectory, qualificationPanel, judgeDesk, judgePractice, assignedBuilds, submittedBuild, surveyDirectory, surveyCreate, surveyManage, surveyResults, ideas, idea, outcomeReview, projects, project, ownPhotoConsent, mediaGallery, mediaDetail, survey, reviewDashboard, teamReview, judgeSummaryPreview, feedback, feedbackFollowup, deliveryHistory, deliveryReview, scorecards, helpQueue, mentor, mentorTeam, help, inbox, resource, clarification, clarificationWorkspace, privateQuestion, home, event, account, registrations, registration]), defaultPendingComponent: Loading })
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
