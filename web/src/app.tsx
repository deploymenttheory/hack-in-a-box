import { ScorecardWorkspace } from './scorecard-workspace'
import { HelpWorkspace } from './help-workspace'
import { MentorWorkspace } from './mentor-workspace'
import { InboxPage, ResourcePage } from './inbox'
import { ClarificationReader } from './clarification-reader'
import { QueryClientProvider } from '@tanstack/react-query'
import { createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { HelpPage } from './help-reader'
import { Failure, Loading } from './components/states'
import { Shell, AccountPage, NotFound, sessionCache } from './app-shell'
import { RegistrationDirectory, RegistrationPage } from './registration'
import { TemplatesPage, TemplatePage } from './templates'
import { EventDirectory, EventWorkspace, NewEvent } from './events'

const root = createRootRoute({ component: Shell, notFoundComponent: NotFound, errorComponent: ({ error, reset }) => <Failure error={error} retry={reset} /> })
const inbox = createRoute({ getParentRoute: () => root, path: '/inbox', component: InboxPage })
const resource = createRoute({ getParentRoute: () => root, path: '/events/$eventId/resources/$resourceId', component: ResourcePage })
const clarification = createRoute({ getParentRoute: () => root, path: '/events/$eventId/clarifications/$answerId', component: ClarificationReader })
const home = createRoute({ getParentRoute: () => root, path: '/', component: EventDirectory })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
const mentor = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor', component: MentorWorkspace })
const scorecards = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor/$teamId/scorecards', component: ScorecardWorkspace })
const mentorTeam = createRoute({ getParentRoute: () => root, path: '/events/$eventId/mentor/$teamId', validateSearch: (s: Record<string, unknown>) => ({ note: typeof s.note === 'string' && /^[a-f0-9-]{36}$/i.test(s.note) ? s.note : undefined, fresh: s.fresh === true || s.fresh === 'true' }), component: MentorWorkspace })
const helpQueue = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help', component: HelpWorkspace })
const help = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help/$requestId', component: HelpPage })
const newEvent = createRoute({ getParentRoute: () => root, path: '/events/new', component: NewEvent })
const templates = createRoute({ getParentRoute: () => root, path: '/templates', component: TemplatesPage })
const template = createRoute({ getParentRoute: () => root, path: '/templates/$templateId', component: TemplatePage })
const registration = createRoute({ getParentRoute: () => root, path: '/register/$eventId', component: RegistrationPage })
const registrations = createRoute({ getParentRoute: () => root, path: '/register', component: RegistrationDirectory })
const account = createRoute({ getParentRoute: () => root, path: '/account', component: AccountPage })
export const router = createRouter({ routeTree: root.addChildren([scorecards, helpQueue, mentor, mentorTeam, help, inbox, resource, clarification, home, newEvent, event, account, templates, template, registrations, registration]), defaultPendingComponent: Loading })
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
