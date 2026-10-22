import { InboxPage, ResourcePage } from './inbox'
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
const home = createRoute({ getParentRoute: () => root, path: '/', component: EventDirectory })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
const help = createRoute({ getParentRoute: () => root, path: '/events/$eventId/help/$requestId', component: HelpPage })
const registration = createRoute({ getParentRoute: () => root, path: '/register/$eventId', component: RegistrationPage })
const registrations = createRoute({ getParentRoute: () => root, path: '/register', component: RegistrationDirectory })
const account = createRoute({ getParentRoute: () => root, path: '/account', component: AccountPage })
export const router = createRouter({ routeTree: root.addChildren([help, inbox, resource, home, event, account, registrations, registration]), defaultPendingComponent: Loading })
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
