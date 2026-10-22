import { QueryClientProvider } from '@tanstack/react-query'
import { createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { Failure, Loading } from './components/states'
import { Shell, AccountPage, NotFound, sessionCache } from './app-shell'
import { RegistrationDirectory, RegistrationPage } from './registration'
import { TemplatesPage, TemplatePage } from './templates'
import { EventDirectory, EventWorkspace, NewEvent } from './events'

const root = createRootRoute({ component: Shell, notFoundComponent: NotFound, errorComponent: ({ error, reset }) => <Failure error={error} retry={reset} /> })
const home = createRoute({ getParentRoute: () => root, path: '/', component: EventDirectory })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
const newEvent = createRoute({ getParentRoute: () => root, path: '/events/new', component: NewEvent })
const templates = createRoute({ getParentRoute: () => root, path: '/templates', component: TemplatesPage })
const template = createRoute({ getParentRoute: () => root, path: '/templates/$templateId', component: TemplatePage })
const registration = createRoute({ getParentRoute: () => root, path: '/register/$eventId', component: RegistrationPage })
const registrations = createRoute({ getParentRoute: () => root, path: '/register', component: RegistrationDirectory })
const account = createRoute({ getParentRoute: () => root, path: '/account', component: AccountPage })
export const router = createRouter({ routeTree: root.addChildren([home, newEvent, event, account, templates, template, registrations, registration]), defaultPendingComponent: Loading })
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
