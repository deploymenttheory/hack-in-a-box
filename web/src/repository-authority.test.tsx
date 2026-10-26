import type { ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { api, privateKey, type Session } from './api/client'
import { IdentityContext } from './session'
import { CommunicationSafety } from './communication-tests'
import { RepositoryWorkspace } from './repository-workspace'
import { Theme } from './components/theme'
import { fixtureEvent, person } from '../e2e/fixtures'
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined, Link: ({ children }: { children: ReactNode }) => <>{children}</> }))
afterEach(() => vi.restoreAllMocks())
const session: Session = { person, draft_scope: '22222222-2222-4222-8222-222222222222', csrf_token: 'synthetic', can_create_events: false }
const success = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
const app = { revision: 1, configured: false, installation_id: 0, webhook_configured: false, diagnostics: { code: 'not_configured', checks: [], checked_at: null } }

test('native readonly event keeps staff history but never promotes Teams StaffRead into App or provisioning write controls', async () => {
  const event = { ...fixtureEvent(), can_edit: false, can_read_staff: true }
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  cache.setQueryData(privateKey(person.id, event.id, 'event'), event)
  const post = vi.spyOn(api, 'POST')
  vi.spyOn(api, 'GET').mockImplementation((async (path: string) => {
    if (path.endsWith('/repositories')) return success([])
    if (path.endsWith('/teams')) return success({ revision: 1, can_manage: true, teams: [] })
    if (path.endsWith('/github-app')) return success(app)
    return success(event)
  }) as typeof api.GET)
  render(<Theme><QueryClientProvider client={cache}><IdentityContext.Provider value={session}><CommunicationSafety><RepositoryWorkspace event={event} administration /></CommunicationSafety></IdentityContext.Provider></QueryClientProvider></Theme>)
  fireEvent.click(await screen.findByRole('button', { name: 'Open App setup' }))
  await screen.findByRole('heading', { name: 'Current App readiness' })
  expect(screen.queryByLabelText('App private key (PEM)', { exact: true })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Review App setup change' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Review published-team provisioning' })).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled(); cache.clear()
})

test('held older collection result cannot restore writes after a successful current parent write loss', async () => {
  const event = fixtureEvent(), cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }), eventKey = privateKey(person.id, event.id, 'event')
  cache.setQueryData(eventKey, event)
  let release!: () => void, reads = 0
  const held = new Promise<void>(resolve => { release = resolve })
  vi.spyOn(api, 'GET').mockImplementation((async (path: string) => {
    if (path.endsWith('/repositories')) { reads++; if (reads > 1) await held; return success([]) }
    if (path.endsWith('/teams')) return success({ revision: 1, can_manage: true, teams: [] })
    if (path.endsWith('/github-app')) return success(app)
    return success(event)
  }) as typeof api.GET)
  // Keep the old event prop deliberately mounted after the current cache success.
  // This exercises the real stale async collection closure before parent teardown.
  render(<Theme><QueryClientProvider client={cache}><IdentityContext.Provider value={session}><CommunicationSafety><RepositoryWorkspace event={event} administration /></CommunicationSafety></IdentityContext.Provider></QueryClientProvider></Theme>)
  const refresh = await screen.findByRole('button', { name: 'Refresh current status' })
  fireEvent.click(refresh); await waitFor(() => expect(reads).toBe(2))
  act(() => cache.setQueryData(eventKey, { ...event, can_edit: false }))
  await act(async () => { release(); await held })
  await waitFor(() => expect(cache.isFetching()).toBe(0))
  fireEvent.click(screen.getByRole('button', { name: 'Open App setup' }))
  await screen.findByRole('heading', { name: 'Current App readiness' })
  expect(screen.queryByLabelText('App private key (PEM)', { exact: true })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Review App setup change' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Review published-team provisioning' })).not.toBeInTheDocument()
  cache.clear()
})

test('coalesced current write loss and restoration permits a new command only after a fresh authorized collection', async () => {
  const event = fixtureEvent(), cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }), eventKey = privateKey(person.id, event.id, 'event')
  cache.setQueryData(eventKey, event)
  const configured = { ...app, configured: true, installation_id: 123, configuration: { app_id: 42, app_slug: 'synthetic', organization_id: 99, organization: 'synthetic', template_repository: 'starter' } }
  vi.spyOn(api, 'GET').mockImplementation((async (path: string) => {
    if (path.endsWith('/repositories')) return success([])
    if (path.endsWith('/teams')) return success({ revision: 1, can_manage: true, teams: [] })
    if (path.endsWith('/github-app')) return success(configured)
    return success(event)
  }) as typeof api.GET)
  let acknowledge!: (value: never) => void
  const post = vi.spyOn(api, 'POST').mockImplementationOnce(() => new Promise(resolve => { acknowledge = resolve }) as never).mockResolvedValue(success(configured) as never)
  render(<Theme><QueryClientProvider client={cache}><IdentityContext.Provider value={session}><CommunicationSafety><RepositoryWorkspace event={event} administration /></CommunicationSafety></IdentityContext.Provider></QueryClientProvider></Theme>)
  await screen.findByRole('button', { name: 'Refresh current status' })
  fireEvent.click(screen.getByRole('button', { name: 'Open App setup' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Change App details' }))
  fireEvent.change(await screen.findByLabelText('App setup action'), { target: { value: 'rotate' } })
  fireEvent.click(screen.getByRole('button', { name: 'Review App setup change' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm App setup change' }))
  await waitFor(() => expect(post).toHaveBeenCalledOnce())
  act(() => { cache.setQueryData(eventKey, { ...event, can_edit: false }); cache.setQueryData(eventKey, { ...event, can_edit: true }) })
  fireEvent.click(screen.getByRole('button', { name: 'Refresh current status' }))
  await waitFor(() => expect(cache.isFetching()).toBe(0))
  await act(async () => { acknowledge(success(configured) as never) })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Review App setup change' })).toBeEnabled())
  // Renewal must never reactivate the original command's captured ticket.
  expect(screen.queryByText('The App configuration request was acknowledged.', { exact: false })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Review App setup change' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm App setup change' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
  expect(post.mock.calls[1][1]).toMatchObject({ body: { action: 'rotate' } }); cache.clear()
})
