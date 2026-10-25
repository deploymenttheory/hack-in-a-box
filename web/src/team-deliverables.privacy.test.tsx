import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { IdentityContext } from './session'
import { APIError, privateKey } from './api/client'
import { TeamDeliverables } from './team-deliverables'
import { PlanningOperations } from './planning-operations'
import { CommunicationSafety, CommunicationCommandHistory, useCommunicationCommandNamespace } from './communication-tests'
import { Theme } from './components/theme'
import { fixtureEvent, person } from '../e2e/fixtures'
import type { components } from './api/schema.gen'

const transport = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get, PUT: transport.put } }))
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))
vi.mock('./personal-preparation', () => ({ PersonalPreparation: () => null }))
const caches: QueryClient[] = []
afterEach(() => { cleanup(); caches.splice(0).forEach(cache => cache.clear()); transport.get.mockReset(); transport.put.mockReset() })
const event = fixtureEvent(), teamID = '164a3cd9-7b9e-45da-b6c7-1b797f11f70c', itemID = '621b317a-b992-44d6-a52c-42e49b7c0fa1', draftScope = 'synthetic-deliverable-session'
type Schema = components['schemas']
function response<T>(data: T) { return { data, response: new Response(null, { status: 200 }) } }
function held<T>() { let resolve!: (value: T) => void, reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function Metadata() { const attempts = useCommunicationCommandNamespace(`${person.id}:${draftScope}:${event.id}:team-deliverables`); return <><CommunicationCommandHistory attempts={attempts} /><pre data-testid="safe-memory">{JSON.stringify(attempts)}</pre></> }
function setup() {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); caches.push(cache)
  const team: Schema['HackTeam'] = { id: teamID, name: 'Synthetic team', state: 'published', frozen: true, captain: person.id, published_captain: person.id, invitation_code: null, draft: [], published: [{ id: person.id, name: person.display_name }] }
  const page: Schema['SubmissionPage'] = { event_revision: event.revision, revision: 0, entry_id: 'b42bf250-a254-4497-ac23-20b591656d30', team_id: teamID, outcome: 'open', frozen_at: null, can_draft: true, can_read_draft: true, can_submit: true, can_review: false, live_start: event.configuration.live_start, cutoff: event.configuration.submission_cutoff, organisers: [], definitions: [{ id: itemID, title: 'Repository', description: 'Synthetic requirement', required: true, due_at: event.configuration.submission_cutoff, evidence_kinds: ['repository'] }], final: null }
  const draft: Schema['SubmissionDraft'] = { revision: 1, evidence: [{ deliverable_id: itemID, kind: 'repository', label: 'PRIVATE SAVED LABEL', url: 'https://example.test/PRIVATE-SAVED-REFERENCE', file_id: null }], updated_at: event.configuration.live_start }
  let denied = false, member = true, pageDenied = false
  transport.get.mockImplementation(async (path: string) => {
    if (path.endsWith('/teams')) { if (denied) throw new APIError(403, 'forbidden'); return response({ revision: event.revision, can_manage: false, can_create: false, frozen: true, freeze_at: event.configuration.team_freeze, teams: member ? [team] : [], people: [], requests: [] }) }
    if (path.endsWith('/submission/draft')) return response(draft)
    if (path.endsWith('/submission')) { if (pageDenied) throw new APIError(403, 'forbidden'); return response(page) }
    if (path.endsWith('/history')) return response({ items: [], next_cursor: null })
    throw new Error(`Unexpected native read: ${path}`)
  })
  const dirty = vi.fn()
  function Tree({ visible = true }: { visible?: boolean }) { return <QueryClientProvider client={cache}><CommunicationSafety><IdentityContext.Provider value={{ person, draft_scope: draftScope, csrf_token: 'synthetic-csrf', can_create_events: false }}><Theme><Metadata /><PlanningOperations>{visible && <TeamDeliverables event={event} onDirty={dirty} />}</PlanningOperations></Theme></IdentityContext.Provider></CommunicationSafety></QueryClientProvider> }
  async function refreshTeams() { await act(async () => { await cache.invalidateQueries({ queryKey: privateKey(person.id, event.id, 'teams'), exact: true }) }) }
  return { cache, Tree, draft, page, refreshTeams, deny: () => { denied = true }, restore: () => { denied = false }, removeMember: () => { member = false }, denyPage: () => { pageDenied = true }, restorePage: () => { pageDenied = false } }
}
async function editAndSave() {
  const input = await screen.findByRole('textbox', { name: 'Reference URL for Repository' })
  fireEvent.change(input, { target: { value: 'https://example.test/PRIVATE-UNSENT-REFERENCE' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save private draft' }))
  fireEvent.click(screen.getByRole('button', { name: 'Confirm private draft' }))
  await waitFor(() => expect(transport.put).toHaveBeenCalledOnce())
}

test('native teams denial clears descendant private caches before partial general access restoration', async () => {
  const f = setup(); render(<f.Tree />)
  await screen.findByDisplayValue('https://example.test/PRIVATE-SAVED-REFERENCE')
  f.deny(); await f.refreshTeams()
  await waitFor(() => expect(screen.queryByDisplayValue('https://example.test/PRIVATE-SAVED-REFERENCE')).not.toBeInTheDocument())
  expect(JSON.stringify(f.cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE-SAVED-REFERENCE')
  f.restore(); f.removeMember(); fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByText(/Your published team deliverables will appear/)
  expect(JSON.stringify(f.cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE-SAVED-REFERENCE')
})

test.each(['acknowledged', 'lost'] as const)('membership loss retains only safe original metadata while a held draft response is %s', async outcome => {
  const f = setup(), write = held<ReturnType<typeof response<Schema['SubmissionDraft']>>>()
  transport.put.mockReturnValue(write.promise); render(<f.Tree />)
  await editAndSave()
  f.removeMember(); await f.refreshTeams()
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  expect(screen.getByRole('complementary')).toHaveTextContent('Awaiting original acknowledgement')
  expect(screen.getByTestId('safe-memory')).not.toHaveTextContent('PRIVATE')
  const submitted = transport.put.mock.calls[0][1].body.evidence
  await act(async () => { if (outcome === 'acknowledged') write.resolve(response({ ...f.draft, revision: 2, evidence: submitted })); else write.reject(new APIError(503, 'unavailable')) })
  expect(JSON.stringify(f.cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE-UNSENT-REFERENCE')
  if (outcome === 'acknowledged') expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
  else { expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed'); await f.refreshTeams(); expect(screen.getByRole('complementary')).toHaveTextContent('Expected revision 1') }
  expect(transport.put).toHaveBeenCalledOnce()
})

test('a native submission denial scrubs its draft and a fresh permitted retry mounts a new private owner', async () => {
  const f = setup(); render(<f.Tree />)
  await screen.findByDisplayValue('https://example.test/PRIVATE-SAVED-REFERENCE')
  f.denyPage()
  await act(async () => { await f.cache.invalidateQueries({ queryKey: privateKey(person.id, event.id, `submission-${teamID}`) }) })
  await screen.findByRole('button', { name: 'Try again' })
  expect(JSON.stringify(f.cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE-SAVED-REFERENCE')
  f.restorePage(); f.draft.evidence[0].url = 'https://example.test/CURRENT-PERMITTED-REFERENCE'
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByDisplayValue('https://example.test/CURRENT-PERMITTED-REFERENCE')
  expect(screen.queryByDisplayValue('https://example.test/PRIVATE-SAVED-REFERENCE')).not.toBeInTheDocument()
})
