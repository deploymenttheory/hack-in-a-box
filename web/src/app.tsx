import { useEffect, useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { createRootRoute, createRoute, createRouter, Link, Outlet, RouterProvider, useNavigate, useRouterState } from '@tanstack/react-router'
import { Menu } from '@base-ui/react/menu'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { api, APIError, getSession, type Session } from './api/client'
import { Button } from './components/ui/button'
import { Theme, useThemePortal } from './components/theme'
import { Failure, FieldError, Loading } from './components/states'

export const sessionCache = new QueryClient({ defaultOptions: { queries: { retry: false } } })

function PrivateCache({ children }: { children: ReactNode }) {
  const [cache] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 15_000 } } }))
  useEffect(() => () => { void cache.cancelQueries(); cache.clear() }, [cache])
  return <QueryClientProvider client={cache}>{children}</QueryClientProvider>
}

function Account({ session }: { session: Session }) {
  const portal = useThemePortal()
  const [failure, setFailure] = useState<unknown>()
  const [pending, setPending] = useState(false)
  async function logout() {
    setPending(true)
    try {
      const { error, response } = await api.POST('/api/v1/logout', { params: { header: { 'X-CSRF-Token': session.csrf_token } } })
      if (error && response.status !== 401) throw new APIError(response.status, error.error.code)
      await sessionCache.cancelQueries()
      sessionCache.clear()
      window.location.assign('/')
    } catch (error) { setFailure(error); setPending(false) }
  }
  return <div>
    <Menu.Root>
      <Menu.Trigger render={<Button variant="outline" />} disabled={pending}>{session.person.display_name || 'Your account'}</Menu.Trigger>
      <Menu.Portal container={portal}>
        <Menu.Positioner sideOffset={8} className="z-50">
          <Menu.Popup className="menu"><Menu.Item className="menu-item" onClick={() => void logout()}>Sign out</Menu.Item></Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
    {!!failure && <Failure error={failure} />}
  </div>
}

function Shell() {
  const session = useQuery({ queryKey: ['session'], queryFn: getSession, retry: false, refetchInterval: 60_000, refetchOnWindowFocus: 'always' }, sessionCache)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  useEffect(() => {
    const expire = () => { sessionCache.setQueryData(['session'], null) }
    window.addEventListener('session-expired', expire)
    return () => window.removeEventListener('session-expired', expire)
  }, [])
  const signedOut = session.data === null || (session.error instanceof APIError && session.error.status === 401)
  return <Theme>
    <a className="skip-link" href="#main">Skip to content</a>
    <header className="header">
      <Link className="brand" to="/" aria-label="Hack in a Box home"><span className="brand-mark" aria-hidden="true">H</span>Hack in a Box</Link>
      <nav aria-label="Main navigation"><Link to="/">Workspace</Link><span className="event-context">{pathname.startsWith('/events/') ? 'Event workspace' : 'No event selected'}</span></nav>
      {session.data && !session.error ? <Account session={session.data} /> : <a className="sign-in" href="/auth/login">Sign in</a>}
    </header>
    <main id="main" tabIndex={-1}>
      {!/^\/$|^\/events\/[a-f0-9-]+$/i.test(pathname) ? <NotFound /> : session.isPending ? <Loading /> : session.error && !signedOut ? <Failure error={session.error} retry={() => void session.refetch()} /> :
        signedOut ? <Welcome /> : session.data ? <PrivateCache key={session.data.person.id}><Outlet /></PrivateCache> : <Welcome />}
    </main>
    <footer>Make something worth sharing.</footer>
  </Theme>
}

function Welcome() {
  return <section className="welcome"><p className="eyebrow">Build together</p><h1>Big ideas.<br />A place to begin.</h1><p>Bring your curiosity, meet your team, and turn a shared challenge into something real.</p><a className="primary-link" href="/auth/login">Sign in with your organisation <span aria-hidden="true">↗</span></a><p className="muted">Sign in to access your events and team workspace.</p></section>
}

const eventSchema = z.object({ event: z.uuid('Enter the event ID supplied by your organiser.') })
function Workspace() {
  const navigate = useNavigate()
  const form = useForm<z.infer<typeof eventSchema>>({ resolver: zodResolver(eventSchema) })
  return <section className="welcome"><p className="eyebrow">Your workspace</p><h1>Ready when you are.</h1><p>Open the event shared by your organiser to find your workspace.</p>
    <form className="event-form" onSubmit={form.handleSubmit(({ event }) => navigate({ to: '/events/$eventId', params: { eventId: event } }))}>
      <label htmlFor="event">Event ID</label><input id="event" {...form.register('event')} aria-invalid={!!form.formState.errors.event} aria-describedby="event-error" autoComplete="off" />
      <FieldError message={form.formState.errors.event?.message} /><Button size="lg" type="submit" disabled={form.formState.isSubmitting}>Open event</Button>
    </form></section>
}
function EventWorkspace() { return <section className="welcome"><p className="eyebrow">Event workspace</p><h1>A shared place to build.</h1><p>Your event details will appear here when they are available.</p><Link to="/">Back to your workspace</Link></section> }
function NotFound() { return <section className="state"><h1>Page not found</h1><p>This link does not lead to a workspace.</p><Link to="/">Return home</Link></section> }
const root = createRootRoute({ component: Shell, notFoundComponent: NotFound, errorComponent: ({ error, reset }) => <Failure error={error} retry={reset} /> })
const home = createRoute({ getParentRoute: () => root, path: '/', component: Workspace })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
export const router = createRouter({ routeTree: root.addChildren([home, event]), defaultPendingComponent: Loading })
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
