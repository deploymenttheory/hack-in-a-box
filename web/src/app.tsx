import { TemplatesPage, TemplatePage } from './templates'
import { GitHubAccount } from './github-account'
import { useEffect, useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { createRootRoute, createRoute, createRouter, Link, Outlet, RouterProvider, useRouterState } from '@tanstack/react-router'
import { Menu } from '@base-ui/react/menu'
import { api, APIError, getSession, type Session } from './api/client'
import { Button } from './components/ui/button'
import { Theme, useThemePortal } from './components/theme'
import { Failure, Loading } from './components/states'
import { clearPrivateDrafts, loginURL } from './auth'
import { IdentityContext, useIdentity } from './session'
import { EventDirectory, EventWorkspace, NewEvent } from './events'

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
      clearPrivateDrafts()
      window.location.assign('/')
    } catch (error) { setFailure(error); setPending(false) }
  }
  return <div>
    <Menu.Root>
      <Menu.Trigger render={<Button variant="outline" />} disabled={pending}>{session.person.display_name || 'Your account'}</Menu.Trigger>
      <Menu.Portal container={portal}>
        <Menu.Positioner sideOffset={8} className="z-50">
          <Menu.Popup className="menu"><Menu.Item className="menu-item" render={<Link to="/account" />}>Account settings</Menu.Item><Menu.Item className="menu-item" onClick={() => void logout()}>Sign out</Menu.Item></Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
    {!!failure && <Failure error={failure} />}
  </div>
}

function Shell() {
  const [expired, setExpired] = useState(false)
  const session = useQuery({ queryKey: ['session'], queryFn: getSession, retry: false, refetchInterval: 60_000, refetchOnWindowFocus: 'always' }, sessionCache)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  useEffect(() => {
    const expire = () => { setExpired(true); sessionCache.setQueryData(['session'], null) }
    window.addEventListener('session-expired', expire)
    return () => window.removeEventListener('session-expired', expire)
  }, [])
  useEffect(() => { if (session.data?.person.id) clearPrivateDrafts(session.data.person.id) }, [session.data?.person.id])
  const signedOut = session.data === null || (session.error instanceof APIError && session.error.status === 401)
  return <Theme>
    <a className="skip-link" href="#main">Skip to content</a>
    <header className="header">
      <Link className="brand" to="/" aria-label="Hack in a Box home"><span className="brand-mark" aria-hidden="true">H</span>Hack in a Box</Link>
      <nav aria-label="Main navigation"><Link to="/">Workspace</Link><span className="event-context">{pathname.startsWith('/events/') ? 'Event workspace' : 'No event selected'}</span></nav>
      {session.data && !session.error ? <Account session={session.data} /> : <a className="sign-in" href={loginURL()}>Sign in</a>}
    </header>
    <main id="main" tabIndex={-1}>
      {!/^\/$|^\/templates(?:\/[a-f0-9-]+)?$|^\/account$|^\/events\/(new|[a-f0-9-]+)$/i.test(pathname) ? <NotFound /> : session.isPending ? <Loading /> : session.error && !signedOut ? <Failure error={session.error} retry={() => void session.refetch()} /> :
        signedOut ? <Welcome expired={expired} /> : session.data ? <IdentityContext.Provider value={session.data}><PrivateCache key={session.data.person.id}><Outlet /></PrivateCache></IdentityContext.Provider> : <Welcome />}
    </main>
    <footer>Make something worth sharing.</footer>
  </Theme>
}

function Welcome({ expired = false }: { expired?: boolean }) {
  return <section className="welcome"><p className="eyebrow">Build together</p><h1>{expired ? 'Sign in to continue.' : <>Big ideas.<br />A place to begin.</>}</h1><p>{expired ? 'Your session has expired. Sign in again to return to this page. Safe event setup drafts remain available to the same account.' : 'Bring your curiosity, meet your team, and turn a shared challenge into something real.'}</p><a className="primary-link" href={loginURL()}>Sign in with your organisation <span aria-hidden="true">↗</span></a><p className="muted">Sign in to access your events and team workspace.</p></section>
}

function AccountPage() {
  const identity = useIdentity()
  return <section className="workspace"><Link to="/">← Your events</Link><div className="page-heading"><div><p className="eyebrow">Your identity</p><h1>Your account</h1><p>{identity.person.display_name}</p></div></div><section className="panel"><h2>Organisation account</h2><label htmlFor="account-id">Your account ID</label><input id="account-id" readOnly value={identity.person.id} /><p>Share this ID with an organiser who needs to assign you an event role.</p></section><GitHubAccount /></section>
}
function NotFound() { return <section className="state"><h1>Page not found</h1><p>This link does not lead to a workspace.</p><Link to="/">Return home</Link></section> }
const root = createRootRoute({ component: Shell, notFoundComponent: NotFound, errorComponent: ({ error, reset }) => <Failure error={error} retry={reset} /> })
const home = createRoute({ getParentRoute: () => root, path: '/', component: EventDirectory })
const event = createRoute({ getParentRoute: () => root, path: '/events/$eventId', component: EventWorkspace })
const newEvent = createRoute({ getParentRoute: () => root, path: '/events/new', component: NewEvent })
const templates = createRoute({ getParentRoute: () => root, path: '/templates', component: TemplatesPage })
const template = createRoute({ getParentRoute: () => root, path: '/templates/$templateId', component: TemplatePage })
const account = createRoute({ getParentRoute: () => root, path: '/account', component: AccountPage })
export const router = createRouter({ routeTree: root.addChildren([home, newEvent, event, account, templates, template]), defaultPendingComponent: Loading })
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
export function App() { return <QueryClientProvider client={sessionCache}><RouterProvider router={router} /></QueryClientProvider> }
