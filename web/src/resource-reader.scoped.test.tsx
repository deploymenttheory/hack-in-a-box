import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, test, vi } from 'vitest'
import { IdentityContext } from './session'
import { APIError } from './api/client'
import { ResourceReader } from './resource-reader'
import { PrivateRequestScope } from './private-request-scope'
import { CommunicationSafety, CommunicationCommandHistory, useCommunicationCommandNamespace } from './communication-tests'
import { PlanningOperations } from './planning-operations'
import { Theme } from './components/theme'
import { fixtureEvent, person } from '../e2e/fixtures'
import type { ContentPage } from './content-model'
const transport = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('./api/client', async original => ({ ...(await original<typeof import('./api/client')>()), api: { GET: transport.get, POST: transport.post } }))
vi.mock('@tanstack/react-router', () => ({ useBlocker: () => undefined }))
const caches: QueryClient[] = []
afterEach(() => { cleanup(); caches.splice(0).forEach(cache => cache.clear()); transport.get.mockReset(); transport.post.mockReset() })
const event = fixtureEvent(), id = '719d60be-7844-4f24-ae5b-055e35005e5a', draft = 'synthetic-scoped-reader'
function page(): ContentPage { return { id, kind: 'page', revision: 3, version: 2, state: 'released', document: { title: 'PRIVATE ORIGINAL TITLE', markdown: 'PRIVATE ORIGINAL BODY', audience: { confirmed: true, roles: [], teams: [] }, attachments: [], scheduled_at: null, require_acknowledgement: true }, released_version: 2, released_at: '2027-06-13T09:00:00Z', acknowledged_at: null, can_manage: false, attachment_details: [], message: '' } }
const response = (data: ContentPage) => ({ data, response: new Response(null, { status: 200 }) })
function held() { let resolve!: (value: ReturnType<typeof response>) => void, reject!: (error: Error) => void; const promise = new Promise<ReturnType<typeof response>>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function Metadata() { const attempts = useCommunicationCommandNamespace(`${person.id}:${draft}:${event.id}:resource-acknowledgement`); return <><CommunicationCommandHistory attempts={attempts} /><pre data-testid="safe-memory">{JSON.stringify(attempts)}</pre></> }
function setup() {
 const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }), parent = new PrivateRequestScope('current-native-role'); caches.push(cache)
 function Tree({ visible = true }: { visible?: boolean }) { return <QueryClientProvider client={cache}><CommunicationSafety><IdentityContext.Provider value={{ person: { ...person, platform_admin: false }, draft_scope: draft, csrf_token: 'synthetic-csrf', can_create_events: false }}><Theme><Metadata /><PlanningOperations key={visible ? 'organiser' : 'judge'}>{visible ? <ResourceReader id={id} event={event} parentScope={parent} back={() => undefined} /> : <p>Private reader removed</p>}</PlanningOperations></Theme></IdentityContext.Provider></CommunicationSafety></QueryClientProvider> }
 return { parent, Tree, cache }
}
async function acknowledge() { await screen.findByText('PRIVATE ORIGINAL BODY'); fireEvent.click(screen.getByRole('button', { name: 'I have read version 2' })); expect(screen.getByRole('alertdialog').closest('.fleet-print-excluded')).toBeInTheDocument(); fireEvent.click(screen.getByRole('button', { name: 'Confirm acknowledgement' })); await waitFor(() => expect(transport.post).toHaveBeenCalledOnce()) }

test('a validated original ACK settles safe metadata after current parent denial without accepting late private output', async () => {
 const { parent, Tree, cache } = setup(), write = held(); transport.get.mockResolvedValue(response(page())); transport.post.mockReturnValue(write.promise)
 const view = render(<Tree />); await acknowledge()
 expect(screen.getByTestId('safe-memory')).not.toHaveTextContent('PRIVATE ORIGINAL TITLE')
 act(() => parent.invalidate()); view.rerender(<Tree visible={false} />)
 expect(screen.getByRole('complementary')).toHaveTextContent('Resource acknowledgement · Expected revision 2 · Awaiting original acknowledgement')
 await act(async () => write.resolve(response({ ...page(), acknowledged_at: '2027-06-13T10:00:00Z' })))
 expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
 expect(screen.queryByText('PRIVATE ORIGINAL BODY')).not.toBeInTheDocument()
 expect(JSON.stringify(cache.getQueryCache().getAll().map(query => query.state.data))).not.toContain('PRIVATE ORIGINAL BODY')
 expect(transport.post.mock.calls[0][1].params.header).toEqual({ 'X-CSRF-Token': 'synthetic-csrf', 'If-Match': '"2"' })
})

test('lost original ACK stays unknown after denial and a new current acknowledged read cannot settle it', async () => {
 const { parent, Tree } = setup(), write = held(); transport.get.mockResolvedValue(response(page())); transport.post.mockReturnValue(write.promise)
 const view = render(<Tree />); await acknowledge(); act(() => parent.invalidate()); view.rerender(<Tree visible={false} />)
 await act(async () => write.reject(new APIError(503, 'unavailable')))
 expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed')
 expect(screen.getByTestId('safe-memory')).not.toHaveTextContent('PRIVATE ORIGINAL TITLE')
 transport.get.mockResolvedValue(response({ ...page(), document: { ...page().document, title: 'Current authorised title', markdown: 'Current authorised body' }, acknowledged_at: '2027-06-13T10:00:00Z' }))
 act(() => parent.renew()); view.rerender(<Tree />)
 await screen.findByText('Current authorised body')
 fireEvent.click(screen.getByRole('button', { name: 'Read current state' }))
 await screen.findByText('The current server facts are displayed. The original unknown outcome remains unconfirmed.')
 expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed')
 expect(transport.post).toHaveBeenCalledOnce()
})

for (const mismatch of ['id', 'version', 'acknowledgement'] as const) test(`an invalid original ${mismatch} never fabricates an acknowledgement receipt`, async () => {
 const { Tree } = setup(); transport.get.mockResolvedValue(response(page())); const result = { ...page(), acknowledged_at: mismatch === 'acknowledgement' ? null : '2027-06-13T10:00:00Z', id: mismatch === 'id' ? '10185018-c771-443c-ae76-146af6d55988' : id, version: mismatch === 'version' ? 3 : 2 }; transport.post.mockResolvedValue(response(result))
 render(<Tree />); await acknowledge(); await screen.findByRole('heading', { name: 'Outcome unknown' })
 expect(screen.getByRole('complementary')).toHaveTextContent('Original outcome remains unconfirmed')
 expect(screen.getByTestId('safe-memory')).not.toHaveTextContent('PRIVATE ORIGINAL TITLE')
 expect(transport.post).toHaveBeenCalledOnce()
})
