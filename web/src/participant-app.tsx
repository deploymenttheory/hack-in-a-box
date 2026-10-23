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
const inbox = createRoute({ getParentRoute: () => root, path: '/inbox', component: InboxPage })
const resource = createRoute({ getParentRoute: () => root, path: '/events/$eventId/resources/$resourceId', component: ResourcePage })
const clarificationWorkspace = createRoute({ getParentRoute: () => root, path: '/events/$eventId/clarifications', component: ClarificationWorkspacePage })
const privateQuestion = createRoute({ getParentRoute: () => root, path: '/events/$eventId/questions/$questionId', component: ClarificationWorkspacePage })
const clarification = createRoute({ getParentRoute: () => root, path: '/events/$eventId/clarifications/$answerId', component: ClarificationReader })
const home = createRoute({ getParentRoute: () => root, path: '/', component: EventDirectory })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
const mentor = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor', component: MentorWorkspace })
const scorecards = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor/$teamId/scorecards', component: ScorecardWorkspace })
const mentorTeam = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor/$teamId', validateSearch: (s: Record<string, unknown>) => ({ note: typeof s.note === 'string' && /^[a-f0-9-]{36}$/i.test(s.note) ? s.note : undefined, fresh: s.fresh === true || s.fresh === 'true' }), component: MentorWorkspace })
const helpQueue = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help', component: HelpWorkspace })
const help = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help/$requestId', component: HelpPage })
const registration = createRoute({ getParentRoute: () => root, path: '/register/$eventId', component: RegistrationPage })
const registrations = createRoute({ getParentRoute: () => root, path: '/register', component: RegistrationDirectory })
const account = createRoute({ getParentRoute: () => root, path: '/account', component: AccountPage })
export const router = createRouter({ routeTree: root.addChildren([scorecards, helpQueue, mentor, mentorTeam, help, inbox, resource, clarification, clarificationWorkspace, privateQuestion, home, event, account, registrations, registration]), defaultPendingComponent: Loading })
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
