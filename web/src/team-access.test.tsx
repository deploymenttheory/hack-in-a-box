import { afterEach, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { IdentityContext } from './session'
import { TeamAccess } from './team-access'
import { fixtureEvent, person } from '../e2e/fixtures'

const transport = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('./api/client', async importOriginal => ({ ...(await importOriginal<typeof import('./api/client')>()), api: { GET: transport.get, POST: transport.post } }))
afterEach(() => { cleanup(); vi.restoreAllMocks(); transport.get.mockReset(); transport.post.mockReset() })

test('revealed values and unexpected private metadata fields never enter either shared cache', async () => {
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  const event = fixtureEvent(), team = '00000000-0000-4000-8000-000000000011', resource = '00000000-0000-4000-8000-000000000022'
  const secret = 'SYNTHETIC-REVEALED-PRIVATE-VALUE', unexpected = 'SYNTHETIC-UNEXPECTED-PRIVATE-METADATA'
  const response = (data: unknown) => ({ data, response: new Response(null, { status: 200 }) })
  transport.get.mockImplementation(async (path: string) => response(path.endsWith('/teams') ? { revision: 1, can_manage: false, teams: [{ id: team, name: 'Synthetic team', state: 'published', published: [{ id: person.id, name: person.display_name }] }] } : { revision: 1, can_manage: false, items: [{ id: resource, label: 'Team account', version: 1, active: true, team_ids: [], updated_at: '2027-06-14T09:00:00Z', resource_type: 'account', available_at: null, expires_at: null, readiness: 'ready', value: unexpected }] }))
  transport.post.mockResolvedValue(response({ id: resource, version: 1, value: secret }))
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(<QueryClientProvider client={cache}><IdentityContext.Provider value={{ person: { ...person, platform_admin: false }, csrf_token: 'synthetic-csrf-token', can_create_events: false }}><TeamAccess event={event} /></IdentityContext.Provider></QueryClientProvider>)
  fireEvent.click(await screen.findByRole('button', { name: 'Reveal Team account' }))
  expect(await screen.findByText(secret)).toBeVisible()
  const contents = JSON.stringify(cache.getQueryCache().getAll().map(query => query.state.data))
  expect(contents).not.toContain(secret)
  expect(contents).not.toContain(unexpected)
  expect(cache.getMutationCache().getAll()).toHaveLength(0)
  view.unmount()
  expect(document.body.textContent).not.toContain(secret)
  expect(JSON.stringify(cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain(secret)
  cache.clear()
})
