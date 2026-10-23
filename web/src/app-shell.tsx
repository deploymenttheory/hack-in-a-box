import { Card } from './components/ui/surfaces'
import { EmailAccount } from './email-account'
import { InboxLink } from './inbox'
import { ConnectionStatus } from './connection-status'
import { installDraftInvalidation } from './private-draft-lifecycle'
import { retainMentorDraftScope } from './mentor-drafts'
import { GitHubAccount } from './github-account'
import { useEffect, useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { Menu } from '@base-ui/react/menu'
import { api, APIError, getSession, type Session } from './api/client'
import { Button } from './components/ui/button'
import { Theme, useThemePortal } from './components/theme'
import { Failure, Loading } from './components/states'
import { clearPrivateDrafts, loginURL } from './auth'
import { IdentityContext, useIdentity } from './session'

export const sessionCache = new QueryClient({ defaultOptions: { queries: { retry: false } } })

function PrivateCache({ children }: { children: ReactNode }) {
  const [cache] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 15_000, refetchOnReconnect: 'always' } } }))
  useEffect(() => () => { void cache.cancelQueries(); cache.clear() }, [cache])
  return <QueryClientProvider client={cache}>{children}</QueryClientProvider>
}

function Account({ session }: { session: Session }) {
  const portal = useThemePortal()
  const [failure, setFailure] = useState<unknown>()
  const [pending, setPending] = useState(false)
  async function logout() {
    window.dispatchEvent(new Event('session-ending'))
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

export function Shell() {
  const [expired, setExpired] = useState(false)
  const session = useQuery({ queryKey: ['session'], queryFn: getSession, retry: false, refetchInterval: 60_000, refetchOnWindowFocus: 'always' }, sessionCache)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  useEffect(() => {
    const expire = () => { setExpired(true); sessionCache.setQueryData(['session'], null) }
    window.addEventListener('session-expired', expire)
    return () => window.removeEventListener('session-expired', expire)
  }, [])
  useEffect(() => installDraftInvalidation(() => { setExpired(true); const cancelled = sessionCache.cancelQueries({ queryKey: ['session'] }); sessionCache.setQueryData(['session'], null); void cancelled.then(() => sessionCache.invalidateQueries({ queryKey: ['session'] })) }), [])
  useEffect(() => {
    const clear = () => sessionCache.removeQueries({ queryKey: ['private'] })
    window.addEventListener('mentor-draft-invalidated', clear)
    return () => window.removeEventListener('mentor-draft-invalidated', clear)
  }, [])
  useEffect(() => {
    if (session.data?.person.id && !session.error) {
      clearPrivateDrafts(session.data.person.id)
      retainMentorDraftScope(session.data.person.id, session.data.draft_scope)
    }
  }, [session.data?.person.id, session.data?.draft_scope, session.error])
  const signedOut = session.data === null || (session.error instanceof APIError && session.error.status === 401)
  return <Theme>
    <a className="skip-link" href="#main">Skip to content</a>
    <header className="header">
      <Link className="brand" to="/" aria-label="Hack in a Box home"><span className="brand-mark" aria-hidden="true">H</span>Hack in a Box{import.meta.env.MODE === 'admin' ? ' · Admin' : ''}</Link>
      <nav aria-label="Main navigation"><Link to="/">Workspace</Link>{session.data && !session.error && <Link to="/ideas">Ideas & projects</Link>}{session.data && !session.error && <IdentityContext.Provider value={session.data}><InboxLink /></IdentityContext.Provider>}{import.meta.env.MODE !== 'admin' && <Link to="/register">Find events</Link>}<span className="event-context">{pathname.startsWith('/events/') ? 'Event workspace' : 'No event selected'}</span></nav>
      {session.data && !session.error ? <Account session={session.data} /> : <a className="sign-in" href={loginURL()}>Sign in</a>}
    </header>
    <ConnectionStatus />
    <main id="main" tabIndex={-1}>
      {!(import.meta.env.MODE === 'admin' ? /^\/$|^\/ideas(?:\/[a-f0-9-]+|\/reviews\/[a-f0-9-]+)?$|^\/projects(?:\/[a-f0-9-]+)?$|^\/register(?:\/[a-f0-9-]+)?$|^\/templates(?:\/[a-f0-9-]+)?$|^\/inbox$|^\/events\/[a-f0-9-]+\/(?:resources|help|clarifications|questions|feedback|feedback-followups|surveys|media)\/[a-f0-9-]+$|^\/events\/[a-f0-9-]+\/(?:help|clarifications|media|photo-consent)$|^\/events\/[a-f0-9-]+\/review(?:\/[a-f0-9-]+(?:\/summary-preview)?)?$|^\/events\/[a-f0-9-]+\/communications\/deliveries(?:\/[a-f0-9-]+)?$|^\/events\/[a-f0-9-]+\/surveys(?:\/new|\/[a-f0-9-]+\/(?:manage|results))?$|^\/account$|^\/events\/(new|[a-f0-9-]+)(?:\/mentor(?:\/[a-f0-9-]+(?:\/scorecards)?)?)?$/i.test(pathname) : /^\/$|^\/ideas(?:\/[a-f0-9-]+|\/reviews\/[a-f0-9-]+)?$|^\/projects(?:\/[a-f0-9-]+)?$|^\/register(?:\/[a-f0-9-]+)?$|^\/inbox$|^\/events\/[a-f0-9-]+\/(?:resources|help|clarifications|questions|feedback|feedback-followups|surveys|media)\/[a-f0-9-]+$|^\/events\/[a-f0-9-]+\/(?:help|clarifications|media|photo-consent)$|^\/events\/[a-f0-9-]+\/review(?:\/[a-f0-9-]+(?:\/summary-preview)?)?$|^\/events\/[a-f0-9-]+\/communications\/deliveries(?:\/[a-f0-9-]+)?$|^\/events\/[a-f0-9-]+\/surveys(?:\/new|\/[a-f0-9-]+\/(?:manage|results))?$|^\/account$|^\/events\/[a-f0-9-]+(?:\/mentor(?:\/[a-f0-9-]+(?:\/scorecards)?)?)?$/i.test(pathname)) ? <NotFound /> : session.isPending ? <Loading /> : session.error && !signedOut ? <Failure error={session.error} retry={() => void session.refetch()} /> :
        signedOut ? <Welcome expired={expired} /> : session.data ? <IdentityContext.Provider value={session.data}><PrivateCache key={`${session.data.person.id}:${session.data.draft_scope}`}><Outlet /></PrivateCache></IdentityContext.Provider> : <Welcome />}
    </main>
    <footer>Make something worth sharing.</footer>
  </Theme>
}

function Welcome({ expired = false }: { expired?: boolean }) {
  return <section className="welcome"><p className="eyebrow">{import.meta.env.MODE === 'admin' ? 'Administration' : 'Build together'}</p><h1>{expired ? 'Sign in to continue.' : <>Big ideas.<br />A place to begin.</>}</h1><p>{expired ? 'Your session has expired. Sign in again to return to this page. Safe event setup drafts remain available to the same account. Private unsent mentor drafts are cleared when your session ends.' : 'Bring your curiosity, meet your team, and turn a shared challenge into something real.'}</p><a className="primary-link" href={loginURL()}>Sign in with your organisation <span aria-hidden="true">↗</span></a><p className="muted">Sign in to access your events and team workspace.</p></section>
}

export function AccountPage() {
  const identity = useIdentity()
  return <section className="workspace"><Link to="/">← Your events</Link><div className="page-heading"><div><p className="eyebrow">Your identity</p><h1>Your account</h1><p>{identity.person.display_name}</p></div></div><Card><h2>Organisation account</h2><label htmlFor="account-id">Your account ID</label><input id="account-id" readOnly value={identity.person.id} /><p>Share this ID with an organiser who needs to assign you an event role.</p></Card><EmailAccount /><GitHubAccount /></section>
}
export function NotFound() { return <section className="state"><h1>Page not found</h1><p>This link does not lead to a workspace.</p><Link to="/">Return home</Link></section> }
